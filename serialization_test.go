package vsm

import (
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
	vs.FlowElements = []FlowElement{{ID: "f1", Type: "kanban", Capacity: 20, ControlTickets: []string{"k1"}}}
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
	if vs.Customer.AvailableTime != 0 || vs.Customer.TaktTime != 6 {
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
		Parameters: ProcessParameters{TimeUnit: TimeUnitMinutes, ProcessTime: 20, ProcessingTime: 5}}},
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
