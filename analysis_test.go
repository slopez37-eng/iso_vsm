package vsm

import (
	"math"
	"testing"
)

func TestAnalyzeIdleTimesMixedUnits(t *testing.T) {
	a := NewAnalyzer()
	got, err := a.AnalyzeIdleTimes(mixedUnitStream())
	if err != nil {
		t.Fatal(err)
	}
	// Minutes (see mixedUnitStream): idle 10 + 6 + 6 = 22, process 2 + 1 + 3 = 6.
	if !approx(val(got.TotalIdleTime), 22) {
		t.Errorf("total idle = %v, want 22", val(got.TotalIdleTime))
	}
	if !approx(val(got.IdleTimeEfficiency), 6.0/28) {
		t.Errorf("idle efficiency = %v, want %v", val(got.IdleTimeEfficiency), 6.0/28)
	}
}

func TestAnalyzeFlowNoInfWhenProcessTimeZero(t *testing.T) {
	a := NewAnalyzer()
	vs := &ValueStream{Processes: []Process{{Name: "P", Parameters: ProcessParameters{WaitingTime: F(10)}}}}
	got, err := a.AnalyzeFlow(vs)
	if err != nil {
		t.Fatal(err)
	}
	for _, fi := range got.FlowInterruptions {
		if math.IsInf(fi.Frequency, 0) || math.IsNaN(fi.Frequency) {
			t.Errorf("interruption %q has non-finite frequency %v", fi.Type, fi.Frequency)
		}
	}
}

func TestAnalyzeFlowEfficiencyMatchesKPI(t *testing.T) {
	vs := mixedUnitStream()
	got, err := NewAnalyzer().AnalyzeFlow(vs)
	if err != nil {
		t.Fatal(err)
	}
	if want := NewCalculator().CalculateKPIs(vs)["value_added_ratio"]; !approx(val(got.FlowEfficiency), want) {
		t.Errorf("flow efficiency = %v, want value_added_ratio %v", val(got.FlowEfficiency), want)
	}
}

func TestPaybackPeriod(t *testing.T) {
	p := NewPlanner()
	tests := []struct {
		name  string
		costs Costs
		want  float64
	}{
		// 12,000 ÷ (25,000 − 1,000) per year × 12 = 6 months
		{"positive net benefit", Costs{Investment: 12000, Operational: 1000, Benefits: 25000}, 6},
		{"never pays back", Costs{Investment: 12000, Operational: 5000, Benefits: 4000}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := p.calculatePaybackPeriod(tt.costs); !approx(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContinuousFlowPrincipleNormalizesUnits(t *testing.T) {
	// In minutes: idle 1 + 1 = 2, process 10 + 1 = 11 -> ratio 0.18 (< 0.5).
	// Summing raw numbers would give 61 / 70 = 0.87 and the wrong answer.
	vs := &ValueStream{Processes: []Process{
		{Parameters: ProcessParameters{TimeUnit: TimeUnitMinutes, ProcessTime: F(10), IdleTime: F(1)}},
		{Parameters: ProcessParameters{TimeUnit: TimeUnitSeconds, ProcessTime: F(60), IdleTime: F(60)}},
	}}
	if !NewDesigner().checkContinuousFlowPrinciple(vs) {
		t.Error("expected continuous flow principle to be detected")
	}
}

// loadStream: demand 200/day, 480 min available per day (takt 2.4 min).
// p1 CT 2 min, p2 CT 180 s = 3 min (mixed units), p3 has no cycle time.
func loadStream() *ValueStream {
	return &ValueStream{
		Customer: &Customer{ID: "c", Demand: F(200), DemandPeriod: DemandPeriodDay, AvailableTime: F(480)},
		Processes: []Process{
			{ID: "p1", Name: "Cut", Parameters: ProcessParameters{TimeUnit: TimeUnitMinutes, CycleTime: F(2)}},
			{ID: "p2", Name: "Weld", Parameters: ProcessParameters{TimeUnit: TimeUnitSeconds, CycleTime: F(180)}},
			{ID: "p3", Name: "Pack", Parameters: ProcessParameters{TimeUnit: TimeUnitMinutes}},
		},
	}
}

func TestCalculateProcessLoad(t *testing.T) {
	a := NewAnalyzer()
	vs := loadStream()
	tests := []struct {
		name           string
		process        Process
		capacity, util float64
		wantOK         bool
	}{
		// capacity = 480 / 2 = 240 per day; utilization = 200 × 2 / 480
		{"minutes", vs.Processes[0], 240, 200.0 * 2 / 480, true},
		// 180 s = 3 min: capacity 160 per day; utilization 1.25 (not capped)
		{"seconds converted", vs.Processes[1], 160, 1.25, true},
		{"no cycle time", vs.Processes[2], 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, u, ok := a.CalculateProcessLoad(tt.process, vs)
			if ok != tt.wantOK || !approx(c, tt.capacity) || !approx(u, tt.util) {
				t.Errorf("got (%v, %v, %v), want (%v, %v, %v)", c, u, ok, tt.capacity, tt.util, tt.wantOK)
			}
		})
	}
	// Utilization = CT ÷ takt, with the same takt CalculateKPIs reports.
	takt := NewCalculator().CalculateKPIs(vs)["customer_takt"]
	if _, u, _ := a.CalculateProcessLoad(vs.Processes[0], vs); !approx(u, 2/takt) {
		t.Errorf("utilization %v != CT/takt %v", u, 2/takt)
	}
}

func TestCalculateProcessLoadMissingInputs(t *testing.T) {
	a := NewAnalyzer()
	cases := map[string]func(*Customer){
		"no period":         func(c *Customer) { c.DemandPeriod = "" },
		"unknown period":    func(c *Customer) { c.DemandPeriod = "fortnight" },
		"no available time": func(c *Customer) { c.AvailableTime = nil },
		"no demand":         func(c *Customer) { c.Demand = nil },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			vs := loadStream()
			mutate(vs.Customer)
			if _, _, ok := a.CalculateProcessLoad(vs.Processes[0], vs); ok {
				t.Error("expected utilization to be skipped")
			}
			b, err := a.IdentifyBottlenecks(vs)
			if err != nil || len(b) != 0 {
				t.Errorf("bottlenecks = %v, %v; want none", b, err)
			}
		})
	}
	vs := loadStream()
	vs.Customer = nil
	if _, _, ok := a.CalculateProcessLoad(vs.Processes[0], vs); ok {
		t.Error("expected utilization to be skipped without a customer")
	}
}

func TestIdentifyBottlenecksUsesDemandPeriod(t *testing.T) {
	b, err := NewAnalyzer().IdentifyBottlenecks(loadStream())
	if err != nil {
		t.Fatal(err)
	}
	// p1 at 83% (medium) and p2 at 125% (high); p3 skipped.
	if len(b) != 2 || b[0].ProcessID != "p2" || b[0].BottleneckLevel != "high" ||
		b[1].ProcessID != "p1" || b[1].BottleneckLevel != "medium" {
		t.Fatalf("unexpected bottlenecks: %+v", b)
	}
	if b[0].CapacityPeriod != DemandPeriodDay || !approx(b[0].Capacity, 160) {
		t.Errorf("capacity = %v per %q, want 160 per day", b[0].Capacity, b[0].CapacityPeriod)
	}
}

func TestAnalyzeCurrentStateNotesMissingLoadInputs(t *testing.T) {
	vs := loadStream()
	vs.Customer.DemandPeriod = ""
	got, err := NewAnalyzer().AnalyzeCurrentState(vs)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Notes) == 0 || got.Notes[0] != missingLoadInputsNote || len(got.Bottlenecks) != 0 {
		t.Errorf("notes = %v, bottlenecks = %v", got.Notes, got.Bottlenecks)
	}
}

// TestProcessLoadCountsResources: two machines with a 2 min cycle time
// make 480 a day in 480 min, consistent with processing time = CT × quantity
// ÷ resources (A.5). At 300 a day that is 62.5% utilization, not a
// bottleneck.
func TestProcessLoadCountsResources(t *testing.T) {
	vs := &ValueStream{
		Customer: &Customer{ID: "c", Demand: F(300), DemandPeriod: DemandPeriodDay, AvailableTime: F(480)},
		Processes: []Process{{ID: "p", Name: "Press", Parameters: ProcessParameters{
			TimeUnit: TimeUnitMinutes, CycleTime: F(2), ResourceCount: I(2),
		}}},
	}
	a := NewAnalyzer()
	c, u, ok := a.CalculateProcessLoad(vs.Processes[0], vs)
	if !ok || !approx(c, 480) || !approx(u, 0.625) {
		t.Errorf("load = (%v, %v, %v), want (480, 0.625, true)", c, u, ok)
	}
	// Capacity agrees with the processing time formula: 480 units on 2
	// resources take 2 × 480 ÷ 2 = 480 min, the whole available time.
	if prt := NewCalculator().CalculateProcessingTime(2, 480, 2); !approx(prt, 480) {
		t.Errorf("processing time for the capacity = %v, want 480", prt)
	}
	if b, _ := a.IdentifyBottlenecks(vs); len(b) != 0 {
		t.Errorf("bottlenecks = %+v, want none", b)
	}
	// Resource count 0 or below counts as one resource.
	vs.Processes[0].Parameters.ResourceCount = nil
	if c, u, _ := a.CalculateProcessLoad(vs.Processes[0], vs); !approx(c, 240) || !approx(u, 1.25) {
		t.Errorf("no resource count: load = (%v, %v), want (240, 1.25)", c, u)
	}
}
