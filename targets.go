package vsm

import (
	"fmt"
	"math"
	"sort"
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

// apply returns the target value for current.
func (t *Target) apply(current float64) (float64, error) {
	switch {
	case t.Value != nil && t.ReductionPercent != nil:
		return 0, fmt.Errorf("set either value or reduction_percent, not both")
	case t.Value != nil:
		if *t.Value < 0 || math.IsNaN(*t.Value) || math.IsInf(*t.Value, 0) {
			return 0, fmt.Errorf("value %v must be a finite number >= 0", *t.Value)
		}
		return *t.Value, nil
	case t.ReductionPercent != nil:
		p := *t.ReductionPercent
		if p < 0 || p > 100 || math.IsNaN(p) {
			return 0, fmt.Errorf("reduction_percent %v must be between 0 and 100", p)
		}
		return current * (1 - p/100), nil
	default:
		return 0, fmt.Errorf("target has neither value nor reduction_percent")
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
//     Calculator.CalculateInventoryTime) when the demand rate is known.
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
//   - A target of 0 is applied as 0 (it is not read as "not given"): the
//     lead time becomes the resulting process + idle time, even when that is
//     0. Process time can never drop below the value adding time, so a
//     target that would do so (for example 0 with value adding time set) is
//     an error.
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
	value  func(*ProcessParameters) *float64
	isRate bool // fraction between 0 and 1
}

// directFields are the targets applied to one stored value each. The
// process_time, idle_time and yield_factor totals are handled separately in
// applyProcessTargets.
var directFields = []processField{
	{"cycle_time", func(t *ProcessTargets) *Target { return t.CycleTime }, func(p *ProcessParameters) *float64 { return &p.CycleTime }, false},
	{"setup_time", func(t *ProcessTargets) *Target { return t.SetupTime }, func(p *ProcessParameters) *float64 { return &p.SetupTime }, false},
	{"waiting_time", func(t *ProcessTargets) *Target { return t.WaitingTime }, func(p *ProcessParameters) *float64 { return &p.WaitingTime }, false},
	{"transport_time", func(t *ProcessTargets) *Target { return t.TransportTime }, func(p *ProcessParameters) *float64 { return &p.TransportTime }, false},
	{"storage_time", func(t *ProcessTargets) *Target { return t.StorageTime }, func(p *ProcessParameters) *float64 { return &p.StorageTime }, false},
	{"non_value_adding_time", func(t *ProcessTargets) *Target { return t.NonValueAddingTime }, func(p *ProcessParameters) *float64 { return &p.NonValueAddingTime }, false},
	{"necessary_non_value_adding_time", func(t *ProcessTargets) *Target { return t.NecessaryNonValueAddingTime }, func(p *ProcessParameters) *float64 { return &p.NecessaryNonValueAddingTime }, false},
	{"stock_quantity", func(t *ProcessTargets) *Target { return t.StockQuantity }, func(p *ProcessParameters) *float64 { return &p.StockQuantity }, false},
	{"scrap_rate", func(t *ProcessTargets) *Target { return t.ScrapRate }, func(p *ProcessParameters) *float64 { return &p.ScrapRate }, true},
	{"rework_rate", func(t *ProcessTargets) *Target { return t.ReworkRate }, func(p *ProcessParameters) *float64 { return &p.ReworkRate }, true},
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

// applyField applies a target to one value, checking rate bounds.
func applyField(name string, t *Target, current float64, isRate bool) (float64, error) {
	v, err := t.apply(current)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	if isRate && v > 1 {
		return 0, fmt.Errorf("%s: %v is above 1; rates are fractions (0.02 = 2%%)", name, v)
	}
	return v, nil
}

// share is one part of a total that a total target may scale.
type share struct {
	value *float64
	fixed bool // has its own target, or is computed from other inputs
}

// scaleShares scales the non-fixed shares in proportion so that all shares
// add up to total. It reports false when that is impossible (the fixed
// shares exceed total, or nothing is free to scale).
func scaleShares(total float64, shares []share) bool {
	var fixed, free float64
	for _, s := range shares {
		if s.fixed {
			fixed += *s.value
		} else {
			free += *s.value
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
		if !s.fixed {
			*s.value *= k
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
// consistent.
func (c *Calculator) applyProcessTargets(params ProcessParameters, own, def *ProcessTargets, ctx streamContext) (ProcessParameters, error) {
	cur, curIssues := c.resolveParameters(params, ctx)
	p := cur
	targeted := map[string]bool{}
	get := func(f func(*ProcessTargets) *Target) *Target { return pick(own, def, f) }

	for _, f := range directFields {
		t := get(f.target)
		if t == nil {
			continue
		}
		v, err := applyField(f.name, t, *f.value(&cur), f.isRate)
		if err != nil {
			return params, err
		}
		*f.value(&p) = v
		targeted[f.name] = true
	}

	// Values computed from other inputs follow those inputs.
	if targeted["transport_time"] && cur.TransportRepetitionTime > 0 && cur.TransportRepetitionCount > 0 {
		return params, fmt.Errorf("transport_time: for this process it is transport_repetition_time × " +
			"transport_repetition_count (A.10); change those in the current state instead")
	}
	// Waiting time of stock is stock_quantity ÷ customer demand rate.
	rate, rateKnown := ctx.demandRate(params.TimeUnit)
	wasStock := rateKnown && cur.StockQuantity > 0
	if targeted["waiting_time"] && (wasStock || (rateKnown && targeted["stock_quantity"])) {
		if targeted["stock_quantity"] {
			return params, fmt.Errorf("waiting_time and stock_quantity: set one, not both; " +
				"waiting_time = stock_quantity ÷ customer demand rate")
		}
		p.StockQuantity = p.WaitingTime * rate
	}
	updateStock := func() {
		if rateKnown && p.StockQuantity > 0 {
			p.WaitingTime = p.StockQuantity / rate
			p.RangeOfInventory = p.WaitingTime
		} else if wasStock {
			p.WaitingTime, p.RangeOfInventory = 0, 0
		}
	}
	updateStock()
	wasProcessing := cur.CycleTime > 0 && cur.OrderQuantity > 0 && cur.ResourceCount > 0
	isProcessing := p.CycleTime > 0 && p.OrderQuantity > 0 && p.ResourceCount > 0
	if isProcessing {
		p.ProcessingTime = p.CycleTime * p.OrderQuantity / float64(p.ResourceCount)
	} else if wasProcessing {
		p.ProcessingTime = 0
	}

	// Process time: A.6 for repetitive processes, else the A.2 parts, else
	// the total alone.
	repetitive := cur.RepetitionTime > 0 && cur.RepetitionCount > 0
	hasParts := c.CalculateProcessTime(cur) > 0
	// A target of 0 is a real value, not "not given": remember which totals
	// were targeted so a zero result still sets the lead time.
	ptTargeted := get(func(t *ProcessTargets) *Target { return t.ProcessTime }) != nil
	itTargeted := get(func(t *ProcessTargets) *Target { return t.IdleTime }) != nil
	if t := get(func(t *ProcessTargets) *Target { return t.ProcessTime }); t != nil {
		v, err := applyField("process_time", t, cur.ProcessTime, false)
		if err != nil {
			return params, err
		}
		if repetitive {
			return params, fmt.Errorf("process_time: for this process it is setup_time + repetition_time × " +
				"repetition_count (A.6); target setup_time, or change the repetition data in the current state")
		}
		ok := scaleShares(v, []share{
			{&p.ProcessingTime, isProcessing},
			{&p.SetupTime, targeted["setup_time"]},
			{&p.HandlingTime, false},
			{&p.WaitingTimeProcess, false},
			{&p.AncillaryTime, false},
		})
		if !hasParts {
			p.ProcessTime = v
		} else if !ok {
			return params, fmt.Errorf("process_time: target %g cannot be met; the parts that are fixed "+
				"(targeted setup_time, or processing_time computed from cycle_time, A.5) add up to %g",
				v, c.CalculateProcessTime(p))
		}
	}
	switch {
	case repetitive:
		p.ProcessTime = c.CalculateRepetitiveProcessTime(p.SetupTime, p.RepetitionTime, p.RepetitionCount)
	case hasParts:
		p.ProcessTime = c.CalculateProcessTime(p)
	case c.CalculateProcessTime(p) > 0 && cur.ProcessTime > 0:
		return params, fmt.Errorf("process_time is given only as a total for this process, so a target " +
			"that sets one of its parts (setup or processing time) would replace it; target process_time " +
			"instead, or add the breakdown to the current state")
	}

	// Idle time: transport + waiting + storage time.
	idleParts := func(q *ProcessParameters) float64 {
		return c.CalculateTotalIdleTime(q.TransportTime, q.WaitingTime, q.StorageTime)
	}
	hasIdleParts := idleParts(&cur) > 0 || idleParts(&p) > 0
	if t := get(func(t *ProcessTargets) *Target { return t.IdleTime }); t != nil {
		v, err := applyField("idle_time", t, cur.IdleTime, false)
		if err != nil {
			return params, err
		}
		if !hasIdleParts {
			p.IdleTime = v
		} else {
			stockDriven := rateKnown && p.StockQuantity > 0
			waiting := p.WaitingTime
			// Transport from repetitions (A.10) is fixed, except that an idle
			// time of 0 removes the transports too.
			repeated := cur.TransportRepetitionTime > 0 && cur.TransportRepetitionCount > 0
			ok := scaleShares(v, []share{
				{&p.TransportTime, targeted["transport_time"] || (repeated && v != 0)},
				{&waiting, targeted["waiting_time"] || (stockDriven && targeted["stock_quantity"])},
				{&p.StorageTime, targeted["storage_time"]},
			})
			if !ok {
				return params, fmt.Errorf("idle_time: target %g cannot be met; the parts that are fixed "+
					"(their own targets, or computed from other inputs) add up to more, or nothing is left to scale", v)
			}
			if repeated && p.TransportTime == 0 {
				p.TransportRepetitionTime, p.TransportRepetitionCount = 0, 0
			}
			if stockDriven && p.WaitingTime > 0 {
				p.StockQuantity *= waiting / p.WaitingTime
				updateStock()
			} else {
				p.WaitingTime = waiting
			}
			p.IdleTime = idleParts(&p)
		}
	} else if hasIdleParts {
		if idleParts(&cur) == 0 && cur.IdleTime > 0 {
			return params, fmt.Errorf("idle_time is given only as a total for this process, so a target that " +
				"sets transport, waiting or storage time (or stock_quantity) would replace it; target idle_time " +
				"instead, or add the breakdown to the current state")
		}
		p.IdleTime = idleParts(&p)
	}

	// Yield: 1 − (scrap + rework) (A.17).
	hasDefects := cur.ScrapRate > 0 || cur.ReworkRate > 0 || p.ScrapRate > 0 || p.ReworkRate > 0
	if t := get(func(t *ProcessTargets) *Target { return t.YieldFactor }); t != nil {
		if t.ReductionPercent != nil {
			return params, fmt.Errorf("yield_factor: use an absolute value, not reduction_percent")
		}
		v, err := applyField("yield_factor", t, cur.YieldFactor, true)
		if err != nil {
			return params, err
		}
		if hasDefects && !scaleShares(1-v, []share{
			{&p.ScrapRate, targeted["scrap_rate"]},
			{&p.ReworkRate, targeted["rework_rate"]},
		}) {
			return params, fmt.Errorf("yield_factor: target %g conflicts with the scrap_rate/rework_rate "+
				"targets; yield_factor = 1 − (scrap_rate + rework_rate) (A.17)", v)
		}
		p.YieldFactor = v
	}
	if hasDefects {
		switch {
		case !defectsImpossible(p):
			p.YieldFactor = c.CalculateYieldFactor(p.ScrapRate, p.ReworkRate)
		case !hasIssue(curIssues, fieldDefects):
			return params, fmt.Errorf("scrap_rate + rework_rate = %g is above 1", p.ScrapRate+p.ReworkRate)
		}
		// Otherwise the current state already has scrap + rework above 1
		// (reported by CheckConsistency); untouched, it is carried over.
	}

	// Lead time and its value classification. The lead time is the flow
	// (process + idle time) whenever the current lead time came from it or
	// a target set part of it, even when the result is 0.
	flow := p.ProcessTime + p.IdleTime
	flowSet := cur.ProcessTime > 0 || cur.IdleTime > 0 || ptTargeted || itTargeted
	ptKnown := cur.ProcessTime > 0 || p.ProcessTime > 0 || ptTargeted
	if ptKnown && p.ValueAddingTime > p.ProcessTime &&
		!consistent(p.ValueAddingTime, p.ProcessTime) && !hasIssue(curIssues, "value_adding_time") {
		return params, fmt.Errorf("process_time: the targets give %g, below value_adding_time %g "+
			"(value adding time is part of process time)", p.ProcessTime, p.ValueAddingTime)
	}
	classified := cur.NonValueAddingTime > 0 || cur.NecessaryNonValueAddingTime > 0 ||
		p.NonValueAddingTime > 0 || p.NecessaryNonValueAddingTime > 0
	switch {
	case flowSet && classified:
		// The lead-time change not already explained by NVAT/NNVAT targets
		// is taken from the untargeted categories. Targets that explain
		// more than the change, or the opposite, are left for the check
		// below to reject.
		change := flow - cur.LeadTime
		explained := (p.NonValueAddingTime - cur.NonValueAddingTime) +
			(p.NecessaryNonValueAddingTime - cur.NecessaryNonValueAddingTime)
		delta := change - explained
		if delta*change < 0 || math.Abs(delta) > math.Abs(change) {
			delta = 0
		}
		absorb := func(field *float64) {
			*field += delta
			delta = 0
			if *field < 0 {
				delta, *field = *field, 0
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
		p.LeadTime = flow
	case flowSet:
		p.LeadTime = flow
	case classified || p.ValueAddingTime > 0:
		p.LeadTime = c.CalculateLeadTime(p)
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
var derivedFields = []func(*ProcessParameters) *float64{
	func(p *ProcessParameters) *float64 { return &p.ProcessTime },
	func(p *ProcessParameters) *float64 { return &p.IdleTime },
	func(p *ProcessParameters) *float64 { return &p.LeadTime },
	func(p *ProcessParameters) *float64 { return &p.ProcessingTime },
	func(p *ProcessParameters) *float64 { return &p.TransportTime },
	func(p *ProcessParameters) *float64 { return &p.WaitingTime },
	func(p *ProcessParameters) *float64 { return &p.RangeOfInventory },
	func(p *ProcessParameters) *float64 { return &p.ValueAddingTime },
	func(p *ProcessParameters) *float64 { return &p.ValueAddingTimeProcess },
	func(p *ProcessParameters) *float64 { return &p.YieldFactor },
}

// trimDerived returns out with derived values that input did not store
// cleared again, wherever ResolveParameters computes the same values without
// them. A designed state then stores what the current state stored (with
// new numbers), instead of every derived value.
func (c *Calculator) trimDerived(input, out ProcessParameters, ctx streamContext) ProcessParameters {
	want, _ := c.resolveParameters(out, ctx)
	for _, field := range derivedFields {
		if *field(&input) != 0 || *field(&out) == 0 {
			continue
		}
		trial := out
		*field(&trial) = 0
		got, _ := c.resolveParameters(trial, ctx)
		same := true
		for _, f := range derivedFields {
			if !consistent(*f(&got), *f(&want)) {
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
// def). When the customer demand rate is known, range = quantity ÷ demand
// rate (see Calculator.CalculateInventoryTime): a quantity target moves a
// stored range with it, a range target sets the quantity, and targeting both
// is an error unless they agree.
func (c *Calculator) applyInventoryTargets(inv Inventory, own, def *InventoryTargets, ctx streamContext) (Inventory, error) {
	out := inv
	targeted := map[string]bool{}
	fields := []struct {
		name string
		get  func(*InventoryTargets) *Target
		val  *float64
	}{
		{"quantity", func(t *InventoryTargets) *Target { return t.Quantity }, &out.Quantity},
		{"range", func(t *InventoryTargets) *Target { return t.Range }, &out.Range},
	}
	for _, f := range fields {
		var t *Target
		if own != nil {
			t = f.get(own)
		}
		if t == nil && def != nil {
			t = f.get(def)
		}
		if t == nil {
			continue
		}
		v, err := applyField(f.name, t, *f.val, false)
		if err != nil {
			return inv, err
		}
		*f.val = v
		targeted[f.name] = true
	}
	rate, ok := ctx.demandRate(ctx.unit)
	if !ok {
		return out, nil
	}
	switch {
	case targeted["quantity"] && targeted["range"]:
		if !consistent(out.Range, out.Quantity/rate) {
			return inv, fmt.Errorf("quantity and range: range = quantity ÷ customer demand rate "+
				"= %g, not %g; set one of them", out.Quantity/rate, out.Range)
		}
	case targeted["range"]:
		out.Quantity = out.Range * rate
	case targeted["quantity"] && inv.Range > 0:
		out.Range = out.Quantity / rate
	}
	return out, nil
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
