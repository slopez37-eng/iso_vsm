package vsm

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func exampleProject() *Project {
	current := exampleStream()
	current.Flashes = []Flash{{Number: 1, Target: "I1", Description: "cut idle time", Wastes: []Waste{WasteWaiting}}}
	future := exampleStream()
	future.State = StateFuture
	future.Idles[0].IdleTime = F(4)
	future.Processes[0].ProcessTime = F(31)
	future.Processes[0].WaitingTime = F(5)
	return &Project{
		Name: "soldering", Current: current, Future: future,
		Measures: []Measure{
			{Number: 1, Potential: "waiting", Description: "FIFO lane", Status: StatusPlanned, Flashes: []int{1}, Cost: F(100), Benefit: F(300)},
			{Number: 2, Potential: "setup", Description: "SMED", Status: StatusPlanned, Cost: F(100), Benefit: F(500),
				Metric: "setup time", Target: F(5), Responsible: "PRO1", Deadline: "2027-03-01"},
			{Number: 3, Potential: "scrap", Status: StatusInProgress},
			{Number: 4, Potential: "done", Status: StatusDone, Cost: F(1), Benefit: F(100)},
		},
	}
}

func TestEvaluateProject(t *testing.T) {
	rep := EvaluateProject(exampleProject())
	if rep.Comparison == nil {
		t.Fatalf("no comparison: %v", rep.Issues)
	}
	c := rep.Comparison
	approx(t, "ω_PT", c.Figures.ProcessTime, 31.0/35)
	approx(t, "ω_IT", c.Figures.IdleTime, 0.4)
	approx(t, "ω_LT", c.Figures.LeadTime, 35.0/45)
	approx(t, "S_NVA,TAR", c.TargetNonValueAddingShare, 4.0/35)
	if want := []int{2, 1, 3}; !slices.Equal(rep.Priority, want) {
		t.Errorf("priority = %v, want %v", rep.Priority, want)
	}
	if len(rep.Measures[1].SMARTGaps) != 0 {
		t.Errorf("measure 2 is complete but has gaps %v", rep.Measures[1].SMARTGaps)
	}
	if len(rep.Measures[0].SMARTGaps) != 3 {
		t.Errorf("measure 1 gaps = %v, want measurable, accepted, time-related", rep.Measures[0].SMARTGaps)
	}
	found := false
	for _, i := range rep.Issues {
		found = found || i.Rule == "4.4.3"
	}
	if !found {
		t.Error("missing workshop note")
	}
}

func TestProjectChecks(t *testing.T) {
	p := exampleProject()
	p.Measures = append(p.Measures, Measure{Number: 1, Flashes: []int{9}})
	rep := EvaluateProject(p)
	var text []string
	for _, i := range rep.Issues {
		text = append(text, i.String())
	}
	all := strings.Join(text, "\n")
	for _, want := range []string{"used twice", "status is not noted", "flash 9"} {
		if !strings.Contains(all, want) {
			t.Errorf("issues do not mention %q:\n%s", want, all)
		}
	}
}

func TestCompareStatesConvertsUnits(t *testing.T) {
	a := Evaluate(&ValueStream{Name: "a", TimeUnit: Hour, Processes: []Process{{ID: "P", ProcessTime: F(1)}}, Idles: []Idle{{ID: "I", IdleTime: F(3)}}})
	b := Evaluate(&ValueStream{Name: "b", TimeUnit: Minute, Processes: []Process{{ID: "P", ProcessTime: F(30)}}, Idles: []Idle{{ID: "I", IdleTime: F(90)}}})
	c, err := CompareStates(a, b)
	if err != nil {
		t.Fatal(err)
	}
	approx(t, "ω_PT", c.Figures.ProcessTime, 0.5)
	approx(t, "ω_LT", c.Figures.LeadTime, 0.5)
	approx(t, "target lead time in h", c.TargetLeadTime, 2)
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	p := exampleProject()
	path := filepath.Join(t.TempDir(), "project.json")
	if err := Save(path, p); err != nil {
		t.Fatal(err)
	}
	got, err := LoadProject(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, p) {
		t.Error("project changed in a save and load round trip")
	}
	if got.Current.SchemaVersion != SchemaVersion {
		t.Errorf("schema version %d not stamped", got.Current.SchemaVersion)
	}
}

func TestReadRejectsUnknownFields(t *testing.T) {
	_, err := ReadValueStream(strings.NewReader(`{"name":"x","time_unit":"s","processes":[{"id":"P","proces_time":3}]}`))
	if err == nil || !strings.Contains(err.Error(), "proces_time") {
		t.Errorf("err = %v, want unknown field proces_time", err)
	}
}

func TestSymbols(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range Symbols {
		if seen[s.ID] {
			t.Errorf("duplicate symbol id %q", s.ID)
		}
		seen[s.ID] = true
	}
	if len(Symbols) != 35 {
		t.Errorf("%d symbols, Table A.1 has 35", len(Symbols))
	}
	if s, ok := SymbolByID("improvement_flash"); !ok || s.Category != CategoryGeneral {
		t.Errorf("improvement flash: %+v", s)
	}
}

func TestInfiniteRatioMarshals(t *testing.T) {
	p := exampleProject()
	p.Measures[0].Cost = F(0)
	data, err := json.Marshal(EvaluateProject(p))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"+Inf"`) {
		t.Error("infinite benefit-cost ratio not written as +Inf")
	}
	if got := Prioritize(p.Measures); got[0] != 1 {
		t.Errorf("a free measure should rank first, got %v", got)
	}
}

func TestReadRejectsTrailingData(t *testing.T) {
	for _, in := range []string{`{"name":"x"}}`, `{"name":"x"}]`, `{"name":"x"} {}`} {
		if _, err := ReadValueStream(strings.NewReader(in)); err == nil {
			t.Errorf("%s accepted", in)
		}
	}
}
