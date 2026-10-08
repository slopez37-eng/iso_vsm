package vsm

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Target is a future-state target for one value. Set exactly one of Value
// (the absolute target) or ReductionPercent (cut from the current value, 0 to
// 100, e.g. 30 means 30% lower).
//
// Absolute times are in the process's own TimeUnit; absolute rates
// (scrap_rate, rework_rate, yield_factor) are fractions between 0 and 1.
type Target struct {
	Value            *float64 `json:"value,omitempty"`
	ReductionPercent *float64 `json:"reduction_percent,omitempty"`
}

// AbsoluteTarget returns a Target that sets the value to v.
func AbsoluteTarget(v float64) *Target { return &Target{Value: &v} }

// ReductionTarget returns a Target that cuts the current value by percent
// (0 to 100).
func ReductionTarget(percent float64) *Target { return &Target{ReductionPercent: &percent} }

// apply returns the target value for current (nil = not entered). An
// absolute target gives its value whether or not current is known. A
// reduction of a value that is not entered gives nil: there is nothing to
// reduce, and the value stays not entered.
func (t *Target) apply(current *float64) (*float64, error) {
	switch {
	case t.Value != nil && t.ReductionPercent != nil:
		return nil, fmt.Errorf("set either value or reduction_percent, not both")
	case t.Value != nil:
		if *t.Value < 0 || math.IsNaN(*t.Value) || math.IsInf(*t.Value, 0) {
			return nil, fmt.Errorf("value %v must be a finite number >= 0", *t.Value)
		}
		return F(*t.Value), nil
	case t.ReductionPercent != nil:
		p := *t.ReductionPercent
		if p < 0 || p > 100 || math.IsNaN(p) {
			return nil, fmt.Errorf("reduction_percent %v must be between 0 and 100", p)
		}
		if current == nil {
			return nil, nil
		}
		return F(*current * (1 - p/100)), nil
	default:
		return nil, fmt.Errorf("target has neither value nor reduction_percent")
	}
}

// ProcessTargets holds optional targets for one process's parameters. A nil
// field means "keep the current value". Targets apply to the current values
// as resolved by Calculator.ResolveParameters, and the future state is kept
// consistent by construction (CheckConsistency finds nothing new in it):
//
//   - Changing a part moves its total: setup_time is part of process_time;
//     transport_time, waiting_time and storage_time are the parts of
//     idle_time; scrap_rate and rework_rate set yield_factor (A.17).
//   - Values computed from other inputs follow them: a cycle_time target
//     changes processing_time (cycle_time × order_quantity ÷
//     resource_count, A.5) where that formula applies, and a stock_quantity
//     target changes waiting_time (stock ÷ customer demand rate, see
//     Calculator.CalculateCustomerInventoryTime) when the demand rate is known.
//   - A total target (process_time, idle_time, yield_factor) is met by
//     scaling, in proportion, the parts that have no target of their own and
//     are not computed from other inputs. A total of 0 zeroes its parts. Where
//     waiting_time is stock_quantity ÷ demand rate, it is scaled through
//     stock_quantity, and a waiting_time target sets stock_quantity.
//   - The value classification follows the lead time: when lead time
//     (process + idle time) changes, the change is taken from
//     non_value_adding_time, then necessary_non_value_adding_time, unless
//     they have their own targets; value_adding_time never changes. Targets
//     that leave VAT + NVAT + NNVAT different from the lead time are an error.
//   - A target of 0 is applied as 0, an entered value like any other: the
//     lead time becomes the resulting process + idle time, even when that is
//     0, and a yield_factor of 0 counts as a known yield. Process time can
//     never drop below the value adding time, so a target that would do so
//     (for example 0 with value adding time set) is an error.
//   - A reduction_percent target on a value that is not entered changes
//     nothing (there is nothing to reduce); an absolute target enters it.
//   - A target on a part of a total that the current state gives only as a
//     total (setup_time of a process with process_time alone, storage_time
//     of one with idle_time alone), even a target of 0, is an error: it
//     would start a breakdown in which the parts not entered count as 0
//     (the blank-parts rule, see ResolveParameters), and the breakdown
//     cannot be inferred from the total. Target the total, or enter the
//     parts first.
//   - A total whose parts include an unknown value (see ResolveParameters)
//     cannot follow targets: a target on it or on its other parts is an
//     error, unless the unknown part itself has a target. Impossible values
//     of the current state without a target are carried over unchanged.
//
// Targets that cannot be met consistently return an error naming the
// conflict, rather than producing a map that contradicts itself.
type ProcessTargets struct {
	ProcessTime                 *Target `json:"process_time,omitempty"`
	CycleTime                   *Target `json:"cycle_time,omitempty"`
	SetupTime                   *Target `json:"setup_time,omitempty"`
	IdleTime                    *Target `json:"idle_time,omitempty"`
	WaitingTime                 *Target `json:"waiting_time,omitempty"`
	TransportTime               *Target `json:"transport_time,omitempty"`
	StorageTime                 *Target `json:"storage_time,omitempty"`
	NonValueAddingTime          *Target `json:"non_value_adding_time,omitempty"`
	NecessaryNonValueAddingTime *Target `json:"necessary_non_value_adding_time,omitempty"`
	StockQuantity               *Target `json:"stock_quantity,omitempty"`
	ScrapRate                   *Target `json:"scrap_rate,omitempty"`
	ReworkRate                  *Target `json:"rework_rate,omitempty"`
	// YieldFactor accepts an absolute value only (a fraction 0 to 1).
	YieldFactor *Target `json:"yield_factor,omitempty"`
}

// InventoryTargets holds optional targets for one inventory point.
type InventoryTargets struct {
	Quantity *Target `json:"quantity,omitempty"`
	Range    *Target `json:"range,omitempty"`
}

// FutureStateTargets are the improvement targets a future state is designed
// from. Per-process (keyed by current process ID) and per-inventory (keyed
// by current inventory ID) targets override the stream-wide defaults field
// by field. Anything without a target keeps its current value: the library
// does not invent improvements.
type FutureStateTargets struct {
	Default          *ProcessTargets             `json:"default,omitempty"`
	Processes        map[string]ProcessTargets   `json:"processes,omitempty"`
	InventoryDefault *InventoryTargets           `json:"inventory_default,omitempty"`
	Inventory        map[string]InventoryTargets `json:"inventory,omitempty"`
}

// processField links one ProcessTargets field to its parameter.
type processField struct {
	name   string
	target func(*ProcessTargets) *Target
	value  func(*ProcessParameters) **float64
	isRate bool // fraction between 0 and 1
}

// directFields are the targets applied to one stored value each. The
// process_time, idle_time and yield_factor totals are handled separately in
// applyProcessTargets.
var directFields = []processField{
	{"cycle_time", func(t *ProcessTargets) *Target { return t.CycleTime }, func(p *ProcessParameters) **float64 { return &p.CycleTime }, false},
	{"setup_time", func(t *ProcessTargets) *Target { return t.SetupTime }, func(p *ProcessParameters) **float64 { return &p.SetupTime }, false},
	{"waiting_time", func(t *ProcessTargets) *Target { return t.WaitingTime }, func(p *ProcessParameters) **float64 { return &p.WaitingTime }, false},
	{"transport_time", func(t *ProcessTargets) *Target { return t.TransportTime }, func(p *ProcessParameters) **float64 { return &p.TransportTime }, false},
	{"storage_time", func(t *ProcessTargets) *Target { return t.StorageTime }, func(p *ProcessParameters) **float64 { return &p.StorageTime }, false},
	{"non_value_adding_time", func(t *ProcessTargets) *Target { return t.NonValueAddingTime }, func(p *ProcessParameters) **float64 { return &p.NonValueAddingTime }, false},
	{"necessary_non_value_adding_time", func(t *ProcessTargets) *Target { return t.NecessaryNonValueAddingTime }, func(p *ProcessParameters) **float64 { return &p.NecessaryNonValueAddingTime }, false},
	{"stock_quantity", func(t *ProcessTargets) *Target { return t.StockQuantity }, func(p *ProcessParameters) **float64 { return &p.StockQuantity }, false},
	{"scrap_rate", func(t *ProcessTargets) *Target { return t.ScrapRate }, func(p *ProcessParameters) **float64 { return &p.ScrapRate }, true},
	{"rework_rate", func(t *ProcessTargets) *Target { return t.ReworkRate }, func(p *ProcessParameters) **float64 { return &p.ReworkRate }, true},
}

// pick returns the per-process target if set, else the default.
func pick(own, def *ProcessTargets, get func(*ProcessTargets) *Target) *Target {
	if own != nil {
		if t := get(own); t != nil {
			return t
		}
	}
	if def != nil {
		return get(def)
	}
	return nil
}

// applyField applies a target to one value (nil = not entered), checking
// rate bounds. The result is nil only for a reduction of a value that is
// not entered.
func applyField(name string, t *Target, current *float64, isRate bool) (*float64, error) {
	v, err := t.apply(current)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if isRate && v != nil && *v > 1 {
		return nil, fmt.Errorf("%s: %v is above 1; rates are fractions (0.02 = 2%%)", name, *v)
	}
	return v, nil
}

// share is one part of a total that a total target may scale.
type share struct {
	value **float64
	fixed bool // has its own target, or is computed from other inputs
}

// scaleShares scales the non-fixed shares in proportion so that all shares
// add up to total. Shares that are not entered (nil) take no part and stay
// nil. It reports false when that is impossible (the fixed shares exceed
// total, or nothing is free to scale). Scaled shares get new pointers.
func scaleShares(total float64, shares []share) bool {
	var fixed, free float64
	for _, s := range shares {
		if *s.value == nil {
			continue
		}
		if s.fixed {
			fixed += **s.value
		} else {
			free += **s.value
		}
	}
	need := total - fixed
	if consistent(total, fixed) {
		need = 0
	}
	if need < 0 || (free == 0 && need != 0) {
		return false
	}
	k := 1.0
	if free > 0 {
		k = need / free
	}
	for _, s := range shares {
		if !s.fixed && *s.value != nil {
			*s.value = F(**s.value * k)
		}
	}
	return true
}

// hasIssue reports whether issues contain one for field.
func hasIssue(issues []ConsistencyIssue, field string) bool {
	for _, issue := range issues {
		if issue.Field == field {
			return true
		}
	}
	return false
}

// applyProcessTargets returns params with the targets applied (own overrides
// def field by field). See ProcessTargets for how the result is kept
// consistent. It never writes through a pointer field of params.
func (c *Calculator) applyProcessTargets(params ProcessParameters, own, def *ProcessTargets, ctx streamContext) (ProcessParameters, error) {
	cr := c.resolve(params, ctx)
	cur, curIssues := cr.params, cr.issues
	p := cur
	targeted := map[string]bool{}
	get := func(f func(*ProcessTargets) *Target) *Target { return pick(own, def, f) }
	// stillUnknown reports whether total has a part that is unknown in the
	// current state (see resolution.unknownParts) and has no target of its
	// own: the parts then cannot give the total, so the targets cannot
	// recompute it.
	stillUnknown := func(total string) bool {
		for _, name := range cr.unknownParts[total] {
			if !targeted[name] {
				return true
			}
		}
		return false
	}
	// cannotMove is the error for a target that would move a total whose
	// parts are not all known.
	cannotMove := func(total string) error {
		return fmt.Errorf("%s cannot be computed for this process: %s unknown in the current state (see "+
			"CheckConsistency), so a target on it or its parts cannot be applied consistently; enter or "+
			"correct those values (or target them) first", total, strings.Join(cr.unknownParts[total], ", "))
	}

	for _, f := range directFields {
		t := get(f.target)
		if t == nil {
			continue
		}
		v, err := applyField(f.name, t, *f.value(&cur), f.isRate)
		if err != nil {
			return params, err
		}
		if v == nil {
			continue // a reduction of a value not entered: nothing changes
		}
		*f.value(&p) = v
		targeted[f.name] = true
	}

	// Values computed from other inputs follow those inputs.
	if targeted["transport_time"] && cur.TransportRepetitionTime != nil && cur.TransportRepetitionCount != nil {
		return params, fmt.Errorf("transport_time: for this process it is transport_repetition_time × " +
			"transport_repetition_count (A.10); change those in the current state instead")
	}
	// Waiting time of stock is stock_quantity ÷ customer demand rate.
	rate, rateKnown := ctx.demandRate(params.TimeUnit)
	wasStock := rateKnown && cur.StockQuantity != nil
	if targeted["waiting_time"] && (wasStock || (rateKnown && targeted["stock_quantity"])) {
		if targeted["stock_quantity"] {
			return params, fmt.Errorf("waiting_time and stock_quantity: set one, not both; " +
				"waiting_time = stock_quantity ÷ customer demand rate")
		}
		p.StockQuantity = F(*p.WaitingTime * rate)
	}
	updateStock := func() {
		if rateKnown && p.StockQuantity != nil {
			p.WaitingTime = F(*p.StockQuantity / rate)
			p.RangeOfInventory = p.WaitingTime
		}
	}
	updateStock()
	if processingFormula(p) {
		p.ProcessingTime = F(processingTime(p))
	}

	// Process time: A.6 for repetitive processes, else the A.2 parts, else
	// the total alone.
	repetitive := cur.RepetitionTime != nil && cur.RepetitionCount != nil
	hasParts := len(processPartsGiven(cur)) > 0
	ptTarget := get(func(t *ProcessTargets) *Target { return t.ProcessTime })
	ptUnknown := stillUnknown("process_time")
	if ptUnknown {
		partTargeted := targeted["setup_time"] || targeted["cycle_time"]
		switch {
		case !hasParts && !partTargeted:
			// No part entered: a target on the total sets it, as for a
			// process time given only as a total.
			ptUnknown = ptTarget == nil
		case ptTarget != nil || (partTargeted && cur.ProcessTime != nil):
			return params, cannotMove("process_time")
		default:
			ptTarget = nil
		}
	}
	if t := ptTarget; t != nil {
		v, err := applyField("process_time", t, cur.ProcessTime, false)
		if err != nil {
			return params, err
		}
		if v != nil {
			targeted["process_time"] = true
			if repetitive {
				return params, fmt.Errorf("process_time: for this process it is setup_time + repetition_time × " +
					"repetition_count (A.6); target setup_time, or change the repetition data in the current state")
			}
			ok := scaleShares(*v, []share{
				{&p.ProcessingTime, processingFormula(p)},
				{&p.SetupTime, targeted["setup_time"]},
				{&p.HandlingTime, false},
				{&p.WaitingTimeProcess, false},
				{&p.AncillaryTime, false},
			})
			if !hasParts {
				p.ProcessTime = v
			} else if !ok {
				return params, fmt.Errorf("process_time: target %g cannot be met; the parts that are fixed "+
					"(targeted setup_time, or processing_time computed from cycle_time, A.5) add up to %g, "+
					"and parts entered as 0 cannot be scaled", *v, c.CalculateProcessTime(p))
			}
		}
	}
	switch {
	case ptUnknown:
		// Unknown parts: the process time stays as it is (stored, or
		// unknown).
	case repetitive:
		p.ProcessTime = F(c.CalculateRepetitiveProcessTime(num(p.SetupTime), *p.RepetitionTime, *p.RepetitionCount))
	case hasParts:
		p.ProcessTime = F(c.CalculateProcessTime(p))
	case len(processPartsGiven(p)) > 0 && params.ProcessTime != nil:
		// A part target (even 0) starts a breakdown, and blank parts are
		// then 0 (see the blank-parts rule at ResolveParameters), so the
		// stored total would be replaced by the parts.
		return params, fmt.Errorf("process_time is given only as a total for this process, so a target " +
			"that sets one of its parts (setup or processing time) would start a breakdown in which the " +
			"parts not entered count as 0, replacing the total; the breakdown cannot be inferred from the " +
			"total: target process_time instead, or enter the parts in the current state first")
	case len(processPartsGiven(p)) > 0:
		// A target entered the first part of a process time not known before.
		p.ProcessTime = F(c.CalculateProcessTime(p))
	}

	// Idle time: transport + waiting + storage time.
	idleParts := func(q *ProcessParameters) float64 {
		return c.CalculateTotalIdleTime(num(q.TransportTime), num(q.WaitingTime), num(q.StorageTime))
	}
	hasIdleParts := idlePartsGiven(cur) || idlePartsGiven(p)
	itTarget := get(func(t *ProcessTargets) *Target { return t.IdleTime })
	itUnknown := stillUnknown("idle_time")
	if itUnknown {
		partTargeted := targeted["transport_time"] || targeted["waiting_time"] || targeted["storage_time"]
		switch {
		case !idlePartsGiven(cur) && !partTargeted:
			// No part entered (only the unknown one): a target on the total
			// sets it, as for an idle time given only as a total.
			hasIdleParts = false
		case itTarget != nil || (partTargeted && cur.IdleTime != nil):
			return params, cannotMove("idle_time")
		default:
			itTarget, hasIdleParts = nil, false
		}
	}
	itTargeted := false
	if t := itTarget; t != nil {
		v, err := applyField("idle_time", t, cur.IdleTime, false)
		if err != nil {
			return params, err
		}
		switch {
		case v == nil:
		case !hasIdleParts:
			itTargeted = true
			targeted["idle_time"] = true
			p.IdleTime = v
		default:
			itTargeted = true
			targeted["idle_time"] = true
			stockDriven := rateKnown && p.StockQuantity != nil
			waiting := p.WaitingTime
			// Transport from repetitions (A.10) is fixed, except that an idle
			// time of 0 removes the transports too.
			repeated := cur.TransportRepetitionTime != nil && cur.TransportRepetitionCount != nil
			ok := scaleShares(*v, []share{
				{&p.TransportTime, targeted["transport_time"] || (repeated && *v != 0)},
				{&waiting, targeted["waiting_time"] || (stockDriven && targeted["stock_quantity"])},
				{&p.StorageTime, targeted["storage_time"]},
			})
			if !ok {
				return params, fmt.Errorf("idle_time: target %g cannot be met; the parts that are fixed "+
					"(their own targets, or computed from other inputs) add up to more, or nothing is left "+
					"to scale (parts entered as 0 cannot be scaled)", *v)
			}
			if repeated && num(p.TransportTime) == 0 {
				p.TransportRepetitionTime, p.TransportRepetitionCount = nil, nil
			}
			if stockDriven && positive(p.WaitingTime) {
				p.StockQuantity = F(*p.StockQuantity * *waiting / *p.WaitingTime)
				updateStock()
			} else {
				p.WaitingTime = waiting
			}
			p.IdleTime = F(idleParts(&p))
		}
	}
	if !itTargeted && hasIdleParts {
		if !idlePartsGiven(cur) && params.IdleTime != nil {
			// As for process time: a part target (even 0) would start a
			// breakdown that replaces the total.
			return params, fmt.Errorf("idle_time is given only as a total for this process, so a target that " +
				"sets transport, waiting or storage time (or stock_quantity) would start a breakdown in which " +
				"the parts not entered count as 0, replacing the total; the breakdown cannot be inferred from " +
				"the total: target idle_time instead, or enter the parts in the current state first")
		}
		p.IdleTime = F(idleParts(&p))
	}

	// Yield: 1 − (scrap + rework) (A.17).
	hasDefects := p.ScrapRate != nil || p.ReworkRate != nil
	yfTarget := get(func(t *ProcessTargets) *Target { return t.YieldFactor })
	if stillUnknown("yield_factor") {
		rateTargeted := targeted["scrap_rate"] || targeted["rework_rate"]
		switch {
		case !hasDefects && !rateTargeted:
			// No usable rate entered: a yield target sets the yield.
		case yfTarget != nil || (rateTargeted && cur.YieldFactor != nil):
			return params, cannotMove("yield_factor")
		default:
			yfTarget, hasDefects = nil, false
		}
	}
	if t := yfTarget; t != nil {
		if t.ReductionPercent != nil {
			return params, fmt.Errorf("yield_factor: use an absolute value, not reduction_percent")
		}
		v, err := applyField("yield_factor", t, cur.YieldFactor, true)
		if err != nil {
			return params, err
		}
		if hasDefects && !scaleShares(1-*v, []share{
			{&p.ScrapRate, targeted["scrap_rate"]},
			{&p.ReworkRate, targeted["rework_rate"]},
		}) {
			return params, fmt.Errorf("yield_factor: target %g conflicts with the scrap_rate/rework_rate "+
				"values or targets (rates entered as 0 cannot be scaled); yield_factor = 1 − "+
				"(scrap_rate + rework_rate) (A.17)", *v)
		}
		p.YieldFactor = v
		targeted["yield_factor"] = true
	}
	if hasDefects {
		switch {
		case !defectsImpossible(p):
			p.YieldFactor = F(c.CalculateYieldFactor(num(p.ScrapRate), num(p.ReworkRate)))
		case !hasIssue(curIssues, fieldDefects):
			return params, fmt.Errorf("scrap_rate + rework_rate = %g is above 1", num(p.ScrapRate)+num(p.ReworkRate))
		default:
			// The current state already has scrap + rework above 1
			// (reported by CheckConsistency); untouched, it is carried
			// over, with its stored yield.
			p.YieldFactor = params.YieldFactor
		}
	}

	// Lead time and its value classification. The lead time is the flow
	// (process + idle time) whenever process or idle time is known, even
	// when the result is 0, unless one of them is unknown (see
	// resolution.unknown): then the classification, else the current lead
	// time.
	flow := num(p.ProcessTime) + num(p.IdleTime)
	flowUnknown := (p.ProcessTime == nil && cr.unknown["process_time"]) || (p.IdleTime == nil && cr.unknown["idle_time"])
	flowSet := (p.ProcessTime != nil || p.IdleTime != nil) && !flowUnknown
	if p.ProcessTime != nil && p.ValueAddingTime != nil && exceeds(*p.ValueAddingTime, *p.ProcessTime) &&
		!hasIssue(curIssues, "value_adding_time") {
		return params, fmt.Errorf("process_time: the targets give %g, below value_adding_time %g "+
			"(value adding time is part of process time)", *p.ProcessTime, *p.ValueAddingTime)
	}
	classified := p.NonValueAddingTime != nil || p.NecessaryNonValueAddingTime != nil
	switch {
	case flowSet && classified:
		// The lead-time change not already explained by NVAT/NNVAT targets
		// is taken from the untargeted categories. Targets that explain
		// more than the change, or the opposite, are left for the check
		// below to reject.
		change := flow - num(cur.LeadTime)
		explained := (num(p.NonValueAddingTime) - num(cur.NonValueAddingTime)) +
			(num(p.NecessaryNonValueAddingTime) - num(cur.NecessaryNonValueAddingTime))
		delta := change - explained
		if delta*change < 0 || math.Abs(delta) > math.Abs(change) {
			delta = 0
		}
		absorb := func(field **float64) {
			v := num(*field) + delta
			delta = 0
			if v < 0 {
				delta, v = v, 0
			}
			if *field != nil || v != 0 {
				*field = F(v)
			}
		}
		if !targeted["non_value_adding_time"] {
			absorb(&p.NonValueAddingTime)
		}
		if delta != 0 && !targeted["necessary_non_value_adding_time"] {
			absorb(&p.NecessaryNonValueAddingTime)
		}
		if class := c.CalculateLeadTime(p); !consistent(class, flow) && !hasIssue(curIssues, fieldClassSum) {
			return params, fmt.Errorf("the targets give lead time (process + idle time) %g, but value adding + "+
				"non-value adding + necessary non-value adding time = %g; these are two breakdowns of the same "+
				"time (A.3), so adjust the process/idle time targets or the NVAT/NNVAT targets to agree "+
				"(untargeted NVAT/NNVAT follow lead time automatically)", flow, class)
		}
		p.LeadTime = F(flow)
	case flowSet:
		p.LeadTime = F(flow)
	case classified:
		p.LeadTime = F(c.CalculateLeadTime(p))
	case flowUnknown && cur.LeadTime != nil && (!sameValue(p.ProcessTime, cur.ProcessTime) || !sameValue(p.IdleTime, cur.IdleTime)):
		// A stored lead time stands in for an unknown part; it cannot follow
		// the changed parts.
		return params, cannotMove("lead_time")
	}

	// Impossible values of the current state (reported by
	// CheckConsistency) that no target replaced are carried over.
	for _, name := range impossibleFields(params) {
		if targeted[name] {
			continue
		}
		for _, b := range boundedFloats {
			if b.name == name {
				*b.value(&p) = *b.value(&params)
			}
		}
		for _, b := range boundedInts {
			if b.name == name {
				*b.value(&p) = *b.value(&params)
			}
		}
	}

	out := c.trimDerived(params, p, ctx)
	_, outIssues := c.resolveParameters(out, ctx)
	for _, issue := range outIssues {
		if !hasIssue(curIssues, issue.Field) {
			return params, fmt.Errorf("these targets would make the process inconsistent: %s", issue.Message)
		}
	}
	return out, nil
}

// derivedFields are the stored values ResolveParameters can compute from
// other values.
var derivedFields = []func(*ProcessParameters) **float64{
	func(p *ProcessParameters) **float64 { return &p.ProcessTime },
	func(p *ProcessParameters) **float64 { return &p.IdleTime },
	func(p *ProcessParameters) **float64 { return &p.LeadTime },
	func(p *ProcessParameters) **float64 { return &p.ProcessingTime },
	func(p *ProcessParameters) **float64 { return &p.TransportTime },
	func(p *ProcessParameters) **float64 { return &p.WaitingTime },
	func(p *ProcessParameters) **float64 { return &p.RangeOfInventory },
	func(p *ProcessParameters) **float64 { return &p.ValueAddingTime },
	func(p *ProcessParameters) **float64 { return &p.ValueAddingTimeProcess },
	func(p *ProcessParameters) **float64 { return &p.YieldFactor },
}

// sameValue reports whether a and b are both not entered, or both entered
// and consistent.
func sameValue(a, b *float64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return consistent(*a, *b)
}

// trimDerived returns out with derived values that input did not store
// cleared again, wherever ResolveParameters computes the same values (and
// the same presence) without them. A designed state then stores what the
// current state stored (with new numbers), instead of every derived value.
func (c *Calculator) trimDerived(input, out ProcessParameters, ctx streamContext) ProcessParameters {
	want, _ := c.resolveParameters(out, ctx)
	for _, field := range derivedFields {
		if *field(&input) != nil || *field(&out) == nil {
			continue
		}
		trial := out
		*field(&trial) = nil
		got, _ := c.resolveParameters(trial, ctx)
		same := true
		for _, f := range derivedFields {
			if !sameValue(*f(&got), *f(&want)) {
				same = false
				break
			}
		}
		if same {
			out = trial
		}
	}
	return out
}

// applyInventoryTargets returns inv with targets applied (own overrides
// def). Targets start from the current values as the model resolves them:
// when the customer demand rate is known and quantity is entered, the
// current range is quantity ÷ demand rate (see
// Calculator.CalculateCustomerInventoryTime), whatever range is stored.
//
//   - Demand rate known: a range target sets the quantity (range × rate), a
//     quantity target moves the range with it, and targeting both is an error
//     unless they agree. The stored form is kept: the range is stored only
//     where the current state stored one (it is computed from the quantity
//     otherwise), and an inventory given by its range alone (quantity not
//     entered) keeps only a range, its quantity being range × rate.
//   - Demand rate unknown: range is proportional to quantity at any constant
//     demand rate, so when both are entered (and above 0) a target on one
//     scales the other by the same factor; targeting both is an error unless
//     the factors agree. Otherwise the target sets its value alone.
//
// A quantity target of 0 empties the inventory (quantity 0, range 0).
func (c *Calculator) applyInventoryTargets(inv Inventory, own, def *InventoryTargets, ctx streamContext) (Inventory, error) {
	pickInv := func(get func(*InventoryTargets) *Target) *Target {
		if own != nil {
			if t := get(own); t != nil {
				return t
			}
		}
		if def != nil {
			return get(def)
		}
		return nil
	}
	qt := pickInv(func(t *InventoryTargets) *Target { return t.Quantity })
	rt := pickInv(func(t *InventoryTargets) *Target { return t.Range })
	if qt == nil && rt == nil {
		return inv, nil
	}

	// Current values as resolved. With the demand rate known, an inventory
	// given by its range alone has quantity range × rate.
	rate, rateKnown := ctx.demandRate(ctx.unit)
	q0, r0 := inv.Quantity, inv.Range
	if r, ok := ctx.inventoryRange(inv); ok {
		r0 = F(r)
	}
	if rateKnown && q0 == nil && r0 != nil {
		q0 = F(*r0 * rate)
	}
	q, r := q0, r0
	var err error
	if qt != nil {
		if q, err = applyField("quantity", qt, q0, false); err != nil {
			return inv, err
		}
		qt = targetIfSet(qt, q)
	}
	if rt != nil {
		if r, err = applyField("range", rt, r0, false); err != nil {
			return inv, err
		}
		rt = targetIfSet(rt, r)
	}
	if qt == nil && rt == nil {
		return inv, nil // reductions of values not entered: nothing changes
	}
	if qt == nil {
		q = q0
	}
	if rt == nil {
		r = r0
	}

	out := inv
	if rateKnown {
		switch {
		case qt != nil && rt != nil:
			if !consistent(*r, *q/rate) {
				return inv, fmt.Errorf("quantity and range: range = quantity ÷ customer demand rate "+
					"= %g, not %g; set one of them", *q/rate, *r)
			}
		case rt != nil:
			q = F(*r * rate)
		default:
			r = F(*q / rate)
		}
		// Keep the stored form: an inventory given by range alone stays so.
		switch {
		case inv.Quantity != nil:
			out.Quantity = q
			if inv.Range != nil {
				out.Range = r
			}
		case inv.Range != nil:
			out.Range = r
		case qt != nil:
			out.Quantity = q
		default:
			out.Range = r
		}
		return out, nil
	}

	// Demand rate unknown.
	if positive(q0) && positive(r0) {
		switch {
		case qt != nil && rt != nil:
			if !consistent(*q / *q0, *r / *r0) {
				return inv, fmt.Errorf("quantity and range: range moves with quantity (×%g), "+
					"but the range target moves it ×%g; set one of them", *q / *q0, *r / *r0)
			}
		case rt != nil:
			q = F(*q0 * *r / *r0)
		default:
			r = F(*r0 * *q / *q0)
		}
	} else if qt != nil && *q == 0 && r0 != nil {
		r = F(0) // emptied: no stock lasts no time
	}
	out.Quantity, out.Range = q, r
	return out, nil
}

// targetIfSet returns t when applying it gave a value, nil when it gave
// none (a reduction of a value that is not entered).
func targetIfSet(t *Target, v *float64) *Target {
	if v == nil {
		return nil
	}
	return t
}

// checkTargetIDs reports per-process or per-inventory targets whose ID is not
// in the current value stream (usually a typo).
func checkTargetIDs(current *ValueStream, targets *FutureStateTargets) error {
	known := map[string]bool{}
	for _, p := range current.Processes {
		known[p.ID] = true
	}
	var unknown []string
	for id := range targets.Processes {
		if !known[id] {
			unknown = append(unknown, "process "+id)
		}
	}
	knownInv := map[string]bool{}
	for _, inv := range current.Inventory {
		knownInv[inv.ID] = true
	}
	for id := range targets.Inventory {
		if !knownInv[id] {
			unknown = append(unknown, "inventory "+id)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return fmt.Errorf("targets reference IDs not in the current state: %v", unknown)
	}
	return nil
}
