package vsm

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// ConsistencyTolerance is the relative difference (0.5%) below which a
// stored value and the value computed from its parts count as equal.
const ConsistencyTolerance = 0.005

// ConsistencyIssue reports a stored value that disagrees with the value the
// model computes from other stored values. Calculations always use the
// computed value; the issue only makes the disagreement visible.
type ConsistencyIssue struct {
	// Element is "process", "inventory", "customer" or "value_stream".
	Element string `json:"element"`
	// ID is the process, inventory or customer ID (empty for the value stream).
	ID string `json:"id,omitempty"`
	// Field is the JSON name of the stored value (or the sum that was checked).
	Field string `json:"field"`
	// StoredValue is the value as entered.
	StoredValue float64 `json:"stored_value"`
	// ComputedValue is the value computed from the other inputs (the value
	// calculations use), or the limit that StoredValue exceeds.
	ComputedValue float64 `json:"computed_value"`
	// Unit is the time unit of both values, when they are times.
	Unit string `json:"unit,omitempty"`
	// Rule is the relationship that was checked.
	Rule string `json:"rule"`
	// Message is a one-line, human-readable summary.
	Message string `json:"message"`
}

// Relationship rules (used in ConsistencyIssue.Rule).
const (
	ruleTransport     = "transport_time = transport_repetition_time × transport_repetition_count (A.10)"
	ruleWaiting       = "waiting_time = stock_quantity ÷ customer demand rate (inventory time: stock ÷ demand per demand_period)"
	ruleRange         = "range_of_inventory = stock_quantity ÷ customer demand rate (stock ÷ demand per demand_period)"
	ruleInvRange      = "inventory range = quantity ÷ customer demand rate (quantity ÷ demand per demand_period)"
	ruleProcessing    = "processing_time = cycle_time × order_quantity ÷ resource_count (A.5)"
	ruleProcessParts  = "process_time = processing_time + setup_time + handling_time + waiting_time_process + ancillary_time (A.2)"
	ruleRepetitive    = "process_time = setup_time + repetition_time × repetition_count (A.6)"
	ruleIdle          = "idle_time = transport_time + waiting_time + storage_time"
	ruleVATp          = "value_adding_time = value_adding_time_process (A.9)"
	ruleVATWithinPT   = "value_adding_time is part of process_time, so it cannot exceed it"
	ruleLeadFlow      = "lead_time = process_time + idle_time"
	ruleLeadClass     = "value_adding_time + non_value_adding_time + necessary_non_value_adding_time = lead_time = process_time + idle_time (A.3)"
	ruleYield         = "yield_factor = 1 − (scrap_rate + rework_rate) (A.17)"
	ruleDefects       = "scrap_rate + rework_rate is the defective share of parts, so it cannot exceed 1"
	fieldDefects      = "scrap_rate + rework_rate"
	ruleTakt          = "takt_time = available_time ÷ demand (A.12)"
	ruleWorkingDays   = "working_days_per_week is between 1 and 7 (absent means 7)"
	ruleStoredKPI     = "stored KPI = KPI computed from the processes (CalculateKPIs)"
	fieldClassSum     = "value_adding_time + non_value_adding_time + necessary_non_value_adding_time"
	fieldProcessParts = "processing_time + setup_time + handling_time + waiting_time_process + ancillary_time"
)

// consistent reports whether a and b agree within ConsistencyTolerance.
func consistent(a, b float64) bool {
	d := math.Abs(a - b)
	return d <= 1e-9 || d <= ConsistencyTolerance*math.Max(math.Abs(a), math.Abs(b))
}

// ResolveParameters returns params with every derived value computed from
// its parts, in params' own time unit. It is the single place where the
// model decides between a stored total and its components; every
// calculation, analysis and design step uses it.
//
// ResolveParameters sees the process alone, so it cannot know the customer
// demand rate and leaves waiting_time and range_of_inventory as stored. Use
// ResolveParametersInStream to also derive them from stock_quantity; every
// value-stream calculation (CalculateKPIs, CheckConsistency, the analyzer
// and designer) does.
//
// The rule: when a value's parts are given (> 0), the value is computed from
// them and a stored value is only checked against it. When only the stored
// value is given (a rough map), it is used as is. Zero means "not given".
// In order:
//
//   - transport_time = transport_repetition_time × transport_repetition_count (A.10)
//   - waiting_time and range_of_inventory = stock_quantity ÷ customer demand
//     rate: the standard VSM inventory time (Rother & Shook: inventory ÷
//     daily demand), as calendar time in params' unit through
//     Customer.DemandPeriod and Customer.WorkingDaysPerWeek. Only in a
//     stream whose customer has Demand and a day or week DemandPeriod (see
//     CalculateCustomerInventoryTime); otherwise the stored values are used
//     and not checked.
//   - processing_time = cycle_time × order_quantity ÷ resource_count (A.5)
//   - process_time = processing + setup + handling + waiting (process) +
//     ancillary time (A.2): when any part is given, the sum of the parts,
//     and a stored process_time that differs in either direction is
//     reported. When repetition data is set, A.6 (setup + repetition_time ×
//     repetition_count) gives process_time instead, and A.2 parts beyond
//     setup_time that add up to something else are reported.
//   - idle_time = transport + waiting + storage time
//   - value_adding_time = value_adding_time_process when that is set (A.9)
//   - lead_time = process_time + idle_time; when neither is given, the value
//     classification VAT + NVAT + NNVAT (A.3); when neither is given, the
//     stored lead_time
//   - yield_factor = 1 − (scrap_rate + rework_rate) (A.17); when the sum
//     of scrap_rate and rework_rate is above 1 (impossible), yield_factor is
//     left as stored,
//     the process's yield counts as unknown and the contradiction is
//     reported
func (c *Calculator) ResolveParameters(params ProcessParameters) ProcessParameters {
	p, _ := c.resolveParameters(params, streamContext{})
	return p
}

// ResolveParametersInStream is ResolveParameters for a process of vs: it
// also derives waiting_time and range_of_inventory from stock_quantity and
// the customer's demand rate (see ResolveParameters). A process without a
// TimeUnit is taken to be in vs's reference unit.
func (c *Calculator) ResolveParametersInStream(params ProcessParameters, vs *ValueStream) ProcessParameters {
	p, _ := c.resolveParameters(params, c.contextOf(vs))
	return p
}

// streamContext is what resolving a process needs from its value stream:
// the customer (for the demand rate stock is used up at) and the reference
// time unit (the unit of processes that declare none). The zero value means
// "process alone".
type streamContext struct {
	customer *Customer
	unit     TimeUnit
}

// contextOf returns the stream context of vs (zero for nil).
func (c *Calculator) contextOf(vs *ValueStream) streamContext {
	if vs == nil {
		return streamContext{}
	}
	return streamContext{customer: vs.Customer, unit: c.ReferenceTimeUnit(vs.Processes)}
}

// unitOf returns the unit a process's times are in: its own, else the
// reference unit.
func (s streamContext) unitOf(u TimeUnit) TimeUnit {
	if u == "" {
		return s.unit
	}
	return u
}

// demandRate returns the customer's demand in units per one unit of
// calendar time u, and false when it cannot be known (see
// CalculateCustomerInventoryTime).
func (s streamContext) demandRate(u TimeUnit) (float64, bool) {
	if s.customer == nil {
		return 0, false
	}
	return customerDemandRate(s.customer, s.unitOf(u))
}

// resolvedIn resolves params in the stream context and converts the result
// to the reference unit.
func (c *Calculator) resolvedIn(params ProcessParameters, ctx streamContext) ProcessParameters {
	p, _ := c.resolveParameters(params, ctx)
	return c.normalizeTimes(p, ctx.unit)
}

// resolveParameters implements ResolveParameters and also returns the
// disagreements it found (Element and ID left empty).
func (c *Calculator) resolveParameters(in ProcessParameters, ctx streamContext) (ProcessParameters, []ConsistencyIssue) {
	p := in
	var issues []ConsistencyIssue
	unit := string(in.TimeUnit)
	report := func(field string, stored, computed float64, rule, format string) {
		issues = append(issues, ConsistencyIssue{
			Field: field, StoredValue: stored, ComputedValue: computed, Unit: unit, Rule: rule,
			Message: fmt.Sprintf(format, field, stored, computed),
		})
	}
	const replaced = "%s: stored %g, but its parts give %g; calculations use %[3]g"
	// derive sets *field to computed, reporting a stored value that disagrees.
	derive := func(name string, field *float64, computed float64, rule string) {
		if *field > 0 && !consistent(*field, computed) {
			report(name, *field, computed, rule, replaced)
		}
		*field = computed
	}

	if p.TransportRepetitionTime > 0 && p.TransportRepetitionCount > 0 {
		derive("transport_time", &p.TransportTime,
			p.TransportRepetitionTime*float64(p.TransportRepetitionCount), ruleTransport)
	}
	if rate, ok := ctx.demandRate(in.TimeUnit); ok && p.StockQuantity > 0 {
		derive("waiting_time", &p.WaitingTime, p.StockQuantity/rate, ruleWaiting)
		derive("range_of_inventory", &p.RangeOfInventory, p.StockQuantity/rate, ruleRange)
	}
	if p.CycleTime > 0 && p.OrderQuantity > 0 && p.ResourceCount > 0 {
		derive("processing_time", &p.ProcessingTime,
			p.CycleTime*p.OrderQuantity/float64(p.ResourceCount), ruleProcessing)
	}

	// Process time (A.2, A.6).
	parts := c.CalculateProcessTime(p)
	given := processPartsGiven(p)
	if p.RepetitionTime > 0 && p.RepetitionCount > 0 {
		// A.6 gives process time. Setup time is in both A.6 and A.2, so the
		// A.2 parts are a second, complete breakdown only when a part other
		// than setup is given; then they must add up to the same value.
		a6 := c.CalculateRepetitiveProcessTime(p.SetupTime, p.RepetitionTime, p.RepetitionCount)
		if p.ProcessTime > 0 && !consistent(p.ProcessTime, a6) {
			report("process_time", p.ProcessTime, a6, ruleRepetitive, replaced)
		}
		if parts > p.SetupTime && !consistent(parts, a6) {
			issues = append(issues, ConsistencyIssue{
				Field: fieldProcessParts, StoredValue: parts, ComputedValue: a6, Unit: unit, Rule: ruleRepetitive,
				Message: fmt.Sprintf("process time parts (%s) sum to %g, but repetition data gives "+
					"setup_time + repetition_time × repetition_count = %g; correct the parts or the "+
					"repetition data; calculations use %[3]g", strings.Join(given, " + "), parts, a6),
			})
		}
		p.ProcessTime = a6
	} else if parts > 0 {
		if p.ProcessTime > 0 && !consistent(p.ProcessTime, parts) {
			fix := "correct the parts or the stored value"
			if parts < p.ProcessTime {
				fix = "enter the missing parts, or clear the given parts to use the stored total"
			}
			report("process_time", p.ProcessTime, parts, ruleProcessParts,
				"%s: stored %g, but its parts sum to %g (only "+strings.Join(given, " + ")+
					" given) — "+fix+"; calculations use %[3]g")
		}
		p.ProcessTime = parts
	}

	// Idle time.
	if idle := c.CalculateTotalIdleTime(p.TransportTime, p.WaitingTime, p.StorageTime); idle > 0 {
		derive("idle_time", &p.IdleTime, idle, ruleIdle)
	}

	// Value adding time (VATp is the per-process value of A.9).
	if p.ValueAddingTimeProcess > 0 {
		derive("value_adding_time", &p.ValueAddingTime, p.ValueAddingTimeProcess, ruleVATp)
	}
	p.ValueAddingTimeProcess = p.ValueAddingTime
	if p.ProcessTime > 0 && p.ValueAddingTime > p.ProcessTime && !consistent(p.ValueAddingTime, p.ProcessTime) {
		report("value_adding_time", p.ValueAddingTime, p.ProcessTime, ruleVATWithinPT,
			"%s: %g exceeds process_time %g")
	}

	// Lead time: flow (PT + IT) first, else the value classification.
	flow := p.ProcessTime + p.IdleTime
	classified := p.NonValueAddingTime > 0 || p.NecessaryNonValueAddingTime > 0
	class := c.CalculateLeadTime(p)
	lt, ltRule := p.LeadTime, ruleLeadFlow
	switch {
	case flow > 0:
		lt = flow
		if classified && !consistent(class, flow) {
			report(fieldClassSum, class, flow, ruleLeadClass,
				"%s: %g, but process time + idle time = %g; calculations use %[3]g")
		}
	case classified || p.ValueAddingTime > 0:
		lt, ltRule = class, ruleLeadClass
	}
	if p.LeadTime > 0 && !consistent(p.LeadTime, lt) {
		report("lead_time", p.LeadTime, lt, ltRule, replaced)
	}
	p.LeadTime = lt

	// Yield. Scrap + rework above 1 is impossible: the yield is not derived
	// from them (it would be negative), the process's yield counts as
	// unknown (see yieldKnown), and the contradiction is reported.
	if defects := p.ScrapRate + p.ReworkRate; defectsImpossible(p) {
		issues = append(issues, ConsistencyIssue{
			Field: fieldDefects, StoredValue: defects, ComputedValue: 1, Rule: ruleDefects,
			Message: fmt.Sprintf("%s = %g is above 1 (more than every part defective); "+
				"the yield of this process is treated as unknown", fieldDefects, defects),
		})
	} else if p.ScrapRate > 0 || p.ReworkRate > 0 {
		derive("yield_factor", &p.YieldFactor, c.CalculateYieldFactor(p.ScrapRate, p.ReworkRate), ruleYield)
	}

	// Rates are fractions, not times.
	for i := range issues {
		if issues[i].Field == "yield_factor" {
			issues[i].Unit = ""
		}
	}
	return p, issues
}

// inventoryRange returns the time inv's quantity lasts at the customer's
// demand rate, in the reference unit, and false when the rate is unknown or
// the quantity is 0.
func (s streamContext) inventoryRange(inv Inventory) (float64, bool) {
	rate, ok := s.demandRate(s.unit)
	if !ok || inv.Quantity <= 0 {
		return 0, false
	}
	return inv.Quantity / rate, true
}

// processPartsGiven lists the A.2 process time parts that are given (> 0).
func processPartsGiven(p ProcessParameters) []string {
	var given []string
	for _, f := range []struct {
		name  string
		value float64
	}{
		{"processing_time", p.ProcessingTime},
		{"setup_time", p.SetupTime},
		{"handling_time", p.HandlingTime},
		{"waiting_time_process", p.WaitingTimeProcess},
		{"ancillary_time", p.AncillaryTime},
	} {
		if f.value > 0 {
			given = append(given, f.name)
		}
	}
	return given
}

// customerTakt returns the takt the model uses: AvailableTime ÷ Demand when
// both are set (both per DemandPeriod, so the period cancels), otherwise the
// stored TaktTime.
func (c *Calculator) customerTakt(cust *Customer) (float64, bool) {
	switch {
	case cust == nil:
		return 0, false
	case cust.AvailableTime > 0 && cust.Demand > 0:
		return c.CalculateCustomerTakt(cust.AvailableTime, cust.Demand), true
	case cust.TaktTime > 0:
		return cust.TaktTime, true
	}
	return 0, false
}

// CheckConsistency reports every stored value in vs that disagrees (by more
// than ConsistencyTolerance) with the value computed from other stored
// values: process totals versus their parts (see
// Calculator.ResolveParametersInStream), each inventory's range versus
// quantity ÷ customer demand rate, the customer's takt versus available time
// ÷ demand, and stored KPIs versus CalculateKPIs. Times are compared in each
// process's own unit. Calculations use the computed values regardless; this
// makes the disagreements visible.
func CheckConsistency(vs *ValueStream) []ConsistencyIssue {
	if vs == nil {
		return nil
	}
	c := NewCalculator()
	ctx := c.contextOf(vs)
	var issues []ConsistencyIssue
	for _, process := range vs.Processes {
		_, found := c.resolveParameters(process.Parameters, ctx)
		for _, issue := range found {
			issue.Element, issue.ID = "process", process.ID
			issue.Message = fmt.Sprintf("process %s: %s", process.ID, issue.Message)
			issues = append(issues, issue)
		}
	}

	unit := string(ctx.unit)
	for _, inv := range vs.Inventory {
		if want, ok := ctx.inventoryRange(inv); ok && inv.Range > 0 && !consistent(inv.Range, want) {
			issues = append(issues, ConsistencyIssue{
				Element: "inventory", ID: inv.ID, Field: "range",
				StoredValue: inv.Range, ComputedValue: want, Unit: unit, Rule: ruleInvRange,
				Message: fmt.Sprintf("inventory %s: range stored %g, but quantity %g ÷ customer demand rate = %g",
					inv.ID, inv.Range, inv.Quantity, want),
			})
		}
	}

	if cust := vs.Customer; cust != nil && !workingDaysValid(cust.WorkingDaysPerWeek) {
		issues = append(issues, ConsistencyIssue{
			Element: "customer", ID: cust.ID, Field: "working_days_per_week",
			StoredValue: cust.WorkingDaysPerWeek, ComputedValue: 7, Rule: ruleWorkingDays,
			Message: fmt.Sprintf("customer %s: working_days_per_week %g is outside 1–7; "+
				"inventory times are not derived from demand", cust.ID, cust.WorkingDaysPerWeek),
		})
	}

	if cust := vs.Customer; cust != nil && cust.TaktTime > 0 && cust.AvailableTime > 0 && cust.Demand > 0 {
		if takt, _ := c.customerTakt(cust); !consistent(cust.TaktTime, takt) {
			issues = append(issues, ConsistencyIssue{
				Element: "customer", ID: cust.ID, Field: "takt_time",
				StoredValue: cust.TaktTime, ComputedValue: takt, Unit: unit, Rule: ruleTakt,
				Message: fmt.Sprintf("customer %s: takt_time stored %g, but available_time ÷ demand = %g; calculations use %[3]g",
					cust.ID, cust.TaktTime, takt),
			})
		}
	}

	if len(vs.KPIs) > 0 {
		computed := c.CalculateKPIs(vs)
		names := make([]string, 0, len(vs.KPIs))
		for name := range vs.KPIs {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			want, ok := computed[name]
			if !ok || consistent(vs.KPIs[name], want) {
				continue
			}
			kpiUnitLabel := kpiUnit(name)
			if kpiUnitLabel == "time" && unit != "" {
				kpiUnitLabel = unit
			}
			issues = append(issues, ConsistencyIssue{
				Element: "value_stream", ID: vs.ID, Field: "kpis." + name,
				StoredValue: vs.KPIs[name], ComputedValue: want, Unit: kpiUnitLabel, Rule: ruleStoredKPI,
				Message: fmt.Sprintf("value stream: stored KPI %s = %g, but the processes give %g (stored KPIs are not used in calculations)",
					name, vs.KPIs[name], want),
			})
		}
	}
	return issues
}
