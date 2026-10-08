package vsm

import (
	"reflect"
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
