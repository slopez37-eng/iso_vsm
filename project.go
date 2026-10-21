package vsm

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	// Difference is benefit − cost. Payback is Cost ÷ Benefit, counted in
	// BenefitPeriod; it is derived, not a figure of the standard.
	Difference    Value  `json:"difference"`
	BenefitPeriod Period `json:"benefit_period,omitempty"`
	Payback       Value  `json:"payback"`
	// Target is the measure's target, as entered.
	Target *float64 `json:"target,omitempty"`
	// Measured is the KPI of the latest snapshot that has it.
	Measured Value `json:"measured"`
	// Met says whether Measured reaches Target in the direction that is better
	// for the KPI. It is set only when both are known. A time KPI has no verdict
	// without a current state, because the target has no unit then.
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
		if m.BenefitPeriod != "" {
			checkEnum(&issues, subject, "4.5.2", "benefit_period", m.BenefitPeriod, Periods)
		}
		mr := MeasureReview{
			Number: m.Number, SMARTGaps: m.SMARTGaps(), BenefitCostRatio: m.BenefitCostRatio(), Target: m.Target,
			Difference: m.Difference(), BenefitPeriod: m.BenefitPeriod, Payback: m.Payback(),
		}
		if m.KPI != "" {
			checkEnum(&issues, subject, "4.4.2", "kpi", m.KPI, KPINames)
			mr.Measured = latestKPI(rep.Monitoring, m.KPI, rep.Current)
			// A time target is in the current state's unit, so without a current
			// state a time KPI has no verdict.
			noUnit := rep.Current == nil && isTimeKPI(m.KPI)
			if m.Target != nil && mr.Measured.Known && !noUnit {
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
// unit of a measure's target. Without a current state they stay in the
// snapshot's unit, and EvaluateProject gives the measure no verdict.
func latestKPI(reports []MonitoringReport, name KPIName, current *Result) Value {
	var best Value
	date := ""
	for _, m := range reports {
		v := m.Result.KPI(name)
		if !v.Known || !isDate(m.Date) || m.Date < date {
			continue
		}
		if current != nil && isTimeKPI(name) {
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

// isTimeKPI reports whether the KPI is a time, in the unit of the value stream.
func isTimeKPI(name KPIName) bool {
	return name == KPILeadTime || name == KPIProcessTime || name == KPIIdleTime
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
// single value stream. A top-level current key makes it a project. Otherwise a
// key that only a value stream has (such as time_unit or processes) makes it a
// value stream, and a key that only a project has (such as phase or measures)
// makes it a project. Otherwise it is a project when every top-level key is a
// field of Project, as in a file with only a name.
func IsProjectJSON(data []byte) (bool, error) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return false, fmt.Errorf("vsm: %w", err)
	}
	if _, ok := probe["current"]; ok {
		return true, nil
	}
	project := jsonFields(reflect.TypeFor[Project]())
	stream := jsonFields(reflect.TypeFor[ValueStream]())
	projectOnly := false
	allProject := true
	for k := range probe {
		if stream[k] && !project[k] {
			return false, nil
		}
		if project[k] && !stream[k] {
			projectOnly = true
		}
		if !project[k] {
			allProject = false
		}
	}
	return projectOnly || allProject, nil
}

// jsonFields returns the JSON key of every field of a struct type.
func jsonFields(t reflect.Type) map[string]bool {
	fields := map[string]bool{}
	for i := range t.NumField() {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		fields[name] = true
	}
	return fields
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
// stamping the current schema version. The JSON is written to a temporary file
// beside the target, which replaces the target only when it is complete, so a
// failed save leaves the existing file unchanged. A symbolic link is followed,
// even when its target does not exist yet, and the file keeps its permissions;
// a new file gets 0644. A read-only file is an error, and nothing is written.
func Save(path string, v any) error {
	path, err := linkTarget(path)
	if err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
		// Renaming over the file needs only a writable folder, so the file itself is
		// checked for writing first.
		f, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			return err
		}
		f.Close()
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name()) // fails harmlessly once the file has been renamed
	if err := Write(f, v); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(f.Name(), mode); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// linkTarget returns the file that a save to path writes. Symbolic links are
// followed one at a time, relative links against the folder of the link, so a
// link whose target does not exist yet is followed too. The loop stops after 40
// links, as the kernel does.
func linkTarget(path string) (string, error) {
	for range 40 {
		fi, err := os.Lstat(path)
		if err != nil || fi.Mode()&os.ModeSymlink == 0 {
			return path, nil
		}
		link, err := os.Readlink(path)
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(link) {
			link = filepath.Join(filepath.Dir(path), link)
		}
		path = link
	}
	return "", fmt.Errorf("vsm: %s: too many levels of symbolic links", path)
}

// Write writes v (a *ValueStream or *Project) as indented JSON, stamping the
// current schema version.
func Write(w io.Writer, v any) error {
	switch x := v.(type) {
	case ValueStream:
		return Write(w, &x)
	case Project:
		return Write(w, &x)
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
