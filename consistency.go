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
// model computes from other stored values, or a value that a stream total
// needs and that is missing (see Kind). Calculations always use the
// computed value, or omit what cannot be computed; the issue makes it
// visible.
type ConsistencyIssue struct {
	// Kind is IssueContradiction (stored values disagree, or a value is
	// impossible or unusable) or IssueMissingData (a value a stream total
	// needs is unknown; the total and the KPIs built on it are omitted).
	// Results written before Kind existed have none: they are
	// contradictions.
	Kind string `json:"kind,omitempty"`
	// Element is "process", "inventory", "customer" or "value_stream".
	Element string `json:"element"`
	// ID is the process, inventory or customer ID (empty for the value stream).
	ID string `json:"id,omitempty"`
	// Field is the JSON name of the stored value (or the sum that was checked).
	Field string `json:"field"`
	// StoredValue is the value as entered (0 for missing data).
	StoredValue float64 `json:"stored_value"`
	// ComputedValue is the value computed from the other inputs (the value
	// calculations use), or the limit that StoredValue exceeds (0 for
	// missing data).
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
	ruleVATWithinLT   = "value_adding_time is part of lead_time, so it cannot exceed it"
	ruleLeadFlow      = "lead_time = process_time + idle_time"
	ruleLeadClass     = "value_adding_time + non_value_adding_time + necessary_non_value_adding_time = lead_time = process_time + idle_time (A.3)"
	ruleYield         = "yield_factor = 1 − (scrap_rate + rework_rate) (A.17)"
	ruleDefects       = "scrap_rate + rework_rate is the defective share of parts, so it cannot exceed 1"
	fieldDefects      = "scrap_rate + rework_rate"
	ruleTakt          = "takt_time = available_time ÷ demand (A.12)"
	ruleWorkingDays   = "working_days_per_week is between 1 and 7 (not entered means 7)"
	ruleInvLink       = "an inventory with process_id is that process's stock_quantity (counted once, through the process)"
	ruleTimeUnit      = "time_unit is seconds, minutes, hours, days or weeks (or empty: the reference unit)"
	ruleStoredKPI     = "stored KPI = KPI computed from the processes (CalculateKPIs)"
	fieldClassSum     = "value_adding_time + non_value_adding_time + necessary_non_value_adding_time"
	fieldProcessParts = "processing_time + setup_time + handling_time + waiting_time_process + ancillary_time"
)

// boundEpsilon is the floating-point rounding allowance for mathematical
// bounds (a share at most 1, a part at most its whole). It is not
// ConsistencyTolerance: that is how far two measurements of the same value
// may differ, and must never let an impossible value through.
const boundEpsilon = 1e-9

// exceeds reports whether v is above limit by more than float rounding
// (boundEpsilon, relative to limit when limit is above 1).
func exceeds(v, limit float64) bool {
	return v > limit+boundEpsilon*math.Max(1, math.Abs(limit))
}

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
// The rule: a value whose parts are entered is computed from them, and a
// stored value is only checked against it. When only the stored value is
// entered (a rough map), it is used as is. "Entered" means not nil: a part
// entered as 0 is a part (a setup time of 0 adds 0 to the process time but
// still makes the parts the source of the process time). In the result,
// every value the model computed is set; values that are neither entered
// nor computable stay nil. Compare with the input to tell entered from
// derived values. The result shares no pointers with params (see
// ProcessParameters.Clone). In order:
//
//   - transport_time = transport_repetition_time × transport_repetition_count (A.10)
//   - waiting_time and range_of_inventory = stock_quantity ÷ customer demand
//     rate: the standard VSM inventory time (Rother & Shook: inventory ÷
//     daily demand), as calendar time in params' unit through
//     Customer.DemandPeriod and Customer.WorkingDaysPerWeek. A stock of 0
//     gives 0. Only in a stream whose customer has a Demand above 0 and a
//     day or week DemandPeriod (see CalculateCustomerInventoryTime);
//     otherwise the stored values are used and not checked.
//   - processing_time = cycle_time × order_quantity ÷ resource_count (A.5),
//     when all three are entered and resource_count is above 0
//   - process_time = processing + setup + handling + waiting (process) +
//     ancillary time (A.2): when any part is entered, the sum of the
//     entered parts, and a stored process_time that differs in either
//     direction is reported. When repetition data (repetition_time and
//     repetition_count) is entered, A.6 (setup + repetition_time ×
//     repetition_count) gives process_time instead, and entered A.2 parts
//     beyond setup_time that add up to something else are reported.
//   - idle_time = transport + waiting + storage time, when any of them is
//     entered (or computed)
//   - value_adding_time = value_adding_time_process when that is entered
//     (A.9); value adding time above process time, or above lead time when
//     process time is not known, is impossible and reported (flow
//     efficiency is then unknown, see CalculateKPIs)
//   - lead_time = process_time + idle_time when either is known; else the
//     value classification VAT + NVAT + NNVAT (A.3) when any of them is
//     entered; else the stored lead_time
//   - yield_factor = 1 − (scrap_rate + rework_rate) (A.17) when either rate
//     is entered (a rate entered as 0 counts: scrap 0 and nothing else
//     gives yield 1); when the sum of scrap_rate and rework_rate is above 1
//     (impossible), yield_factor is left as stored, the process's yield
//     counts as unknown and the contradiction is reported. A stored
//     yield_factor (0 included) is compared. Bounds like this one (a share
//     at most 1, a part at most its whole) allow only float rounding, not
//     ConsistencyTolerance.
func (c *Calculator) ResolveParameters(params ProcessParameters) ProcessParameters {
	p, _ := c.resolveParameters(params, streamContext{})
	return p.Clone()
}

// ResolveParametersInStream is ResolveParameters for a process of vs: it
// also derives waiting_time and range_of_inventory from stock_quantity and
// the customer's demand rate (see ResolveParameters). A process without a
// TimeUnit is taken to be in vs's reference unit. The result shares no
// pointers with params (see ProcessParameters.Clone).
func (c *Calculator) ResolveParametersInStream(params ProcessParameters, vs *ValueStream) ProcessParameters {
	p, _ := c.resolveParameters(params, c.contextOf(vs))
	return p.Clone()
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
// to the reference unit. ok is false when params' time unit is not
// supported (see isKnownTimeUnit): its times cannot be converted, and are
// returned unconverted, so they must not be added to or compared with
// other processes' times. An empty unit is the reference unit.
func (c *Calculator) resolvedIn(params ProcessParameters, ctx streamContext) (p ProcessParameters, ok bool) {
	p, _ = c.resolveParameters(params, ctx)
	if !unitSupported(params.TimeUnit) {
		return p, false
	}
	return c.normalizeTimes(p, ctx.unit), true
}

// unitSupported reports whether u is a time unit the model can convert, or
// empty (taken to be the reference unit).
func unitSupported(u TimeUnit) bool {
	return u == "" || isKnownTimeUnit(u)
}

// resolveParameters implements ResolveParameters and also returns the
// disagreements it found (Element and ID left empty).
//
// It never writes through a pointer field of in (the result shares the
// pointers of the values it did not change); computed values get new
// pointers.
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
	// derive sets *field to computed, reporting an entered value that
	// disagrees.
	derive := func(name string, field **float64, computed float64, rule string) {
		if *field != nil && !consistent(**field, computed) {
			report(name, **field, computed, rule, replaced)
		}
		*field = F(computed)
	}

	if !unitSupported(in.TimeUnit) {
		issues = append(issues, ConsistencyIssue{
			Field: "time_unit", Unit: unit, Rule: ruleTimeUnit,
			Message: fmt.Sprintf("time_unit %q is not supported (use seconds, minutes, hours, days or weeks); "+
				"this process's times cannot be converted, so the stream totals and KPIs that need them are "+
				"not computed", unit),
		})
	}
	if p.TransportRepetitionTime != nil && p.TransportRepetitionCount != nil {
		derive("transport_time", &p.TransportTime,
			*p.TransportRepetitionTime*float64(*p.TransportRepetitionCount), ruleTransport)
	}
	if rate, ok := ctx.demandRate(in.TimeUnit); ok && p.StockQuantity != nil {
		derive("waiting_time", &p.WaitingTime, *p.StockQuantity/rate, ruleWaiting)
		derive("range_of_inventory", &p.RangeOfInventory, *p.StockQuantity/rate, ruleRange)
	}
	if processingFormula(p) {
		derive("processing_time", &p.ProcessingTime,
			*p.CycleTime**p.OrderQuantity/float64(*p.ResourceCount), ruleProcessing)
	}

	// Process time (A.2, A.6).
	parts := c.CalculateProcessTime(p)
	given := processPartsGiven(p)
	if p.RepetitionTime != nil && p.RepetitionCount != nil {
		// A.6 gives process time. Setup time is in both A.6 and A.2, so the
		// A.2 parts are a second, complete breakdown only when a part other
		// than setup is entered; then they must add up to the same value.
		a6 := c.CalculateRepetitiveProcessTime(num(p.SetupTime), *p.RepetitionTime, *p.RepetitionCount)
		if p.ProcessTime != nil && !consistent(*p.ProcessTime, a6) {
			report("process_time", *p.ProcessTime, a6, ruleRepetitive, replaced)
		}
		beyondSetup := p.ProcessingTime != nil || p.HandlingTime != nil ||
			p.WaitingTimeProcess != nil || p.AncillaryTime != nil
		if beyondSetup && !consistent(parts, a6) {
			issues = append(issues, ConsistencyIssue{
				Field: fieldProcessParts, StoredValue: parts, ComputedValue: a6, Unit: unit, Rule: ruleRepetitive,
				Message: fmt.Sprintf("process time parts (%s) sum to %g, but repetition data gives "+
					"setup_time + repetition_time × repetition_count = %g; correct the parts or the "+
					"repetition data; calculations use %[3]g", strings.Join(given, " + "), parts, a6),
			})
		}
		p.ProcessTime = F(a6)
	} else if len(given) > 0 {
		if p.ProcessTime != nil && !consistent(*p.ProcessTime, parts) {
			fix := "correct the parts or the stored value"
			if parts < *p.ProcessTime {
				fix = "enter the missing parts, or clear the given parts to use the stored total"
			}
			report("process_time", *p.ProcessTime, parts, ruleProcessParts,
				"%s: stored %g, but its parts sum to %g (only "+strings.Join(given, " + ")+
					" given) — "+fix+"; calculations use %[3]g")
		}
		p.ProcessTime = F(parts)
	}

	// Idle time.
	if idlePartsGiven(p) {
		derive("idle_time", &p.IdleTime,
			c.CalculateTotalIdleTime(num(p.TransportTime), num(p.WaitingTime), num(p.StorageTime)), ruleIdle)
	}

	// Value adding time (VATp is the per-process value of A.9).
	if p.ValueAddingTimeProcess != nil {
		derive("value_adding_time", &p.ValueAddingTime, *p.ValueAddingTimeProcess, ruleVATp)
	}
	p.ValueAddingTimeProcess = p.ValueAddingTime
	vatAbovePT := p.ProcessTime != nil && p.ValueAddingTime != nil && exceeds(*p.ValueAddingTime, *p.ProcessTime)
	if vatAbovePT {
		report("value_adding_time", *p.ValueAddingTime, *p.ProcessTime, ruleVATWithinPT,
			"%s: %g exceeds process_time %g; flow efficiency is treated as unknown")
	}

	// Lead time: flow (PT + IT) first, else the value classification.
	flowKnown := p.ProcessTime != nil || p.IdleTime != nil
	flow := num(p.ProcessTime) + num(p.IdleTime)
	classified := p.NonValueAddingTime != nil || p.NecessaryNonValueAddingTime != nil
	class := c.CalculateLeadTime(p)
	lt, ltRule := p.LeadTime, ruleLeadFlow
	switch {
	case flowKnown:
		lt = F(flow)
		if classified && !consistent(class, flow) {
			report(fieldClassSum, class, flow, ruleLeadClass,
				"%s: %g, but process time + idle time = %g; calculations use %[3]g")
		}
	case classified || p.ValueAddingTime != nil:
		lt, ltRule = F(class), ruleLeadClass
	}
	if p.LeadTime != nil && lt != nil && !consistent(*p.LeadTime, *lt) {
		report("lead_time", *p.LeadTime, *lt, ltRule, replaced)
	}
	p.LeadTime = lt
	// Value adding time is part of the lead time too; this catches it when
	// process_time is not known (otherwise the check above already has).
	if !vatAbovePT && p.ValueAddingTime != nil && p.LeadTime != nil && exceeds(*p.ValueAddingTime, *p.LeadTime) {
		report("value_adding_time", *p.ValueAddingTime, *p.LeadTime, ruleVATWithinLT,
			"%s: %g exceeds lead_time %g; flow efficiency is treated as unknown")
	}

	// Yield. Scrap + rework above 1 is impossible: the yield is not derived
	// from them (it would be negative), the process's yield counts as
	// unknown (see yieldKnown), and the contradiction is reported.
	if defects := num(p.ScrapRate) + num(p.ReworkRate); defectsImpossible(p) {
		issues = append(issues, ConsistencyIssue{
			Field: fieldDefects, StoredValue: defects, ComputedValue: 1, Rule: ruleDefects,
			Message: fmt.Sprintf("%s = %g is above 1 (more than every part defective); "+
				"the yield of this process is treated as unknown", fieldDefects, defects),
		})
	} else if p.ScrapRate != nil || p.ReworkRate != nil {
		computed := c.CalculateYieldFactor(num(p.ScrapRate), num(p.ReworkRate))
		if p.YieldFactor != nil && !consistent(*p.YieldFactor, computed) {
			report("yield_factor", *p.YieldFactor, computed, ruleYield, replaced)
		}
		p.YieldFactor = F(computed)
	}

	// Rates are fractions, not times.
	for i := range issues {
		if issues[i].Field == "yield_factor" {
			issues[i].Unit = ""
		}
	}
	return p, issues
}

// processingFormula reports whether processing_time is computed from
// cycle_time × order_quantity ÷ resource_count (A.5): all three entered and
// resource_count above 0 (it divides).
func processingFormula(p ProcessParameters) bool {
	return p.CycleTime != nil && p.OrderQuantity != nil && p.ResourceCount != nil && *p.ResourceCount > 0
}

// idlePartsGiven reports whether any part of idle time (transport, waiting
// or storage time) is entered or computed.
func idlePartsGiven(p ProcessParameters) bool {
	return p.TransportTime != nil || p.WaitingTime != nil || p.StorageTime != nil
}

// inventoryRange returns the time inv's quantity lasts at the customer's
// demand rate, in the reference unit, and false when the rate is unknown or
// the quantity is not entered. A quantity of 0 lasts 0.
func (s streamContext) inventoryRange(inv Inventory) (float64, bool) {
	rate, ok := s.demandRate(s.unit)
	if !ok || inv.Quantity == nil {
		return 0, false
	}
	return *inv.Quantity / rate, true
}

// stockHolder returns the process whose stock_quantity inv is, when inv
// names one with process_id and that process has stock_quantity entered
// (0 included). inv is then counted through the process, not on its own.
func stockHolder(vs *ValueStream, inv Inventory) (*Process, bool) {
	if inv.ProcessID == "" {
		return nil, false
	}
	for i := range vs.Processes {
		if p := &vs.Processes[i]; p.ID == inv.ProcessID {
			return p, p.Parameters.StockQuantity != nil
		}
	}
	return nil, false
}

// timeThroughProcess reports whether inv's time is already in the lead time
// through holder, the process whose stock inv is (see stockHolder): the
// process's resolved waiting_time is known (from stock ÷ demand rate, or
// stored), or its idle_time is entered only as a total, which is taken to
// include the wait. Otherwise, for example stock_quantity without a demand
// rate and no waiting_time, the process does not account for the time, and
// the inventory's own time (its range) is the only measure of it: it is
// then counted on its own. Its stock is still counted once, through the
// process (see Calculator.figures).
func (c *Calculator) timeThroughProcess(holder *Process, ctx streamContext) bool {
	p, _ := c.resolveParameters(holder.Parameters, ctx)
	idleTotalOnly := !idlePartsGiven(p) && p.IdleTime != nil
	return p.WaitingTime != nil || idleTotalOnly
}

// processPartsGiven lists the A.2 process time parts that are entered (or
// computed, for processing_time), 0 included.
func processPartsGiven(p ProcessParameters) []string {
	var given []string
	for _, f := range []struct {
		name  string
		value *float64
	}{
		{"processing_time", p.ProcessingTime},
		{"setup_time", p.SetupTime},
		{"handling_time", p.HandlingTime},
		{"waiting_time_process", p.WaitingTimeProcess},
		{"ancillary_time", p.AncillaryTime},
	} {
		if f.value != nil {
			given = append(given, f.name)
		}
	}
	return given
}

// customerTakt returns the takt the model uses: AvailableTime ÷ Demand when
// both are entered and demand is above 0 (both per DemandPeriod, so the
// period cancels), otherwise the stored TaktTime when entered.
func (c *Calculator) customerTakt(cust *Customer) (float64, bool) {
	switch {
	case cust == nil:
		return 0, false
	case cust.AvailableTime != nil && positive(cust.Demand):
		return c.CalculateCustomerTakt(*cust.AvailableTime, *cust.Demand), true
	case cust.TaktTime != nil:
		return *cust.TaktTime, true
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
// makes the disagreements visible. These issues have Kind
// IssueContradiction, as do impossible or unusable values (scrap + rework
// above 1, working_days_per_week outside 1–7, an unsupported time_unit).
//
// After them come the issues of Kind IssueMissingData: every element on the
// map whose part of a stream total cannot be determined (a process whose
// lead time, process time or value adding time is unknown, stock that
// waits for an unknown time, an inventory whose time or stock is unknown).
// The totals that need such a value, and every KPI built on them, are
// omitted (see Calculator.CalculateKPIs), never counted as 0.
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
		if want, ok := ctx.inventoryRange(inv); ok && inv.Range != nil && !consistent(*inv.Range, want) {
			issues = append(issues, ConsistencyIssue{
				Element: "inventory", ID: inv.ID, Field: "range",
				StoredValue: *inv.Range, ComputedValue: want, Unit: unit, Rule: ruleInvRange,
				Message: fmt.Sprintf("inventory %s: range stored %g, but quantity %g ÷ customer demand rate = %g",
					inv.ID, *inv.Range, *inv.Quantity, want),
			})
		}
	}

	rangeChecked := func(inv Inventory) bool {
		_, ok := ctx.inventoryRange(inv)
		return ok
	}
	for _, inv := range vs.Inventory {
		if inv.ProcessID == "" {
			continue
		}
		holder, linked := stockHolder(vs, inv)
		issue := ConsistencyIssue{Element: "inventory", ID: inv.ID, Rule: ruleInvLink}
		switch {
		case holder == nil:
			issue.Field = "process_id"
			issue.Message = fmt.Sprintf("inventory %s: process_id %q names no process; the inventory is counted on its own",
				inv.ID, inv.ProcessID)
		case !linked && num(inv.Quantity) == 0 && num(inv.Range) == 0:
			// Empty (0) or nothing entered: nothing contradicts the link
			// (an inventory with nothing entered is reported as missing
			// data, see Calculator.figures).
			continue
		case !linked:
			issue.Field = "process_id"
			issue.Message = fmt.Sprintf("inventory %s: linked to process %s, which has no stock_quantity; "+
				"the inventory is counted on its own", inv.ID, inv.ProcessID)
		case inv.Quantity != nil && !consistent(*inv.Quantity, *holder.Parameters.StockQuantity):
			stock := *holder.Parameters.StockQuantity
			issue.Field, issue.StoredValue, issue.ComputedValue = "quantity", *inv.Quantity, stock
			issue.Message = fmt.Sprintf("inventory %s: quantity %g, but it is the stock of process %s, "+
				"whose stock_quantity is %g; calculations count the process's stock", inv.ID, *inv.Quantity,
				inv.ProcessID, stock)
		case inv.Range != nil && !rangeChecked(inv) && c.timeThroughProcess(holder, ctx):
			// The range is not checked against quantity ÷ demand rate, so
			// check it against the process's waiting time, the same time.
			p, ok := c.resolvedIn(holder.Parameters, ctx)
			switch {
			case !ok:
				continue
			case p.WaitingTime != nil:
				if consistent(*inv.Range, *p.WaitingTime) {
					continue
				}
				issue.Field, issue.StoredValue, issue.ComputedValue, issue.Unit = "range", *inv.Range, *p.WaitingTime, unit
				issue.Message = fmt.Sprintf("inventory %s: range %g, but it is the stock of process %s, "+
					"whose waiting_time is %g; calculations count the process's waiting time", inv.ID, *inv.Range,
					inv.ProcessID, *p.WaitingTime)
			default:
				continue
			}
		default:
			continue
		}
		issues = append(issues, issue)
	}

	if cust := vs.Customer; cust != nil && !workingDaysValid(cust.WorkingDaysPerWeek) {
		issues = append(issues, ConsistencyIssue{
			Element: "customer", ID: cust.ID, Field: "working_days_per_week",
			StoredValue: *cust.WorkingDaysPerWeek, ComputedValue: 7, Rule: ruleWorkingDays,
			Message: fmt.Sprintf("customer %s: working_days_per_week %g is outside 1–7; "+
				"inventory times are not derived from demand", cust.ID, *cust.WorkingDaysPerWeek),
		})
	}

	if cust := vs.Customer; cust != nil && cust.TaktTime != nil && cust.AvailableTime != nil && positive(cust.Demand) {
		if takt, _ := c.customerTakt(cust); !consistent(*cust.TaktTime, takt) {
			issues = append(issues, ConsistencyIssue{
				Element: "customer", ID: cust.ID, Field: "takt_time",
				StoredValue: *cust.TaktTime, ComputedValue: takt, Unit: unit, Rule: ruleTakt,
				Message: fmt.Sprintf("customer %s: takt_time stored %g, but available_time ÷ demand = %g; calculations use %[3]g",
					cust.ID, *cust.TaktTime, takt),
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
	for i := range issues {
		issues[i].Kind = IssueContradiction
	}
	return append(issues, c.figures(vs).missing...)
}
