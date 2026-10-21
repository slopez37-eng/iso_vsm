package vsm

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
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
			{Number: 1, Potential: "waiting", Description: "FIFO lane", Status: StatusMeasureChecked, Flashes: []int{1}, Cost: F(100), Benefit: F(300)},
			{Number: 2, Potential: "setup", Description: "SMED", Status: StatusMeasureChecked, Cost: F(100), Benefit: F(500),
				Metric: "setup time", Target: F(5), Responsible: "PRO1", Deadline: "2027-03-01"},
			{Number: 3, Potential: "scrap", Status: StatusImplementing},
			{Number: 4, Potential: "done", Status: StatusImplemented, Cost: F(1), Benefit: F(100)},
		},
	}
}

func TestEvaluateProject(t *testing.T) {
	rep := EvaluateProject(exampleProject())
	if rep.Comparison == nil {
		t.Fatalf("no comparison: %v", rep.Issues)
	}
	c := rep.Comparison
	approx(t, "ω_PT", c.Figures.ProcessTime.Value, 31.0/35)
	approx(t, "ω_IT", c.Figures.IdleTime.Value, 0.4)
	approx(t, "ω_LT", c.Figures.LeadTime.Value, 35.0/45)
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
	approx(t, "ω_PT", c.Figures.ProcessTime.Value, 0.5)
	approx(t, "ω_LT", c.Figures.LeadTime.Value, 0.5)
	approx(t, "target lead time in h", c.TargetLeadTime, 2)
}

func TestCompareStatesWithoutIdleTime(t *testing.T) {
	a := Evaluate(&ValueStream{Name: "a", TimeUnit: Minute, Processes: []Process{{ID: "P", ProcessTime: F(4)}}, Idles: []Idle{{ID: "I", IdleTime: F(0)}}})
	b := Evaluate(&ValueStream{Name: "b", TimeUnit: Minute, Processes: []Process{{ID: "P", ProcessTime: F(2)}}, Idles: []Idle{{ID: "I", IdleTime: F(0)}}})
	c, err := CompareStates(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if c.Figures.IdleTime.Known {
		t.Errorf("ω_IT = %v, want unknown", c.Figures.IdleTime)
	}
	approx(t, "ω_PT", c.Figures.ProcessTime.Value, 0.5)
	approx(t, "S_VA,ACT", c.ActualValueAddingShare, 1)
}

func TestWorkshopDateCheckedWithoutMeasures(t *testing.T) {
	p := exampleProject()
	p.Measures = nil
	p.Workshop = &Workshop{Date: "31/01/2027"}
	found := false
	for _, i := range EvaluateProject(p).Issues {
		found = found || (i.Severity == SeverityError && i.Subject == "workshop")
	}
	if !found {
		t.Error("invalid workshop date not reported")
	}
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

func TestSaveFailureKeepsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stream.json")
	if err := os.WriteFile(path, []byte("old"), 0o640); err != nil {
		t.Fatal(err)
	}
	bad := &ValueStream{Name: "x", Processes: []Process{{ID: "P", ProcessTime: F(math.NaN())}}}
	if err := Save(path, bad); err == nil {
		t.Fatal("saving a NaN process time did not fail")
	}
	if data, _ := os.ReadFile(path); string(data) != "old" {
		t.Errorf("file changed to %q after a failed save", data)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("%d files in the folder after a failed save, want only the original", len(entries))
	}
}

func TestSaveKeepsPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stream.json")
	if err := os.WriteFile(path, []byte("{}"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, exampleStream()); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o640 {
		t.Errorf("mode after save = %v, want 0640", fi.Mode().Perm())
	}
}

func TestSaveRefusesReadOnlyFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write a read-only file")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "stream.json")
	if err := os.WriteFile(path, []byte("old"), 0o444); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, exampleStream()); err == nil {
		t.Fatal("saving over a read-only file did not fail")
	}
	if data, _ := os.ReadFile(path); string(data) != "old" {
		t.Errorf("read-only file changed to %q", data)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("%d files in the folder after a refused save, want only the original", len(entries))
	}
}

func TestSaveWritesThroughDanglingSymlink(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "sub", "link.json")
	// The relative target is read from the link's folder, and does not exist yet.
	if err := os.Symlink("../target.json", link); err != nil {
		t.Fatal(err)
	}
	if err := Save(link, exampleStream()); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Error("the symbolic link was replaced by a regular file")
	}
	if _, err := LoadValueStream(filepath.Join(dir, "target.json")); err != nil {
		t.Errorf("the link's target was not written: %v", err)
	}
}

func TestWriteValueStampsSchemaVersion(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, *exampleStream()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"schema_version": 1`) {
		t.Errorf("value stream written without its schema version:\n%s", buf.String())
	}
	buf.Reset()
	if err := Write(&buf, *exampleProject()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"schema_version": 1`) {
		t.Errorf("project written without its schema version:\n%s", buf.String())
	}
}

func TestProjectDetectedByAnyProjectKey(t *testing.T) {
	project := `{"name":"p","current":{"name":"c","time_unit":"s"},"measurs":[]}`
	if ok, err := IsProjectJSON([]byte(project)); err != nil || !ok {
		t.Fatalf("IsProjectJSON = %v, %v; want a project", ok, err)
	}
	if _, err := ReadProject(strings.NewReader(project)); err == nil || !strings.Contains(err.Error(), "measurs") {
		t.Errorf("err = %v, want unknown field measurs", err)
	}
	stream := `{"name":"x","time_unit":"s","measurs":[]}`
	if ok, err := IsProjectJSON([]byte(stream)); err != nil || ok {
		t.Errorf("IsProjectJSON = %v, %v for a value stream; want false", ok, err)
	}
}

// A stray key does not decide the kind of file: a value stream with one project
// key is a value stream and names that key, and a project with one misspelt key
// is a project and names the misspelt key.
func TestStrayKeyDoesNotDecideTheKind(t *testing.T) {
	stream := `{"name":"x","time_unit":"s","processes":[{"id":"P"}],"phase":"analysis"}`
	if ok, err := IsProjectJSON([]byte(stream)); err != nil || ok {
		t.Fatalf("IsProjectJSON = %v, %v for a value stream; want false", ok, err)
	}
	if _, err := ReadValueStream(strings.NewReader(stream)); err == nil || !strings.Contains(err.Error(), "phase") {
		t.Errorf("err = %v, want unknown field phase", err)
	}
	project := `{"name":"p","measures":[],"measurs":[]}`
	if ok, err := IsProjectJSON([]byte(project)); err != nil || !ok {
		t.Fatalf("IsProjectJSON = %v, %v for a project; want true", ok, err)
	}
	if _, err := ReadProject(strings.NewReader(project)); err == nil || !strings.Contains(err.Error(), "measurs") {
		t.Errorf("err = %v, want unknown field measurs", err)
	}
}

func TestReadRejectsUnknownFields(t *testing.T) {
	_, err := ReadValueStream(strings.NewReader(`{"name":"x","time_unit":"s","processes":[{"id":"P","proces_time":3}]}`))
	if err == nil || !strings.Contains(err.Error(), "proces_time") {
		t.Errorf("err = %v, want unknown field proces_time", err)
	}
}

func TestMonitoringSnapshots(t *testing.T) {
	p := exampleProject()
	snap := exampleStream()
	snap.Idles[0].IdleTime = F(4)
	p.Monitoring = []Snapshot{
		{Date: "2027-02-01", State: snap},
		{Date: "2027-01-01", State: exampleStream()},
		{Date: "2027-03-01"},
	}
	rep := EvaluateProject(p)
	if len(rep.Monitoring) != 2 || rep.Monitoring[0].Comparison == nil {
		t.Fatalf("monitoring = %+v, want two snapshots with the first compared", rep.Monitoring)
	}
	approx(t, "ω_IT", rep.Monitoring[0].Comparison.Figures.IdleTime.Value, 0.4)
	var order, noState bool
	for _, i := range rep.Issues {
		order = order || (i.Severity == SeverityWarning && i.Subject == "monitoring 2027-01-01")
		noState = noState || (i.Severity == SeverityError && i.Subject == "monitoring 2027-03-01")
	}
	if !order || !noState {
		t.Errorf("date order warning %v, missing state error %v:\n%v", order, noState, rep.Issues)
	}
}

func TestMeasureTargetMet(t *testing.T) {
	for _, c := range []struct {
		kpi    KPIName
		target float64
		met    bool
	}{
		{KPILeadTime, 40, true},           // lead time 39 is below 40, and lower is better
		{KPILeadTime, 38, false},          // 39 is above 38
		{KPIValueAddingShare, 0.85, true}, // 35 ÷ 39 is about 0.90, and higher is better
		{KPIValueAddingShare, 0.95, false},
	} {
		p := exampleProject()
		snap := exampleStream()
		snap.Idles[0].IdleTime = F(4)
		p.Monitoring = []Snapshot{{Date: "2027-01-01", State: snap}}
		p.Measures = []Measure{{Number: 1, Potential: "waiting", Description: "FIFO lane", KPI: c.kpi, Target: F(c.target)}}
		m := EvaluateProject(p).Measures[0]
		if m.Met == nil {
			t.Fatalf("%s: no verdict, measured %v", c.kpi, m.Measured)
		}
		if *m.Met != c.met {
			t.Errorf("%s target %v, measured %v: met = %v, want %v", c.kpi, c.target, m.Measured, *m.Met, c.met)
		}
	}
}

// TestMeasureTargetUsesLatestDateInCurrentUnit lists the latest snapshot
// first and records it in minutes; the current state is in seconds.
func TestMeasureTargetUsesLatestDateInCurrentUnit(t *testing.T) {
	p := exampleProject()
	latest := &ValueStream{Name: "m", TimeUnit: Minute,
		Processes: []Process{{ID: "P1", ProcessTime: F(0.5)}},
		Idles:     []Idle{{ID: "I1", From: "P1", IdleTime: F(0.1)}}} // lead time 0.6 min = 36 s
	p.Monitoring = []Snapshot{{Date: "2027-03-01", State: latest}, {Date: "2027-02-01", State: exampleStream()}}
	p.Measures = []Measure{{Number: 1, Potential: "waiting", Description: "FIFO lane", KPI: KPILeadTime, Target: F(30)}}
	m := EvaluateProject(p).Measures[0]
	approx(t, "measured lead time in s", m.Measured.Value, 36)
	if m.Met == nil || *m.Met {
		t.Errorf("36 s against a 30 s target: met = %v, want false", m.Met)
	}
}

// Without a current state a time target has no unit, so a time KPI gets no
// verdict; the measured value is still reported, in the snapshot's unit.
func TestMeasureTimeTargetNeedsCurrentState(t *testing.T) {
	p := exampleProject()
	p.Current = nil
	snap := &ValueStream{Name: "m", TimeUnit: Second, Processes: []Process{{ID: "P1", ProcessTime: F(600)}}}
	p.Monitoring = []Snapshot{{Date: "2027-01-01", State: snap}}
	p.Measures = []Measure{{Number: 1, Potential: "waiting", Description: "FIFO lane", KPI: KPIProcessTime, Target: F(20)}}
	m := EvaluateProject(p).Measures[0]
	if !m.Measured.Known {
		t.Fatal("process time not measured")
	}
	approx(t, "measured process time in s", m.Measured.Value, 600)
	if m.Met != nil {
		t.Errorf("600 s against a target of 20 with no current state: met = %v, want no verdict", *m.Met)
	}
}

func TestUnknownKPIName(t *testing.T) {
	p := exampleProject()
	p.Measures = []Measure{{Number: 1, Potential: "waiting", Description: "FIFO lane", KPI: "throughput"}}
	found := false
	for _, i := range EvaluateProject(p).Issues {
		found = found || (i.Severity == SeverityError && i.Subject == "measure 1" && i.Rule == "4.4.2")
	}
	if !found {
		t.Error("unknown KPI name not reported")
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

func TestPrioritizeLeavesOutImplementedAndCancelled(t *testing.T) {
	measures := []Measure{
		{Number: 1, Status: StatusProblemDetected, Cost: F(1), Benefit: F(2)},
		{Number: 2, Status: StatusImplemented, Cost: F(1), Benefit: F(9)},
		{Number: 3, Status: StatusCancelled, Cost: F(1), Benefit: F(8)},
		{Number: 4, Status: StatusImplementing, Cost: F(1), Benefit: F(4)},
	}
	if got, want := Prioritize(measures), []int{4, 1}; !slices.Equal(got, want) {
		t.Errorf("priority = %v, want %v", got, want)
	}
}

func TestProjectPhaseIsChecked(t *testing.T) {
	p := exampleProject()
	p.Phase = "review"
	if !slices.ContainsFunc(EvaluateProject(p).Issues, func(i Issue) bool {
		return i.Rule == "4.1" && i.Severity == SeverityError
	}) {
		t.Error("unknown phase not reported")
	}
	p.Phase = PhaseAssessment
	if got := EvaluateProject(p).Issues; slices.ContainsFunc(got, func(i Issue) bool { return i.Rule == "4.1" }) {
		t.Errorf("valid phase reported: %v", got)
	}
}

func TestDifferenceAndPayback(t *testing.T) {
	tests := []struct {
		name        string
		m           Measure
		difference  Value
		payback     Value
		paybackWant float64
	}{
		{"monthly benefit", Measure{Cost: F(120), Benefit: F(30), BenefitPeriod: PeriodMonth}, knownValue(-90, "4.5.2"), knownValue(4, paybackSource), 4},
		{"yearly benefit", Measure{Cost: F(120), Benefit: F(60), BenefitPeriod: PeriodYear}, knownValue(-60, "4.5.2"), knownValue(2, paybackSource), 2},
		{"zero benefit", Measure{Cost: F(100), Benefit: F(0), BenefitPeriod: PeriodWeek}, knownValue(-100, "4.5.2"), knownValue(math.Inf(1), paybackSource), math.Inf(1)},
		{"zero cost", Measure{Cost: F(0), Benefit: F(50), BenefitPeriod: PeriodDay}, knownValue(50, "4.5.2"), knownValue(0, paybackSource), 0},
		{"no period", Measure{Cost: F(100), Benefit: F(50)}, knownValue(-50, "4.5.2"), Value{}, 0},
		{"no benefit", Measure{Cost: F(100), BenefitPeriod: PeriodMonth}, Value{}, Value{}, 0},
		{"negative cost", Measure{Cost: F(-1), Benefit: F(50), BenefitPeriod: PeriodMonth}, Value{}, Value{}, 0},
	}
	for _, c := range tests {
		if got := c.m.Difference(); got != c.difference {
			t.Errorf("%s: difference = %+v, want %+v", c.name, got, c.difference)
		}
		got := c.m.Payback()
		if got.Known != c.payback.Known || got.Source != c.payback.Source {
			t.Errorf("%s: payback = %+v, want %+v", c.name, got, c.payback)
		} else if got.Known && got.Value != c.paybackWant {
			t.Errorf("%s: payback value = %v, want %v", c.name, got.Value, c.paybackWant)
		}
	}
}

func TestMeasurePeriodIsChecked(t *testing.T) {
	p := exampleProject()
	p.Measures[0].BenefitPeriod = "fortnight"
	rep := EvaluateProject(p)
	if !slices.ContainsFunc(rep.Issues, func(i Issue) bool {
		return i.Rule == "4.5.2" && i.Severity == SeverityError
	}) {
		t.Error("unknown benefit period not reported")
	}
	if rep.Measures[0].Payback.Known {
		t.Error("payback known for an unknown benefit period")
	}
	p.Measures[0].BenefitPeriod = PeriodYear
	rep = EvaluateProject(p)
	if rep.Measures[0].Payback.Value != 100.0/300 {
		t.Errorf("payback = %v, want 100/300 years", rep.Measures[0].Payback.Value)
	}
	if slices.ContainsFunc(rep.Issues, func(i Issue) bool { return i.Rule == "4.5.2" }) {
		t.Errorf("valid benefit period reported: %v", rep.Issues)
	}
}

func TestReadRejectsTrailingData(t *testing.T) {
	for _, in := range []string{`{"name":"x"}}`, `{"name":"x"}]`, `{"name":"x"} {}`} {
		if _, err := ReadValueStream(strings.NewReader(in)); err == nil {
			t.Errorf("%s accepted", in)
		}
	}
}
