package vsm

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
)

// Value is a calculated or entered number together with where it came from.
type Value struct {
	Value float64
	Known bool
	// Source is "entered" or the formula that produced the value, such as
	// "A.6". It is empty when the value is unknown.
	Source string
}

// MarshalJSON writes an unknown value as null and a known one as
// {"value": v, "source": s}.
func (v Value) MarshalJSON() ([]byte, error) {
	if !v.Known {
		return []byte("null"), nil
	}
	var value any = v.Value
	if math.IsInf(v.Value, 0) {
		value = fmt.Sprint(v.Value) // JSON has no infinity; write "+Inf" or "-Inf"
	}
	return json.Marshal(struct {
		Value  any    `json:"value"`
		Source string `json:"source"`
	}{value, v.Source})
}

func (v Value) String() string {
	if !v.Known {
		return "unknown"
	}
	return fmt.Sprintf("%g (%s)", v.Value, v.Source)
}

// Severity grades an Issue.
type Severity string

const (
	// SeverityError marks input that was ignored because it is invalid.
	SeverityError Severity = "error"
	// SeverityWarning marks entered or derived values that contradict a
	// formula of the standard. Nothing is overwritten; the user decides.
	SeverityWarning Severity = "warning"
	// SeverityNote explains a gap or a choice the evaluation made.
	SeverityNote Severity = "note"
)

// Issue is one finding of Evaluate.
type Issue struct {
	Severity Severity `json:"severity"`
	Subject  string   `json:"subject"`        // e.g. "process P2", "customer", "stream"
	Rule     string   `json:"rule,omitempty"` // the formula or clause involved
	Message  string   `json:"message"`
}

func (i Issue) String() string {
	rule := ""
	if i.Rule != "" {
		rule = " [" + i.Rule + "]"
	}
	return fmt.Sprintf("%s: %s: %s%s", i.Severity, i.Subject, i.Message, rule)
}

// Result is the evaluation of one value stream.
type Result struct {
	Name       string          `json:"name"`
	State      State           `json:"state,omitempty"`
	TimeUnit   TimeUnit        `json:"time_unit"`
	Processes  []ProcessResult `json:"processes"`
	Idles      []IdleResult    `json:"idles"`
	Totals     Totals          `json:"totals"`
	KPIs       KPIs            `json:"kpis"`
	Bottleneck string          `json:"bottleneck,omitempty"` // ID of the most heavily loaded process (3.2)
	Issues     []Issue         `json:"issues"`
}

// ProcessResult holds the resolved times and rates of one process.
type ProcessResult struct {
	ID                      string `json:"id"`
	Name                    string `json:"name"`
	ProcessTime             Value  `json:"process_time"`
	CycleTime               Value  `json:"cycle_time"`
	SetupTime               Value  `json:"setup_time"`
	ProcessingTime          Value  `json:"processing_time"`
	HandlingTime            Value  `json:"handling_time"`
	WaitingTime             Value  `json:"waiting_time"`
	Downtime                Value  `json:"downtime"`
	ValueAdding             Value  `json:"value_adding_time"`
	NonValueAdding          Value  `json:"non_value_adding_time"`
	NecessaryNonValueAdding Value  `json:"necessary_non_value_adding_time"`
	ScrapRate               Value  `json:"scrap_rate"`
	YieldFactor             Value  `json:"yield_factor"`
	// TaktLoad is cycle time ÷ customer takt: the share of the process's
	// capacity that customer demand takes up. Above 1 the process cannot meet
	// demand. It says nothing about how busy each resource is.
	TaktLoad Value `json:"takt_load"`
}

// IdleResult holds the resolved times of one idle segment.
type IdleResult struct {
	ID                      string  `json:"id"`
	From                    string  `json:"from,omitempty"`
	To                      string  `json:"to,omitempty"`
	IdleTime                Value   `json:"idle_time"`
	TransportTime           Value   `json:"transport_time"`
	WaitingTime             Value   `json:"waiting_time"`
	StorageTime             Value   `json:"storage_time"`
	Ranges                  []Value `json:"ranges,omitempty"`
	NonValueAdding          Value   `json:"non_value_adding_time"`
	NecessaryNonValueAdding Value   `json:"necessary_non_value_adding_time"`
}

// Totals are the stream totals. A total is unknown when any of its parts is.
type Totals struct {
	ProcessTime             Value `json:"process_time"`   // ΣPT
	IdleTime                Value `json:"idle_time"`      // ΣIT
	LeadTime                Value `json:"lead_time"`      // A.4
	TransportTime           Value `json:"transport_time"` // ΣTT
	WaitingTime             Value `json:"waiting_time"`   // ΣWT
	StorageTime             Value `json:"storage_time"`   // ΣSTT
	ValueAdding             Value `json:"value_adding_time"`
	NonValueAdding          Value `json:"non_value_adding_time"`
	NecessaryNonValueAdding Value `json:"necessary_non_value_adding_time"`
}

// KPIs are the key performance indicators of A.3.3 and the shares of 4.5.2.
type KPIs struct {
	CustomerTakt        Value `json:"customer_takt"`          // A.12
	InventoryTurns      Value `json:"inventory_turns"`        // A.13
	OnTimeInFull        Value `json:"on_time_in_full"`        // A.14
	ScrapRate           Value `json:"scrap_rate"`             // K_SR from stream quality counts
	YieldFactor         Value `json:"yield_factor"`           // A.17
	ValueAddedRatio     Value `json:"value_added_ratio"`      // A.16
	ValueAddingShare    Value `json:"value_adding_share"`     // S_VA = ΣPT ÷ LT
	NonValueAddingShare Value `json:"non_value_adding_share"` // S_NVA = ΣIT ÷ LT
}

// cell is one number during evaluation.
type cell struct {
	name  string
	v     float64
	known bool
	src   string
}

func (c *cell) set(v float64, src string) { c.v, c.known, c.src = v, true, src }

func (c *cell) value() Value {
	if !c.known {
		return Value{}
	}
	return Value{Value: c.v, Known: true, Source: c.src}
}

func known(name string, v float64, src string) *cell {
	return &cell{name: name, v: v, known: true, src: src}
}

func knownValue(v float64, src string) Value { return Value{Value: v, Known: true, Source: src} }

type processCells struct {
	p                                        *Process
	subject                                  string
	pt, ct, st, prt, ht, wt, dt, rt, rep, pq *cell
	res, ttr, batch                          *cell
	vat, nvat, nnvat, sr, yf                 *cell
	// a8 are the parts of process time in A.8 for this process type.
	a8 []*cell
}

type idleCells struct {
	d                             *Idle
	subject                       string
	it, tt, trep, trt, wt, orders *cell
	stt                           *cell
	ranges, quantities            []*cell
	nvat, nnvat                   *cell
}

type evaluator struct {
	issueList
	vs       *ValueStream
	checking bool
	takt     *cell
	demand   *cell
	period   *cell
	procs    []*processCells
	idles    []*idleCells
}

// Evaluate resolves a value stream: it takes the entered values, derives
// what the formulas of Annex A allow, checks every formula whose terms are all
// known, and computes totals and KPIs. It never overwrites an entered value;
// contradictions are reported as issues.
func Evaluate(vs *ValueStream) Result {
	e := &evaluator{vs: vs}
	e.validate()
	e.customer()
	for i := range vs.Processes {
		e.procs = append(e.procs, e.processCells(&vs.Processes[i]))
	}
	for i := range vs.Idles {
		e.idles = append(e.idles, e.idleCells(&vs.Idles[i]))
	}
	for e.derive() {
	}
	e.checking = true
	e.derive()
	e.notes()
	return e.result()
}

// issueList collects the issues of an evaluation.
type issueList []Issue

func (l *issueList) add(sev Severity, subject, rule, format string, args ...any) {
	*l = append(*l, Issue{Severity: sev, Subject: subject, Rule: rule, Message: fmt.Sprintf(format, args...)})
}

// list returns the issues, never nil, so JSON shows an empty list.
func (l issueList) list() []Issue {
	if l == nil {
		return []Issue{}
	}
	return l
}

func (l *issueList) checkSchema(subject string, version int) {
	if version > SchemaVersion {
		l.add(SeverityError, subject, "", "schema_version %d is newer than this library supports (%d)", version, SchemaVersion)
	}
}

// checkEnum reports v when it is not one of valid.
func checkEnum[T ~string](l *issueList, subject, rule, field string, v T, valid []T) {
	if slices.Contains(valid, v) {
		return
	}
	names := make([]string, len(valid))
	for i, x := range valid {
		names[i] = string(x)
	}
	last := len(names) - 1
	l.add(SeverityError, subject, rule, "%s %q is not %s or %s", field, v, strings.Join(names[:last], ", "), names[last])
}

// entered turns an optional input into a cell. Negative numbers, and rates
// above 1, are reported and ignored.
func (e *evaluator) entered(subject, name string, p *float64, rate bool) *cell {
	c := &cell{name: name}
	if p == nil {
		return c
	}
	v := *p
	switch {
	case math.IsNaN(v) || math.IsInf(v, 0):
		e.add(SeverityError, subject, "", "%s is not a finite number; ignored", name)
	case v < 0:
		e.add(SeverityError, subject, "", "%s is negative (%g); ignored", name, v)
	case rate && v > 1:
		e.add(SeverityError, subject, "", "%s is above 1 (%g); rates are fractions, so 25 %% is 0.25; ignored", name, v)
	default:
		c.set(v, "entered")
	}
	return c
}

func (e *evaluator) validate() {
	vs := e.vs
	e.checkSchema("stream", vs.SchemaVersion)
	if !vs.TimeUnit.Valid() {
		e.add(SeverityError, "stream", "", "time_unit %q is not one of s, min, h, d", vs.TimeUnit)
	}
	if vs.State != "" {
		checkEnum(&e.issueList, "stream", "", "state", vs.State, States)
	}
	if len(vs.Processes) == 0 {
		e.add(SeverityError, "stream", "", "the value stream has no processes")
	}
	ids := map[string]string{}
	claim := func(id, what string) {
		if id == "" {
			e.add(SeverityError, what, "", "missing id")
			return
		}
		if prev, ok := ids[id]; ok {
			e.add(SeverityError, what+" "+id, "", "id is already used by a %s", prev)
			return
		}
		ids[id] = what
	}
	for _, s := range vs.Suppliers {
		claim(s.ID, "supplier")
	}
	pacemakers := 0
	for _, p := range vs.Processes {
		claim(p.ID, "process")
		if p.Type != "" {
			checkEnum(&e.issueList, "process "+p.ID, "", "type", p.Type, ProcessTypes)
		}
		if p.Pacemaker {
			pacemakers++
		}
	}
	if pacemakers > 1 {
		e.add(SeverityWarning, "stream", "3.7", "%d processes are marked as pacemaker; a value stream normally has one", pacemakers)
	}
	for _, d := range vs.Idles {
		claim(d.ID, "idle")
	}
	for _, d := range vs.Idles {
		for _, end := range []string{d.From, d.To} {
			if end == "" {
				continue
			}
			if kind, ok := ids[end]; !ok || (kind != "process" && kind != "supplier") {
				e.add(SeverityError, "idle "+d.ID, "", "%q is not a process or supplier id", end)
			}
		}
	}
	for _, f := range vs.Flashes {
		for _, w := range f.Wastes {
			if !slices.Contains(Wastes, w) {
				e.add(SeverityError, fmt.Sprintf("improvement flash %d", f.Number), "4.3.2", "%q is not one of the seven wastes", w)
			}
		}
	}
	seen := map[Guideline]bool{}
	for _, g := range vs.Guidelines {
		if !slices.Contains(Guidelines, g.Guideline) {
			e.add(SeverityError, "design guidelines", "4.3.4", "%q is not one of the eight design guidelines", g.Guideline)
		} else if seen[g.Guideline] {
			e.add(SeverityError, "design guidelines", "4.3.4", "%q is listed twice", g.Guideline)
		}
		seen[g.Guideline] = true
	}
	if vs.State == StateFuture && len(vs.Guidelines) > 0 {
		var missing []string
		for _, g := range Guidelines {
			if !seen[g] {
				missing = append(missing, string(g))
			}
		}
		if len(missing) > 0 {
			e.add(SeverityNote, "design guidelines", "4.3.4", "not yet considered: %s", strings.Join(missing, ", "))
		}
	}
}

func (e *evaluator) customer() {
	c := e.vs.Customer
	e.demand = e.entered("customer", "demand", c.Demand, false)
	ot := e.entered("customer", "operating_time", c.OperatingTime, false)
	e.period = e.entered("customer", "period_length", c.PeriodLength, false)
	for _, c := range []*cell{ot, e.period} {
		if c.known && c.v == 0 {
			e.add(SeverityError, "customer", "", "%s is 0; it must be positive, so it is ignored", c.name)
			c.known = false
		}
	}
	e.takt = &cell{name: "customer takt"}
	switch {
	case e.demand.known && e.demand.v == 0:
		e.add(SeverityError, "customer", "A.12", "demand is 0, so customer takt is undefined")
		e.demand.known = false
	case !e.demand.known || !ot.known:
		e.add(SeverityNote, "customer", "A.12", "customer takt needs demand and operating_time")
	default:
		v, _ := CustomerTakt(ot.v, e.demand.v)
		e.takt.set(v, "A.12")
	}
}

func (e *evaluator) processCells(p *Process) *processCells {
	s := "process " + p.ID
	c := &processCells{p: p, subject: s}
	c.pt = e.entered(s, "process_time", p.ProcessTime, false)
	c.ct = e.entered(s, "cycle_time", p.CycleTime, false)
	c.st = e.entered(s, "setup_time", p.SetupTime, false)
	c.prt = e.entered(s, "processing_time", p.ProcessingTime, false)
	c.ht = e.entered(s, "handling_time", p.HandlingTime, false)
	c.wt = e.entered(s, "waiting_time", p.WaitingTime, false)
	c.dt = e.entered(s, "downtime", p.Downtime, false)
	c.rt = e.entered(s, "repetition_time", p.RepetitionTime, false)
	c.rep = e.entered(s, "repetitions", p.Repetitions, false)
	c.pq = e.entered(s, "process_quantity", p.ProcessQuantity, false)
	c.res = e.entered(s, "resources", p.Resources, false)
	c.vat = e.entered(s, "value_adding_time", p.ValueAdding, false)
	c.nvat = e.entered(s, "non_value_adding_time", p.NonValueAdding, false)
	c.nnvat = e.entered(s, "necessary_non_value_adding_time", p.NecessaryNonValueAdding, false)
	c.sr = e.entered(s, "scrap_rate", p.ScrapRate, true)
	c.yf = e.entered(s, "yield_factor", p.YieldFactor, true)
	e.entered(s, "rework_rate", p.ReworkRate, true)
	c.batch = e.entered(s, "batch_size", p.BatchSize, false)
	c.a8 = c.a8Parts()
	for _, f := range []struct {
		name string
		p    *float64
	}{
		{"operators", p.Operators}, {"ancillary_time", p.AncillaryTime}, {"transport_time", p.TransportTime},
		{"time_between_failures", p.TimeBetweenFailures}, {"failures", p.Failures},
		{"variants", p.Variants},
	} {
		e.entered(s, f.name, f.p, false)
	}
	c.ttr = &cell{name: "Σ times_to_repair"}
	if len(p.TimesToRepair) > 0 {
		valid := true
		for _, t := range p.TimesToRepair {
			if t < 0 || math.IsNaN(t) || math.IsInf(t, 0) {
				e.add(SeverityError, s, "A.1", "times_to_repair contains an invalid value (%g); ignored", t)
				valid = false
			}
		}
		if valid {
			c.ttr.set(Downtime(p.TimesToRepair...), "A.1")
		}
	}
	return c
}

// a8Parts returns the parts of process time in A.8. By Figure A.1, downtime
// is part of it only for automated processes (case B), and handling time is
// not part of it for data-related processes (case C).
func (c *processCells) a8Parts() []*cell {
	parts := []*cell{c.st, c.prt, c.wt}
	if c.p.Type != ProcessData {
		parts = append(parts, c.ht)
	}
	if c.p.Type != ProcessManual && c.p.Type != ProcessData {
		parts = append(parts, c.dt)
	}
	return parts
}

func (e *evaluator) idleCells(d *Idle) *idleCells {
	s := "idle " + d.ID
	c := &idleCells{d: d, subject: s}
	c.it = e.entered(s, "idle_time", d.IdleTime, false)
	c.tt = e.entered(s, "transport_time", d.TransportTime, false)
	c.trep = e.entered(s, "transport_repetitions", d.TransportRepetitions, false)
	c.trt = e.entered(s, "transport_repetition_time", d.TransportRepetitionTime, false)
	c.wt = e.entered(s, "waiting_time", d.WaitingTime, false)
	c.orders = e.entered(s, "waiting_orders", d.WaitingOrders, false)
	c.stt = e.entered(s, "storage_time", d.StorageTime, false)
	c.nvat = e.entered(s, "non_value_adding_time", d.NonValueAdding, false)
	c.nnvat = e.entered(s, "necessary_non_value_adding_time", d.NecessaryNonValueAdding, false)
	for i, inv := range d.Inventories {
		name := inventoryName(inv, i)
		if !slices.Contains(InventoryKinds, inv.Kind) {
			e.add(SeverityError, s, "", "%s has unknown kind %q", name, inv.Kind)
		}
		c.ranges = append(c.ranges, e.entered(s, name+" range", inv.Range, false))
		c.quantities = append(c.quantities, e.entered(s, name+" quantity", inv.Quantity, false))
	}
	return c
}

func inventoryName(inv Inventory, i int) string {
	if inv.ID != "" {
		return "inventory " + inv.ID
	}
	return fmt.Sprintf("inventory #%d", i+1)
}

// derive runs every formula once. In checking mode it reports contradictions
// instead of deriving. It returns whether any value was derived.
func (e *evaluator) derive() bool {
	changed := false
	for _, c := range e.procs {
		s := c.subject
		changed = e.sumRel(s, "A.1", c.dt, c.ttr) || changed
		if c.rt.known && c.rep.known {
			term := known("repetition_time × repetitions", c.rt.v*c.rep.v, "A.6")
			changed = e.sumRel(s, "A.6", c.pt, c.st, term) || changed
		}
		changed = e.sumRel(s, "A.8", c.pt, c.a8...) || changed
		changed = e.sumRel(s, "A.3 per process (process lead time equals process time)", c.pt, c.vat, c.nvat, c.nnvat) || changed
		changed = e.sumRel(s, "A.17", hundredPercent, c.sr, c.yf) || changed
	}
	for _, c := range e.idles {
		s := c.subject
		changed = e.productRel(s, "A.10", c.tt, c.trep, c.trt) || changed
		changed = e.productRel(s, "A.11", c.wt, c.orders, e.takt) || changed
		for i := range c.ranges {
			changed = e.rangeRel(s, c.ranges[i], c.quantities[i]) || changed
		}
		if len(c.ranges) > 0 {
			changed = e.sumRel(s, "A.9", c.stt, c.ranges...) || changed
		}
		changed = e.sumRel(s, "A.2", c.it, c.tt, c.wt, c.stt) || changed
		changed = e.sumRel(s, "A.2 (idle time adds no value)", c.it, c.nvat, c.nnvat) || changed
	}
	return changed
}

// noteA5 reports what A.5 would give for a process whose process time is
// still unknown. A.5 is never applied: it holds only if process quantity is
// the work in process per resource, which Table A.2 does not say.
// hundredPercent is the left side of A.17, 100 % = scrap rate + yield factor.
var hundredPercent = known("100 %", 1, "A.17")

// notes explains, per process, what A.5 would give and that a batch's
// process time is a piece's dwell time.
func (e *evaluator) notes() {
	for _, c := range e.procs {
		if !c.pt.known && c.ct.known && c.pq.known && c.res.known {
			e.add(SeverityNote, c.subject, "A.5",
				"A.5 as printed would give a process time of %g (cycle time × process quantity × resources), but it holds only if process quantity is the work in process per resource, so it is not applied; enter process_time",
				ProcessTimeFromCycleTime(c.ct.v, c.pq.v, c.res.v))
		}
		n := c.batch
		if !n.known {
			n = c.rep
		}
		if n.known && n.v > 1 && c.pt.known {
			e.add(SeverityNote, c.subject, "Table A.2", "process time %g is the dwell time of one piece, which stays for its whole batch of %g", c.pt.v, n.v)
		}
	}
}

// sumRel applies total = Σ parts. With every term known it checks the
// relation; with one term unknown it derives it. A derived part is never
// negative: if the known parts already exceed the total, that is reported.
func (e *evaluator) sumRel(subject, rule string, total *cell, parts ...*cell) bool {
	unknown, sum := -1, 0.0
	missing := 0
	for i, p := range parts {
		if p.known {
			sum += p.v
		} else {
			missing++
			unknown = i
		}
	}
	switch {
	case missing == 0:
		return e.settle(subject, rule, total, sum, names(parts)+" sum to")
	case missing == 1 && total.known:
		rest := total.v - sum
		if rest < 0 && !near(total.v, sum) {
			if e.checking {
				e.add(SeverityWarning, subject, rule, "%s add up to %g, more than %s (%g), so %s cannot be derived",
					names(knownOnly(parts)), sum, total.name, total.v, parts[unknown].name)
			}
			return false
		}
		if !e.checking {
			parts[unknown].set(math.Max(rest, 0), rule)
			return true
		}
	}
	return false
}

// productRel applies total = a × b, deriving the total or checking it.
func (e *evaluator) productRel(subject, rule string, total, a, b *cell) bool {
	if !a.known || !b.known {
		return false
	}
	return e.settle(subject, rule, total, a.v*b.v, a.name+" × "+b.name+" is")
}

// rangeRel applies A.7, converted to time with the period length.
func (e *evaluator) rangeRel(subject string, r, q *cell) bool {
	if !q.known || !e.demand.known {
		return false
	}
	if !e.period.known {
		if e.checking && !r.known {
			e.add(SeverityNote, subject, "A.7", "%s is in periods; set customer.period_length to convert it to time", r.name)
		}
		return false
	}
	periods, _ := Range(q.v, e.demand.v)
	return e.settle(subject, "A.7", r, periods*e.period.v, "quantity ÷ demand × period_length is")
}

// settle sets an unknown target to v while deriving. In the checking pass it
// warns when a known target differs from v, which formula describes. It
// returns whether the target was derived.
func (e *evaluator) settle(subject, rule string, target *cell, v float64, formula string) bool {
	if target.known {
		if e.checking && !near(target.v, v) {
			e.add(SeverityWarning, subject, rule, "%s is %g but %s %g", target.name, target.v, formula, v)
		}
		return false
	}
	if e.checking {
		return false
	}
	target.set(v, rule)
	return true
}

func (e *evaluator) result() Result {
	vs := e.vs
	r := Result{Name: vs.Name, State: vs.State, TimeUnit: vs.TimeUnit, Processes: []ProcessResult{}, Idles: []IdleResult{}}
	var pts, its, tts, wts, stts, vats, nvats, nnvats []*cell
	for _, c := range e.procs {
		pr := ProcessResult{
			ID: c.p.ID, Name: c.p.Name,
			ProcessTime: c.pt.value(), CycleTime: c.ct.value(), SetupTime: c.st.value(),
			ProcessingTime: c.prt.value(), HandlingTime: c.ht.value(), WaitingTime: c.wt.value(),
			Downtime: c.dt.value(), ValueAdding: c.vat.value(), NonValueAdding: c.nvat.value(),
			NecessaryNonValueAdding: c.nnvat.value(), ScrapRate: c.sr.value(), YieldFactor: c.yf.value(),
		}
		if c.ct.known && e.takt.known {
			pr.TaktLoad = knownValue(c.ct.v/e.takt.v, "A.12")
		}
		r.Processes = append(r.Processes, pr)
		pts = append(pts, c.pt)
		vats = append(vats, c.vat)
		nvats = append(nvats, c.nvat)
		nnvats = append(nnvats, c.nnvat)
	}
	for _, c := range e.idles {
		ir := IdleResult{
			ID: c.d.ID, From: c.d.From, To: c.d.To,
			IdleTime: c.it.value(), TransportTime: c.tt.value(), WaitingTime: c.wt.value(),
			StorageTime: c.stt.value(), NonValueAdding: c.nvat.value(), NecessaryNonValueAdding: c.nnvat.value(),
		}
		for _, rc := range c.ranges {
			ir.Ranges = append(ir.Ranges, rc.value())
		}
		r.Idles = append(r.Idles, ir)
		its = append(its, c.it)
		tts = append(tts, c.tt)
		wts = append(wts, c.wt)
		stts = append(stts, c.stt)
		nvats = append(nvats, c.nvat)
		nnvats = append(nnvats, c.nnvat)
	}

	t := &r.Totals
	t.ProcessTime = total(pts, "A.4")
	t.IdleTime = total(its, "A.4")
	t.TransportTime = total(tts, "A.2")
	t.WaitingTime = total(wts, "A.2")
	t.StorageTime = total(stts, "A.2")
	t.ValueAdding = total(vats, "A.3")
	t.NonValueAdding = total(nvats, "A.3")
	t.NecessaryNonValueAdding = total(nnvats, "A.3")
	if t.ProcessTime.Known && t.IdleTime.Known {
		t.LeadTime = knownValue(LeadTime(values(pts), values(its)), "A.4")
	} else {
		var missing []string
		for _, c := range e.procs {
			if !c.pt.known {
				missing = append(missing, c.subject+" (process_time)")
			}
		}
		for _, c := range e.idles {
			if !c.it.known {
				parts := names(unknownOnly([]*cell{c.tt, c.wt, c.stt}))
				missing = append(missing, fmt.Sprintf("%s (%s; enter 0 for none)", c.subject, parts))
			}
		}
		e.add(SeverityNote, "stream", "A.4", "lead time is unknown; missing: %s", strings.Join(missing, ", "))
	}

	k := &r.KPIs
	k.CustomerTakt = e.takt.value()
	if t.LeadTime.Known && t.LeadTime.Value > 0 {
		lt := t.LeadTime.Value
		if t.ValueAdding.Known {
			v, _ := ValueAddedRatio(t.ValueAdding.Value, lt)
			k.ValueAddedRatio = knownValue(v, "A.16")
		}
		va, _ := ValueAddingShare(t.ProcessTime.Value, lt)
		nva, _ := NonValueAddingShare(t.IdleTime.Value, lt)
		k.ValueAddingShare = knownValue(va, "4.5.2")
		k.NonValueAddingShare = knownValue(nva, "4.5.2")
	}
	if d := vs.Deliveries; d != nil {
		if v, err := OnTimeInFull(d.CompleteOnTime, d.Total); err == nil && d.CompleteOnTime >= 0 {
			k.OnTimeInFull = knownValue(v, "A.14")
		} else {
			e.add(SeverityError, "deliveries", "A.14", "need 0 ≤ complete_on_time ≤ total and total > 0")
		}
	}
	if q := vs.Quality; q != nil {
		if q.Total > 0 && q.Defective >= 0 && q.Defective <= q.Total {
			sr := q.Defective / q.Total
			k.ScrapRate = knownValue(sr, "A.3.3 scrap rate")
			k.YieldFactor = knownValue(YieldFactor(sr), "A.17")
		} else {
			e.add(SeverityError, "quality", "A.17", "need 0 ≤ defective ≤ total and total > 0")
		}
	}
	if inv := vs.Inventory; inv != nil {
		if inv.AverageStock > 0 && inv.Usage >= 0 {
			v, _ := InventoryTurns(inv.Usage, inv.AverageStock)
			k.InventoryTurns = knownValue(v, "A.13")
		} else {
			e.add(SeverityError, "inventory_turnover", "A.13", "need usage ≥ 0 and average_stock > 0")
		}
	}

	r.Bottleneck = e.bottleneck()
	r.Issues = e.list()
	return r
}

// bottleneck returns the most heavily loaded process (definition 3.2). Cycle
// time is the interval at which the whole process completes products, and all
// processes share the stream's operating time and product family, so the
// longest cycle time carries the highest load. Processes without a cycle time
// are skipped.
func (e *evaluator) bottleneck() string {
	best, bestCT, skipped := "", -1.0, 0
	for _, c := range e.procs {
		if !c.ct.known {
			skipped++
			continue
		}
		if c.ct.v > bestCT {
			best, bestCT = c.p.ID, c.ct.v
		}
	}
	if best != "" && skipped > 0 {
		e.add(SeverityNote, "stream", "3.2", "bottleneck ignores %d process(es) without a cycle time", skipped)
	}
	return best
}

func total(cells []*cell, rule string) Value {
	if len(unknownOnly(cells)) > 0 {
		return Value{}
	}
	return knownValue(sum(values(cells)), rule)
}

func values(cells []*cell) []float64 {
	v := make([]float64, len(cells))
	for i, c := range cells {
		v[i] = c.v
	}
	return v
}

func near(a, b float64) bool {
	return math.Abs(a-b) <= 1e-9*math.Max(1, math.Max(math.Abs(a), math.Abs(b)))
}

func names(cells []*cell) string {
	n := make([]string, len(cells))
	for i, c := range cells {
		n[i] = c.name
	}
	return strings.Join(n, " + ")
}

func unknownOnly(cells []*cell) []*cell {
	var out []*cell
	for _, c := range cells {
		if !c.known {
			out = append(out, c)
		}
	}
	return out
}

func knownOnly(cells []*cell) []*cell {
	var out []*cell
	for _, c := range cells {
		if c.known {
			out = append(out, c)
		}
	}
	return out
}
