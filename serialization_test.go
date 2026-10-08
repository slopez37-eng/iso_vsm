package vsm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestValueStreamRoundTrip(t *testing.T) {
	vs := mixedUnitStream()
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	vs.State = StateCurrent
	vs.ProductFamily = "Brackets"
	vs.Supplier = &Supplier{ID: "s1", Name: "Steel", DeliveryTime: 2}
	vs.FlowElements = []FlowElement{{ID: "f1", Type: "kanban", Capacity: F(20), ControlTickets: []string{"k1"}}}
	vs.KPIs = NewCalculator().CalculateKPIs(vs)
	vs.CreatedAt, vs.UpdatedAt, vs.Version = ts, ts, "1.0"

	s := NewSerializer()
	data, err := s.SerializeValueStream(vs)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	back, err := s.DeserializeValueStream(data)
	if err != nil {
		t.Fatalf("deserialize: %v", err)
	}
	if !reflect.DeepEqual(vs, back) {
		t.Errorf("round trip mismatch\n got: %+v\nwant: %+v", back, vs)
	}
}

func TestCustomerWithoutAvailableTimeStillLoads(t *testing.T) {
	// JSON written before AvailableTime existed must still load.
	data := []byte(`{"id":"vs","name":"Old","state":"current","product_family":"x",
		"customer":{"id":"c","name":"C","demand":10,"takt_time":6},
		"processes":[{"id":"p","name":"P","type":"material","parameters":{"process_time":1}}],
		"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","version":"1"}`)
	vs, err := NewSerializer().DeserializeValueStream(data)
	if err != nil {
		t.Fatalf("deserialize: %v", err)
	}
	if num(vs.Customer.AvailableTime) != 0 || num(vs.Customer.TaktTime) != 6 {
		t.Errorf("unexpected customer: %+v", vs.Customer)
	}
}

func TestExportValueStreamMapShowsRatiosAsPercent(t *testing.T) {
	vs := mixedUnitStream()
	vs.KPIs = nil // the map computes its KPIs; 3.25 ÷ 28 = 11.6%
	out, err := NewSerializer().ExportValueStreamMap(vs)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"value_added_ratio: 11.6%", "total_lead_time: 28.0000", "Takt Time: 0.50 minutes"} {
		if !strings.Contains(out, want) {
			t.Errorf("map text missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Consistency issues") {
		t.Errorf("consistent stream lists issues:\n%s", out)
	}
}

// TestExportUsesComputedKPIsAndListsIssues: stale stored totals and KPIs
// are not printed as facts. The map shows the values the model computes
// and lists the disagreements.
func TestExportUsesComputedKPIsAndListsIssues(t *testing.T) {
	vs := &ValueStream{ID: "vs", Name: "Stale", Processes: []Process{{ID: "p", Name: "Press", Type: ProcessTypeMaterial,
		Parameters: ProcessParameters{TimeUnit: TimeUnitMinutes, ProcessTime: F(20), ProcessingTime: F(5)}}},
		KPIs: map[string]float64{"total_lead_time": 20, "team_morale": 7},
	}
	out, err := NewSerializer().ExportValueStreamMap(vs)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Process Time: 5.00 minutes",
		"total_lead_time: 5.0000",
		"team_morale: 7.0000", // a KPI the library does not compute is kept
		"Consistency issues (2)",
		"process p: process_time: stored 20",
		"stored KPI total_lead_time = 20",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("map text missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "total_lead_time: 20") {
		t.Errorf("map text prints the stale stored lead time:\n%s", out)
	}
}

// TestDeserializeVSMProjectRefreshesIssues: a project load reports each
// state's consistency issues like a value stream load does, and does not
// keep a stale issues list from the file.
func TestDeserializeVSMProjectRefreshesIssues(t *testing.T) {
	stream := func(id string, processTime float64) ValueStream {
		return ValueStream{ID: id, Name: id, Processes: []Process{{ID: "p", Name: "P",
			Parameters: ProcessParameters{TimeUnit: TimeUnitMinutes, ProcessTime: F(processTime), ProcessingTime: F(5)}}},
			// A stale list, as if written by an older run.
			ConsistencyIssues: []ConsistencyIssue{{Element: "process", ID: "gone", Field: "idle_time", Message: "stale"}},
		}
	}
	future, ideal := stream("future", 5), stream("ideal", 9)
	project := &VSMProject{ID: "proj", Name: "Project", CurrentState: stream("current", 20),
		FutureState: &future, IdealState: &ideal}
	data, err := json.Marshal(project)
	if err != nil {
		t.Fatal(err)
	}
	back, err := NewSerializer().DeserializeVSMProject(data)
	if err != nil {
		t.Fatal(err)
	}
	check := func(name string, vs *ValueStream, wantStored float64) {
		issues := contradictions(vs.ConsistencyIssues)
		if wantStored == 0 {
			if len(issues) != 0 {
				t.Errorf("%s: issues = %+v, want none", name, vs.ConsistencyIssues)
			}
			return
		}
		if len(issues) != 1 || issues[0].Field != "process_time" || issues[0].StoredValue != wantStored {
			t.Errorf("%s: issues = %+v, want one process_time issue (stored %v)", name, vs.ConsistencyIssues, wantStored)
		}
	}
	check("current", &back.CurrentState, 20)
	check("future", back.FutureState, 0)
	check("ideal", back.IdealState, 9)
}

// TestProjectAndDesignFileLoadsRefreshIssues: the file and design-result
// load paths also replace stale issue lists.
func TestProjectAndDesignFileLoadsRefreshIssues(t *testing.T) {
	stale := []ConsistencyIssue{{Element: "process", ID: "gone", Message: "stale"}}
	vs := ValueStream{ID: "vs", Name: "VS", ConsistencyIssues: stale, Processes: []Process{{ID: "p", Name: "P",
		Parameters: ProcessParameters{ProcessTime: F(20), ProcessingTime: F(5)}}}}
	s := NewSerializer()

	path := t.TempDir() + "/project.json"
	if err := s.SaveToFile(&VSMProject{ID: "proj", Name: "P", CurrentState: vs}, path); err != nil {
		t.Fatal(err)
	}
	var project VSMProject
	if err := s.LoadFromFile(path, &project); err != nil {
		t.Fatal(err)
	}
	if is := contradictions(project.CurrentState.ConsistencyIssues); len(is) != 1 || is[0].Field != "process_time" {
		t.Errorf("project file issues = %+v", is)
	}

	future := vs
	data, _ := json.Marshal(&DesignResult{ValueStreamID: "vs", FutureState: &future})
	design, err := s.DeserializeDesignResult(data)
	if err != nil {
		t.Fatal(err)
	}
	if is := contradictions(design.FutureState.ConsistencyIssues); len(is) != 1 || is[0].Field != "process_time" {
		t.Errorf("design result issues = %+v", is)
	}
}

// legacyStream is a value stream as written before format_version 2: every
// blank number stored as 0.
const legacyStream = `{
  "id": "vs", "name": "Legacy", "state": "current", "product_family": "f",
  "customer": {"id": "c", "name": "C", "demand": 100, "takt_time": 0,
               "demand_period": "day", "available_time": 0},
  "processes": [
    {"id": "p1", "name": "Press", "type": "material", "parameters": {
      "process_time": 30, "cycle_time": 2, "setup_time": 0, "processing_time": 0,
      "idle_time": 0, "waiting_time": 0, "transport_time": 0, "storage_time": 0,
      "transport_repetition_count": 0, "repetition_count": 0, "resource_count": 0,
      "stock_quantity": 0, "value_adding_time": 10, "non_value_adding_time": 0,
      "yield_factor": 0, "scrap_rate": 0.02, "rework_rate": 0, "lead_time": 0,
      "time_unit": "minutes"}},
    {"id": "p2", "name": "Scrap all", "type": "material", "parameters": {
      "process_time": 20, "setup_time": 0, "yield_factor": 0, "yield_factor_given": true,
      "scrap_rate": 0, "rework_rate": 0, "time_unit": "minutes"}},
    {"id": "p3", "name": "Paint", "type": "material", "parameters": {
      "process_time": 10, "yield_factor": 0, "time_unit": "minutes"}}
  ],
  "inventory": [{"id": "i", "name": "WIP", "quantity": 0, "range": 2880, "type": "stock"}],
  "flow_elements": [{"id": "k", "type": "kanban", "capacity": 0}],
  "created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-01T00:00:00Z", "version": "1"
}`

// A legacy file keeps its old meaning: 0 is "not entered", except a yield
// marked by yield_factor_given. Saving writes the current format, and
// loading that gives the same values and results.
func TestLegacyFileZerosBecomeNotEntered(t *testing.T) {
	s := NewSerializer()
	vs, err := s.DeserializeValueStream([]byte(legacyStream))
	if err != nil {
		t.Fatal(err)
	}
	p1, p2, p3 := vs.Processes[0].Parameters, vs.Processes[1].Parameters, vs.Processes[2].Parameters
	for name, v := range map[string]*float64{
		"setup_time": p1.SetupTime, "processing_time": p1.ProcessingTime, "idle_time": p1.IdleTime,
		"waiting_time": p1.WaitingTime, "stock_quantity": p1.StockQuantity, "yield_factor": p1.YieldFactor,
		"rework_rate": p1.ReworkRate, "lead_time": p1.LeadTime, "non_value_adding_time": p1.NonValueAddingTime,
		"p2 scrap_rate": p2.ScrapRate, "p3 yield_factor": p3.YieldFactor,
		"takt_time": vs.Customer.TaktTime, "available_time": vs.Customer.AvailableTime,
		"inventory quantity": vs.Inventory[0].Quantity, "capacity": vs.FlowElements[0].Capacity,
	} {
		if v != nil {
			t.Errorf("legacy %s = %v, want not entered", name, *v)
		}
	}
	if p1.ResourceCount != nil || p1.RepetitionCount != nil || p1.TransportRepetitionCount != nil {
		t.Errorf("legacy zero counts must be not entered: %+v", p1)
	}
	if p2.YieldFactor == nil || *p2.YieldFactor != 0 {
		t.Errorf("yield_factor_given zero yield = %v, want an entered 0", p2.YieldFactor)
	}
	if num(p1.ProcessTime) != 30 || num(p1.ScrapRate) != 0.02 || num(vs.Inventory[0].Range) != 2880 {
		t.Errorf("non-zero values changed: %+v", p1)
	}
	if len(contradictions(vs.ConsistencyIssues)) != 0 {
		t.Errorf("legacy stream issues: %+v", vs.ConsistencyIssues)
	}
	// Same results as before the format change: lead time 30 + 20 + 10 +
	// 2 days of inventory; yields 0.98 (p1) and 0 (p2), p3 unknown.
	kpis := NewCalculator().CalculateKPIs(vs)
	if !approx(kpis["total_lead_time"], 60+2880) || !approx(kpis["average_yield_factor"], 0.49) ||
		!approx(kpis["inventory_turns"], 0.5) {
		t.Errorf("legacy KPIs = %v", kpis)
	}

	// Saved in the current format: marked, zeros that meant "not entered"
	// gone, the entered zero yield kept.
	data, err := s.SerializeValueStream(vs)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, `"format_version": 2`) || strings.Contains(text, `"setup_time"`) ||
		strings.Contains(text, `"yield_factor_given"`) || strings.Count(text, `"yield_factor": 0`) != 1 {
		t.Errorf("saved stream:\n%s", text)
	}
	back, err := s.DeserializeValueStream(data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back.Processes, vs.Processes) || !reflect.DeepEqual(back.Inventory, vs.Inventory) ||
		!reflect.DeepEqual(back.Customer, vs.Customer) {
		t.Errorf("save → load changed the stream")
	}
	if !reflect.DeepEqual(NewCalculator().CalculateKPIs(back), kpis) {
		t.Errorf("KPIs after save → load differ")
	}
}

// In the current format, 0 is an entered zero and an absent (or null)
// field is not entered.
func TestFormat2ZeroVersusAbsent(t *testing.T) {
	data := `{"format_version": 2, "id": "vs", "name": "New", "processes": [
	  {"id": "a", "name": "A", "parameters": {"process_time": 30, "setup_time": 0, "time_unit": "minutes"}},
	  {"id": "b", "name": "B", "parameters": {"process_time": 30, "setup_time": null, "time_unit": "minutes"}}],
	  "inventory": [{"id": "i", "name": "I", "quantity": 0}]}`
	vs, err := NewSerializer().DeserializeValueStream([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	a, b := vs.Processes[0].Parameters, vs.Processes[1].Parameters
	if a.SetupTime == nil || *a.SetupTime != 0 || b.SetupTime != nil {
		t.Fatalf("setup a %v b %v, want entered 0 and not entered", a.SetupTime, b.SetupTime)
	}
	if q := vs.Inventory[0].Quantity; q == nil || *q != 0 {
		t.Errorf("inventory quantity %v, want an entered 0", q)
	}
	// The entered 0 setup is a part: process a's time is its parts (0),
	// and the stored 30 is reported; b keeps its total.
	if is := contradictions(vs.ConsistencyIssues); len(is) != 1 || is[0].ID != "a" {
		t.Errorf("issues = %+v, want one on process a", vs.ConsistencyIssues)
	}
	c := NewCalculator()
	if pt := num(c.ResolveParametersInStream(a, vs).ProcessTime); pt != 0 {
		t.Errorf("process a time = %v, want 0", pt)
	}
	if pt := num(c.ResolveParametersInStream(b, vs).ProcessTime); pt != 30 {
		t.Errorf("process b time = %v, want 30", pt)
	}
}

// Saving and loading keeps which values were entered, zeros included.
func TestSaveLoadPreservesPresence(t *testing.T) {
	vs := &ValueStream{ID: "vs", Name: "P",
		Customer: &Customer{ID: "c", Name: "C", Demand: F(0), WorkingDaysPerWeek: F(5)},
		Processes: []Process{{ID: "p", Name: "P", Parameters: ProcessParameters{
			TimeUnit: TimeUnitMinutes, ProcessTime: F(12), SetupTime: F(0), ProcessingTime: F(12),
			StockQuantity: F(0), YieldFactor: F(0), ReworkRate: F(0), ScrapRate: F(1),
			ResourceCount: I(0), RepetitionTime: nil}}},
		Inventory:    []Inventory{{ID: "i", Name: "I", Quantity: F(0)}, {ID: "j", Name: "J", Range: F(3)}},
		FlowElements: []FlowElement{{ID: "f", Type: "fifo", Capacity: F(0)}, {ID: "g", Type: "kanban"}},
	}
	path := filepath.Join(t.TempDir(), "vs.json")
	s := NewSerializer()
	if err := s.SaveToFile(vs, path); err != nil {
		t.Fatal(err)
	}
	back, _, err := s.LoadValueStreamFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back.Processes, vs.Processes) || !reflect.DeepEqual(back.Customer, vs.Customer) ||
		!reflect.DeepEqual(back.Inventory, vs.Inventory) || !reflect.DeepEqual(back.FlowElements, vs.FlowElements) {
		t.Errorf("presence changed by save → load:\nsaved  %+v\nloaded %+v", vs.Processes[0].Parameters, back.Processes[0].Parameters)
	}
}

// Projects and design results: a nested stream follows its own
// format_version, else the container's; a legacy container (no marker)
// is read with the old meaning.
func TestNestedStreamsFollowFormatVersion(t *testing.T) {
	stream := func(marker string) string {
		return `{` + marker + `"id": "s", "name": "S", "processes": [{"id": "p", "name": "P",
		  "parameters": {"process_time": 10, "setup_time": 0, "processing_time": 10}}]}`
	}
	setup := func(vs *ValueStream) *float64 { return vs.Processes[0].Parameters.SetupTime }
	s := NewSerializer()

	// Legacy project: setup 0 is not entered.
	legacy := `{"id": "pr", "name": "Pr", "current_state": ` + stream("") + `, "future_state": ` + stream("") + `}`
	project, err := s.DeserializeVSMProject([]byte(legacy))
	if err != nil {
		t.Fatal(err)
	}
	if setup(&project.CurrentState) != nil || setup(project.FutureState) != nil || project.IdealState != nil {
		t.Errorf("legacy project: setup %v / %v, ideal %v", setup(&project.CurrentState), setup(project.FutureState), project.IdealState)
	}
	// Marked project: nested streams without a marker are format 2.
	marked := `{"format_version": 2, "id": "pr", "name": "Pr", "current_state": ` + stream("") +
		`, "future_state": ` + stream(`"format_version": 1, `) + `}`
	if project, err = s.DeserializeVSMProject([]byte(marked)); err != nil {
		t.Fatal(err)
	}
	if v := setup(&project.CurrentState); v == nil || *v != 0 {
		t.Errorf("marked project: current setup %v, want an entered 0", v)
	}
	if v := setup(project.FutureState); v != nil {
		t.Errorf("stream marked format 1 in a format 2 project: setup %v, want not entered", *v)
	}
	// Written projects mark the container and every stream.
	data, err := s.SerializeVSMProject(project)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(data), `"format_version": 2`); n != 3 {
		t.Errorf("saved project has %d format markers, want 3 (project, current, future)", n)
	}
	again, err := s.DeserializeVSMProject(data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(again.CurrentState.Processes, project.CurrentState.Processes) ||
		!reflect.DeepEqual(again.FutureState.Processes, project.FutureState.Processes) {
		t.Errorf("project save → load changed the streams")
	}

	// Design results.
	design := `{"value_stream_id": "s", "future_state": ` + stream("") + `, "ideal_state": null}`
	result, err := s.DeserializeDesignResult([]byte(design))
	if err != nil {
		t.Fatal(err)
	}
	if setup(result.FutureState) != nil || result.IdealState != nil {
		t.Errorf("legacy design result: setup %v", setup(result.FutureState))
	}
	if result, err = s.DeserializeDesignResult([]byte(`{"format_version": 2, ` + design[1:])); err != nil {
		t.Fatal(err)
	}
	if v := setup(result.FutureState); v == nil || *v != 0 {
		t.Errorf("marked design result: setup %v, want an entered 0", v)
	}
}

// Legacy assessment results: working_days_per_week 0 is not entered, and a
// target of 0 counts only with has_target.
func TestLegacyAssessmentResult(t *testing.T) {
	legacy := `{"value_stream_id": "vs", "working_days_per_week": 0, "kpis": {
	  "total_lead_time": {"name": "total_lead_time", "value": 10, "target": 0, "has_target": true},
	  "value_added_ratio": {"name": "value_added_ratio", "value": 0.2, "target": 0}}}`
	got, err := NewSerializer().DeserializeAssessmentResult([]byte(legacy))
	if err != nil {
		t.Fatal(err)
	}
	if got.WorkingDaysPerWeek != nil {
		t.Errorf("working days = %v, want not entered", *got.WorkingDaysPerWeek)
	}
	if tgt := got.KPIs["total_lead_time"].Target; tgt == nil || *tgt != 0 {
		t.Errorf("has_target zero target = %v, want an entered 0", tgt)
	}
	if tgt := got.KPIs["value_added_ratio"].Target; tgt != nil {
		t.Errorf("target without has_target = %v, want none", *tgt)
	}
	data, err := NewSerializer().SerializeAssessmentResult(got)
	if err != nil {
		t.Fatal(err)
	}
	back, err := NewSerializer().DeserializeAssessmentResult(data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"format_version": 2`) || !reflect.DeepEqual(back.KPIs, got.KPIs) {
		t.Errorf("assessment save → load: %s", data)
	}
}

// An assessment result as main wrote it (no format_version, no
// has_target; written by main's SerializeAssessmentResult): a target that
// is not 0 is kept, a target of 0 is not entered (main wrote 0 for "no
// target" too).
func TestMainFormatAssessmentKeepsTargets(t *testing.T) {
	main := `{
  "value_stream_id": "vs",
  "assessment_date": "2025-01-02T00:00:00Z",
  "kpis": {
    "total_lead_time": {
      "name": "total_lead_time", "value": 120, "unit": "time", "target": 100, "threshold": 0,
      "status": "warning", "trend": "stable", "measured_at": "2025-01-02T00:00:00Z", "description": ""
    },
    "value_added_ratio": {
      "name": "value_added_ratio", "value": 0.2, "unit": "%", "target": 0, "threshold": 0,
      "status": "excellent", "trend": "stable", "measured_at": "2025-01-02T00:00:00Z", "description": ""
    }
  },
  "performance_index": 0, "target_achievement": null, "trends": null, "action_items": null,
  "pdca_status": {
    "plan": {"status": "", "completed_at": "0001-01-01T00:00:00Z"},
    "do": {"status": "", "completed_at": "0001-01-01T00:00:00Z"},
    "check": {"status": "", "completed_at": "0001-01-01T00:00:00Z"},
    "act": {"status": "", "completed_at": "0001-01-01T00:00:00Z"}
  },
  "recommendations": null
}`
	got, err := NewSerializer().DeserializeAssessmentResult([]byte(main))
	if err != nil {
		t.Fatal(err)
	}
	if tgt := got.KPIs["total_lead_time"].Target; tgt == nil || *tgt != 100 {
		t.Errorf("main-format target 100 = %v, want kept", tgt)
	}
	if tgt := got.KPIs["value_added_ratio"].Target; tgt != nil {
		t.Errorf("main-format target 0 = %v, want not entered", *tgt)
	}
}

// A file whose process has a time_unit the model cannot convert (here
// "min", which main accepted and saved) still loads and saves, in the
// legacy and the current format: the unit is kept as entered, reported as
// a contradiction, and that process's times count as unknown in the stream
// totals. The Validator itself still rejects the unit.
func TestUnsupportedTimeUnitLoads(t *testing.T) {
	legacy := strings.Replace(legacyStream,
		`"process_time": 10, "yield_factor": 0, "time_unit": "minutes"`,
		`"process_time": 10, "yield_factor": 0, "time_unit": "min"`, 1)
	if !strings.Contains(legacy, `"min"`) {
		t.Fatal("fixture not changed")
	}
	v2 := `{"format_version": 2, "id": "vs", "name": "New", "processes": [
	  {"id": "p1", "name": "A", "parameters": {"process_time": 5, "value_adding_time": 1, "time_unit": "minutes"}},
	  {"id": "p3", "name": "B", "parameters": {"process_time": 10, "value_adding_time": 2, "time_unit": "min"}}]}`
	s := NewSerializer()
	dir := t.TempDir()
	for name, data := range map[string]string{"legacy": legacy, "format 2": v2} {
		t.Run(name, func(t *testing.T) {
			check := func(how string, vs *ValueStream, issues []ConsistencyIssue) {
				t.Helper()
				if u := vs.Processes[len(vs.Processes)-1].Parameters.TimeUnit; u != "min" {
					t.Errorf("%s: time_unit %q, want %q kept as entered", how, u, "min")
				}
				if issue, ok := findIssue(issues, "process", "p3", "time_unit"); !ok || issue.Kind != IssueContradiction {
					t.Errorf("%s: issues %+v, want a time_unit contradiction on p3", how, issues)
				}
				if lt, ok := NewCalculator().CalculateKPIs(vs)["total_lead_time"]; ok {
					t.Errorf("%s: total_lead_time %v, want not computed", how, lt)
				}
			}
			vs, issues, err := s.DeserializeValueStreamWithIssues([]byte(data))
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			check("deserialize", vs, issues)
			project, err := s.DeserializeVSMProject([]byte(`{"id": "pr", "name": "Pr", "current_state": ` + data + `}`))
			if err != nil {
				t.Fatalf("project load: %v", err)
			}
			check("project", &project.CurrentState, project.CurrentState.ConsistencyIssues)
			out, err := s.SerializeValueStream(vs)
			if err != nil {
				t.Fatalf("save: %v", err)
			}
			file := filepath.Join(dir, strings.ReplaceAll(name, " ", "_")+".json")
			if err := os.WriteFile(file, out, 0o644); err != nil {
				t.Fatal(err)
			}
			back, issues, err := s.LoadValueStreamFromFile(file)
			if err != nil {
				t.Fatalf("reload: %v", err)
			}
			check("reload", back, issues)
			if _, err := s.SerializeVSMProject(project); err != nil {
				t.Errorf("project save: %v", err)
			}
		})
	}
	vs, _ := s.DeserializeValueStream([]byte(v2))
	if err := NewValidator().ValidateValueStream(vs); err == nil {
		t.Error("Validator accepted time_unit \"min\"")
	}
}

// A legacy assessment file's "threshold" was a built-in default: 0 and the
// old default for that KPI load as not set; any other value is kept.
func TestLegacyAssessmentThreshold(t *testing.T) {
	legacy := `{"value_stream_id": "vs", "assessment_date": "2025-01-02T00:00:00Z", "kpis": {
	  "total_lead_time": {"name": "total_lead_time", "value": 120, "target": 100, "threshold": 100},
	  "inventory_turns": {"name": "inventory_turns", "value": 3, "target": 6, "threshold": 0},
	  "value_added_ratio": {"name": "value_added_ratio", "value": 0.2, "target": 0.3, "threshold": 0.4}}}`
	got, err := NewSerializer().DeserializeAssessmentResult([]byte(legacy))
	if err != nil {
		t.Fatal(err)
	}
	if th := got.KPIs["total_lead_time"].Threshold; th != nil {
		t.Errorf("legacy threshold equal to the old default = %v, want not set", *th)
	}
	if th := got.KPIs["inventory_turns"].Threshold; th != nil {
		t.Errorf("legacy threshold 0 = %v, want not set", *th)
	}
	if th := got.KPIs["value_added_ratio"].Threshold; th == nil || *th != 0.4 {
		t.Errorf("legacy threshold 0.4 = %v, want kept", th)
	}
	// Current format: nothing is dropped.
	v2 := `{"format_version": 2, "value_stream_id": "vs", "kpis": {
	  "total_lead_time": {"name": "total_lead_time", "value": 120, "target": 100, "threshold": 100}}}`
	cur, err := NewSerializer().DeserializeAssessmentResult([]byte(v2))
	if err != nil {
		t.Fatal(err)
	}
	if th := cur.KPIs["total_lead_time"].Threshold; th == nil || *th != 100 {
		t.Errorf("current-format threshold = %v, want kept", th)
	}
}
