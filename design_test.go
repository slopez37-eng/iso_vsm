package vsm

import (
	"encoding/json"
	"fmt"
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
				TimeUnit: TimeUnitMinutes, ProcessTime: 10, SetupTime: 4, HandlingTime: 6, CycleTime: 3,
				IdleTime: 30, WaitingTime: 20, TransportTime: 10, ValueAddingTime: 5,
				ScrapRate: 0.05, ReworkRate: 0.05, YieldFactor: 0.9,
			}},
			{ID: "p2", Name: "Weld", Parameters: ProcessParameters{
				TimeUnit: TimeUnitMinutes, ProcessingTime: 6, SetupTime: 2, CycleTime: 4,
				WaitingTime: 40, ValueAddingTime: 4,
			}},
		},
		// 200 at 100 per day last 2 days = 2880 min.
		Inventory: []Inventory{{ID: "i1", Name: "WIP", Quantity: 200, Range: 2880}},
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
		{"inventory range follows quantity (150 ÷ 100 per day)", future.Inventory[0].Range, 2160},
	}
	for _, c := range checks {
		if !approx(c.got, c.want) {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}

	// KPIs come from the calculator: lead time = (7 + 20) + (8 + 5) = 40
	// for the processes + 2160 for the inventory.
	if lt := future.KPIs["total_lead_time"]; !approx(lt, 2200) {
		t.Errorf("future lead time = %v, want 2200", lt)
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
	// Idle 0 everywhere: process lead time = process time only = 10 + 8 =
	// 18, plus the untargeted inventory (2880); the idle components are
	// zeroed too so they cannot creep back in.
	if lt := got.FutureState.KPIs["total_lead_time"]; !approx(lt, 18+2880) {
		t.Errorf("future lead time = %v, want 2898", lt)
	}
	if p2 := got.FutureState.Processes[1].Parameters; p2.WaitingTime != 0 {
		t.Errorf("p2 waiting = %v, want 0", p2.WaitingTime)
	}
	// Current lead time = (10 + 30) + (8 + 40) + 2880 = 2968.
	if r := got.ImprovementMetrics["total_lead_time_improvement_ratio"]; !approx(r, 2898.0/2968) {
		t.Errorf("lead time ratio = %v, want %v", r, 2898.0/2968)
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

// richStream is a consistent stream that exercises every relationship:
// A.2 parts with a stored total, A.2 parts alone, A.5 processing
// time, A.6 repetition, A.10 transport repetitions, stock-driven waiting
// time (stock ÷ demand rate), a value classification and scrap/rework.
func richStream() *ValueStream {
	vs := targetStream()
	vs.Processes = append(vs.Processes,
		Process{ID: "p3", Name: "Batch", Parameters: ProcessParameters{
			TimeUnit: TimeUnitMinutes, CycleTime: 2, OrderQuantity: 10, ResourceCount: 1, SetupTime: 5,
			// PT 20 + 5 + 3 = 28; IT 6 + 72 (5 in stock at 100/day) + 4 = 82; LT 110.
			HandlingTime: 3, StockQuantity: 5, TransportRepetitionTime: 3, TransportRepetitionCount: 2,
			StorageTime: 4, ValueAddingTime: 12, NonValueAddingTime: 92, NecessaryNonValueAddingTime: 6,
			ScrapRate: 0.04, ReworkRate: 0.06,
		}},
		Process{ID: "p4", Name: "Repeat", Parameters: ProcessParameters{
			TimeUnit: TimeUnitHours, SetupTime: 0.5, RepetitionTime: 0.25, RepetitionCount: 4,
			IdleTime: 2, YieldFactor: 0.97,
		}},
	)
	return vs
}

// TestTargetsOutputIsConsistent: whatever targets are applied, the future
// state contains no contradictions.
func TestTargetsOutputIsConsistent(t *testing.T) {
	if issues := CheckConsistency(richStream()); len(issues) != 0 {
		t.Fatalf("richStream inconsistent: %+v", issues)
	}
	cases := map[string]*FutureStateTargets{
		"none":              nil,
		"cycle time −50%":   {Default: &ProcessTargets{CycleTime: ReductionTarget(50)}},
		"setup −50%":        {Default: &ProcessTargets{SetupTime: ReductionTarget(50)}},
		"idle −30%":         {Default: &ProcessTargets{IdleTime: ReductionTarget(30)}},
		"idle 0":            {Default: &ProcessTargets{IdleTime: AbsoluteTarget(0)}},
		"waiting −40%":      {Processes: map[string]ProcessTargets{"p1": {WaitingTime: ReductionTarget(40)}, "p3": {WaitingTime: ReductionTarget(40)}}},
		"stock −50%":        {Default: &ProcessTargets{StockQuantity: ReductionTarget(50)}},
		"storage 0":         {Default: &ProcessTargets{StorageTime: AbsoluteTarget(0)}},
		"scrap/rework −50%": {Default: &ProcessTargets{ScrapRate: ReductionTarget(50), ReworkRate: ReductionTarget(50)}},
		"scrap 0 rework 0":  {Default: &ProcessTargets{ScrapRate: AbsoluteTarget(0), ReworkRate: AbsoluteTarget(0)}},
		"yield 0.98":        {Processes: map[string]ProcessTargets{"p1": {YieldFactor: AbsoluteTarget(0.98)}, "p3": {YieldFactor: AbsoluteTarget(0.98)}}},
		"process time −20%": {Processes: map[string]ProcessTargets{"p1": {ProcessTime: ReductionTarget(20)}, "p2": {ProcessTime: ReductionTarget(20)}, "p3": {ProcessTime: ReductionTarget(10)}}},
		"mixed": {
			Default: &ProcessTargets{IdleTime: ReductionTarget(25), SetupTime: ReductionTarget(20)},
			Processes: map[string]ProcessTargets{
				"p3": {CycleTime: AbsoluteTarget(1.5), StorageTime: AbsoluteTarget(1), ScrapRate: AbsoluteTarget(0.01)},
			},
		},
	}
	for name, targets := range cases {
		t.Run(name, func(t *testing.T) {
			future, err := NewDesigner().DesignFutureStateFromTargets(richStream(), targets)
			if err != nil {
				t.Fatal(err)
			}
			if issues := CheckConsistency(future); len(issues) != 0 {
				t.Errorf("future state issues: %+v", issues)
			}
		})
	}
}

func TestTargetsFollowFormulas(t *testing.T) {
	d := NewDesigner()
	param := func(vs *ValueStream, id string) ProcessParameters {
		for _, p := range vs.Processes {
			if p.ID == id+"_future" {
				return d.calculator.ResolveParametersInStream(p.Parameters, vs)
			}
		}
		t.Fatalf("no process %s", id)
		return ProcessParameters{}
	}
	run := func(targets *FutureStateTargets) *ValueStream {
		t.Helper()
		future, err := d.DesignFutureStateFromTargets(richStream(), targets)
		if err != nil {
			t.Fatal(err)
		}
		return future
	}

	// Cycle time halves: processing time (A.5) halves. Stock waiting time
	// does not change: it is stock ÷ demand rate, not stock × cycle time.
	p3 := param(run(&FutureStateTargets{Processes: map[string]ProcessTargets{"p3": {CycleTime: ReductionTarget(50)}}}), "p3")
	if !approx(p3.ProcessingTime, 10) || !approx(p3.WaitingTime, 72) || !approx(p3.ProcessTime, 18) || !approx(p3.IdleTime, 82) {
		t.Errorf("cycle time target: PRT %v WT %v PT %v IT %v, want 10 72 18 82", p3.ProcessingTime, p3.WaitingTime, p3.ProcessTime, p3.IdleTime)
	}
	// Lead time fell 10 (110 → 100): taken from NVAT (92 → 82).
	if !approx(p3.NonValueAddingTime, 82) || !approx(p3.NecessaryNonValueAddingTime, 6) {
		t.Errorf("NVAT %v NNVAT %v, want 82 and 6", p3.NonValueAddingTime, p3.NecessaryNonValueAddingTime)
	}

	// A waiting-time target on a stock-driven process sets the stock:
	// 10 min at 100 per day (1440 min).
	perMinute := 100.0 / 1440
	p3 = param(run(&FutureStateTargets{Processes: map[string]ProcessTargets{"p3": {WaitingTime: AbsoluteTarget(10)}}}), "p3")
	if !approx(p3.StockQuantity, 10*perMinute) || !approx(p3.WaitingTime, 10) {
		t.Errorf("waiting target: SQ %v WT %v, want %v and 10", p3.StockQuantity, p3.WaitingTime, 10*perMinute)
	}

	// Idle time −50% with transport fixed by A.10 (6): waiting and storage
	// (72 + 4) scale to 41 − 6 = 35; waiting through the stock.
	p3 = param(run(&FutureStateTargets{Processes: map[string]ProcessTargets{"p3": {IdleTime: ReductionTarget(50)}}}), "p3")
	if !approx(p3.IdleTime, 41) || !approx(p3.TransportTime, 6) || !approx(p3.WaitingTime, 72*35.0/76) ||
		!approx(p3.StockQuantity, p3.WaitingTime*perMinute) {
		t.Errorf("idle target: IT %v TT %v WT %v SQ %v", p3.IdleTime, p3.TransportTime, p3.WaitingTime, p3.StockQuantity)
	}

	// Process time target on p1 (PT 10 = setup 4 + handling 6): −50%
	// scales both; with setup also targeted to 1, handling takes the rest.
	p1 := run(&FutureStateTargets{Processes: map[string]ProcessTargets{"p1": {ProcessTime: ReductionTarget(50)}}}).Processes[0].Parameters
	if !approx(p1.ProcessTime, 5) || !approx(p1.SetupTime, 2) {
		t.Errorf("PT −50%%: PT %v ST %v, want 5 and 2", p1.ProcessTime, p1.SetupTime)
	}
	p1 = run(&FutureStateTargets{Processes: map[string]ProcessTargets{"p1": {ProcessTime: AbsoluteTarget(5), SetupTime: AbsoluteTarget(1)}}}).Processes[0].Parameters
	if !approx(p1.ProcessTime, 5) || !approx(p1.SetupTime, 1) {
		t.Errorf("PT 5 + setup 1: PT %v ST %v", p1.ProcessTime, p1.SetupTime)
	}

	// Yield target splits over untargeted scrap and rework in proportion.
	p3 = param(run(&FutureStateTargets{Processes: map[string]ProcessTargets{"p3": {YieldFactor: AbsoluteTarget(0.95)}}}), "p3")
	if !approx(p3.ScrapRate, 0.02) || !approx(p3.ReworkRate, 0.03) || !approx(p3.YieldFactor, 0.95) {
		t.Errorf("yield target: SR %v RR %v YF %v", p3.ScrapRate, p3.ReworkRate, p3.YieldFactor)
	}
	// Scrap and rework to 0: yield 1, stored.
	p1 = run(&FutureStateTargets{Processes: map[string]ProcessTargets{"p1": {ScrapRate: AbsoluteTarget(0), ReworkRate: AbsoluteTarget(0)}}}).Processes[0].Parameters
	if p1.YieldFactor != 1 {
		t.Errorf("yield with no defects = %v, want 1", p1.YieldFactor)
	}

	// Setup on the repetitive process (A.6) moves its process time.
	p4 := param(run(&FutureStateTargets{Processes: map[string]ProcessTargets{"p4": {SetupTime: AbsoluteTarget(0.25)}}}), "p4")
	if !approx(p4.ProcessTime, 1.25) {
		t.Errorf("A.6 process time = %v, want 1.25", p4.ProcessTime)
	}
}

func TestTargetsThatCannotBeConsistent(t *testing.T) {
	bad := map[string]*FutureStateTargets{
		// NVAT changes but lead time does not: the two breakdowns would disagree.
		"NVAT alone": {Processes: map[string]ProcessTargets{"p3": {NonValueAddingTime: ReductionTarget(50)}}},
		// Transport is transport_repetition_time × count (A.10).
		"transport on A.10": {Processes: map[string]ProcessTargets{"p3": {TransportTime: AbsoluteTarget(1)}}},
		// Waiting is stock ÷ demand rate: set one of them.
		"waiting and stock": {Processes: map[string]ProcessTargets{"p3": {WaitingTime: AbsoluteTarget(1), StockQuantity: AbsoluteTarget(1)}}},
		// Process time is setup + repetitions (A.6).
		"process time on A.6": {Processes: map[string]ProcessTargets{"p4": {ProcessTime: ReductionTarget(10)}}},
		// Idle time is only a total on p4: a part would replace it.
		"part of total-only idle": {Processes: map[string]ProcessTargets{"p4": {WaitingTime: AbsoluteTarget(0.5)}}},
		// Targeted setup above the targeted process time.
		"setup above process time": {Processes: map[string]ProcessTargets{"p1": {ProcessTime: AbsoluteTarget(2), SetupTime: AbsoluteTarget(3)}}},
		// Process time below the value adding time.
		"process time below VAT": {Processes: map[string]ProcessTargets{"p1": {ProcessTime: AbsoluteTarget(2)}}},
		// Yield that the targeted scrap and rework contradict.
		"yield vs scrap/rework": {Processes: map[string]ProcessTargets{"p1": {YieldFactor: AbsoluteTarget(0.99), ScrapRate: AbsoluteTarget(0.05), ReworkRate: AbsoluteTarget(0.05)}}},
	}
	for name, targets := range bad {
		t.Run(name, func(t *testing.T) {
			if _, err := NewDesigner().DesignFutureStateFromTargets(richStream(), targets); err == nil {
				t.Error("expected an error")
			}
		})
	}
	// NVAT targeted together with a matching lead-time change is fine:
	// p3 cycle time −50% removes 10 min; NVAT 92 → 82 says the same.
	ok := &FutureStateTargets{Processes: map[string]ProcessTargets{"p3": {CycleTime: ReductionTarget(50), NonValueAddingTime: AbsoluteTarget(82)}}}
	if _, err := NewDesigner().DesignFutureStateFromTargets(richStream(), ok); err != nil {
		t.Errorf("matching NVAT target: %v", err)
	}
}

// A current state that contradicts itself still gets a consistent future:
// stored totals are replaced by their parts.
func TestTargetsFromContradictingCurrentState(t *testing.T) {
	vs := targetStream()
	vs.Processes[0].Parameters.IdleTime = 99 // parts: 20 + 10
	future, err := NewDesigner().DesignFutureStateFromTargets(vs, &FutureStateTargets{
		Default: &ProcessTargets{WaitingTime: ReductionTarget(50)}})
	if err != nil {
		t.Fatal(err)
	}
	if issues := CheckConsistency(future); len(issues) != 0 {
		t.Errorf("future issues: %+v", issues)
	}
	if it := future.Processes[0].Parameters.IdleTime; !approx(it, 20) {
		t.Errorf("p1 idle = %v, want 20 (10 transport + 10 waiting)", it)
	}
}

// Process time is strict (A.2): a stored total that its parts do not
// explain is replaced by the parts sum in the future state; a total-only
// process can take a process_time target but not a target on one of its
// parts; a repetitive process (A.6) whose A.2 parts agree stays consistent.
func TestTargetsStrictProcessTime(t *testing.T) {
	stream := func(params ...ProcessParameters) *ValueStream {
		vs := &ValueStream{ID: "vs"}
		for i, p := range params {
			p.TimeUnit = TimeUnitMinutes
			vs.Processes = append(vs.Processes, Process{ID: fmt.Sprintf("p%d", i+1), Parameters: p})
		}
		return vs
	}
	design := func(vs *ValueStream, targets *ProcessTargets) (*ValueStream, error) {
		return NewDesigner().DesignFutureStateFromTargets(vs, &FutureStateTargets{Default: targets})
	}

	// Shortfall in the current state (stored 20, parts 4): the future uses
	// the parts and is consistent.
	future, err := design(stream(ProcessParameters{ProcessTime: 20, SetupTime: 4, IdleTime: 10}),
		&ProcessTargets{SetupTime: ReductionTarget(50)})
	if err != nil {
		t.Fatal(err)
	}
	if issues := CheckConsistency(future); len(issues) != 0 {
		t.Errorf("shortfall future issues: %+v", issues)
	}
	if pt := NewCalculator().ResolveParameters(future.Processes[0].Parameters).ProcessTime; !approx(pt, 2) {
		t.Errorf("shortfall future PT = %v, want 2", pt)
	}

	// Total only: a process_time target sets the total.
	future, err = design(stream(ProcessParameters{ProcessTime: 12, IdleTime: 10}),
		&ProcessTargets{ProcessTime: ReductionTarget(25)})
	if err != nil {
		t.Fatal(err)
	}
	if pt := future.Processes[0].Parameters.ProcessTime; !approx(pt, 9) {
		t.Errorf("total-only PT = %v, want 9", pt)
	}
	if issues := CheckConsistency(future); len(issues) != 0 {
		t.Errorf("total-only future issues: %+v", issues)
	}
	// Total only: a setup target would replace the total with a part.
	if _, err := design(stream(ProcessParameters{ProcessTime: 12}),
		&ProcessTargets{SetupTime: AbsoluteTarget(2)}); err == nil {
		t.Error("setup target on a total-only process: expected an error")
	}

	// Repetitive with agreeing A.2 parts: setup 2 + 3 × 2 = 8 = setup 2 +
	// processing 6. A setup target keeps A.6 and A.2 in agreement.
	future, err = design(stream(ProcessParameters{SetupTime: 2, ProcessingTime: 6, RepetitionTime: 3, RepetitionCount: 2}),
		&ProcessTargets{SetupTime: AbsoluteTarget(1)})
	if err != nil {
		t.Fatal(err)
	}
	if issues := CheckConsistency(future); len(issues) != 0 {
		t.Errorf("repetitive future issues: %+v", issues)
	}
	if pt := NewCalculator().ResolveParameters(future.Processes[0].Parameters).ProcessTime; !approx(pt, 7) {
		t.Errorf("repetitive future PT = %v, want 7", pt)
	}
}

// TestZeroTargetsRecomputeLeadTime: a target of 0 is a real target, not
// "not given": the lead time follows the resulting flow, and process time
// cannot drop below a known value adding time.
func TestZeroTargetsRecomputeLeadTime(t *testing.T) {
	stream := func(p ProcessParameters) *ValueStream {
		p.TimeUnit = TimeUnitMinutes
		return &ValueStream{ID: "vs", Name: "Zero", Processes: []Process{{ID: "p", Name: "P", Parameters: p}}}
	}
	cases := []struct {
		name    string
		current ProcessParameters
		targets ProcessTargets
		lead    float64
		nvat    float64
	}{
		{"zero process time", ProcessParameters{ProcessTime: 10, LeadTime: 10},
			ProcessTargets{ProcessTime: AbsoluteTarget(0)}, 0, 0},
		{"zero process and idle time", ProcessParameters{ProcessTime: 10, IdleTime: 5, LeadTime: 15},
			ProcessTargets{ProcessTime: AbsoluteTarget(0), IdleTime: AbsoluteTarget(0)}, 0, 0},
		{"zero process time, classified", ProcessParameters{ProcessTime: 10, IdleTime: 5, NonValueAddingTime: 15},
			ProcessTargets{ProcessTime: AbsoluteTarget(0)}, 5, 5},
		{"zero process and idle time, classified", ProcessParameters{ProcessTime: 10, IdleTime: 5, NonValueAddingTime: 15},
			ProcessTargets{ProcessTime: AbsoluteTarget(0), IdleTime: AbsoluteTarget(0)}, 0, 0},
	}
	c := NewCalculator()
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			current := stream(tt.current)
			if issues := CheckConsistency(current); len(issues) != 0 {
				t.Fatalf("current inconsistent: %+v", issues)
			}
			future, err := NewDesigner().DesignFutureStateFromTargets(current, &FutureStateTargets{Default: &tt.targets})
			if err != nil {
				t.Fatal(err)
			}
			if issues := CheckConsistency(future); len(issues) != 0 {
				t.Errorf("future issues: %+v", issues)
			}
			got := c.ResolveParametersInStream(future.Processes[0].Parameters, future)
			if got.ProcessTime != 0 || !approx(got.LeadTime, tt.lead) || !approx(got.NonValueAddingTime, tt.nvat) {
				t.Errorf("PT %v LT %v NVAT %v, want 0, %v, %v", got.ProcessTime, got.LeadTime, got.NonValueAddingTime, tt.lead, tt.nvat)
			}
			if lt := future.KPIs["total_lead_time"]; !approx(lt, tt.lead) {
				t.Errorf("total_lead_time KPI = %v, want %v", lt, tt.lead)
			}
		})
	}

	// Value adding time 4 is part of process time: a process time of 0
	// (alone or with idle time 0) is rejected.
	for name, targets := range map[string]ProcessTargets{
		"zero process time":          {ProcessTime: AbsoluteTarget(0)},
		"zero process and idle time": {ProcessTime: AbsoluteTarget(0), IdleTime: AbsoluteTarget(0)},
		"process time −100%":         {ProcessTime: ReductionTarget(100)},
	} {
		current := stream(ProcessParameters{ProcessTime: 10, IdleTime: 5, ValueAddingTime: 4})
		if _, err := NewDesigner().DesignFutureStateFromTargets(current, &FutureStateTargets{Default: &targets}); err == nil ||
			!strings.Contains(err.Error(), "value_adding_time") {
			t.Errorf("%s with value adding time 4: err = %v, want a value_adding_time error", name, err)
		}
	}
}

// A current state with scrap + rework above 1 can still be carried into a
// future state without targets (the contradiction stays reported), but a
// target that creates such a state is rejected.
func TestTargetsWithImpossibleDefects(t *testing.T) {
	vs := &ValueStream{ID: "vs", Processes: []Process{{ID: "p", Parameters: ProcessParameters{
		ProcessTime: 10, ScrapRate: 0.8, ReworkRate: 0.4, TimeUnit: TimeUnitMinutes}}}}
	future, err := NewDesigner().DesignFutureStateFromTargets(vs, nil)
	if err != nil {
		t.Fatalf("no targets: %v", err)
	}
	if _, ok := future.KPIs["average_yield_factor"]; ok {
		t.Errorf("impossible yield must stay unknown, got %v", future.KPIs["average_yield_factor"])
	}
	ok := &ValueStream{ID: "vs", Processes: []Process{{ID: "p", Parameters: ProcessParameters{
		ProcessTime: 10, ScrapRate: 0.1, ReworkRate: 0.1, TimeUnit: TimeUnitMinutes}}}}
	_, err = NewDesigner().DesignFutureStateFromTargets(ok, &FutureStateTargets{Default: &ProcessTargets{ScrapRate: AbsoluteTarget(0.95)}})
	if err == nil {
		t.Errorf("scrap target 0.95 with rework 0.1 should be rejected")
	}
}

// Yields 0.9 and 1 with no defect breakdown; a yield target of 0 on the
// first process is a real target: the future average is (0 + 1) ÷ 2 = 0.5,
// not 1 (the zero must not read as "yield not given").
func TestZeroYieldTargetKept(t *testing.T) {
	vs := &ValueStream{ID: "vs", Processes: []Process{
		{ID: "a", Parameters: ProcessParameters{ProcessTime: 10, YieldFactor: 0.9, TimeUnit: TimeUnitMinutes}},
		{ID: "b", Parameters: ProcessParameters{ProcessTime: 10, YieldFactor: 1, TimeUnit: TimeUnitMinutes}},
	}}
	future, err := NewDesigner().DesignFutureStateFromTargets(vs, &FutureStateTargets{
		Processes: map[string]ProcessTargets{"a": {YieldFactor: AbsoluteTarget(0)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := future.KPIs["average_yield_factor"]; !approx(got, 0.5) {
		t.Errorf("average_yield_factor = %v, want 0.5", got)
	}
	if issues := CheckConsistency(future); len(issues) != 0 {
		t.Errorf("future state issues: %+v", issues)
	}
	// The marker survives a JSON round trip.
	data, err := NewSerializer().SerializeValueStream(future)
	if err != nil {
		t.Fatal(err)
	}
	back, err := NewSerializer().DeserializeValueStream(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := NewCalculator().CalculateKPIs(back)["average_yield_factor"]; !approx(got, 0.5) {
		t.Errorf("after round trip average_yield_factor = %v, want 0.5", got)
	}
}

// Inventory range targets start from the range resolved from quantity and
// demand, not from a stored range that is absent or contradicts it.
func TestInventoryRangeTargetUsesResolvedRange(t *testing.T) {
	stream := func(storedRange float64) *ValueStream {
		return &ValueStream{ID: "vs", Name: "r",
			Customer: &Customer{ID: "c", Demand: 100, DemandPeriod: DemandPeriodDay},
			Processes: []Process{{ID: "p", Name: "p", Parameters: ProcessParameters{
				ProcessTime: 0.01, TimeUnit: TimeUnitDays}}},
			Inventory: []Inventory{{ID: "i", Name: "i", Quantity: 200, Range: storedRange}},
		}
	}
	targets := &FutureStateTargets{InventoryDefault: &InventoryTargets{Range: ReductionTarget(50)}}
	for name, stored := range map[string]float64{"range omitted": 0, "range contradicts (5 days)": 5} {
		future, err := NewDesigner().DesignFutureStateFromTargets(stream(stored), targets)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		inv := future.Inventory[0]
		if !approx(inv.Quantity, 100) {
			t.Errorf("%s: quantity = %v, want 100", name, inv.Quantity)
		}
		if r := NewCalculator().contextOf(future).inventoryTime(inv); !approx(r, 1) {
			t.Errorf("%s: range = %v days, want 1", name, r)
		}
		if issues := CheckConsistency(future); len(issues) != 0 {
			t.Errorf("%s: future issues %+v", name, issues)
		}
	}

	// Demand rate unknown: quantity and range still move together (range ∝
	// quantity at any constant demand rate).
	vs := stream(2)
	vs.Customer = nil
	future, err := NewDesigner().DesignFutureStateFromTargets(vs, &FutureStateTargets{
		InventoryDefault: &InventoryTargets{Quantity: ReductionTarget(25)}})
	if err != nil {
		t.Fatal(err)
	}
	if inv := future.Inventory[0]; !approx(inv.Quantity, 150) || !approx(inv.Range, 1.5) {
		t.Errorf("rate unknown: inventory = %+v, want quantity 150, range 1.5", inv)
	}
}

// An inventory given by range alone (quantity 0), demand rate known: a
// quantity cut is a cut of the stock that range stands for.
func TestInventoryRangeOnlyTargets(t *testing.T) {
	vs := &ValueStream{ID: "vs", Name: "r",
		Customer:  &Customer{ID: "c", Demand: 100, DemandPeriod: DemandPeriodDay},
		Processes: []Process{{ID: "p", Name: "p", Parameters: ProcessParameters{ProcessTime: 0.01, TimeUnit: TimeUnitDays}}},
		Inventory: []Inventory{{ID: "i", Name: "i", Range: 2}},
	}
	future, err := NewDesigner().DesignFutureStateFromTargets(vs, &FutureStateTargets{
		InventoryDefault: &InventoryTargets{Quantity: ReductionTarget(50)}})
	if err != nil {
		t.Fatal(err)
	}
	if inv := future.Inventory[0]; inv.Quantity != 0 || !approx(inv.Range, 1) {
		t.Errorf("inventory = %+v, want range 1 (quantity left unstored)", inv)
	}
}
