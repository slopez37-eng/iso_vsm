package vsm

import (
	"fmt"
	"math"
)

// Stream figures: the one implementation of every number that adds up times
// or stock across a value stream. CalculateKPIs, the analyzer (idle time,
// waste, flow efficiency), the designer (continuous-flow check),
// CheckConsistency (missing data) and the text export all read them from
// figures, so a rule changed here changes every result the same way.
//
// The rule for unknown values: a total is known only when every part of it
// is known. A process's own totals follow ResolveParameters (inside one
// process, parts that are not entered add nothing, so a process with a
// process time and no idle time has lead time = process time). A stream
// total needs, from every element on the map, the value it adds: a process
// whose lead time cannot be determined, or an inventory whose time cannot be
// determined, makes the stream lead time unknown, not smaller. An element
// that is not on the map adds nothing. Every unknown part is reported as a
// ConsistencyIssue of Kind IssueMissingData (see CheckConsistency), and the
// KPIs that need it are omitted.

// Issue kinds (ConsistencyIssue.Kind).
const (
	// IssueContradiction: stored values that disagree with each other, or a
	// value that is impossible or unusable (above its bound, an unsupported
	// time unit). Calculations use the computed value, or treat the value
	// as unknown.
	IssueContradiction = "contradiction"
	// IssueMissingData: a value that a stream total needs is not entered
	// and cannot be computed. The total, and every KPI built on it, is
	// omitted.
	IssueMissingData = "missing_data"
)

// processFigures is one process as the stream totals count it.
type processFigures struct {
	// params are the resolved parameters (see ResolveParametersInStream) in
	// the reference unit.
	params ProcessParameters
	// unitOK is false when the process's time unit is not supported: its
	// times cannot be expressed in the reference unit and count as unknown.
	unitOK bool
	// leadTime and idle are the lead time and idle time the stream counts
	// for the process (nil = unknown). idle is 0 when the lead time is the
	// process time alone (no idle time given).
	leadTime, idle *float64
}

// inventoryFigures is one inventory point as the stream totals count it.
type inventoryFigures struct {
	// counted is true when the inventory adds its own time to the lead time
	// (not through a linked process's waiting or idle time, see
	// Inventory.ProcessID).
	counted bool
	// time is the time it adds when counted, in the reference unit (nil =
	// unknown).
	time *float64
	// stock is the stock it adds to the stream's stock (0 when it is a
	// linked process's stock, counted there; nil = unknown).
	stock *float64
}

// streamFigures are a value stream's totals, each nil when unknown.
type streamFigures struct {
	unit        TimeUnit
	processes   []processFigures   // in vs.Processes order
	inventories []inventoryFigures // in vs.Inventory order
	// Totals over the processes (and, for idle, waiting and lead time,
	// the inventories counted on their own), in the reference unit. With
	// no processes they are 0.
	processTime, idleTime, leadTime, valueAdding *float64
	// transport, waiting and storage are the idle time parts: the entered
	// (or computed) transport, waiting and storage times, waiting
	// including the inventories counted on their own. A process idle time
	// given only as a total is not split, so they can add up to less than
	// idleTime. nil when idleTime is unknown.
	transport, waiting, storage *float64
	// stock is the stream's stock, each stock counted once (see
	// Inventory.ProcessID): every process's stock_quantity plus each
	// inventory's stock.
	stock *float64
	// vatImpossible is true when a process's value adding time exceeds its
	// process or lead time, or the stream's exceeds its lead time: flow
	// efficiency is then unknown (CheckConsistency reports it).
	vatImpossible bool
	// missing explains every unknown part (Kind IssueMissingData).
	missing []ConsistencyIssue
}

// sum adds optional values; it is unknown once any part is unknown.
type sum struct {
	v       float64
	unknown bool
}

func (s *sum) add(p *float64) {
	if p == nil {
		s.unknown = true
		return
	}
	s.v += *p
}

func (s sum) value() *float64 {
	if s.unknown {
		return nil
	}
	return F(s.v)
}

// Rules of the missing-data issues.
const (
	ruleLeadTotal  = "total_lead_time = Σ process lead times + Σ inventory times; unknown when any part is unknown"
	ruleVATTotal   = "total_value_adding_time = Σ process value adding times; unknown when any part is unknown"
	rulePTTotal    = "total process time = Σ process times (process_efficiency); unknown when any part is unknown"
	ruleStockTotal = "stream stock = Σ process stock_quantity + Σ inventory stock (inventory_turns); unknown when any part is unknown"
)

// figures computes vs's stream figures.
func (c *Calculator) figures(vs *ValueStream) streamFigures {
	ctx := c.contextOf(vs)
	f := streamFigures{unit: ctx.unit}
	if vs == nil {
		return f
	}
	var pt, idle, lt, vat, transport, waiting, storage, stock sum
	missing := func(element, id, field, rule, format string, args ...any) {
		f.missing = append(f.missing, ConsistencyIssue{
			Kind: IssueMissingData, Element: element, ID: id, Field: field, Rule: rule,
			Message: fmt.Sprintf("%s %s: ", element, id) + fmt.Sprintf(format, args...),
		})
	}
	const ltKPIs = "total_lead_time and the KPIs built on it (value_added_ratio, process_efficiency) are not computed"

	// Which processes have an inventory counted on its own as their stock
	// (see timeThroughProcess): that inventory carries the stock's time.
	carried := map[string]bool{}
	for _, inv := range vs.Inventory {
		if holder, linked := stockHolder(vs, inv); linked && !c.timeThroughProcess(holder, ctx) {
			carried[holder.ID] = true
		}
	}

	for _, process := range vs.Processes {
		p, ok := c.resolvedIn(process.Parameters, ctx)
		pf := processFigures{params: p, unitOK: ok}
		stock.add(F(num(process.Parameters.StockQuantity)))
		if !ok {
			// Reported by resolveParameters (unsupported time unit).
			for _, s := range []*sum{&pt, &idle, &lt, &vat} {
				s.add(nil)
			}
			f.processes = append(f.processes, pf)
			continue
		}
		pf.leadTime, pf.idle = p.LeadTime, p.IdleTime
		if pf.idle == nil && p.ProcessTime != nil {
			pf.idle = F(0) // lead time = process time (ResolveParameters)
		}
		idleTotalOnly := !idlePartsGiven(p) && p.IdleTime != nil
		if positive(p.StockQuantity) && p.WaitingTime == nil && !idleTotalOnly && !carried[process.ID] {
			// Stock that waits for a time nothing on the map gives.
			pf.leadTime, pf.idle = nil, nil
			missing("process", process.ID, "waiting_time", ruleLeadTotal,
				"stock_quantity %g waits for an unknown time: enter waiting_time or idle_time, a range on an "+
					"inventory linked to this process (process_id), or customer demand with a day or week "+
					"demand_period; "+ltKPIs, *p.StockQuantity)
		} else if pf.leadTime == nil {
			missing("process", process.ID, "lead_time", ruleLeadTotal,
				"lead time unknown: enter process_time (or its parts), idle_time, the value classification or "+
					"lead_time; "+ltKPIs)
		}
		if p.ProcessTime == nil && pf.leadTime != nil {
			missing("process", process.ID, "process_time", rulePTTotal,
				"process_time unknown (not entered, and no parts given); process_efficiency and the idle "+
					"time totals are not computed")
		}
		if p.ValueAddingTime == nil {
			missing("process", process.ID, "value_adding_time", ruleVATTotal,
				"value_adding_time not entered; total_value_adding_time and value_added_ratio are not computed")
		}
		if vatImpossible(p) {
			f.vatImpossible = true
		}
		pt.add(p.ProcessTime)
		idle.add(pf.idle)
		lt.add(pf.leadTime)
		vat.add(p.ValueAddingTime)
		// The idle parts (not entered: none); only used when the idle
		// total is known.
		transport.add(F(num(p.TransportTime)))
		waiting.add(F(num(p.WaitingTime)))
		storage.add(F(num(p.StorageTime)))
		f.processes = append(f.processes, pf)
	}

	rate, rateKnown := ctx.demandRate(ctx.unit)
	for _, inv := range vs.Inventory {
		holder, linked := stockHolder(vs, inv)
		var inf inventoryFigures
		switch {
		case linked:
			inf.stock = F(0) // the process's stock_quantity, counted there
		case inv.Quantity != nil:
			inf.stock = inv.Quantity
		case inv.Range != nil && (*inv.Range == 0 || rateKnown):
			inf.stock = F(*inv.Range * rate)
		}
		inf.counted = !linked || !c.timeThroughProcess(holder, ctx)
		if inf.counted {
			if r, ok := ctx.inventoryRange(inv); ok {
				inf.time = F(r)
			} else {
				inf.time = inv.Range
			}
		}
		switch {
		case inf.counted && inf.time == nil && inf.stock == nil:
			missing("inventory", inv.ID, "quantity", ruleLeadTotal,
				"neither quantity nor range entered: its waiting time and stock are unknown, so "+
					"total_lead_time, inventory_turns and the KPIs built on them are not computed")
		case inf.counted && inf.time == nil:
			missing("inventory", inv.ID, "range", ruleLeadTotal,
				"quantity %g entered, but no range and no customer demand rate to compute it (demand with a "+
					"day or week demand_period); "+ltKPIs, num(inv.Quantity))
		case inf.stock == nil && vs.Customer != nil && positive(vs.Customer.Demand):
			missing("inventory", inv.ID, "quantity", ruleStockTotal,
				"range entered but no quantity, and no customer demand rate to turn the range into stock; "+
					"inventory_turns is not computed")
		}
		if inf.counted {
			idle.add(inf.time)
			lt.add(inf.time)
			waiting.add(inf.time)
		}
		stock.add(inf.stock)
		f.inventories = append(f.inventories, inf)
	}

	f.processTime, f.idleTime, f.leadTime, f.valueAdding = pt.value(), idle.value(), lt.value(), vat.value()
	if f.idleTime != nil {
		f.transport, f.waiting, f.storage = transport.value(), waiting.value(), storage.value()
	}
	f.stock = stock.value()
	if f.valueAdding != nil && f.leadTime != nil && exceeds(*f.valueAdding, *f.leadTime) {
		f.vatImpossible = true
	}
	return f
}

// flowEfficiency returns value adding time ÷ lead time (value_added_ratio,
// A.16), and false when it is unknown: either total unknown, lead time 0,
// or a value adding time larger than its whole (vatImpossible).
func (f streamFigures) flowEfficiency() (float64, bool) {
	if f.valueAdding == nil || f.leadTime == nil || *f.leadTime <= 0 || f.vatImpossible {
		return 0, false
	}
	return *f.valueAdding / *f.leadTime, true
}

// processEfficiency returns process time ÷ lead time (process_efficiency:
// the share of the lead time spent in processes), and false when either is
// unknown or the lead time is 0. It is also the idle time analysis's
// efficiency, process time ÷ (process time + idle time): the lead time is
// that sum.
func (f streamFigures) processEfficiency() (float64, bool) {
	if f.processTime == nil || f.leadTime == nil || *f.leadTime <= 0 {
		return 0, false
	}
	return *f.processTime / *f.leadTime, true
}

// unknownTotal is NaN, the value of a float64-only total that is unknown.
func unknownTotal(p *float64) float64 {
	if p == nil {
		return math.NaN()
	}
	return *p
}
