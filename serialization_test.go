package vsm

import (
	"encoding/json"
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
		if wantStored == 0 {
			if len(vs.ConsistencyIssues) != 0 {
				t.Errorf("%s: issues = %+v, want none", name, vs.ConsistencyIssues)
			}
			return
		}
		if len(vs.ConsistencyIssues) != 1 || vs.ConsistencyIssues[0].Field != "process_time" ||
			vs.ConsistencyIssues[0].StoredValue != wantStored {
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
	if is := project.CurrentState.ConsistencyIssues; len(is) != 1 || is[0].Field != "process_time" {
		t.Errorf("project file issues = %+v", is)
	}

	future := vs
	data, _ := json.Marshal(&DesignResult{ValueStreamID: "vs", FutureState: &future})
	design, err := s.DeserializeDesignResult(data)
	if err != nil {
		t.Fatal(err)
	}
	if is := design.FutureState.ConsistencyIssues; len(is) != 1 || is[0].Field != "process_time" {
		t.Errorf("design result issues = %+v", is)
	}
}
