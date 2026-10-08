package vsm

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"math"
	"strings"
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
			ProcessingTime: F(10), SetupTime: F(2), HandlingTime: F(1), WaitingTimeProcess: F(3), AncillaryTime: F(4),
			ProcessTime: F(999), // stored value is not read by the component formula
		}), 20},
		// A.3: LT = VAT + NVAT + NNVAT = 5 + 3 + 2
		{"lead time A.3", c.CalculateLeadTime(ProcessParameters{
			ValueAddingTime: F(5), NonValueAddingTime: F(3), NecessaryNonValueAddingTime: F(2),
		}), 10},
		// A.5: PRT = CT × (PQ ÷ #RES) = 2 × (10 ÷ 4)
		{"processing time A.5", c.CalculateProcessingTime(2, 10, 4), 5},
		{"processing time zero resources", c.CalculateProcessingTime(2, 10, 0), 0},
		// A.6: PT = ST + RT × #Rep = 5 + 2 × 3
		{"repetitive process time A.6", c.CalculateRepetitiveProcessTime(5, 2, 3), 11},
		// A.2 rearranged: HT = PT − (PRT + ST + WTp + AT) = 30 − (20 + 3 + 2 + 1)
		{"handling time from A.2", c.CalculateHandlingTime(ProcessParameters{
			ProcessTime: F(30), ProcessingTime: F(20), SetupTime: F(3), WaitingTimeProcess: F(2), AncillaryTime: F(1),
			HandlingTime: F(99), Downtime: F(50), // neither feeds the result
		}), 4},
		{"handling time inconsistent inputs go negative", c.CalculateHandlingTime(ProcessParameters{
			ProcessTime: F(10), ProcessingTime: F(12),
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
		TimeUnit: TimeUnitMinutes, ProcessTime: F(2), IdleTime: F(3), ValueAddingTime: F(1),
		TimeToRepair: F(0.5), ScrapRate: F(0.1), StockQuantity: F(7),
	}
	n := c.normalizeTimes(p, TimeUnitSeconds)
	if num(n.ProcessTime) != 120 || num(n.IdleTime) != 180 || num(n.ValueAddingTime) != 60 || num(n.TimeToRepair) != 30 {
		t.Errorf("time fields not converted: %+v", n)
	}
	if num(n.ScrapRate) != 0.1 || num(n.StockQuantity) != 7 {
		t.Errorf("non-time fields must not change: %+v", n)
	}
	if n.TimeUnit != TimeUnitSeconds {
		t.Errorf("TimeUnit = %q, want seconds", n.TimeUnit)
	}
	// Empty source unit: assumed already in the reference unit.
	if got := c.normalizeTimes(ProcessParameters{ProcessTime: F(2)}, TimeUnitSeconds); num(got.ProcessTime) != 2 {
		t.Errorf("empty unit converted: %v", num(got.ProcessTime))
	}
}

func TestAggregatesUseComponentsOverStoredTotals(t *testing.T) {
	c := NewCalculator()
	procs := []Process{
		// Stored PT 10 contradicts setup 4 + handling 2: the parts win
		// (PT 6). Stored IT 5 contradicts WT 100: the parts win (IT 100).
		{Parameters: ProcessParameters{ProcessTime: F(10), SetupTime: F(4), HandlingTime: F(2), IdleTime: F(5), WaitingTime: F(100),
			ValueAddingTime: F(6)}},
		// No stored PT/IT: A.2 components and TT + WT + ST_T.
		{Parameters: ProcessParameters{ProcessingTime: F(3), SetupTime: F(1), TransportTime: F(2), WaitingTime: F(3),
			StorageTime: F(1), ValueAddingTimeProcess: F(2)}},
	}
	if got := c.CalculateTotalProcessTime(procs); !approx(got, 10) {
		t.Errorf("total process time = %v, want 10", got)
	}
	if got := c.CalculateOverallLeadTime(procs); !approx(got, 6+100+4+6) {
		t.Errorf("overall lead time = %v, want 116", got)
	}
	if got := c.CalculateValueAddingTime(procs); !approx(got, 8) {
		t.Errorf("value adding time = %v, want 8", got)
	}
}

func TestValidateProcessParameters(t *testing.T) {
	c := NewCalculator()
	if errs := c.ValidateProcessParameters(ProcessParameters{ProcessTime: F(10), ValueAddingTime: F(5), YieldFactor: F(0.9)}); len(errs) != 0 {
		t.Errorf("valid params gave errors: %v", errs)
	}
	bad := ProcessParameters{ProcessTime: F(-1), CycleTime: F(-1), CustomerDemand: F(-1), ValueAddingTime: F(5),
		YieldFactor: F(2), ScrapRate: F(-0.1), ReworkRate: F(1.5)}
	// The entered process time -1 is an error, and value adding time 5
	// exceeds it; scrap + rework = 1.4 is the 8th.
	if errs := c.ValidateProcessParameters(bad); len(errs) != 8 {
		t.Errorf("got %d errors, want 8 (VAT above process time, scrap + rework = 1.4 too): %v", len(errs), errs)
	}
	for name, tc := range map[string]struct {
		p    ProcessParameters
		want int
	}{
		"VAT above PT":              {ProcessParameters{ProcessTime: F(4), ValueAddingTime: F(5)}, 1},
		"no PT, VAT above lead":     {ProcessParameters{IdleTime: F(10), ValueAddingTime: F(20)}, 1},
		"no PT, VAT within lead":    {ProcessParameters{IdleTime: F(30), ValueAddingTime: F(20)}, 0},
		"defects 1.004 (tolerance)": {ProcessParameters{ScrapRate: F(0.6), ReworkRate: F(0.404)}, 1},
	} {
		if errs := c.ValidateProcessParameters(tc.p); len(errs) != tc.want {
			t.Errorf("%s: errors = %v, want %d", name, errs, tc.want)
		}
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
			ID: "c1", Name: "Customer", Demand: F(960), AvailableTime: F(480), // 480 min / 960 units
			TaktTime: F(0.5), // = AvailableTime ÷ Demand (a different value is a consistency issue)
		},
		Processes: []Process{
			{ID: "p1", Name: "Cut", Parameters: ProcessParameters{
				TimeUnit: TimeUnitMinutes, ProcessTime: F(2), ValueAddingTime: F(1), IdleTime: F(10), YieldFactor: F(0.9),
			}},
			{ID: "p2", Name: "Weld", Parameters: ProcessParameters{
				TimeUnit: TimeUnitSeconds, ProcessingTime: F(30), SetupTime: F(30), ValueAddingTimeProcess: F(45),
				TransportTime: F(120), WaitingTime: F(240),
			}},
			{ID: "p3", Name: "Paint", Parameters: ProcessParameters{
				TimeUnit: TimeUnitHours, ProcessTime: F(0.05), ValueAddingTime: F(0.025), IdleTime: F(0.1), YieldFactor: F(0.8),
			}},
		},
		Inventory: []Inventory{
			{ID: "i1", Name: "Raw", Quantity: F(200)},
			{ID: "i2", Name: "WIP", Quantity: F(40)},
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
			vs: &ValueStream{Customer: &Customer{Demand: F(460)},
				Processes: []Process{{Parameters: ProcessParameters{ProcessTime: F(1)}}}},
			absent: []string{"customer_takt"},
		},
		{
			name:    "takt time used when available time missing",
			vs:      &ValueStream{Customer: &Customer{Demand: F(460), TaktTime: F(60)}},
			present: map[string]float64{"customer_takt": 60},
		},
		{
			name: "available time with zero demand falls back to takt time",
			vs:   &ValueStream{Customer: &Customer{AvailableTime: F(27600), TaktTime: F(55)}},
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
			vs: &ValueStream{Customer: &Customer{TaktTime: F(1)},
				Inventory: []Inventory{{ID: "i", Quantity: F(10)}}},
			absent: []string{"inventory_turns"},
		},
		{
			name:   "no yield factors omits average yield",
			vs:     &ValueStream{Processes: []Process{{Parameters: ProcessParameters{ProcessTime: F(1)}}}},
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
	current := &ValueStream{Processes: []Process{{Parameters: ProcessParameters{ProcessTime: F(5), IdleTime: F(15)}}}}
	future := &ValueStream{Processes: []Process{{Parameters: ProcessParameters{ProcessTime: F(5), IdleTime: F(5)}}}}
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

// TestStockQuantityFormulaDeprecated: process time × quantity is not a
// quantity (its value depends on the time unit), so CalculateStockQuantity
// is marked deprecated and nothing in the library calls it.
func TestStockQuantityFormulaDeprecated(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, pkg := range pkgs {
		for name, file := range pkg.Files {
			ast.Inspect(file, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.FuncDecl:
					if n.Name.Name == "CalculateStockQuantity" {
						found = true
						if n.Doc == nil || !strings.Contains(n.Doc.Text(), "\nDeprecated: ") {
							t.Errorf("CalculateStockQuantity is not marked Deprecated")
						}
					}
				case *ast.SelectorExpr:
					if n.Sel.Name == "CalculateStockQuantity" {
						t.Errorf("%s calls CalculateStockQuantity", name)
					}
				}
				return true
			})
		}
	}
	if !found {
		t.Error("CalculateStockQuantity not found (it must stay for compatibility)")
	}
}

// TestAverageYieldCountsKnownZero: a process that scraps everything has a
// known yield of 0 and counts in the average; a process with no quality data
// (no yield, scrap or rework given) does not.
func TestAverageYieldCountsKnownZero(t *testing.T) {
	stream := func(params ...ProcessParameters) *ValueStream {
		vs := &ValueStream{ID: "vs"}
		for i, p := range params {
			vs.Processes = append(vs.Processes, Process{ID: fmt.Sprint("p", i), Parameters: p})
		}
		return vs
	}
	c := NewCalculator()
	cases := []struct {
		name string
		vs   *ValueStream
		want float64
	}{
		{"one total failure, one perfect", stream(ProcessParameters{ScrapRate: F(1)}, ProcessParameters{YieldFactor: F(1)}), 0.5},
		{"complete failure everywhere", stream(ProcessParameters{ScrapRate: F(1)}, ProcessParameters{ScrapRate: F(0.4), ReworkRate: F(0.6)}), 0},
		{"unknown yield left out", stream(ProcessParameters{ScrapRate: F(1)}, ProcessParameters{ProcessTime: F(3)}), 0},
	}
	for _, tt := range cases {
		got, ok := c.CalculateKPIs(tt.vs)["average_yield_factor"]
		if !ok || !approx(got, tt.want) {
			t.Errorf("%s: average_yield_factor = %v (present %v), want %v", tt.name, got, ok, tt.want)
		}
	}
	if got, ok := c.CalculateKPIs(stream(ProcessParameters{ProcessTime: F(3)}))["average_yield_factor"]; ok {
		t.Errorf("no quality data: average_yield_factor = %v, want omitted", got)
	}
}

// TestImprovementMetricsAcrossUnits: a current state in minutes and a
// future state in hours (or with demand per week instead of per day) are
// compared in one unit, so equal values show no change.
func TestImprovementMetricsAcrossUnits(t *testing.T) {
	c := NewCalculator()
	current := &ValueStream{
		Customer:  &Customer{ID: "c", Demand: F(100), DemandPeriod: DemandPeriodDay, AvailableTime: F(480)},
		Processes: []Process{{Parameters: ProcessParameters{TimeUnit: TimeUnitMinutes, ProcessTime: F(20), IdleTime: F(40), ValueAddingTime: F(10)}}},
		Inventory: []Inventory{{ID: "i", Quantity: F(50)}},
	}
	future := &ValueStream{
		// 700 per week = 100 per day; takt = 56 h ÷ 700 = 0.08 h = 4.8 min,
		// the current takt.
		Customer:  &Customer{ID: "c", Demand: F(700), DemandPeriod: DemandPeriodWeek, AvailableTime: F(56)},
		Processes: []Process{{Parameters: ProcessParameters{TimeUnit: TimeUnitHours, ProcessTime: F(0.25), IdleTime: F(0.5), ValueAddingTime: F(1.0 / 6)}}},
		Inventory: []Inventory{{ID: "i", Quantity: F(25)}},
	}
	m := c.CalculateImprovementMetrics(current, future)
	want := map[string]float64{
		// Lead time includes the inventory points: 60 min + 50 ÷ 100 per day
		// (720 min) → 45 min + 25 ÷ 100 per day (360 min).
		"total_lead_time_improvement_ratio":           405.0 / 780,
		"total_lead_time_improvement_percent":         (405.0 - 780) / 780 * 100,
		"total_value_adding_time_improvement_ratio":   1, // 10 min both
		"customer_takt_improvement_ratio":             1, // 4.8 min both
		"inventory_turns_improvement_ratio":           2, // 2 per day → 28 per week = 4 per day
		"inventory_turns_improvement_percent":         100,
		"value_added_ratio_improvement_ratio":         (10.0 / 405) / (10.0 / 780),
		"total_value_adding_time_improvement_percent": 0,
	}
	for name, w := range want {
		if got, ok := m[name]; !ok || !approx(got, w) {
			t.Errorf("%s = %v (present %v), want %v", name, got, ok, w)
		}
	}

	// Demand per month cannot be put on a daily basis: inventory turns are
	// not compared.
	future.Customer = &Customer{ID: "c", Demand: F(3000), DemandPeriod: DemandPeriodMonth}
	if got, ok := c.CalculateImprovementMetrics(current, future)["inventory_turns_improvement_ratio"]; ok {
		t.Errorf("turns per day vs per month compared: %v", got)
	}
}
