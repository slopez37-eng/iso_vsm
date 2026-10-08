package vsm

import (
	"math"
	"testing"
)

const eps = 1e-9

func approx(a, b float64) bool { return math.Abs(a-b) <= eps*math.Max(1, math.Abs(b)) }

func TestScalarFormulas(t *testing.T) {
	c := NewCalculator()
	tests := []struct {
		name string
		got  float64
		want float64
	}{
		// A.2: PT = PRT + ST + HT + WTp + AT = 10 + 2 + 1 + 3 + 4
		{"process time A.2", c.CalculateProcessTime(ProcessParameters{
			ProcessingTime: 10, SetupTime: 2, HandlingTime: 1, WaitingTimeProcess: 3, AncillaryTime: 4,
			ProcessTime: 999, // stored value is not read by the component formula
		}), 20},
		// A.3: LT = VAT + NVAT + NNVAT = 5 + 3 + 2
		{"lead time A.3", c.CalculateLeadTime(ProcessParameters{
			ValueAddingTime: 5, NonValueAddingTime: 3, NecessaryNonValueAddingTime: 2,
		}), 10},
		// A.5: PRT = CT × (PQ ÷ #RES) = 2 × (10 ÷ 4)
		{"processing time A.5", c.CalculateProcessingTime(2, 10, 4), 5},
		{"processing time zero resources", c.CalculateProcessingTime(2, 10, 0), 0},
		// A.6: PT = ST + RT × #Rep = 5 + 2 × 3
		{"repetitive process time A.6", c.CalculateRepetitiveProcessTime(5, 2, 3), 11},
		// A.7: SQ = PT × PQ
		{"stock quantity A.7", c.CalculateStockQuantity(2, 3), 6},
		// A.8: HT = PT − (PRT + ST + WTp + AT) = 30 − (20 + 3 + 2 + 1)
		{"handling time A.8", c.CalculateHandlingTime(ProcessParameters{
			ProcessTime: 30, ProcessingTime: 20, SetupTime: 3, WaitingTimeProcess: 2, AncillaryTime: 1,
			HandlingTime: 99, Downtime: 50, // neither feeds the result
		}), 4},
		{"handling time inconsistent inputs go negative", c.CalculateHandlingTime(ProcessParameters{
			ProcessTime: 10, ProcessingTime: 12,
		}), -2},
		// A.10: TT = RT × #TRep
		{"transport repetition time A.10", c.CalculateTransportRepetitionTime(2.5, 4), 10},
		// A.11: WT = SQ × CT
		{"waiting time A.11", c.CalculateWaitingTime(50, 1.2), 60},
		// A.12: takt = 27,600 s available ÷ 460 units = 60 s/unit
		{"customer takt A.12", c.CalculateCustomerTakt(27600, 460), 60},
		{"customer takt zero demand", c.CalculateCustomerTakt(27600, 0), 0},
		// A.13: IT = CD ÷ SQ
		{"inventory turns A.13", c.CalculateInventoryTurns(1200, 300), 4},
		{"inventory turns zero stock", c.CalculateInventoryTurns(1200, 0), 0},
		// OTIF from joint count: 90 of 100 both on time and complete
		{"OTIF joint", c.CalculateOTIF(90, 100), 0.9},
		{"OTIF joint zero total", c.CalculateOTIF(90, 0), 0},
		// A.14 estimate: (90/100) × (80/100)
		{"OTIF estimate A.14", c.CalculateOnTimeInFull(90, 80, 100), 0.72},
		{"OTIF estimate zero total", c.CalculateOnTimeInFull(90, 80, 0), 0},
		{"OTIF estimate perfect", c.CalculateOnTimeInFull(100, 100, 100), 1},
		// A.15: RR = Σ RR_i
		{"rework rate A.15", c.CalculateReworkRate([]float64{0.02, 0.03, 0.05}), 0.10},
		{"rework rate empty", c.CalculateReworkRate(nil), 0},
		// A.16: VAR = VAT ÷ LT
		{"value added ratio A.16", c.CalculateValueAddedRatio(30, 120), 0.25},
		{"value added ratio zero lead time", c.CalculateValueAddedRatio(30, 0), 0},
		// A.17: first-pass yield = 1 − (SR + RR)
		{"yield factor A.17", c.CalculateYieldFactor(0.02, 0.03), 0.95},
		// OT = available − breaks − downtime
		{"operating time", c.CalculateOperatingTime(480, 30, 20), 430},
		// RI = 120 parts × 30 s = 3600 s = 1 h
		{"range of inventory", c.CalculateRangeOfInventory(120, 30, TimeUnitSeconds, TimeUnitHours), 1},
		{"total idle time", c.CalculateTotalIdleTime(1, 2, 3), 6},
		{"process lead time", c.CalculateProcessLeadTime(4, 6), 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !approx(tt.got, tt.want) {
				t.Errorf("got %v, want %v", tt.got, tt.want)
			}
		})
	}
}

func TestOTIFEstimateWithinBounds(t *testing.T) {
	c := NewCalculator()
	complete, inTime, total := 90.0, 80.0, 100.0
	got := c.CalculateOnTimeInFull(complete, inTime, total)
	lower := math.Max(0, complete+inTime-total) / total
	upper := math.Min(complete, inTime) / total
	if got < lower || got > upper {
		t.Errorf("estimate %v outside feasible range [%v, %v]", got, lower, upper)
	}
}

func TestDowntimeAndAvailability(t *testing.T) {
	c := NewCalculator()
	tests := []struct {
		name      string
		tbf, ttr  []float64
		wantDT    float64
		wantAvail float64
		wantErr   bool
	}{
		// DT = 5 + 15; A = 300 ÷ (300 + 20)
		{"two failures", []float64{100, 200}, []float64{5, 15}, 20, 0.9375, false},
		{"empty log", nil, nil, 0, 0, false},
		{"mismatched lengths", []float64{1}, []float64{1, 2}, 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dt, err := c.CalculateDowntime(tt.tbf, tt.ttr)
			if (err != nil) != tt.wantErr {
				t.Fatalf("downtime err = %v, wantErr %v", err, tt.wantErr)
			}
			if !approx(dt, tt.wantDT) {
				t.Errorf("downtime = %v, want %v", dt, tt.wantDT)
			}
			a, err := c.CalculateAvailability(tt.tbf, tt.ttr)
			if (err != nil) != tt.wantErr {
				t.Fatalf("availability err = %v, wantErr %v", err, tt.wantErr)
			}
			if !approx(a, tt.wantAvail) {
				t.Errorf("availability = %v, want %v", a, tt.wantAvail)
			}
		})
	}
}

func TestConvertTimeUnit(t *testing.T) {
	c := NewCalculator()
	// One week expressed in each unit.
	week := map[TimeUnit]float64{
		TimeUnitSeconds: 604800,
		TimeUnitMinutes: 10080,
		TimeUnitHours:   168,
		TimeUnitDays:    7,
		TimeUnitWeeks:   1,
	}
	for from, fv := range week {
		for to, tv := range week {
			if got := c.ConvertTimeUnit(fv, from, to); !approx(got, tv) {
				t.Errorf("%v %s -> %s = %v, want %v", fv, from, to, got, tv)
			}
			// Round trip returns the original value.
			if got := c.ConvertTimeUnit(c.ConvertTimeUnit(3.7, from, to), to, from); !approx(got, 3.7) {
				t.Errorf("round trip %s -> %s -> %s = %v, want 3.7", from, to, from, got)
			}
		}
	}
	// Unknown source unit: value passes through unchanged.
	if got := c.ConvertTimeUnit(5, "", TimeUnitHours); got != 5 {
		t.Errorf("unknown source unit: got %v, want 5", got)
	}
	// Unknown target unit: value is returned in seconds.
	if got := c.ConvertTimeUnit(2, TimeUnitMinutes, "fortnights"); got != 120 {
		t.Errorf("unknown target unit: got %v, want 120", got)
	}
}

func TestReferenceTimeUnitAndNormalize(t *testing.T) {
	c := NewCalculator()
	procs := []Process{
		{Parameters: ProcessParameters{}},
		{Parameters: ProcessParameters{TimeUnit: TimeUnitHours}},
		{Parameters: ProcessParameters{TimeUnit: TimeUnitSeconds}},
	}
	if got := c.ReferenceTimeUnit(procs); got != TimeUnitHours {
		t.Errorf("ReferenceTimeUnit = %q, want hours", got)
	}
	if got := c.ReferenceTimeUnit([]Process{{}}); got != "" {
		t.Errorf("ReferenceTimeUnit with no units = %q, want empty", got)
	}

	p := ProcessParameters{
		TimeUnit: TimeUnitMinutes, ProcessTime: 2, IdleTime: 3, ValueAddingTime: 1,
		TimeToRepair: 0.5, ScrapRate: 0.1, StockQuantity: 7,
	}
	n := c.normalizeTimes(p, TimeUnitSeconds)
	if n.ProcessTime != 120 || n.IdleTime != 180 || n.ValueAddingTime != 60 || n.TimeToRepair != 30 {
		t.Errorf("time fields not converted: %+v", n)
	}
	if n.ScrapRate != 0.1 || n.StockQuantity != 7 {
		t.Errorf("non-time fields must not change: %+v", n)
	}
	if n.TimeUnit != TimeUnitSeconds {
		t.Errorf("TimeUnit = %q, want seconds", n.TimeUnit)
	}
	// Empty source unit: assumed already in the reference unit.
	if got := c.normalizeTimes(ProcessParameters{ProcessTime: 2}, TimeUnitSeconds); got.ProcessTime != 2 {
		t.Errorf("empty unit converted: %v", got.ProcessTime)
	}
}

func TestAggregatesUseComponentsOverStoredTotals(t *testing.T) {
	c := NewCalculator()
	procs := []Process{
		// Stored PT 10 with only setup 4 given: handling time is the rest
		// (A.8), so PT 10 stands. Stored IT 5 contradicts WT 100: the parts
		// win (IT 100).
		{Parameters: ProcessParameters{ProcessTime: 10, SetupTime: 4, IdleTime: 5, WaitingTime: 100,
			ValueAddingTime: 6}},
		// No stored PT/IT: A.2 components and TT + WT + ST_T.
		{Parameters: ProcessParameters{ProcessingTime: 3, SetupTime: 1, TransportTime: 2, WaitingTime: 3,
			StorageTime: 1, ValueAddingTimeProcess: 2}},
	}
	if got := c.CalculateTotalProcessTime(procs); !approx(got, 14) {
		t.Errorf("total process time = %v, want 14", got)
	}
	if got := c.CalculateOverallLeadTime(procs); !approx(got, 10+100+4+6) {
		t.Errorf("overall lead time = %v, want 120", got)
	}
	if got := c.CalculateValueAddingTime(procs); !approx(got, 8) {
		t.Errorf("value adding time = %v, want 8", got)
	}
}

func TestValidateProcessParameters(t *testing.T) {
	c := NewCalculator()
	if errs := c.ValidateProcessParameters(ProcessParameters{ProcessTime: 10, ValueAddingTime: 5, YieldFactor: 0.9}); len(errs) != 0 {
		t.Errorf("valid params gave errors: %v", errs)
	}
	bad := ProcessParameters{ProcessTime: -1, CycleTime: -1, CustomerDemand: -1, ValueAddingTime: 5,
		YieldFactor: 2, ScrapRate: -0.1, ReworkRate: 1.5}
	if errs := c.ValidateProcessParameters(bad); len(errs) != 7 {
		t.Errorf("got %d errors, want 7: %v", len(errs), errs)
	}
}

// mixedUnitStream is a 3-process stream in minutes, seconds and hours.
// Reference unit is minutes (first process). Per process, in minutes:
//
//	P1: PT 2,                VAT 1,     IT 10           -> PLT 12
//	P2: PT 30+30 s = 1,      VATp 0.75, IT 120+240 s = 6 -> PLT 7
//	P3: PT 0.05 h = 3,       VAT 1.5,   IT 0.1 h = 6     -> PLT 9
//
// LT = 28, VAT = 3.25, PT = 6.
func mixedUnitStream() *ValueStream {
	return &ValueStream{
		ID:   "vs1",
		Name: "Mixed units",
		Customer: &Customer{
			ID: "c1", Name: "Customer", Demand: 960, AvailableTime: 480, // 480 min / 960 units
			TaktTime: 0.5, // = AvailableTime ÷ Demand (a different value is a consistency issue)
		},
		Processes: []Process{
			{ID: "p1", Name: "Cut", Parameters: ProcessParameters{
				TimeUnit: TimeUnitMinutes, ProcessTime: 2, ValueAddingTime: 1, IdleTime: 10, YieldFactor: 0.9,
			}},
			{ID: "p2", Name: "Weld", Parameters: ProcessParameters{
				TimeUnit: TimeUnitSeconds, ProcessingTime: 30, SetupTime: 30, ValueAddingTimeProcess: 45,
				TransportTime: 120, WaitingTime: 240,
			}},
			{ID: "p3", Name: "Paint", Parameters: ProcessParameters{
				TimeUnit: TimeUnitHours, ProcessTime: 0.05, ValueAddingTime: 0.025, IdleTime: 0.1, YieldFactor: 0.8,
			}},
		},
		Inventory: []Inventory{
			{ID: "i1", Name: "Raw", Quantity: 200},
			{ID: "i2", Name: "WIP", Quantity: 40},
		},
	}
}

func TestCalculateKPIsMixedUnits(t *testing.T) {
	c := NewCalculator()
	got := c.CalculateKPIs(mixedUnitStream())
	want := map[string]float64{
		"customer_takt":           0.5,
		"total_lead_time":         28,
		"total_value_adding_time": 3.25,
		"value_added_ratio":       3.25 / 28,
		"process_efficiency":      6.0 / 28,
		"inventory_turns":         4,
		"average_yield_factor":    0.85,
	}
	if len(got) != len(want) {
		t.Errorf("got %d KPIs, want %d: %v", len(got), len(want), got)
	}
	for k, w := range want {
		g, ok := got[k]
		if !ok {
			t.Errorf("missing KPI %q", k)
			continue
		}
		if !approx(g, w) {
			t.Errorf("%s = %v, want %v", k, g, w)
		}
	}
}

func TestCalculateKPIsOmitsUndefined(t *testing.T) {
	c := NewCalculator()

	if got := c.CalculateKPIs(nil); len(got) != 0 {
		t.Errorf("nil stream: got %v", got)
	}

	tests := []struct {
		name    string
		vs      *ValueStream
		present map[string]float64
		absent  []string
	}{
		{
			name: "demand only gives no takt (old bug returned demand as takt)",
			vs: &ValueStream{Customer: &Customer{Demand: 460},
				Processes: []Process{{Parameters: ProcessParameters{ProcessTime: 1}}}},
			absent: []string{"customer_takt"},
		},
		{
			name:    "takt time used when available time missing",
			vs:      &ValueStream{Customer: &Customer{Demand: 460, TaktTime: 60}},
			present: map[string]float64{"customer_takt": 60},
		},
		{
			name: "available time with zero demand falls back to takt time",
			vs:   &ValueStream{Customer: &Customer{AvailableTime: 27600, TaktTime: 55}},
			present: map[string]float64{
				"customer_takt": 55,
			},
		},
		{
			name:   "no processes gives no time KPIs",
			vs:     &ValueStream{},
			absent: []string{"total_lead_time", "total_value_adding_time", "value_added_ratio", "process_efficiency"},
		},
		{
			name:    "zero lead time omits ratios",
			vs:      &ValueStream{Processes: []Process{{Parameters: ProcessParameters{}}}},
			present: map[string]float64{"total_lead_time": 0, "total_value_adding_time": 0},
			absent:  []string{"value_added_ratio", "process_efficiency"},
		},
		{
			name: "zero demand omits inventory turns",
			vs: &ValueStream{Customer: &Customer{TaktTime: 1},
				Inventory: []Inventory{{ID: "i", Quantity: 10}}},
			absent: []string{"inventory_turns"},
		},
		{
			name:   "no yield factors omits average yield",
			vs:     &ValueStream{Processes: []Process{{Parameters: ProcessParameters{ProcessTime: 1}}}},
			absent: []string{"average_yield_factor"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.CalculateKPIs(tt.vs)
			for k, w := range tt.present {
				if g, ok := got[k]; !ok || !approx(g, w) {
					t.Errorf("%s = %v (present %v), want %v", k, g, ok, w)
				}
			}
			for _, k := range tt.absent {
				if v, ok := got[k]; ok {
					t.Errorf("%s should be omitted, got %v", k, v)
				}
			}
		})
	}
}

func TestCalculateImprovementMetrics(t *testing.T) {
	c := NewCalculator()
	current := &ValueStream{Processes: []Process{{Parameters: ProcessParameters{ProcessTime: 5, IdleTime: 15}}}}
	future := &ValueStream{Processes: []Process{{Parameters: ProcessParameters{ProcessTime: 5, IdleTime: 5}}}}
	m := c.CalculateImprovementMetrics(current, future)
	if !approx(m["total_lead_time_improvement_ratio"], 0.5) {
		t.Errorf("ratio = %v, want 0.5", m["total_lead_time_improvement_ratio"])
	}
	if !approx(m["total_lead_time_improvement_percent"], -50) {
		t.Errorf("percent = %v, want -50", m["total_lead_time_improvement_percent"])
	}
	if got := c.CalculateImprovementMetrics(nil, future); len(got) != 0 {
		t.Errorf("nil current: got %v", got)
	}
}
