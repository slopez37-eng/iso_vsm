package vsm

import (
	"reflect"
	"strings"
	"testing"
)

func TestRatingPriority(t *testing.T) {
	cases := []struct {
		impact, effort *int
		want           int
		ok             bool
	}{
		{I(5), I(1), 1, true},
		{I(3), I(3), 2, true},
		{I(1), I(5), 3, true},
		{I(4), nil, 0, false},
		{nil, I(2), 0, false},
		{I(6), I(1), 0, false},
		{I(0), I(1), 0, false},
	}
	for _, c := range cases {
		if got, ok := RatingPriority(c.impact, c.effort); got != c.want || ok != c.ok {
			t.Errorf("RatingPriority(%v, %v) = %d, %v; want %d, %v", val64(c.impact), val64(c.effort), got, ok, c.want, c.ok)
		}
	}
}

// ratingAnalysis is an analysis result with one bottleneck, one waste type
// and a push flow, built by hand so the test does not depend on the
// analyzer's thresholds.
func ratingAnalysis() *AnalysisResult {
	return &AnalysisResult{
		Bottlenecks:   []BottleneckInfo{{ProcessID: "p2", ProcessName: "Weld", Utilization: 0.95}},
		WasteAnalysis: WasteAnalysis{WasteByType: map[string]float64{"waiting": 60, "motion": 5}},
		FlowAnalysis:  FlowAnalysis{FlowType: FlowTypePush},
	}
}

func TestPotentialsAreNotPrioritisedWithoutRatings(t *testing.T) {
	got, err := NewDesigner().IdentifyImprovementPotentials(targetStream(), ratingAnalysis())
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, p := range got {
		ids = append(ids, p.ID)
		if p.Priority != 0 || p.ImpactRating != nil || p.EffortRating != nil || !strings.HasPrefix(p.PriorityNote, "Not prioritised") {
			t.Errorf("%s: %+v, want not prioritised", p.ID, p)
		}
	}
	// Fixed order: bottlenecks, waste types sorted, push-to-pull.
	if want := []string{"bottleneck_p2", "waste_motion", "waste_waiting", "flow_push_to_pull"}; !reflect.DeepEqual(ids, want) {
		t.Errorf("ids %v, want %v", ids, want)
	}
	if got[0].ProcessID != "p2" {
		t.Errorf("bottleneck process ID %q", got[0].ProcessID)
	}
}

func TestDesignAppliesEnteredRatings(t *testing.T) {
	inputs := map[string]MeasureInput{
		"waste_waiting":    {ImpactRating: I(5), EffortRating: I(2)},
		"bottleneck_p2":    {ImpactRating: I(2), EffortRating: I(4)},
		"ci_bottleneck_p2": {ImpactRating: I(3), EffortRating: I(3)},
		// Impact only: still not prioritised.
		"flow_push_to_pull": {ImpactRating: I(4)},
	}
	got, err := NewDesigner().DesignFutureStateWithInputs(targetStream(), ratingAnalysis(), nil, inputs)
	if err != nil {
		t.Fatal(err)
	}
	priorities := map[string]int{}
	for _, p := range got.ImprovementPotentials {
		priorities[p.ID] = p.Priority
	}
	if want := map[string]int{"bottleneck_p2": 3, "waste_motion": 0, "waste_waiting": 1, "flow_push_to_pull": 0}; !reflect.DeepEqual(priorities, want) {
		t.Errorf("priorities %v, want %v", priorities, want)
	}
	if got.ImprovementPotentials[0].ID != "bottleneck_p2" {
		t.Errorf("order changed: first is %s", got.ImprovementPotentials[0].ID)
	}
	for _, p := range got.ImprovementPotentials {
		if p.ID == "flow_push_to_pull" && p.PriorityNote != "Not prioritised: effort rating not entered" {
			t.Errorf("note %q", p.PriorityNote)
		}
	}
	for _, ci := range got.ContinuousImprovements {
		want := 0
		if ci.ID == "ci_bottleneck_p2" {
			want = 2
		}
		if ci.Priority != want {
			t.Errorf("continuous improvement %s priority %d, want %d", ci.ID, ci.Priority, want)
		}
	}
	noted := false
	for _, n := range got.Notes {
		noted = noted || strings.Contains(n, "not prioritised")
	}
	if !noted {
		t.Errorf("no not-prioritised note: %v", got.Notes)
	}

	// The planner carries the design's ratings into the catalogue.
	r, err := NewPlanner().PlanImplementation(got, &VSMProject{ID: "p", Name: "P", CurrentState: *targetStream()})
	if err != nil {
		t.Fatal(err)
	}
	if first := r.CatalogueOfMeasures.Measures[0]; first.ID != "waste_waiting" || first.Priority != 1 {
		t.Errorf("first measure %s priority %d, want waste_waiting priority 1", first.ID, first.Priority)
	}
}

func TestDesignRejectsInvalidRatings(t *testing.T) {
	for name, inputs := range map[string]map[string]MeasureInput{
		"impact above 5": {"waste_waiting": {ImpactRating: I(9)}},
		"effort below 1": {"waste_waiting": {EffortRating: I(0)}},
		"unknown ID":     {"no_such_potential": {ImpactRating: I(3)}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewDesigner().DesignFutureStateWithInputs(targetStream(), ratingAnalysis(), nil, inputs); err == nil {
				t.Error("no error")
			}
		})
	}
	potentials, _ := NewDesigner().IdentifyImprovementPotentials(targetStream(), ratingAnalysis())
	if _, err := NewDesigner().PrioritizeImprovementPotentials(potentials, map[string]MeasureInput{"x": {}}); err == nil {
		t.Error("PrioritizeImprovementPotentials accepted an unknown ID")
	}
	out, err := NewDesigner().PrioritizeImprovementPotentials(potentials, map[string]MeasureInput{"bottleneck_p2": {ImpactRating: I(4), EffortRating: I(1)}})
	if err != nil || out[0].Priority != 1 || potentials[0].Priority != 0 {
		t.Errorf("PrioritizeImprovementPotentials: %v, priority %d (input left at %d)", err, out[0].Priority, potentials[0].Priority)
	}
}
