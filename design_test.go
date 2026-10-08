package vsm

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// targetStream: one process with a stored ProcessTime and IdleTime, one
// described by components only, plus one inventory point.
func targetStream() *ValueStream {
	return &ValueStream{
		ID:       "vs",
		Name:     "Targets",
		Customer: &Customer{ID: "c", Demand: 100, DemandPeriod: DemandPeriodDay, AvailableTime: 480},
		Processes: []Process{
			{ID: "p1", Name: "Cut", Parameters: ProcessParameters{
				TimeUnit: TimeUnitMinutes, ProcessTime: 10, SetupTime: 4, CycleTime: 3,
				IdleTime: 30, WaitingTime: 20, TransportTime: 10, ValueAddingTime: 5,
				ScrapRate: 0.05, ReworkRate: 0.05, YieldFactor: 0.9,
			}},
			{ID: "p2", Name: "Weld", Parameters: ProcessParameters{
				TimeUnit: TimeUnitMinutes, ProcessingTime: 6, SetupTime: 2, CycleTime: 4,
				WaitingTime: 40, ValueAddingTime: 4,
			}},
		},
		Inventory: []Inventory{{ID: "i1", Name: "WIP", Quantity: 200, Range: 2}},
	}
}

func TestDesignFutureStateInventsNoImprovements(t *testing.T) {
	current := targetStream()
	analysis, err := NewAnalyzer().AnalyzeCurrentState(current)
	if err != nil {
		t.Fatal(err)
	}
	got, err := NewDesigner().DesignFutureState(current, analysis)
	if err != nil {
		t.Fatal(err)
	}
	for i, p := range got.FutureState.Processes {
		if !reflect.DeepEqual(p.Parameters, current.Processes[i].Parameters) {
			t.Errorf("process %s changed without targets:\n got %+v\nwant %+v", p.ID, p.Parameters, current.Processes[i].Parameters)
		}
	}
	if !reflect.DeepEqual(got.FutureState.Inventory, current.Inventory) {
		t.Errorf("inventory changed without targets: %+v", got.FutureState.Inventory)
	}
	if want := NewCalculator().CalculateKPIs(current); !reflect.DeepEqual(got.FutureState.KPIs, want) {
		t.Errorf("future KPIs %v, want current KPIs %v", got.FutureState.KPIs, want)
	}
	if len(got.Notes) == 0 || !strings.Contains(got.Notes[0], "DesignFutureStateWithTargets") {
		t.Errorf("missing no-targets note: %v", got.Notes)
	}
	if got.ImprovementMetrics != nil {
		t.Errorf("improvement metrics without targets: %v", got.ImprovementMetrics)
	}
	for _, fe := range got.FutureState.FlowElements {
		if fe.Capacity != 0 || len(fe.ControlTickets) != 0 {
			t.Errorf("flow element %s has invented sizing: %+v", fe.ID, fe)
		}
	}
	// Current state must not be modified.
	if !reflect.DeepEqual(current, targetStream()) {
		t.Error("current state was modified")
	}
}

func TestDesignFutureStateFromTargets(t *testing.T) {
	current := targetStream()
	targets := &FutureStateTargets{
		// Stream-wide: waiting time −50%.
		Default: &ProcessTargets{WaitingTime: ReductionTarget(50)},
		Processes: map[string]ProcessTargets{
			// p1: setup to 1 min absolute, scrap −100%, cycle time −1/3.
			"p1": {SetupTime: AbsoluteTarget(1), ScrapRate: ReductionTarget(100), CycleTime: AbsoluteTarget(2)},
			// p2: per-process waiting overrides the default.
			"p2": {WaitingTime: AbsoluteTarget(5)},
		},
		Inventory: map[string]InventoryTargets{"i1": {Quantity: ReductionTarget(25)}},
	}
	future, err := NewDesigner().DesignFutureStateFromTargets(current, targets)
	if err != nil {
		t.Fatal(err)
	}
	p1, p2 := future.Processes[0].Parameters, future.Processes[1].Parameters

	checks := []struct {
		name      string
		got, want float64
	}{
		{"p1 setup", p1.SetupTime, 1},
		{"p1 process time follows setup (10 − 3)", p1.ProcessTime, 7},
		{"p1 waiting (default −50%)", p1.WaitingTime, 10},
		{"p1 idle follows waiting (30 − 10)", p1.IdleTime, 20},
		{"p1 scrap", p1.ScrapRate, 0},
		{"p1 yield follows scrap (0.9 + 0.05)", p1.YieldFactor, 0.95},
		{"p1 cycle time", p1.CycleTime, 2},
		{"p2 waiting (own target wins)", p2.WaitingTime, 5},
		{"p2 setup untouched", p2.SetupTime, 2},
		{"inventory quantity", future.Inventory[0].Quantity, 150},
		{"inventory range untouched", future.Inventory[0].Range, 2},
	}
	for _, c := range checks {
		if !approx(c.got, c.want) {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}

	// KPIs come from the calculator: lead time = (7 + 20) + (8 + 5) = 40.
	if lt := future.KPIs["total_lead_time"]; !approx(lt, 40) {
		t.Errorf("future lead time = %v, want 40", lt)
	}
	if !reflect.DeepEqual(future.KPIs, NewCalculator().CalculateKPIs(future)) {
		t.Error("future KPIs do not match the calculator")
	}
}

func TestDesignFutureStateWithTargetsMetrics(t *testing.T) {
	current := targetStream()
	analysis, err := NewAnalyzer().AnalyzeCurrentState(current)
	if err != nil {
		t.Fatal(err)
	}
	targets := &FutureStateTargets{Default: &ProcessTargets{IdleTime: AbsoluteTarget(0)}}
	got, err := NewDesigner().DesignFutureStateWithTargets(current, analysis, targets)
	if err != nil {
		t.Fatal(err)
	}
	// Idle 0 everywhere: lead time = process time only = 10 + 8 = 18; the
	// idle components are zeroed too so they cannot creep back in.
	if lt := got.FutureState.KPIs["total_lead_time"]; !approx(lt, 18) {
		t.Errorf("future lead time = %v, want 18", lt)
	}
	if p2 := got.FutureState.Processes[1].Parameters; p2.WaitingTime != 0 {
		t.Errorf("p2 waiting = %v, want 0", p2.WaitingTime)
	}
	// Current lead time = (10 + 30) + (8 + 40) = 88.
	if r := got.ImprovementMetrics["total_lead_time_improvement_ratio"]; !approx(r, 18.0/88) {
		t.Errorf("lead time ratio = %v, want %v", r, 18.0/88)
	}
}

func TestDesignFutureStateTargetErrors(t *testing.T) {
	d := NewDesigner()
	bad := map[string]*FutureStateTargets{
		"unknown process":   {Processes: map[string]ProcessTargets{"nope": {SetupTime: AbsoluteTarget(1)}}},
		"unknown inventory": {Inventory: map[string]InventoryTargets{"nope": {Quantity: AbsoluteTarget(1)}}},
		"both value and percent": {Default: &ProcessTargets{SetupTime: &Target{
			Value: AbsoluteTarget(1).Value, ReductionPercent: ReductionTarget(10).ReductionPercent}}},
		"empty target":       {Default: &ProcessTargets{SetupTime: &Target{}}},
		"percent above 100":  {Default: &ProcessTargets{SetupTime: ReductionTarget(150)}},
		"negative value":     {Default: &ProcessTargets{SetupTime: AbsoluteTarget(-1)}},
		"rate given as %":    {Default: &ProcessTargets{ScrapRate: AbsoluteTarget(2)}},
		"yield as reduction": {Default: &ProcessTargets{YieldFactor: ReductionTarget(10)}},
	}
	for name, targets := range bad {
		t.Run(name, func(t *testing.T) {
			if _, err := d.DesignFutureStateFromTargets(targetStream(), targets); err == nil {
				t.Error("expected an error")
			}
		})
	}
}

func TestFutureStateTargetsJSON(t *testing.T) {
	data := []byte(`{"default":{"waiting_time":{"reduction_percent":50}},
		"processes":{"p1":{"setup_time":{"value":1}}}}`)
	var targets FutureStateTargets
	if err := json.Unmarshal(data, &targets); err != nil {
		t.Fatal(err)
	}
	future, err := NewDesigner().DesignFutureStateFromTargets(targetStream(), &targets)
	if err != nil {
		t.Fatal(err)
	}
	if p1 := future.Processes[0].Parameters; p1.SetupTime != 1 || p1.WaitingTime != 10 {
		t.Errorf("p1 setup %v waiting %v, want 1 and 10", p1.SetupTime, p1.WaitingTime)
	}
}

func TestDesignResultWithoutNewFieldsStillLoads(t *testing.T) {
	data := []byte(`{"value_stream_id":"vs","improvement_potentials":[],"ideal_state":null,
		"future_state":null,"design_principles":[],"continuous_improvements":[],
		"designed_at":"2026-01-01T00:00:00Z"}`)
	got, err := NewSerializer().DeserializeDesignResult(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.Notes != nil || got.ImprovementMetrics != nil {
		t.Errorf("unexpected new fields: %+v", got)
	}
}
