package vsm

import (
	"math"
	"strings"
	"testing"
)

// findIssue returns the issue for element id's field, if any.
func findIssue(issues []ConsistencyIssue, element, id, field string) (ConsistencyIssue, bool) {
	for _, issue := range issues {
		if issue.Element == element && issue.ID == id && issue.Field == field {
			return issue, true
		}
	}
	return ConsistencyIssue{}, false
}

// Review finding: a cycle_time target on a process known only by its value
// classification (VAT 5 + NVAT 5) adds processing time 2 × 10 ÷ 1 = 20.
// The totals are recomputed from the new part (process and lead time 20)
// and the untargeted NVAT takes the change (15). Failed before 095cf19
// (the final consistency check rejected the targets).
func TestTargetAddsProcessingTimeToClassifiedProcess(t *testing.T) {
	vs := &ValueStream{ID: "s", Name: "s", Processes: []Process{{ID: "p", Name: "p", Parameters: ProcessParameters{
		ValueAddingTime: F(5), NonValueAddingTime: F(5), OrderQuantity: F(10), ResourceCount: I(1),
		TimeUnit: TimeUnitMinutes}}}}
	future, err := NewDesigner().DesignFutureStateFromTargets(vs, &FutureStateTargets{
		Processes: map[string]ProcessTargets{"p": {CycleTime: AbsoluteTarget(2)}}})
	if err != nil {
		t.Fatalf("feasible target rejected: %v", err)
	}
	r := NewCalculator().ResolveParametersInStream(future.Processes[0].Parameters, future)
	if !approx(val(r.ProcessingTime), 20) || !approx(val(r.ProcessTime), 20) || !approx(val(r.LeadTime), 20) ||
		!approx(val(r.NonValueAddingTime), 15) || !approx(val(r.ValueAddingTime), 5) {
		t.Errorf("future: processing %v, process %v, lead %v, NVAT %v, VAT %v; want 20, 20, 20, 15, 5",
			val(r.ProcessingTime), val(r.ProcessTime), val(r.LeadTime), val(r.NonValueAddingTime), val(r.ValueAddingTime))
	}
	if issues := CheckConsistency(future); len(issues) != 0 {
		t.Errorf("future issues: %+v", issues)
	}
}

// Review finding: an inventory linked to a process whose idle time is given
// only as a total waits within that idle time. A range longer than the
// whole idle time contradicts it and is reported; before, it was skipped
// (lead time 6 days, no issue).
func TestLinkedRangeLongerThanIdleTotal(t *testing.T) {
	stream := func(idle, rangeDays float64) *ValueStream {
		return &ValueStream{ID: "s", Name: "s", Processes: []Process{{ID: "p", Name: "p", Parameters: ProcessParameters{
			ProcessTime: F(1), IdleTime: F(idle), StockQuantity: F(100), ValueAddingTime: F(1), TimeUnit: TimeUnitDays}}},
			Inventory: []Inventory{{ID: "i", Name: "i", Quantity: F(100), Range: F(rangeDays), ProcessID: "p"}}}
	}
	vs := stream(5, 10)
	issues := CheckConsistency(vs)
	issue, ok := findIssue(issues, "inventory", "i", "range")
	if !ok || issue.Kind != IssueContradiction || issue.Rule != ruleInvWithinIdle ||
		issue.StoredValue != 10 || issue.ComputedValue != 5 {
		t.Fatalf("issues = %+v, want inventory i range 10 above idle 5", issues)
	}
	if len(issues) != 1 {
		t.Errorf("issues = %+v, want only the range issue", issues)
	}
	if lt := NewCalculator().CalculateKPIs(vs)["total_lead_time"]; !approx(lt, 6) {
		t.Errorf("lead time = %v, want 6 (the process's idle time counts)", lt)
	}
	// A range within the idle time is consistent.
	if issues := CheckConsistency(stream(5, 4)); len(issues) != 0 {
		t.Errorf("range 4 within idle 5: issues %+v", issues)
	}

	// Targeting the idle time below the range would create the same
	// contradiction: rejected.
	_, err := NewDesigner().DesignFutureStateFromTargets(stream(10, 10), &FutureStateTargets{
		Processes: map[string]ProcessTargets{"p": {IdleTime: AbsoluteTarget(5)}}})
	if err == nil || !strings.Contains(err.Error(), "inventory i") {
		t.Errorf("idle 10 → 5 with a 10-day linked range: err = %v, want a rejection naming inventory i", err)
	}
	// Halving the stock moves the linked inventory with it.
	future, err := NewDesigner().DesignFutureStateFromTargets(stream(10, 10), &FutureStateTargets{
		Processes: map[string]ProcessTargets{"p": {StockQuantity: ReductionTarget(50)}}})
	if err != nil {
		t.Fatal(err)
	}
	if inv := future.Inventory[0]; !approx(val(inv.Quantity), 50) || !approx(val(inv.Range), 5) {
		t.Errorf("future inventory = %v units, %v days; want 50, 5", val(inv.Quantity), val(inv.Range))
	}
	if issues := CheckConsistency(future); len(issues) != 0 {
		t.Errorf("future issues: %+v", issues)
	}
}

// Review finding: value adding time 20 in a process time of 10 is
// impossible, so flow efficiency is unknown in the analysis exactly as in
// the KPIs (before: FlowEfficiency 2, i.e. 200%), and no efficiency-based
// recommendation is made.
func TestAnalysisFlowEfficiencyUnknownWhenImpossible(t *testing.T) {
	vs := &ValueStream{ID: "s", Name: "s", Processes: []Process{{ID: "p", Name: "p", Parameters: ProcessParameters{
		ProcessTime: F(10), ValueAddingTime: F(20), TimeUnit: TimeUnitMinutes}}}}
	result, err := NewAnalyzer().AnalyzeCurrentState(vs)
	if err != nil {
		t.Fatal(err)
	}
	if e := result.FlowAnalysis.FlowEfficiency; e != nil {
		t.Errorf("flow efficiency = %v, want unknown", *e)
	}
	if v, ok := result.KPIs["value_added_ratio"]; ok {
		t.Errorf("value_added_ratio = %v, want omitted", v)
	}
	for _, r := range result.Recommendations {
		if strings.Contains(r, "flow efficiency") || strings.Contains(r, "value-added ratio") {
			t.Errorf("efficiency recommendation with unknown efficiency: %q", r)
		}
	}
}

// Review finding: the same stock gives the same waste whether it is drawn
// as a separate inventory or as process stock with a linked inventory
// (before: waste 0 versus 1 day, with the same 2-day lead time).
func TestWasteSameForEquivalentStock(t *testing.T) {
	customer := &Customer{ID: "c", Demand: F(100), DemandPeriod: DemandPeriodDay}
	separate := &ValueStream{ID: "a", Name: "a", Customer: customer,
		Processes: []Process{{ID: "p", Name: "p", Parameters: ProcessParameters{
			ProcessTime: F(1), ValueAddingTime: F(1), TimeUnit: TimeUnitDays}}},
		Inventory: []Inventory{{ID: "i", Name: "i", Quantity: F(100)}}}
	linked := &ValueStream{ID: "b", Name: "b", Customer: customer,
		Processes: []Process{{ID: "p", Name: "p", Parameters: ProcessParameters{
			ProcessTime: F(1), ValueAddingTime: F(1), StockQuantity: F(100), TimeUnit: TimeUnitDays}}},
		Inventory: []Inventory{{ID: "i", Name: "i", Quantity: F(100), ProcessID: "p"}}}
	c, a := NewCalculator(), NewAnalyzer()
	for _, vs := range []*ValueStream{separate, linked} {
		if lt := c.CalculateKPIs(vs)["total_lead_time"]; !approx(lt, 2) {
			t.Errorf("%s: lead time = %v, want 2", vs.ID, lt)
		}
	}
	wa, _ := a.AnalyzeWaste(separate)
	wb, _ := a.AnalyzeWaste(linked)
	if !approx(val(wa.TotalWaste), 1) || !approx(val(wb.TotalWaste), 1) {
		t.Errorf("total waste: separate %v, linked %v; want 1 day each", val(wa.TotalWaste), val(wb.TotalWaste))
	}
	for _, kind := range wasteKinds {
		if !approx(wa.WasteByType[kind], wb.WasteByType[kind]) {
			t.Errorf("%s waste: separate %v, linked %v", kind, wa.WasteByType[kind], wb.WasteByType[kind])
		}
	}
	if !approx(wa.WasteByInventory["i"], 1) {
		t.Errorf("separate inventory waste = %v, want 1", wa.WasteByInventory)
	}
}

// Review finding: a time unit the model cannot convert ("min") is rejected
// by validation, reported by CheckConsistency, and its times are never
// added unconverted (before: 1 hour + 60 "min" = lead time 61 hours).
func TestUnsupportedTimeUnit(t *testing.T) {
	vs := &ValueStream{ID: "s", Name: "s", Processes: []Process{
		{ID: "p1", Name: "p1", Parameters: ProcessParameters{ProcessTime: F(1), ValueAddingTime: F(1), TimeUnit: TimeUnitHours}},
		{ID: "p2", Name: "p2", Parameters: ProcessParameters{ProcessTime: F(60), ValueAddingTime: F(30), TimeUnit: "min"}},
	}}
	if err := NewValidator().ValidateValueStream(vs); err == nil || !strings.Contains(err.Error(), `"min"`) {
		t.Errorf("validation = %v, want the unsupported unit rejected", err)
	}
	c := NewCalculator()
	if errs := c.ValidateProcessParameters(vs.Processes[1].Parameters); len(errs) != 1 {
		t.Errorf("ValidateProcessParameters = %v, want the unit error", errs)
	}
	issues := CheckConsistency(vs)
	if issue, ok := findIssue(issues, "process", "p2", "time_unit"); !ok || issue.Kind != IssueContradiction {
		t.Errorf("issues = %+v, want a time_unit issue on p2", issues)
	}
	kpis := c.CalculateKPIs(vs)
	for _, name := range []string{"total_lead_time", "total_value_adding_time", "value_added_ratio", "process_efficiency"} {
		if v, ok := kpis[name]; ok {
			t.Errorf("%s = %v, want omitted (p2 cannot be converted)", name, v)
		}
	}
	if lt := c.CalculateOverallLeadTime(vs.Processes); !math.IsNaN(lt) {
		t.Errorf("CalculateOverallLeadTime = %v, want NaN (unknown)", lt)
	}
	// The reference unit is the first supported one; the documented empty
	// unit is still the reference unit.
	if u := c.ReferenceTimeUnit([]Process{{Parameters: ProcessParameters{TimeUnit: "min"}}, vs.Processes[0]}); u != TimeUnitHours {
		t.Errorf("reference unit = %q, want hours", u)
	}
	vs.Processes[1].Parameters.TimeUnit = ""
	if lt := c.CalculateKPIs(vs)["total_lead_time"]; !approx(lt, 61) {
		t.Errorf("empty unit (taken as hours): lead time = %v, want 61", lt)
	}
	if err := NewValidator().ValidateValueStream(vs); err != nil {
		t.Errorf("empty unit rejected: %v", err)
	}
}

// Part B: an element on the map whose part of a total is unknown makes the
// total unknown (omitted, never counted as 0), and CheckConsistency names
// it as missing data. An element that is not on the map adds nothing.
func TestUnknownPartsMakeTotalsUnknown(t *testing.T) {
	base := func() *ValueStream {
		return &ValueStream{ID: "s", Name: "s",
			Customer: &Customer{ID: "c", Demand: F(100), DemandPeriod: DemandPeriodDay},
			Processes: []Process{{ID: "p", Name: "p", Parameters: ProcessParameters{
				ProcessTime: F(1), ValueAddingTime: F(0.5), TimeUnit: TimeUnitDays}}}}
	}
	ltKPIs := []string{"total_lead_time", "value_added_ratio", "process_efficiency"}
	c := NewCalculator()

	// Complete: no inventory on the map, nothing missing.
	if kpis := c.CalculateKPIs(base()); !approx(kpis["total_lead_time"], 1) || !approx(kpis["value_added_ratio"], 0.5) {
		t.Errorf("complete stream KPIs = %v", kpis)
	}
	if issues := CheckConsistency(base()); len(issues) != 0 {
		t.Errorf("complete stream issues = %+v", issues)
	}

	for _, tc := range []struct {
		name          string
		edit          func(*ValueStream)
		element, id   string
		field         string
		omitted, kept []string
	}{
		{"inventory with nothing entered", func(vs *ValueStream) {
			vs.Inventory = []Inventory{{ID: "i", Name: "i"}}
		}, "inventory", "i", "quantity", append(ltKPIs, "inventory_turns"), []string{"total_value_adding_time"}},
		{"inventory quantity without a demand rate or range", func(vs *ValueStream) {
			vs.Customer.DemandPeriod = DemandPeriodMonth
			vs.Inventory = []Inventory{{ID: "i", Name: "i", Quantity: F(50)}}
		}, "inventory", "i", "range", ltKPIs, []string{"inventory_turns", "total_value_adding_time"}},
		{"process with unknown lead time", func(vs *ValueStream) {
			vs.Processes = append(vs.Processes, Process{ID: "q", Name: "q", Parameters: ProcessParameters{
				CycleTime: F(0.1), TimeUnit: TimeUnitDays}})
		}, "process", "q", "lead_time", append(ltKPIs, "total_value_adding_time"), nil},
		{"process without value adding time", func(vs *ValueStream) {
			vs.Processes = append(vs.Processes, Process{ID: "q", Name: "q", Parameters: ProcessParameters{
				ProcessTime: F(1), TimeUnit: TimeUnitDays}})
		}, "process", "q", "value_adding_time", []string{"total_value_adding_time", "value_added_ratio"},
			[]string{"total_lead_time", "process_efficiency"}},
		{"stock waiting for an unknown time", func(vs *ValueStream) {
			vs.Customer = nil
			vs.Processes[0].Parameters.StockQuantity = F(100)
		}, "process", "p", "waiting_time", ltKPIs, []string{"total_value_adding_time"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vs := base()
			tc.edit(vs)
			kpis := c.CalculateKPIs(vs)
			for _, name := range tc.omitted {
				if v, ok := kpis[name]; ok {
					t.Errorf("%s = %v, want omitted", name, v)
				}
			}
			for _, name := range tc.kept {
				if _, ok := kpis[name]; !ok {
					t.Errorf("%s omitted, want computed: %v", name, kpis)
				}
			}
			issues := CheckConsistency(vs)
			issue, ok := findIssue(issues, tc.element, tc.id, tc.field)
			if !ok || issue.Kind != IssueMissingData || !strings.Contains(issue.Message, tc.element+" "+tc.id+":") {
				t.Errorf("issues = %+v, want missing data on %s %s %s", issues, tc.element, tc.id, tc.field)
			}
			if len(contradictions(issues)) != 0 {
				t.Errorf("contradictions = %+v, want none", contradictions(issues))
			}
		})
	}

	// The stock's time carried by a linked inventory's range is known.
	vs := base()
	vs.Customer = nil
	vs.Processes[0].Parameters.StockQuantity = F(100)
	vs.Inventory = []Inventory{{ID: "i", Name: "i", Quantity: F(100), Range: F(2), ProcessID: "p"}}
	if lt := c.CalculateKPIs(vs)["total_lead_time"]; !approx(lt, 3) {
		t.Errorf("stock timed by its linked inventory: lead time = %v, want 1 + 2", lt)
	}
	if issues := CheckConsistency(vs); len(issues) != 0 {
		t.Errorf("issues = %+v", issues)
	}
}

// Part B everywhere: the analysis, design, assessment and text export show
// the same unknowns as the KPIs and name the missing value.
func TestMissingDataShownInEveryResult(t *testing.T) {
	vs := &ValueStream{ID: "s", Name: "s",
		Customer: &Customer{ID: "c", Demand: F(100), DemandPeriod: DemandPeriodDay},
		Processes: []Process{{ID: "p", Name: "p", Parameters: ProcessParameters{
			ProcessTime: F(1), ValueAddingTime: F(0.5), TimeUnit: TimeUnitDays}}},
		Inventory: []Inventory{{ID: "i", Name: "Raw"}}}

	analysis, err := NewAnalyzer().AnalyzeCurrentState(vs)
	if err != nil {
		t.Fatal(err)
	}
	if analysis.IdleTimeAnalysis.TotalIdleTime != nil || analysis.IdleTimeAnalysis.IdleTimeEfficiency != nil ||
		analysis.IdleTimeAnalysis.IdleTimeBreakdown != nil {
		t.Errorf("idle analysis = %+v, want unknown totals", analysis.IdleTimeAnalysis)
	}
	if analysis.WasteAnalysis.TotalWaste != nil || len(analysis.WasteAnalysis.WasteByType) != 0 {
		t.Errorf("waste = %+v, want unknown", analysis.WasteAnalysis)
	}
	if analysis.FlowAnalysis.FlowEfficiency != nil {
		t.Errorf("flow efficiency = %v, want unknown", *analysis.FlowAnalysis.FlowEfficiency)
	}
	if len(missingData(analysis.ConsistencyIssues)) != 1 || !strings.Contains(strings.Join(analysis.Notes, "\n"), "missing") {
		t.Errorf("analysis issues %+v, notes %v", analysis.ConsistencyIssues, analysis.Notes)
	}

	design, err := NewDesigner().DesignFutureState(vs, analysis)
	if err != nil {
		t.Fatal(err)
	}
	if len(missingData(design.ConsistencyIssues)) != 1 || !strings.Contains(strings.Join(design.Notes, "\n"), "missing") {
		t.Errorf("design issues %+v, notes %v", design.ConsistencyIssues, design.Notes)
	}

	assessment, err := NewAssessor().AssessValueStream(vs, map[string]float64{"total_lead_time": 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := assessment.KPIs["total_lead_time"]; ok || len(missingData(assessment.ConsistencyIssues)) != 1 ||
		!strings.Contains(strings.Join(assessment.Recommendations, "\n"), "total_lead_time has a target but cannot be computed") {
		t.Errorf("assessment KPIs %v, issues %+v, recommendations %v", assessment.KPIs,
			assessment.ConsistencyIssues, assessment.Recommendations)
	}

	text, err := NewSerializer().ExportValueStreamMap(vs)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Raw: not given (range unknown)", "Missing data (1)",
		"inventory i: neither quantity nor range entered"} {
		if !strings.Contains(text, want) {
			t.Errorf("map text missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "total_lead_time:") || strings.Contains(text, "Consistency issues") {
		t.Errorf("map shows an unknown lead time or a contradiction:\n%s", text)
	}
}

// Part A: every result reads the same stream figures, so the analysis
// numbers equal the KPIs and are known exactly when the KPIs are.
func TestAnalysisAgreesWithKPIs(t *testing.T) {
	streams := map[string]*ValueStream{
		"mixed units": mixedUnitStream(),
		"inventory":   invStream(),
		"impossible VAT": {ID: "v", Name: "v", Processes: []Process{{ID: "p", Name: "p", Parameters: ProcessParameters{
			ProcessTime: F(10), ValueAddingTime: F(20)}}}},
		"missing inventory": {ID: "m", Name: "m", Processes: []Process{{ID: "p", Name: "p", Parameters: ProcessParameters{
			ProcessTime: F(10), ValueAddingTime: F(2)}}}, Inventory: []Inventory{{ID: "i"}}},
	}
	c, a := NewCalculator(), NewAnalyzer()
	same := func(p *float64, v float64, ok bool) bool {
		return (p == nil) == !ok && (p == nil || approx(*p, v))
	}
	for name, vs := range streams {
		kpis := c.CalculateKPIs(vs)
		flow, _ := a.AnalyzeFlow(vs)
		idle, _ := a.AnalyzeIdleTimes(vs)
		waste, _ := a.AnalyzeWaste(vs)
		ratio, hasRatio := kpis["value_added_ratio"]
		if !same(flow.FlowEfficiency, ratio, hasRatio) {
			t.Errorf("%s: flow efficiency %v, value_added_ratio %v (%v)", name, flow.FlowEfficiency, ratio, hasRatio)
		}
		pe, hasPE := kpis["process_efficiency"]
		if !same(idle.IdleTimeEfficiency, pe, hasPE) {
			t.Errorf("%s: idle efficiency %v, process_efficiency %v (%v)", name, idle.IdleTimeEfficiency, pe, hasPE)
		}
		lt, hasLT := kpis["total_lead_time"]
		if (waste.TotalWaste != nil) != hasLT {
			t.Errorf("%s: waste known %v, lead time known %v", name, waste.TotalWaste != nil, hasLT)
		}
		if idle.TotalIdleTime != nil && hasLT && hasPE && !approx(*idle.TotalIdleTime, lt*(1-pe)) {
			t.Errorf("%s: idle %v, want lead time − process time = %v", name, *idle.TotalIdleTime, lt*(1-pe))
		}
	}
}
