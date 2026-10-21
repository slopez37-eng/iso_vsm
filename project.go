package vsm

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"slices"
	"strings"
	"time"
)

// Phase is the phase of the VSM procedure (clause 4.1) that a project is in.
type Phase string

const (
	PhaseAnalysis       Phase = "analysis"
	PhaseDesign         Phase = "design"
	PhasePlanning       Phase = "planning"
	PhaseImplementation Phase = "implementation"
	PhaseAssessment     Phase = "assessment"
	PhaseAdjustment     Phase = "adjustment"
)

// Phases lists every Phase.
var Phases = []Phase{PhaseAnalysis, PhaseDesign, PhasePlanning, PhaseImplementation, PhaseAssessment, PhaseAdjustment}

// Project is one run of the VSM procedure (clause 4.1) for a product family:
// the current state, an optional ideal state as guidance, the future state,
// the catalogue of measures, the monitoring snapshots and the workshop.
type Project struct {
	SchemaVersion int          `json:"schema_version"`
	Name          string       `json:"name"`
	Phase         Phase        `json:"phase,omitempty"`
	Current       *ValueStream `json:"current"`
	Ideal         *ValueStream `json:"ideal,omitempty"`
	Future        *ValueStream `json:"future,omitempty"`
	Monitoring    []Snapshot   `json:"monitoring,omitempty"`
	Measures      []Measure    `json:"measures,omitempty"`
	Workshop      *Workshop    `json:"workshop,omitempty"`
}

// Snapshot is the value stream as it was found on one date, after the future
// state was set, so that the measures can be followed up.
type Snapshot struct {
	Date  string       `json:"date"`  // YYYY-MM-DD
	State *ValueStream `json:"state"` // the stream as measured on that date
}

// ProjectReport is the evaluation of a project.
type ProjectReport struct {
	Name    string  `json:"name"`
	Current *Result `json:"current,omitempty"`
	Ideal   *Result `json:"ideal,omitempty"`
	Future  *Result `json:"future,omitempty"`
	// Comparison is the 4.5.2 assessment of the future against the current
	// state, when both lead times are known.
	Comparison *StateComparison `json:"comparison,omitempty"`
	// Monitoring is the evaluation of each snapshot, in the order given.
	Monitoring []MonitoringReport `json:"monitoring,omitempty"`
	Measures   []MeasureReview    `json:"measures,omitempty"`
	// Priority lists open measure numbers in cost-benefit order.
	Priority []int   `json:"priority,omitempty"`
	Issues   []Issue `json:"issues"`
}

// MeasureReview is the check of one measure.
type MeasureReview struct {
	Number           int      `json:"number"`
	SMARTGaps        []string `json:"smart_gaps,omitempty"`
	BenefitCostRatio Value    `json:"benefit_cost_ratio"`
	// Target is the measure's target, as entered.
	Target *float64 `json:"target,omitempty"`
	// Measured is the KPI of the latest snapshot that has it.
	Measured Value `json:"measured"`
	// Met says whether Measured reaches Target in the direction that is better
	// for the KPI. It is set only when both are known.
	Met *bool `json:"met,omitempty"`
}

// MonitoringReport is the evaluation of one snapshot.
type MonitoringReport struct {
	Date   string  `json:"date"`
	Result *Result `json:"result"`
	// Comparison is the 4.5.2 assessment of the current state against the
	// snapshot, when both lead times are known.
	Comparison *StateComparison `json:"comparison,omitempty"`
}

// EvaluateProject evaluates every state, compares current and future, and
// checks the catalogue of measures. Issues of each state are in its Result;
// Issues of the report concern the project as a whole.
func EvaluateProject(p *Project) ProjectReport {
	rep := ProjectReport{Name: p.Name}
	var issues issueList
	add := issues.add
	issues.checkSchema("project", p.SchemaVersion)
	if p.Phase != "" {
		checkEnum(&issues, "project", "4.1", "phase", p.Phase, Phases)
	}
	eval := func(vs *ValueStream, want State, label string) *Result {
		if vs == nil {
			return nil
		}
		if vs.State != "" && vs.State != want {
			add(SeverityWarning, label, "", "the %s state is marked %q", label, vs.State)
		}
		r := Evaluate(vs)
		return &r
	}
	if p.Current == nil {
		add(SeverityError, "project", "4.2.4", "a project needs a current state")
	}
	rep.Current = eval(p.Current, StateCurrent, "current")
	rep.Ideal = eval(p.Ideal, StateIdeal, "ideal")
	rep.Future = eval(p.Future, StateFuture, "future")

	if rep.Current != nil && rep.Future != nil {
		if c, err := CompareStates(*rep.Current, *rep.Future); err == nil {
			rep.Comparison = &c
		} else {
			add(SeverityNote, "project", "4.5.2", "no comparison of current and future: %v", err)
		}
	}

	// Monitoring snapshots are evaluated in the order given and compared with
	// the current state.
	var last string
	for i, s := range p.Monitoring {
		subject := "monitoring " + s.Date
		if s.Date == "" {
			subject = fmt.Sprintf("monitoring #%d", i+1)
		}
		if !isDate(s.Date) {
			add(SeverityError, subject, "4.5.2", "date is missing or not a YYYY-MM-DD date")
		} else {
			if s.Date < last {
				add(SeverityWarning, subject, "4.5.2", "date is earlier than the snapshot before it (%s)", last)
			}
			last = s.Date
		}
		if s.State == nil {
			add(SeverityError, subject, "4.5.2", "the snapshot has no state; ignored")
			continue
		}
		mr := MonitoringReport{Date: s.Date, Result: eval(s.State, StateCurrent, subject)}
		if rep.Current != nil {
			if c, err := CompareStates(*rep.Current, *mr.Result); err == nil {
				mr.Comparison = &c
			} else {
				add(SeverityNote, subject, "4.5.2", "no comparison of current and this snapshot: %v", err)
			}
		}
		rep.Monitoring = append(rep.Monitoring, mr)
	}

	flashes := map[int]bool{}
	for _, vs := range []*ValueStream{p.Current, p.Future} {
		if vs != nil {
			for _, f := range vs.Flashes {
				flashes[f.Number] = true
			}
		}
	}
	numbers := map[int]bool{}
	for _, m := range p.Measures {
		subject := fmt.Sprintf("measure %d", m.Number)
		if numbers[m.Number] {
			add(SeverityError, subject, "4.4.2", "measure number is used twice")
		}
		numbers[m.Number] = true
		if m.Status == "" {
			add(SeverityWarning, subject, "4.4.2", "status is not noted")
		} else {
			checkEnum(&issues, subject, "4.4.2", "status", m.Status, MeasureStatuses)
		}
		for _, f := range m.Flashes {
			if !flashes[f] {
				add(SeverityError, subject, "4.3.2", "improvement flash %d is not on the current or future map", f)
			}
		}
		if (m.Cost != nil && *m.Cost < 0) || (m.Benefit != nil && *m.Benefit < 0) {
			add(SeverityError, subject, "4.5.2", "cost and benefit cannot be negative")
		}
		mr := MeasureReview{Number: m.Number, SMARTGaps: m.SMARTGaps(), BenefitCostRatio: m.BenefitCostRatio(), Target: m.Target}
		if m.KPI != "" {
			checkEnum(&issues, subject, "4.4.2", "kpi", m.KPI, KPINames)
			mr.Measured = latestKPI(rep.Monitoring, m.KPI, rep.Current)
			if m.Target != nil && mr.Measured.Known {
				met := meetsTarget(m.KPI, mr.Measured.Value, *m.Target)
				mr.Met = &met
			}
		}
		rep.Measures = append(rep.Measures, mr)
	}
	rep.Priority = Prioritize(p.Measures)
	if len(p.Measures) > 0 && p.Workshop == nil {
		add(SeverityNote, "project", "4.4.3", "the catalogue has not been through a workshop yet")
	}
	if p.Workshop != nil && p.Workshop.Date != "" {
		if !isDate(p.Workshop.Date) {
			add(SeverityError, "workshop", "4.4.3", "date is not a YYYY-MM-DD date")
		}
	}
	rep.Issues = issues.list()
	return rep
}

// latestKPI returns the named KPI of the snapshot with the latest valid date
// that has it. Times are converted to the current state's unit, which is the
// unit of a measure's target.
func latestKPI(reports []MonitoringReport, name KPIName, current *Result) Value {
	var best Value
	date := ""
	for _, m := range reports {
		v := m.Result.KPI(name)
		if !v.Known || !isDate(m.Date) || m.Date < date {
			continue
		}
		if current != nil && (name == KPILeadTime || name == KPIProcessTime || name == KPIIdleTime) {
			x, err := Convert(v.Value, m.Result.TimeUnit, current.TimeUnit)
			if err != nil {
				continue
			}
			v.Value = x
		}
		best, date = v, m.Date
	}
	return best
}

// meetsTarget reports whether v reaches target in the direction that is better
// for the KPI: lead time, process time, idle time, non-value adding share and
// scrap rate are better when lower.
func meetsTarget(name KPIName, v, target float64) bool {
	switch name {
	case KPILeadTime, KPIProcessTime, KPIIdleTime, KPINonValueAddingShare, KPIScrapRate:
		return v <= target
	}
	return v >= target
}

// AllIssues returns the issues of the project, its states and its snapshots.
func (r ProjectReport) AllIssues() []Issue {
	all := slices.Clone(r.Issues)
	for _, s := range []*Result{r.Current, r.Ideal, r.Future} {
		if s != nil {
			all = append(all, s.Issues...)
		}
	}
	for _, m := range r.Monitoring {
		all = append(all, m.Result.Issues...)
	}
	return all
}

// HasErrors reports whether the project or any of its states has an error.
func (r ProjectReport) HasErrors() bool { return hasErrors(r.AllIssues()) }

// HasErrors reports whether the evaluation found an error.
func (r Result) HasErrors() bool { return hasErrors(r.Issues) }

func hasErrors(issues []Issue) bool {
	return slices.ContainsFunc(issues, func(i Issue) bool { return i.Severity == SeverityError })
}

func isDate(s string) bool {
	_, err := time.Parse(time.DateOnly, s)
	return err == nil
}

// IsProjectJSON reports whether a JSON object is a project rather than a
// single value stream: it is a project when every top-level key is a field of
// Project.
func IsProjectJSON(data []byte) (bool, error) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return false, fmt.Errorf("vsm: %w", err)
	}
	fields := map[string]bool{}
	t := reflect.TypeFor[Project]()
	for i := range t.NumField() {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		fields[name] = true
	}
	for k := range probe {
		if !fields[k] {
			return false, nil
		}
	}
	return true, nil
}

// ReadValueStream decodes a value stream from JSON. Unknown fields are an
// error, so a misspelt field is not silently dropped.
func ReadValueStream(r io.Reader) (*ValueStream, error) { return read[ValueStream](r) }

// ReadProject decodes a project from JSON, rejecting unknown fields.
func ReadProject(r io.Reader) (*Project, error) { return read[Project](r) }

// LoadValueStream reads a value stream from a JSON file.
func LoadValueStream(path string) (*ValueStream, error) { return load[ValueStream](path) }

// LoadProject reads a project from a JSON file.
func LoadProject(path string) (*Project, error) { return load[Project](path) }

func load[T any](path string) (*T, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return read[T](f)
}

// read decodes one JSON value, rejecting unknown fields and trailing data.
func read[T any](r io.Reader) (*T, error) {
	var v T
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("vsm: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("vsm: unexpected data after the JSON value")
	}
	return &v, nil
}

// Save writes v (a *ValueStream or *Project) to a file as indented JSON,
// stamping the current schema version.
func Save(path string, v any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := Write(f, v); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// Write writes v (a *ValueStream or *Project) as indented JSON, stamping the
// current schema version.
func Write(w io.Writer, v any) error {
	switch x := v.(type) {
	case *ValueStream:
		x.SchemaVersion = SchemaVersion
	case *Project:
		x.SchemaVersion = SchemaVersion
		streams := []*ValueStream{x.Current, x.Ideal, x.Future}
		for _, s := range x.Monitoring {
			streams = append(streams, s.State)
		}
		for _, vs := range streams {
			if vs != nil {
				vs.SchemaVersion = SchemaVersion
			}
		}
	default:
		return fmt.Errorf("vsm: Write takes a *ValueStream or *Project, not %T", v)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
