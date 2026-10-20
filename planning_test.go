package vsm

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"
)

// planningDesign is a design result for targetStream with two potentials
// and one continuous improvement, built without the analyzer so the
// planning tests do not depend on what the analysis finds. targets (nil =
// none) shape the future state.
func planningDesign(t *testing.T, targets *FutureStateTargets) (*DesignResult, *VSMProject) {
	t.Helper()
	current := targetStream()
	future, err := NewDesigner().DesignFutureStateFromTargets(current, targets)
	if err != nil {
		t.Fatal(err)
	}
	design := &DesignResult{
		ValueStreamID: current.ID,
		ImprovementPotentials: []ImprovementPotential{
			{ID: "bottleneck_p2", Type: "bottleneck", Location: "Weld", ProcessID: "p2"},
			{ID: "waste_waiting", Type: "waste", Location: "Value Stream"},
		},
		ContinuousImprovements: []ContinuousImprovement{
			{ID: "ci_waste_reduction", Type: "major_change"},
		},
		FutureState: future,
		DesignedAt:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	return design, &VSMProject{ID: "proj", Name: "Proj", CurrentState: *current}
}

func day(d int) *time.Time {
	v := time.Date(2026, 3, d, 0, 0, 0, 0, time.UTC)
	return &v
}

func measureByID(t *testing.T, r *PlanningResult, id string) ImprovementMeasure {
	t.Helper()
	for _, m := range r.CatalogueOfMeasures.Measures {
		if m.ID == id {
			return m
		}
	}
	t.Fatalf("measure %s not in catalogue", id)
	return ImprovementMeasure{}
}

func TestPayback(t *testing.T) {
	tests := []struct {
		name   string
		costs  Costs
		months *float64
		never  bool
	}{
		// 12,000 ÷ (25,000 − 1,000) per year × 12 = 6 months
		{"positive net benefit", Costs{Investment: F(12000), AnnualOperatingCost: F(1000), AnnualBenefit: F(25000)}, F(6), false},
		{"never pays back", Costs{Investment: F(12000), AnnualOperatingCost: F(5000), AnnualBenefit: F(4000)}, nil, true},
		{"net benefit exactly 0", Costs{Investment: F(1), AnnualOperatingCost: F(10), AnnualBenefit: F(10)}, nil, true},
		{"no investment pays back at once", Costs{Investment: F(0), AnnualOperatingCost: F(0), AnnualBenefit: F(10)}, F(0), false},
		{"operating cost not entered", Costs{Investment: F(12000), AnnualBenefit: F(25000)}, nil, false},
		{"nothing entered", Costs{}, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			months, never := calculatePayback(tt.costs)
			if never != tt.never || (months == nil) != (tt.months == nil) || (months != nil && !approx(*months, *tt.months)) {
				t.Errorf("got (%v, %v), want (%v, %v)", fmtPtr(months), never, fmtPtr(tt.months), tt.never)
			}
		})
	}
}

func fmtPtr(p *float64) any {
	if p == nil {
		return "nil"
	}
	return *p
}

func TestPlanningWithoutInputsInventsNothing(t *testing.T) {
	design, project := planningDesign(t, nil)
	r, err := NewPlanner().PlanImplementation(design, project)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range r.CatalogueOfMeasures.Measures {
		if m.Priority != 0 || m.PriorityNote == "" {
			t.Errorf("%s: priority %d note %q, want not prioritised with a note", m.ID, m.Priority, m.PriorityNote)
		}
		if c := m.Costs; c.Investment != nil || c.AnnualBenefit != nil || c.AnnualOperatingCost != nil ||
			c.PaybackMonths != nil || c.NeverPaysBack || !strings.Contains(c.Note, "not estimated") {
			t.Errorf("%s: costs %+v, want not estimated", m.ID, c)
		}
		if m.SMART.TimeBound != nil {
			t.Errorf("%s: due date %v invented", m.ID, m.SMART.TimeBound)
		}
		if !strings.Contains(m.SMART.Measurable, "target not set") || strings.Contains(m.SMART.Measurable, " from ") {
			t.Errorf("%s: measurable %q, want target not set and no change", m.ID, m.SMART.Measurable)
		}
	}
	tot := r.CostTotals
	if tot.Investment != nil || tot.AnnualBenefit != nil || tot.PaybackMonths != nil || len(tot.MissingInputs) != 3 {
		t.Errorf("cost totals %+v, want unknown with 3 missing", tot)
	}
	plan := r.ImplementationPlan
	if len(plan.UnprioritizedMeasures) != 3 {
		t.Errorf("unprioritised %v, want all 3", plan.UnprioritizedMeasures)
	}
	for _, ph := range plan.Phases {
		if ph.StartDate != nil || ph.EndDate != nil || len(ph.Measures) != 0 {
			t.Errorf("phase %+v, want empty and undated", ph)
		}
	}
	if plan.Timeline.StartDate != nil || plan.Timeline.EndDate != nil || plan.Timeline.Duration != nil {
		t.Errorf("timeline %+v, want undated", plan.Timeline)
	}
	for _, ms := range plan.Milestones {
		if ms.Date != nil {
			t.Errorf("milestone %s dated %v", ms.ID, ms.Date)
		}
	}
	for _, rr := range plan.ResourceRequirements {
		if rr.Quantity != nil {
			t.Errorf("resource %s quantity %v invented", rr.ID, *rr.Quantity)
		}
	}
	for _, risk := range r.RiskAssessment {
		if risk.Probability != nil || risk.Impact != nil || risk.Severity != nil || risk.Note == "" {
			t.Errorf("risk %+v, want not assessed", risk)
		}
	}
	if r.WorkshopPlan.Date != nil {
		t.Errorf("workshop date %v invented", r.WorkshopPlan.Date)
	}
	if len(r.Notes) == 0 {
		t.Error("no notes about what was not entered")
	}
}

func TestPlanningWithInputs(t *testing.T) {
	design, project := planningDesign(t, nil)
	inputs := &PlanningInputs{
		StartDate:    day(1),
		WorkshopDate: day(2),
		Measures: map[string]MeasureInput{
			// impact > effort: priority 1
			"bottleneck_p2": {ImpactRating: I(5), EffortRating: I(2), Investment: F(12000),
				AnnualBenefit: F(25000), AnnualOperatingCost: F(1000), DueDate: day(10), Responsible: "Ann"},
			// equal: priority 2; never pays back
			"waste_waiting": {ImpactRating: I(3), EffortRating: I(3), Investment: F(100),
				AnnualBenefit: F(50), AnnualOperatingCost: F(50), DueDate: day(20)},
		},
		Risks: map[string]RiskInput{"risk_scope_creep": {Probability: F(0.5), Impact: F(0.4)}},
	}
	r, err := NewPlanner().PlanImplementationWithInputs(design, project, inputs)
	if err != nil {
		t.Fatal(err)
	}

	// Order: priority 1, priority 2, then the unrated measure.
	var ids []string
	for _, m := range r.CatalogueOfMeasures.Measures {
		ids = append(ids, m.ID)
	}
	if want := []string{"bottleneck_p2", "waste_waiting", "ci_waste_reduction"}; !reflect.DeepEqual(ids, want) {
		t.Errorf("order %v, want %v", ids, want)
	}
	b := measureByID(t, r, "bottleneck_p2")
	if b.Priority != 1 || val(b.Costs.PaybackMonths) != 6 || b.Responsible != "Ann" || !b.SMART.TimeBound.Equal(*day(10)) {
		t.Errorf("bottleneck measure %+v", b)
	}
	w := measureByID(t, r, "waste_waiting")
	if w.Priority != 2 || !w.Costs.NeverPaysBack || w.Costs.PaybackMonths != nil {
		t.Errorf("waste measure priority %d costs %+v, want 2 and never pays back", w.Priority, w.Costs)
	}
	if ci := measureByID(t, r, "ci_waste_reduction"); ci.Priority != 0 || ci.Responsible != "TBD" {
		t.Errorf("unrated measure %+v", ci)
	}

	// Totals: unknown, because ci_waste_reduction has no amounts.
	if tot := r.CostTotals; tot.Investment != nil || !reflect.DeepEqual(tot.MissingInputs, []string{"ci_waste_reduction"}) {
		t.Errorf("totals %+v, want unknown with ci_waste_reduction missing", tot)
	}

	plan := r.ImplementationPlan
	p1, p2, p3 := plan.Phases[0], plan.Phases[1], plan.Phases[2]
	if !p1.StartDate.Equal(*day(1)) || !p1.EndDate.Equal(*day(10)) || !p2.StartDate.Equal(*day(10)) ||
		!p2.EndDate.Equal(*day(20)) || p3.StartDate != nil || len(p3.Measures) != 0 {
		t.Errorf("phases %+v", plan.Phases)
	}
	if !reflect.DeepEqual(plan.UnprioritizedMeasures, []string{"ci_waste_reduction"}) {
		t.Errorf("unprioritised %v", plan.UnprioritizedMeasures)
	}
	// The unrated measure has no due date: the timeline end is unknown.
	if !plan.Timeline.StartDate.Equal(*day(1)) || plan.Timeline.EndDate != nil || plan.Timeline.Duration != nil {
		t.Errorf("timeline %+v", plan.Timeline)
	}
	if plan.Milestones[0].Date == nil || !plan.Milestones[0].Date.Equal(*day(10)) {
		t.Errorf("quick wins milestone %+v", plan.Milestones[0])
	}

	if r.RiskAssessment[0].ID != "risk_scope_creep" || !approx(val(r.RiskAssessment[0].Severity), 0.2) {
		t.Errorf("first risk %+v, want the assessed risk with severity 0.2", r.RiskAssessment[0])
	}
	for _, risk := range r.RiskAssessment[1:] {
		if risk.Severity != nil {
			t.Errorf("risk %s assessed without input", risk.ID)
		}
	}
	if !r.WorkshopPlan.Date.Equal(*day(2)) {
		t.Errorf("workshop date %v", r.WorkshopPlan.Date)
	}
}

func TestPlanningCostTotalsAllEntered(t *testing.T) {
	measures := []ImprovementMeasure{
		{ID: "a", Costs: measureCosts(MeasureInput{Investment: F(10000), AnnualBenefit: F(20000), AnnualOperatingCost: F(2000)})},
		{ID: "b", Costs: measureCosts(MeasureInput{Investment: F(2000), AnnualBenefit: F(4000), AnnualOperatingCost: F(0)})},
	}
	tot := calculateCostTotals(measures)
	// 12,000 ÷ (24,000 − 2,000) × 12 = 6.545… months
	if val(tot.Investment) != 12000 || val(tot.AnnualBenefit) != 24000 || val(tot.AnnualOperatingCost) != 2000 ||
		!approx(val(tot.PaybackMonths), 12000.0/22000*12) || len(tot.MissingInputs) != 0 {
		t.Errorf("totals %+v", tot)
	}
	// One amount missing: only that total is unknown, never a partial sum.
	measures[1].Costs = measureCosts(MeasureInput{Investment: F(2000), AnnualOperatingCost: F(0)})
	tot = calculateCostTotals(measures)
	if val(tot.Investment) != 12000 || tot.AnnualBenefit != nil || tot.PaybackMonths != nil || tot.NeverPaysBack ||
		!reflect.DeepEqual(tot.MissingInputs, []string{"b"}) {
		t.Errorf("totals with a missing benefit %+v", tot)
	}
}

func TestPlanningBudgetIsTotalInvestment(t *testing.T) {
	design, project := planningDesign(t, nil)
	in := MeasureInput{Investment: F(100), AnnualBenefit: F(10), AnnualOperatingCost: F(0)}
	r, err := NewPlanner().PlanImplementationWithInputs(design, project, &PlanningInputs{Measures: map[string]MeasureInput{
		"bottleneck_p2": in, "waste_waiting": in, "ci_waste_reduction": in,
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, rr := range r.ImplementationPlan.ResourceRequirements {
		if rr.Type == "budget" && val(rr.Quantity) != 300 {
			t.Errorf("budget %v, want total investment 300", fmtPtr(rr.Quantity))
		}
	}
	if val(r.CostTotals.Investment) != 300 || !approx(val(r.CostTotals.PaybackMonths), 300.0/30*12) {
		t.Errorf("totals %+v", r.CostTotals)
	}
}

func TestPlanningRejectsInvalidInputs(t *testing.T) {
	cases := map[string]*PlanningInputs{
		"negative investment": {Measures: map[string]MeasureInput{"bottleneck_p2": {Investment: F(-1)}}},
		"negative benefit":    {Measures: map[string]MeasureInput{"bottleneck_p2": {AnnualBenefit: F(-5)}}},
		"NaN operating cost":  {Measures: map[string]MeasureInput{"bottleneck_p2": {AnnualOperatingCost: F(math.NaN())}}},
		"rating above 5":      {Measures: map[string]MeasureInput{"bottleneck_p2": {ImpactRating: I(6)}}},
		"rating 0":            {Measures: map[string]MeasureInput{"bottleneck_p2": {EffortRating: I(0)}}},
		"unknown measure":     {Measures: map[string]MeasureInput{"nope": {}}},
		"risk probability":    {Risks: map[string]RiskInput{"risk_resistance": {Probability: F(1.5)}}},
		"risk impact NaN":     {Risks: map[string]RiskInput{"risk_resistance": {Impact: F(math.NaN())}}},
		"unknown risk":        {Risks: map[string]RiskInput{"nope": {}}},
		"due before start":    {StartDate: day(10), Measures: map[string]MeasureInput{"bottleneck_p2": {DueDate: day(5)}}},
		// waste_waiting is priority 2: its phase starts when phase 1 ends (day 20).
		"due before its phase starts": {StartDate: day(1), Measures: map[string]MeasureInput{
			"bottleneck_p2": {ImpactRating: I(5), EffortRating: I(1), DueDate: day(20)},
			"waste_waiting": {ImpactRating: I(2), EffortRating: I(2), DueDate: day(15)},
		}},
	}
	for name, inputs := range cases {
		t.Run(name, func(t *testing.T) {
			design, project := planningDesign(t, nil)
			if _, err := NewPlanner().PlanImplementationWithInputs(design, project, inputs); err == nil {
				t.Error("no error")
			}
		})
	}
}

func TestSMARTGoalFromTargets(t *testing.T) {
	// Weld (p2): process time 6 + 2 = 8, waiting 40, lead time 48. A setup
	// target of 0 makes the lead time 46.
	targets := &FutureStateTargets{Processes: map[string]ProcessTargets{"p2": {SetupTime: AbsoluteTarget(0)}}}
	design, project := planningDesign(t, targets)
	r, err := NewPlanner().PlanImplementation(design, project)
	if err != nil {
		t.Fatal(err)
	}
	got := measureByID(t, r, "bottleneck_p2").SMART.Measurable
	for _, want := range []string{
		"Cycle time at Weld: target not set (current 4 minutes)",
		"reduce lead time at Weld from 48 minutes to 46 minutes (-4.2%)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("measurable %q, want it to contain %q", got, want)
		}
	}
	// Stream-level goal: from the calculator's KPIs of both states.
	cur := NewCalculator().CalculateKPIs(&project.CurrentState)["total_lead_time"]
	fut := design.FutureState.KPIs["total_lead_time"]
	stream := measureByID(t, r, "waste_waiting").SMART.Measurable
	if want := "Reduce total lead time from " + formatNumber(cur) + " minutes to " + formatNumber(fut) + " minutes"; !strings.HasPrefix(stream, want) {
		t.Errorf("stream goal %q, want prefix %q", stream, want)
	}
}

func TestSMARTGoalWithoutFutureState(t *testing.T) {
	design, project := planningDesign(t, nil)
	design.FutureState = nil
	r, err := NewPlanner().PlanImplementation(design, project)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range r.CatalogueOfMeasures.Measures {
		if !strings.Contains(m.SMART.Measurable, "target not set (no future state)") {
			t.Errorf("%s: %q", m.ID, m.SMART.Measurable)
		}
	}
}

func TestPlanningRatingsFromDesign(t *testing.T) {
	design, project := planningDesign(t, nil)
	design.ImprovementPotentials[1].ImpactRating, design.ImprovementPotentials[1].EffortRating = I(1), I(4)
	r, err := NewPlanner().PlanImplementationWithInputs(design, project, &PlanningInputs{Measures: map[string]MeasureInput{
		// Planning input replaces the design's ratings.
		"bottleneck_p2": {ImpactRating: I(2), EffortRating: I(2)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if m := measureByID(t, r, "waste_waiting"); m.Priority != 3 || val64(m.EffortRating) != 4 {
		t.Errorf("waste measure %+v, want priority 3 from the design's ratings", m)
	}
	if m := measureByID(t, r, "bottleneck_p2"); m.Priority != 2 {
		t.Errorf("bottleneck priority %d, want 2", m.Priority)
	}
}

func val64(p *int) int {
	if p == nil {
		return -1
	}
	return *p
}

func TestWorkshopDurationIsAgendaSum(t *testing.T) {
	design, project := planningDesign(t, nil)
	w, err := NewPlanner().PlanWorkshop(design, project)
	if err != nil {
		t.Fatal(err)
	}
	var sum time.Duration
	for i, item := range w.Agenda {
		if item.Time != sum {
			t.Errorf("item %d starts at %v, want %v (end of the previous item)", i, item.Time, sum)
		}
		sum += item.Duration
	}
	if w.Duration != sum || w.Date != nil {
		t.Errorf("duration %v date %v, want %v and not scheduled", w.Duration, w.Date, sum)
	}
}

func TestPlanningResultJSON(t *testing.T) {
	design, project := planningDesign(t, nil)
	r, err := NewPlanner().PlanImplementationWithInputs(design, project, &PlanningInputs{Measures: map[string]MeasureInput{
		"bottleneck_p2": {Investment: F(0), AnnualBenefit: F(10), AnnualOperatingCost: F(0)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	s := NewSerializer()
	data, err := s.SerializePlanningResult(r)
	if err != nil {
		t.Fatal(err)
	}
	var head struct {
		FormatVersion int `json:"format_version"`
	}
	if err := json.Unmarshal(data, &head); err != nil || head.FormatVersion != FormatVersion {
		t.Fatalf("format_version %d (%v)", head.FormatVersion, err)
	}
	back, err := s.DeserializePlanningResult(data)
	if err != nil {
		t.Fatal(err)
	}
	// An entered 0 investment survives as 0, not as "not entered".
	if c := measureByID(t, back, "bottleneck_p2").Costs; c.Investment == nil || *c.Investment != 0 || val(c.PaybackMonths) != 0 {
		t.Errorf("round-tripped costs %+v", c)
	}
	if c := measureByID(t, back, "waste_waiting").Costs; c.Investment != nil {
		t.Errorf("not-entered investment came back as %v", *c.Investment)
	}
}

func TestLegacyCostsLoadAsNotEntered(t *testing.T) {
	var c Costs
	legacy := `{"investment": 12000, "operational": 1200, "benefits": 25000, "payback_period": 6}`
	if err := json.Unmarshal([]byte(legacy), &c); err != nil {
		t.Fatal(err)
	}
	if c.Investment != nil || c.AnnualBenefit != nil || c.PaybackMonths != nil || c.Note == "" {
		t.Errorf("legacy costs %+v, want not entered with a note", c)
	}
	var project VSMProject
	file := `{"id":"p","name":"P","current_state":{"id":"vs","name":"VS","processes":[]},
		"catalogue_of_measures":{"id":"c","measures":[{"id":"m","priority":1,
		"smart":{"time_bound":"2026-01-01T00:00:00Z"},"costs":` + legacy + `}]}}`
	if err := json.Unmarshal([]byte(file), &project); err != nil {
		t.Fatal(err)
	}
	// The old planner made up the due date and priority too.
	if m := project.CatalogueOfMeasures.Measures[0]; m.Costs.Investment != nil || m.SMART.TimeBound != nil ||
		m.Priority != 0 || m.PriorityNote == "" {
		t.Errorf("legacy measure %+v", m)
	}
	var d DesignResult
	if err := json.Unmarshal([]byte(`{"improvement_potentials":[{"id":"x","priority":1,"impact":0.8,"effort":0.4}],
		"continuous_improvements":[{"id":"ci","priority":2},{"id":"ok","priority":3,"impact_rating":4,"effort_rating":1}]}`), &d); err != nil {
		t.Fatal(err)
	}
	if p := d.ImprovementPotentials[0]; p.Priority != 0 || p.ImpactRating != nil {
		t.Errorf("legacy potential %+v, want not prioritised", p)
	}
	if ci := d.ContinuousImprovements; ci[0].Priority != 0 || ci[1].Priority != 1 {
		t.Errorf("continuous improvements %+v, want priority 0 and 1 (recomputed from ratings)", ci)
	}
}
