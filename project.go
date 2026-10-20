package vsm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

// Project is one run of the VSM procedure (clause 4.1) for a product family:
// the current state, an optional ideal state as guidance, the future state,
// the catalogue of measures and the workshop.
type Project struct {
	SchemaVersion int          `json:"schema_version"`
	Name          string       `json:"name"`
	Current       *ValueStream `json:"current"`
	Ideal         *ValueStream `json:"ideal,omitempty"`
	Future        *ValueStream `json:"future,omitempty"`
	Measures      []Measure    `json:"measures,omitempty"`
	Workshop      *Workshop    `json:"workshop,omitempty"`
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
	Measures   []MeasureReview  `json:"measures,omitempty"`
	// Priority lists open measure numbers in cost-benefit order.
	Priority []int   `json:"priority,omitempty"`
	Issues   []Issue `json:"issues"`
}

// MeasureReview is the check of one measure.
type MeasureReview struct {
	Number           int      `json:"number"`
	SMARTGaps        []string `json:"smart_gaps,omitempty"`
	BenefitCostRatio Value    `json:"benefit_cost_ratio"`
}

// EvaluateProject evaluates every state, compares current and future, and
// checks the catalogue of measures. Issues of each state are in its Result;
// Issues of the report concern the project as a whole.
func EvaluateProject(p *Project) ProjectReport {
	rep := ProjectReport{Name: p.Name, Issues: []Issue{}}
	add := func(sev Severity, subject, rule, format string, args ...any) {
		rep.Issues = append(rep.Issues, Issue{Severity: sev, Subject: subject, Rule: rule, Message: fmt.Sprintf(format, args...)})
	}
	if p.SchemaVersion > SchemaVersion {
		add(SeverityError, "project", "", "schema_version %d is newer than this library supports (%d)", p.SchemaVersion, SchemaVersion)
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
		switch m.Status {
		case StatusPlanned, StatusInProgress, StatusDone, StatusCancelled:
		case "":
			add(SeverityWarning, subject, "4.4.2", "status is not noted")
		default:
			add(SeverityError, subject, "4.4.2", "status %q is not planned, in_progress, done or cancelled", m.Status)
		}
		for _, f := range m.Flashes {
			if !flashes[f] {
				add(SeverityError, subject, "4.3.2", "improvement flash %d is not on the current or future map", f)
			}
		}
		if (m.Cost != nil && *m.Cost < 0) || (m.Benefit != nil && *m.Benefit < 0) {
			add(SeverityError, subject, "4.5.2", "cost and benefit cannot be negative")
		}
		rep.Measures = append(rep.Measures, MeasureReview{Number: m.Number, SMARTGaps: m.SMARTGaps(), BenefitCostRatio: m.BenefitCostRatio()})
	}
	rep.Priority = Prioritize(p.Measures)
	if len(p.Measures) > 0 {
		switch {
		case p.Workshop == nil:
			add(SeverityNote, "project", "4.4.3", "the catalogue has not been through a workshop yet")
		case p.Workshop.Date != "":
			if _, err := time.Parse(time.DateOnly, p.Workshop.Date); err != nil {
				add(SeverityError, "workshop", "4.4.3", "date is not a YYYY-MM-DD date")
			}
		}
	}
	return rep
}

// ReadValueStream decodes a value stream from JSON. Unknown fields are an
// error, so a misspelt field is not silently dropped.
func ReadValueStream(r io.Reader) (*ValueStream, error) {
	var vs ValueStream
	if err := decodeStrict(r, &vs); err != nil {
		return nil, err
	}
	return &vs, nil
}

// ReadProject decodes a project from JSON, rejecting unknown fields.
func ReadProject(r io.Reader) (*Project, error) {
	var p Project
	if err := decodeStrict(r, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// LoadValueStream reads a value stream from a JSON file.
func LoadValueStream(path string) (*ValueStream, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ReadValueStream(f)
}

// LoadProject reads a project from a JSON file.
func LoadProject(path string) (*Project, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ReadProject(f)
}

// Save writes v (a *ValueStream or *Project) as indented JSON, stamping the
// current schema version.
func Save(path string, v any) error {
	switch x := v.(type) {
	case *ValueStream:
		x.SchemaVersion = SchemaVersion
	case *Project:
		x.SchemaVersion = SchemaVersion
		for _, vs := range []*ValueStream{x.Current, x.Ideal, x.Future} {
			if vs != nil {
				vs.SchemaVersion = SchemaVersion
			}
		}
	default:
		return fmt.Errorf("vsm: Save takes a *ValueStream or *Project, not %T", v)
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func decodeStrict(r io.Reader, v any) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("vsm: %w", err)
	}
	if dec.More() {
		return fmt.Errorf("vsm: unexpected data after the JSON value")
	}
	return nil
}
