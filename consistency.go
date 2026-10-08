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
	ruleInvWithinIdle = "an inventory with process_id waits within that process's idle_time, so its range cannot exceed it"
	ruleInvWithinLead = "an inventory with process_id waits within that process's lead_time, so its range cannot exceed it"
	ruleInvLinkOnce   = "a process's stock_quantity is one inventory: at most one inventory links to a process (process_id)"
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
// demand rate: a stock_quantity above 0 then waits for a time it cannot
// compute (see below). Use ResolveParametersInStream to also derive
// waiting_time and range_of_inventory from stock_quantity; every
// value-stream calculation (CalculateKPIs, CheckConsistency, the analyzer
// and designer) does.
//
// # The blank-parts rule ("blank = none, once started")
//
// One rule decides every total that is defined by parts: process_time
// (A.2: processing + setup + handling + waiting (process) + ancillary
// time; A.6: setup + repetition_time × repetition_count), idle_time
// (transport + waiting + storage time), a process's lead_time (process
// time + idle time; or the value classification VAT + NVAT + NNVAT, A.3)
// and yield_factor (1 − (scrap_rate + rework_rate), A.17). It is
// implemented once, by sumParts and resolution.total:
//
//   - Once any part is entered (not nil; 0 is entered), the breakdown is
//     complete: a part left blank (nil) counts as 0, and the total is the
//     sum. A stored total that differs is a contradiction (reported, and
//     calculations use the sum). A process time entered with idle time
//     blank has lead time = process time (idle 0), and the reverse; scrap 0
//     entered with rework blank gives yield 1. A blank process_time,
//     idle_time or value_adding_time that counts as 0 is set to 0 in the
//     result (they are read on their own, e.g. in the stream totals); other
//     blank parts stay nil and add 0 wherever they are summed.
//   - When no part is entered, the stored total is used as is; when it is
//     blank too, the value is not known (nil). A stream total that needs
//     it is then unknown (missing data, see CheckConsistency).
//   - A part that is unknown is not blank: an impossible entered value
//     (below), a value computed from unknown or unusable inputs (processing
//     time with resource_count 0), or the waiting time of stock_quantity
//     above 0 when it cannot be derived (no customer demand rate and no
//     waiting_time entered; the stock waits, for an unknown time). The
//     parts then cannot give the total: a stored total is used unchecked
//     (it is the only measure, and includes the unknown part), otherwise
//     the total is unknown (nil, never counted as 0).
//
// Two details keep the breakdowns apart. value_adding_time is entered for
// its own sake (it is the value adding part of process time, A.9), so on
// its own it does not start the VAT + NVAT + NNVAT lead-time
// classification: maps commonly record VAT as a share of process time
// without splitting the rest into NVAT and NNVAT, and treating VAT alone as
// a complete classification (blank NVAT and NNVAT = 0, lead time = VAT)
// would make every such map contradict its flow lead time (process + idle
// time). The classification starts when non_value_adding_time or
// necessary_non_value_adding_time is entered, and then a blank
// value_adding_time is 0. Setup time is in both A.6 and A.2, so A.2
// is a second breakdown of an A.6 process time only when a part other
// than setup is entered. Formulas that multiply or divide (transport_time
// = transport_repetition_time × transport_repetition_count, processing
// time, waiting time from stock) are not breakdowns: they need every
// input.
//
// # Impossible values
//
// Times, quantities and counts cannot be negative, and scrap_rate,
// rework_rate and yield_factor are fractions between 0 and 1 (bounds allow
// only float rounding, boundEpsilon, not ConsistencyTolerance). An entered
// value outside its bounds is reported as a contradiction and is unknown:
// it is nil in the result, and so is everything computed from it unless a
// stored total stands in (above). Scrap + rework above 1 makes the yield
// unknown (the rates themselves are fractions, so the parts decide, and a
// stored yield_factor is not used).
//
// # In order
//
//   - transport_time = transport_repetition_time × transport_repetition_count (A.10)
//   - waiting_time and range_of_inventory = stock_quantity ÷ customer demand
//     rate: the standard VSM inventory time (Rother & Shook: inventory ÷
//     daily demand), as calendar time in params' unit through
//     Customer.DemandPeriod and Customer.WorkingDaysPerWeek, in a stream
//     whose customer has a Demand above 0 and a day or week DemandPeriod
//     (see CalculateCustomerInventoryTime). A stock of 0 gives 0 with or
//     without a demand rate. Stock above 0 without a demand rate: an
//     entered waiting_time is used as is, otherwise the waiting time is
//     unknown.
//   - processing_time = cycle_time × order_quantity ÷ resource_count (A.5),
//     when cycle_time and order_quantity are entered. A resource_count
//     not entered counts as 1 (a process runs on at least one resource);
//     entered as 0 there is no resource, so the processing time is
//     unknown, and a resource_count of 0 is a contradiction when the
//     process has an order to process or the customer has demand.
//   - process_time from its A.2 parts, or from A.6 when repetition data
//     (repetition_time and repetition_count) is entered; A.2 parts beyond
//     setup_time that add up to something else are reported.
//   - idle_time from transport, waiting and storage time.
//   - value_adding_time = value_adding_time_process when that is entered
//     (A.9); value adding time above process time, or above lead time when
//     process time is not known, is impossible and reported (flow
//     efficiency is then unknown, see CalculateKPIs).
//   - lead_time = process_time + idle_time; when that cannot be known, the
//     value classification VAT + NVAT + NNVAT (A.3) once started; else the
//     stored lead_time. When both are known they must agree.
//   - yield_factor = 1 − (scrap_rate + rework_rate) (A.17).
//
// In the result, every value the model computed is set; values that are
// neither entered nor computable stay nil. Compare with the input to tell
// entered from derived values. The result shares no pointers with params
// (see ProcessParameters.Clone).
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
func (c *Calculator) resolveParameters(in ProcessParameters, ctx streamContext) (ProcessParameters, []ConsistencyIssue) {
	r := c.resolve(in, ctx)
	return r.params, r.issues
}

// resolution is one process resolved (see resolve).
type resolution struct {
	params ProcessParameters
	issues []ConsistencyIssue
	// unknown names (JSON field names) the values that cannot be known: an
	// impossible entered value, or a value computed from unknown or
	// unusable inputs. They are nil in params but, unlike a blank value,
	// never count as 0 (see the blank-parts rule at ResolveParameters).
	unknown map[string]bool
	// unknownParts maps each total whose parts include an unknown value to
	// those parts. The parts cannot give the total: it is the stored value,
	// used unchecked, or unknown.
	unknownParts map[string][]string
}

// part is one part of a total, for sumParts.
type part struct {
	name    string
	value   *float64
	unknown bool // has a value that cannot be known (not blank)
}

// sumParts is the blank-parts rule for the parts of one total (see
// ResolveParameters): started is true when any part is entered or
// unknown, known is true when started and no part is unknown (the parts
// give the total), and sum adds the entered parts (a blank part adds 0).
func sumParts(parts []part) (sum float64, started, known bool) {
	known = true
	for _, pt := range parts {
		switch {
		case pt.unknown:
			started, known = true, false
		case pt.value != nil:
			started = true
			sum += *pt.value
		}
	}
	return sum, started, started && known
}

// unknownNames lists the unknown parts.
func unknownNames(parts []part) []string {
	var names []string
	for _, pt := range parts {
		if pt.unknown {
			names = append(names, pt.name)
		}
	}
	return names
}

// part returns the part for field name of r.params.
func (r *resolution) part(name string, value *float64) part {
	return part{name: name, value: value, unknown: r.unknown[name]}
}

// total applies the blank-parts rule to *field, the total of parts, and
// reports whether the parts gave it: then *field is their sum, and check
// (when not nil) is called first with a stored value. When a part is
// unknown, a stored value is kept unchecked and a blank one is unknown;
// with no part started, *field is kept as is.
func (r *resolution) total(name string, field **float64, parts []part, check func(stored, sum float64)) bool {
	sum, started, known := sumParts(parts)
	switch {
	case !started:
		return false
	case !known:
		r.unknownParts[name] = unknownNames(parts)
		if *field == nil {
			r.unknown[name] = true
		}
		return false
	}
	if *field != nil && check != nil {
		check(**field, sum)
	}
	*field = F(sum)
	return true
}

// Bounds of entered values (see "Impossible values" at ResolveParameters).
const (
	ruleNonNegative = "times, quantities and counts cannot be negative"
	ruleFraction    = "scrap_rate, rework_rate and yield_factor are fractions between 0 and 1"
	ruleResources   = "resource_count is the number of parallel resources: with 0 nothing is processed (processing_time = cycle_time × order_quantity ÷ resource_count, A.5; capacity = available time × resource_count ÷ cycle_time)"
)

// boundedFloat is an optional number of ProcessParameters with its bounds.
type boundedFloat struct {
	name     string
	value    func(*ProcessParameters) **float64
	time     bool // in the process's time unit
	fraction bool // at most 1
}

// boundedFloats are every optional number of ProcessParameters: all are at
// least 0, the rates at most 1.
var boundedFloats = func() []boundedFloat {
	var out []boundedFloat
	for _, f := range []struct {
		name string
		get  func(*ProcessParameters) **float64
	}{
		{"process_time", func(p *ProcessParameters) **float64 { return &p.ProcessTime }},
		{"cycle_time", func(p *ProcessParameters) **float64 { return &p.CycleTime }},
		{"setup_time", func(p *ProcessParameters) **float64 { return &p.SetupTime }},
		{"downtime", func(p *ProcessParameters) **float64 { return &p.Downtime }},
		{"idle_time", func(p *ProcessParameters) **float64 { return &p.IdleTime }},
		{"waiting_time", func(p *ProcessParameters) **float64 { return &p.WaitingTime }},
		{"waiting_time_process", func(p *ProcessParameters) **float64 { return &p.WaitingTimeProcess }},
		{"transport_time", func(p *ProcessParameters) **float64 { return &p.TransportTime }},
		{"transport_repetition_time", func(p *ProcessParameters) **float64 { return &p.TransportRepetitionTime }},
		{"storage_time", func(p *ProcessParameters) **float64 { return &p.StorageTime }},
		{"handling_time", func(p *ProcessParameters) **float64 { return &p.HandlingTime }},
		{"ancillary_time", func(p *ProcessParameters) **float64 { return &p.AncillaryTime }},
		{"time_between_failures", func(p *ProcessParameters) **float64 { return &p.TimeBetweenFailures }},
		{"time_to_repair", func(p *ProcessParameters) **float64 { return &p.TimeToRepair }},
		{"repetition_time", func(p *ProcessParameters) **float64 { return &p.RepetitionTime }},
		{"value_adding_time", func(p *ProcessParameters) **float64 { return &p.ValueAddingTime }},
		{"non_value_adding_time", func(p *ProcessParameters) **float64 { return &p.NonValueAddingTime }},
		{"necessary_non_value_adding_time", func(p *ProcessParameters) **float64 { return &p.NecessaryNonValueAddingTime }},
		{"value_adding_time_process", func(p *ProcessParameters) **float64 { return &p.ValueAddingTimeProcess }},
		{"lead_time", func(p *ProcessParameters) **float64 { return &p.LeadTime }},
		{"operating_time", func(p *ProcessParameters) **float64 { return &p.OperatingTime }},
		{"processing_time", func(p *ProcessParameters) **float64 { return &p.ProcessingTime }},
		{"range_of_inventory", func(p *ProcessParameters) **float64 { return &p.RangeOfInventory }},
	} {
		out = append(out, boundedFloat{name: f.name, value: f.get, time: true})
	}
	for _, f := range []struct {
		name     string
		get      func(*ProcessParameters) **float64
		fraction bool
	}{
		{"customer_demand", func(p *ProcessParameters) **float64 { return &p.CustomerDemand }, false},
		{"order_quantity", func(p *ProcessParameters) **float64 { return &p.OrderQuantity }, false},
		{"stock_quantity", func(p *ProcessParameters) **float64 { return &p.StockQuantity }, false},
		{"employee_count", func(p *ProcessParameters) **float64 { return &p.EmployeeCount }, false},
		{"yield_factor", func(p *ProcessParameters) **float64 { return &p.YieldFactor }, true},
		{"scrap_rate", func(p *ProcessParameters) **float64 { return &p.ScrapRate }, true},
		{"rework_rate", func(p *ProcessParameters) **float64 { return &p.ReworkRate }, true},
	} {
		out = append(out, boundedFloat{name: f.name, value: f.get, fraction: f.fraction})
	}
	return out
}()

// boundedInts are the optional counts of ProcessParameters (at least 0).
var boundedInts = []struct {
	name  string
	value func(*ProcessParameters) **int
}{
	{"transport_repetition_count", func(p *ProcessParameters) **int { return &p.TransportRepetitionCount }},
	{"repetition_count", func(p *ProcessParameters) **int { return &p.RepetitionCount }},
	{"batch_size", func(p *ProcessParameters) **int { return &p.BatchSize }},
	{"resource_count", func(p *ProcessParameters) **int { return &p.ResourceCount }},
	{"failure_count", func(p *ProcessParameters) **int { return &p.FailureCount }},
}

// outOfBounds reports whether v is outside [0, 1] (fraction) or below 0,
// beyond float rounding (see exceeds).
func outOfBounds(v float64, fraction bool) bool {
	return exceeds(0, v) || (fraction && exceeds(v, 1)) || math.IsNaN(v) || math.IsInf(v, 0)
}

// impossibleFields lists the fields of p whose entered value is out of
// bounds (see "Impossible values" at ResolveParameters).
func impossibleFields(p ProcessParameters) []string {
	var names []string
	for _, b := range boundedFloats {
		if v := *b.value(&p); v != nil && outOfBounds(*v, b.fraction) {
			names = append(names, b.name)
		}
	}
	for _, b := range boundedInts {
		if v := *b.value(&p); v != nil && *v < 0 {
			names = append(names, b.name)
		}
	}
	return names
}

// resourceCount returns the number of parallel resources of a process:
// ResourceCount when entered, 1 when not (a process runs on at least one
// resource). ok is false when it is entered as 0 or less: with no
// resource, processing time (A.5) and capacity are not defined.
func resourceCount(p ProcessParameters) (float64, bool) {
	if p.ResourceCount == nil {
		return 1, true
	}
	return float64(*p.ResourceCount), *p.ResourceCount > 0
}

// resolve implements ResolveParameters.
//
// It never writes through a pointer field of in (the result shares the
// pointers of the values it did not change); computed values get new
// pointers.
func (c *Calculator) resolve(in ProcessParameters, ctx streamContext) resolution {
	r := resolution{params: in, unknown: map[string]bool{}, unknownParts: map[string][]string{}}
	p := &r.params
	unit := string(in.TimeUnit)
	report := func(field string, stored, computed float64, rule, format string) {
		r.issues = append(r.issues, ConsistencyIssue{
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
	// check reports a stored total that differs from its parts' sum.
	check := func(name, rule string) func(stored, sum float64) {
		return func(stored, sum float64) {
			if !consistent(stored, sum) {
				report(name, stored, sum, rule, replaced)
			}
		}
	}
	unk := func(names ...string) bool {
		for _, n := range names {
			if r.unknown[n] {
				return true
			}
		}
		return false
	}

	if !unitSupported(in.TimeUnit) {
		r.issues = append(r.issues, ConsistencyIssue{
			Field: "time_unit", Unit: unit, Rule: ruleTimeUnit,
			Message: fmt.Sprintf("time_unit %q is not supported (use seconds, minutes, hours, days or weeks); "+
				"this process's times cannot be converted, so the stream totals and KPIs that need them are "+
				"not computed", unit),
		})
	}

	// Impossible values are reported and unknown.
	for _, b := range boundedFloats {
		field := b.value(p)
		if *field == nil || !outOfBounds(**field, b.fraction) {
			continue
		}
		v, limit, bound, rule := **field, 0.0, "below 0", ruleNonNegative
		if b.fraction {
			rule = ruleFraction
			if v > 0 {
				limit, bound = 1, "above 1"
			}
		}
		issue := ConsistencyIssue{Field: b.name, StoredValue: v, ComputedValue: limit, Rule: rule,
			Message: fmt.Sprintf("%s: %g is impossible (%s); it is treated as unknown, and so is what is "+
				"computed from it", b.name, v, bound)}
		if b.time {
			issue.Unit = unit
		}
		r.issues = append(r.issues, issue)
		*field = nil
		r.unknown[b.name] = true
	}
	for _, b := range boundedInts {
		field := b.value(p)
		if *field == nil || **field >= 0 {
			continue
		}
		v := **field
		r.issues = append(r.issues, ConsistencyIssue{Field: b.name, StoredValue: float64(v), Rule: ruleNonNegative,
			Message: fmt.Sprintf("%s: %d is impossible (below 0); it is treated as unknown, and so is what is "+
				"computed from it", b.name, v)})
		*field = nil
		r.unknown[b.name] = true
	}

	// Transport time (A.10).
	switch {
	case p.TransportRepetitionTime != nil && p.TransportRepetitionCount != nil:
		derive("transport_time", &p.TransportTime,
			*p.TransportRepetitionTime*float64(*p.TransportRepetitionCount), ruleTransport)
	case p.TransportTime == nil && unk("transport_repetition_time", "transport_repetition_count"):
		r.unknown["transport_time"] = true
	}

	// Waiting time of stock.
	rate, rateKnown := ctx.demandRate(in.TimeUnit)
	switch {
	case p.StockQuantity != nil && (*p.StockQuantity == 0 || rateKnown):
		wait := 0.0
		if *p.StockQuantity != 0 {
			wait = *p.StockQuantity / rate
		}
		derive("waiting_time", &p.WaitingTime, wait, ruleWaiting)
		derive("range_of_inventory", &p.RangeOfInventory, wait, ruleRange)
	case p.WaitingTime == nil && (p.StockQuantity != nil || unk("stock_quantity")):
		// The stock waits, for a time nothing here gives.
		r.unknown["waiting_time"] = true
	}

	// Processing time (A.5).
	resources, resourcesOK := resourceCount(*p)
	ordered := (p.CycleTime != nil || unk("cycle_time")) && (p.OrderQuantity != nil || unk("order_quantity"))
	switch {
	case p.CycleTime != nil && p.OrderQuantity != nil && resourcesOK && !unk("resource_count"):
		derive("processing_time", &p.ProcessingTime, *p.CycleTime**p.OrderQuantity/resources, ruleProcessing)
	case ordered && p.ProcessingTime == nil:
		r.unknown["processing_time"] = true
	}
	if p.ResourceCount != nil && *p.ResourceCount == 0 {
		demand := ctx.customer != nil && positive(ctx.customer.Demand)
		if demand || (ordered && positive(p.OrderQuantity)) {
			r.issues = append(r.issues, ConsistencyIssue{Field: "resource_count", ComputedValue: 1, Rule: ruleResources,
				Message: "resource_count: 0, but the process has an order or customer demand to process; with " +
					"no resource its processing time, capacity and utilization are unknown"})
		}
	}

	// Process time (A.2, A.6).
	a2 := []part{
		r.part("processing_time", p.ProcessingTime),
		r.part("setup_time", p.SetupTime),
		r.part("handling_time", p.HandlingTime),
		r.part("waiting_time_process", p.WaitingTimeProcess),
		r.part("ancillary_time", p.AncillaryTime),
	}
	given := processPartsGiven(*p)
	if (p.RepetitionTime != nil || unk("repetition_time")) && (p.RepetitionCount != nil || unk("repetition_count")) {
		// A.6 gives process time. Setup time is in both A.6 and A.2, so the
		// A.2 parts are a second, complete breakdown only when a part other
		// than setup is entered; then they must add up to the same value.
		a6Parts := []part{r.part("setup_time", p.SetupTime), r.part("repetition_time", p.RepetitionTime)}
		if unk("repetition_count") {
			a6Parts = append(a6Parts, part{name: "repetition_count", unknown: true})
		}
		if _, _, known := sumParts(a6Parts); !known {
			r.unknownParts["process_time"] = unknownNames(a6Parts)
			if p.ProcessTime == nil {
				r.unknown["process_time"] = true
			}
		} else {
			a6 := c.CalculateRepetitiveProcessTime(num(p.SetupTime), *p.RepetitionTime, *p.RepetitionCount)
			if p.ProcessTime != nil && !consistent(*p.ProcessTime, a6) {
				report("process_time", *p.ProcessTime, a6, ruleRepetitive, replaced)
			}
			beyond := a2[:1:1]
			beyond = append(beyond, a2[2:]...)
			_, beyondSetup, _ := sumParts(beyond)
			if parts, _, known := sumParts(a2); beyondSetup && known && !consistent(parts, a6) {
				r.issues = append(r.issues, ConsistencyIssue{
					Field: fieldProcessParts, StoredValue: parts, ComputedValue: a6, Unit: unit, Rule: ruleRepetitive,
					Message: fmt.Sprintf("process time parts (%s) sum to %g, but repetition data gives "+
						"setup_time + repetition_time × repetition_count = %g; correct the parts or the "+
						"repetition data; calculations use %[3]g", strings.Join(given, " + "), parts, a6),
				})
			}
			p.ProcessTime = F(a6)
		}
	} else {
		r.total("process_time", &p.ProcessTime, a2, func(stored, parts float64) {
			if consistent(stored, parts) {
				return
			}
			fix := "correct the parts or the stored value"
			if parts < stored {
				fix = "enter the missing parts, or clear the given parts to use the stored total"
			}
			report("process_time", stored, parts, ruleProcessParts,
				"%s: stored %g, but its parts sum to %g (only "+strings.Join(given, " + ")+
					" given; blank parts count as 0) — "+fix+"; calculations use %[3]g")
		})
	}

	// Idle time.
	idle := []part{
		r.part("transport_time", p.TransportTime),
		r.part("waiting_time", p.WaitingTime),
		r.part("storage_time", p.StorageTime),
	}
	r.total("idle_time", &p.IdleTime, idle, check("idle_time", ruleIdle))

	// Value adding time (VATp is the per-process value of A.9). The value
	// classification starts with NVAT or NNVAT; then a blank VAT is 0.
	switch {
	case p.ValueAddingTimeProcess != nil:
		derive("value_adding_time", &p.ValueAddingTime, *p.ValueAddingTimeProcess, ruleVATp)
	case p.ValueAddingTime == nil && unk("value_adding_time_process"):
		r.unknown["value_adding_time"] = true
	}
	class := []part{
		r.part("value_adding_time", p.ValueAddingTime),
		r.part("non_value_adding_time", p.NonValueAddingTime),
		r.part("necessary_non_value_adding_time", p.NecessaryNonValueAddingTime),
	}
	_, classStarted, _ := sumParts(class[1:])
	if classStarted && p.ValueAddingTime == nil && !unk("value_adding_time") {
		p.ValueAddingTime = F(0)
		class[0].value = p.ValueAddingTime
	}
	p.ValueAddingTimeProcess = p.ValueAddingTime

	// Lead time: the flow (process time + idle time), else the value
	// classification, else the stored value. Once the flow is started, a
	// blank process or idle time is 0.
	flowParts := []part{r.part("process_time", p.ProcessTime), r.part("idle_time", p.IdleTime)}
	flow, flowStarted, flowKnown := sumParts(flowParts)
	ptBlank := false
	if flowStarted {
		if p.ProcessTime == nil && !unk("process_time") {
			p.ProcessTime, ptBlank = F(0), true
		}
		if p.IdleTime == nil && !unk("idle_time") {
			p.IdleTime = F(0)
		}
	}
	classSum, _, classKnown := sumParts(class)
	classified := classStarted && classKnown
	lt, ltRule, computed := p.LeadTime, ruleLeadFlow, true
	switch {
	case flowKnown:
		lt = F(flow)
		if classified && !consistent(classSum, flow) {
			report(fieldClassSum, classSum, flow, ruleLeadClass,
				"%s: %g, but process time + idle time = %g; calculations use %[3]g")
		}
	case classified:
		lt, ltRule = F(classSum), ruleLeadClass
	default:
		computed = false
		if flowStarted || classStarted {
			names := unknownNames(flowParts)
			if classStarted {
				names = append(names, unknownNames(class)...)
			}
			r.unknownParts["lead_time"] = names
			if p.LeadTime == nil {
				r.unknown["lead_time"] = true
			}
		}
	}
	if computed && p.LeadTime != nil && !consistent(*p.LeadTime, *lt) {
		report("lead_time", *p.LeadTime, *lt, ltRule, replaced)
	}
	p.LeadTime = lt

	// Value adding time is part of process time and of lead time.
	vatAbovePT := p.ProcessTime != nil && p.ValueAddingTime != nil && exceeds(*p.ValueAddingTime, *p.ProcessTime)
	if vatAbovePT {
		blank := ""
		if ptBlank {
			blank = " (process_time not entered: with idle_time entered it counts as 0; enter process_time)"
		}
		report("value_adding_time", *p.ValueAddingTime, *p.ProcessTime, ruleVATWithinPT,
			"%s: %g exceeds process_time %g"+blank+"; flow efficiency is treated as unknown")
	}
	if !vatAbovePT && p.ValueAddingTime != nil && p.LeadTime != nil && exceeds(*p.ValueAddingTime, *p.LeadTime) {
		report("value_adding_time", *p.ValueAddingTime, *p.LeadTime, ruleVATWithinLT,
			"%s: %g exceeds lead_time %g; flow efficiency is treated as unknown")
	}

	// Yield (A.17). Scrap + rework above 1 is impossible: the yield is
	// unknown (it would be negative), and the contradiction is reported.
	quality := []part{r.part("scrap_rate", p.ScrapRate), r.part("rework_rate", p.ReworkRate)}
	switch defects, started, known := sumParts(quality); {
	case !started:
	case !known:
		r.unknownParts["yield_factor"] = unknownNames(quality)
		if p.YieldFactor == nil {
			r.unknown["yield_factor"] = true
		}
	case exceeds(defects, 1):
		r.issues = append(r.issues, ConsistencyIssue{
			Field: fieldDefects, StoredValue: defects, ComputedValue: 1, Rule: ruleDefects,
			Message: fmt.Sprintf("%s = %g is above 1 (more than every part defective); "+
				"the yield of this process is treated as unknown", fieldDefects, defects),
		})
		p.YieldFactor = nil
		r.unknown["yield_factor"] = true
	default:
		computed := c.CalculateYieldFactor(num(p.ScrapRate), num(p.ReworkRate))
		if p.YieldFactor != nil && !consistent(*p.YieldFactor, computed) {
			report("yield_factor", *p.YieldFactor, computed, ruleYield, replaced)
		}
		p.YieldFactor = F(computed)
	}

	// Rates are fractions, not times.
	for i := range r.issues {
		if r.issues[i].Field == "yield_factor" {
			r.issues[i].Unit = ""
		}
	}
	return r
}

// processingFormula reports whether processing_time is computed from
// cycle_time × order_quantity ÷ resource_count (A.5): cycle_time and
// order_quantity entered, and resource_count not entered (1) or above 0.
func processingFormula(p ProcessParameters) bool {
	_, ok := resourceCount(p)
	return p.CycleTime != nil && p.OrderQuantity != nil && ok
}

// processingTime returns cycle_time × order_quantity ÷ resource_count
// (A.5); only call it when processingFormula(p).
func processingTime(p ProcessParameters) float64 {
	n, _ := resourceCount(p)
	return *p.CycleTime * *p.OrderQuantity / n
}

// idlePartsGiven reports whether any part of idle time (transport, waiting
// or storage time) is entered or computed.
func idlePartsGiven(p ProcessParameters) bool {
	return p.TransportTime != nil || p.WaitingTime != nil || p.StorageTime != nil
}

// inventoryRange returns the time inv's quantity lasts at the customer's
// demand rate, in the reference unit, and false when the quantity is not
// entered (or impossible, below 0) or, for a quantity above 0, the rate is
// unknown. A quantity of 0 lasts 0, with or without a demand rate.
func (s streamContext) inventoryRange(inv Inventory) (float64, bool) {
	if inv.Quantity == nil || *inv.Quantity < 0 {
		return 0, false
	}
	if *inv.Quantity == 0 {
		return 0, true
	}
	rate, ok := s.demandRate(s.unit)
	if !ok {
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
// entered), or a stored idle_time or lead_time stands in for the stock's
// unknown wait (see the blank-parts rule at ResolveParameters). Otherwise,
// for example stock_quantity without a demand rate and nothing else
// entered for its wait, the process does not account for the time, and the
// inventory's own time (its range) is the only measure of it: it is then
// counted on its own, and the process counts its other times with the
// wait left out (see Calculator.figures). Its stock is still counted once,
// through the process.
func (c *Calculator) timeThroughProcess(holder *Process, ctx streamContext) bool {
	p, _ := c.resolveParameters(holder.Parameters, ctx)
	return p.WaitingTime != nil || p.IdleTime != nil || p.LeadTime != nil
}

// linkedInventories returns, for each inventory of vs, whether it is the
// stock of a process (see stockHolder) and, for the second and later
// inventories linked to the same process, the ID of the first: a process's
// stock is one inventory, so only the first one counts (CheckConsistency
// reports the others).
func linkedInventories(vs *ValueStream) (linked []bool, duplicateOf []string) {
	linked, duplicateOf = make([]bool, len(vs.Inventory)), make([]string, len(vs.Inventory))
	first := map[string]string{}
	for i, inv := range vs.Inventory {
		holder, ok := stockHolder(vs, inv)
		if !ok {
			continue
		}
		linked[i] = true
		if id, seen := first[holder.ID]; seen {
			duplicateOf[i] = id
		} else {
			first[holder.ID] = inv.ID
		}
	}
	return linked, duplicateOf
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
	case nonNegative(cust.AvailableTime) != nil && positive(cust.Demand):
		return c.CalculateCustomerTakt(*cust.AvailableTime, *cust.Demand), true
	case cust.AvailableTime != nil && positive(cust.Demand):
		return 0, false // impossible available time (reported by CheckConsistency)
	case nonNegative(cust.TaktTime) != nil:
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
// IssueContradiction, as do impossible or unusable values (a negative
// time, quantity or count, a rate or yield outside 0–1, scrap + rework
// above 1, resource_count 0 for a process with demand or an order,
// working_days_per_week outside 1–7, a waste assumption or
// information_flow_quality outside 0–1, an unsupported time_unit, an
// inventory linked to a process whose idle or lead time is shorter than the
// inventory's range, a second inventory linked to the same process, a
// range stored for an inventory of quantity 0).
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
		for _, f := range []struct {
			name  string
			value *float64
			unit  string
		}{{"quantity", inv.Quantity, ""}, {"range", inv.Range, unit}} {
			if f.value != nil && nonNegative(f.value) == nil {
				issues = append(issues, ConsistencyIssue{
					Element: "inventory", ID: inv.ID, Field: f.name, StoredValue: *f.value, Unit: f.unit,
					Rule: ruleNonNegative,
					Message: fmt.Sprintf("inventory %s: %s %g is impossible (below 0); it is treated as unknown",
						inv.ID, f.name, *f.value),
				})
			}
		}
		want, ok := ctx.inventoryRange(inv)
		if !ok || nonNegative(inv.Range) == nil || consistent(*inv.Range, want) {
			continue
		}
		issue := ConsistencyIssue{
			Element: "inventory", ID: inv.ID, Field: "range",
			StoredValue: *inv.Range, ComputedValue: want, Unit: unit, Rule: ruleInvRange,
			Message: fmt.Sprintf("inventory %s: range stored %g, but quantity %g ÷ customer demand rate = %g",
				inv.ID, *inv.Range, *inv.Quantity, want),
		}
		if *inv.Quantity == 0 {
			issue.Message = fmt.Sprintf("inventory %s: range stored %g, but quantity is 0: an empty inventory "+
				"lasts 0; calculations use 0", inv.ID, *inv.Range)
		}
		issues = append(issues, issue)
	}

	rangeChecked := func(inv Inventory) bool {
		_, ok := ctx.inventoryRange(inv)
		return ok
	}
	_, duplicateOf := linkedInventories(vs)
	for i, inv := range vs.Inventory {
		if inv.ProcessID == "" {
			continue
		}
		holder, linked := stockHolder(vs, inv)
		issue := ConsistencyIssue{Element: "inventory", ID: inv.ID, Rule: ruleInvLink}
		switch {
		case duplicateOf[i] != "":
			issue.Field = "process_id"
			issue.Rule = ruleInvLinkOnce
			issue.Message = fmt.Sprintf("inventory %s: linked to process %s, whose stock is already inventory %s; "+
				"a process's stock is one inventory, so calculations count only %[3]s (remove one link, or "+
				"merge the two inventories)", inv.ID, inv.ProcessID, duplicateOf[i])
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
			// check it against the process's waiting time, the same time,
			// or, when only the idle total is given, against that total,
			// which includes the wait.
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
			case p.IdleTime != nil:
				if !exceeds(*inv.Range, *p.IdleTime) {
					continue
				}
				issue.Field, issue.StoredValue, issue.ComputedValue, issue.Unit = "range", *inv.Range, *p.IdleTime, unit
				issue.Rule = ruleInvWithinIdle
				issue.Message = fmt.Sprintf("inventory %s: range %g exceeds idle_time %g of process %s, whose "+
					"stock it is (the wait in that stock is part of the idle time); correct the range or the "+
					"idle time; calculations count the process's idle time", inv.ID, *inv.Range, *p.IdleTime,
					inv.ProcessID)
			case p.LeadTime != nil && exceeds(*inv.Range, *p.LeadTime):
				issue.Field, issue.StoredValue, issue.ComputedValue, issue.Unit = "range", *inv.Range, *p.LeadTime, unit
				issue.Rule = ruleInvWithinLead
				issue.Message = fmt.Sprintf("inventory %s: range %g exceeds lead_time %g of process %s, whose "+
					"stock it is (the wait in that stock is part of the lead time); correct the range or the "+
					"lead time; calculations count the process's lead time", inv.ID, *inv.Range, *p.LeadTime,
					inv.ProcessID)
			default:
				continue
			}
		default:
			continue
		}
		issues = append(issues, issue)
	}

	if cust := vs.Customer; cust != nil {
		for _, f := range []struct {
			name  string
			value *float64
			unit  string
		}{{"demand", cust.Demand, ""}, {"available_time", cust.AvailableTime, unit}, {"takt_time", cust.TaktTime, unit}} {
			if f.value != nil && nonNegative(f.value) == nil {
				issues = append(issues, ConsistencyIssue{
					Element: "customer", ID: cust.ID, Field: f.name, StoredValue: *f.value, Unit: f.unit,
					Rule: ruleNonNegative,
					Message: fmt.Sprintf("customer %s: %s %g is impossible (below 0); it is treated as unknown",
						cust.ID, f.name, *f.value),
				})
			}
		}
	}
	if cust := vs.Customer; cust != nil && !workingDaysValid(cust.WorkingDaysPerWeek) {
		issues = append(issues, ConsistencyIssue{
			Element: "customer", ID: cust.ID, Field: "working_days_per_week",
			StoredValue: *cust.WorkingDaysPerWeek, ComputedValue: 7, Rule: ruleWorkingDays,
			Message: fmt.Sprintf("customer %s: working_days_per_week %g is outside 1–7; "+
				"inventory times are not derived from demand", cust.ID, *cust.WorkingDaysPerWeek),
		})
	}

	if cust := vs.Customer; cust != nil && nonNegative(cust.TaktTime) != nil && nonNegative(cust.AvailableTime) != nil && positive(cust.Demand) {
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
	issues = append(issues, assumptionIssues(vs)...)
	for i := range issues {
		issues[i].Kind = IssueContradiction
	}
	return append(issues, c.figures(vs).missing...)
}
