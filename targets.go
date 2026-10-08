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
// field means "keep the current value".
//
// Totals and their parts stay consistent: process_time is a total that
// includes setup_time, and idle_time is a total of transport_time,
// waiting_time and storage_time (it is the stored value or, when not stored,
// that sum). If a part gets a target but its total does not, a stored total
// moves by the same amount. A total targeted to 0 sets its parts to 0. If
// scrap_rate or rework_rate changes and yield_factor has no target, the
// stored yield factor moves by the opposite of the change in scrap + rework
// (A.17: YF = 1 − (SR + RR)).
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

// Component fields (everything except the process_time and idle_time
// totals and yield_factor, which are handled separately in applyProcessTargets).
var componentFields = []processField{
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

// applyProcessTargets returns params with the targets applied (own overrides
// def field by field). See ProcessTargets for how totals and parts are kept
// consistent.
func (c *Calculator) applyProcessTargets(params ProcessParameters, own, def *ProcessTargets) (ProcessParameters, error) {
	p := params

	for _, f := range componentFields {
		if t := pick(own, def, f.target); t != nil {
			v, err := applyField(f.name, t, *f.value(&params), f.isRate)
			if err != nil {
				return p, err
			}
			*f.value(&p) = v
		}
	}

	// Process time total (stored ProcessTime, else the A.2 component sum).
	if t := pick(own, def, func(t *ProcessTargets) *Target { return t.ProcessTime }); t != nil {
		v, err := applyField("process_time", t, c.effectiveProcessTime(params), false)
		if err != nil {
			return p, err
		}
		p.ProcessTime = v
		if v == 0 {
			p.ProcessingTime, p.SetupTime, p.HandlingTime, p.WaitingTimeProcess, p.AncillaryTime = 0, 0, 0, 0, 0
		}
	} else if params.ProcessTime > 0 {
		p.ProcessTime = math.Max(0, params.ProcessTime-(params.SetupTime-p.SetupTime))
	}

	// Idle time total (stored IdleTime, else TT + WT + ST_T).
	if t := pick(own, def, func(t *ProcessTargets) *Target { return t.IdleTime }); t != nil {
		v, err := applyField("idle_time", t, c.effectiveIdleTime(params), false)
		if err != nil {
			return p, err
		}
		p.IdleTime = v
		if v == 0 {
			p.TransportTime, p.WaitingTime, p.StorageTime = 0, 0, 0
		}
	} else if params.IdleTime > 0 {
		cut := (params.TransportTime - p.TransportTime) + (params.WaitingTime - p.WaitingTime) +
			(params.StorageTime - p.StorageTime)
		p.IdleTime = math.Max(0, params.IdleTime-cut)
	}

	// Yield factor: absolute target only; else follow scrap + rework.
	if t := pick(own, def, func(t *ProcessTargets) *Target { return t.YieldFactor }); t != nil {
		if t.ReductionPercent != nil {
			return p, fmt.Errorf("yield_factor: use an absolute value, not reduction_percent")
		}
		v, err := applyField("yield_factor", t, params.YieldFactor, true)
		if err != nil {
			return p, err
		}
		p.YieldFactor = v
	} else if params.YieldFactor > 0 {
		delta := (params.ScrapRate + params.ReworkRate) - (p.ScrapRate + p.ReworkRate)
		p.YieldFactor = math.Min(1, math.Max(0, params.YieldFactor+delta))
	}

	return p, nil
}

// applyInventoryTargets returns inv with targets applied (own overrides def).
func applyInventoryTargets(inv Inventory, own, def *InventoryTargets) (Inventory, error) {
	out := inv
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
			return out, err
		}
		*f.val = v
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
