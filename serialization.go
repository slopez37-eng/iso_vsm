package vsm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
)

// File format.
//
// FormatVersion is written as "format_version" in the JSON of every
// ValueStream, VSMProject, DesignResult and AssessmentResult (by their
// MarshalJSON methods, so it is there whichever way they are written:
// Serializer methods, SaveToFile or json.Marshal).
//
// Format 2 (current): optional numbers are pointers in Go (nil = not
// entered, see ProcessParameters). In JSON a value that was not entered is
// omitted (or null), and 0 is an entered zero: "setup_time": 0 is a setup
// time of zero, a missing setup_time is unknown.
//
// Legacy files (written before format 2, without "format_version") stored
// every blank number as 0, and the model read 0 as "not entered". Reading
// one (UnmarshalJSON, so every load path) keeps that meaning: every numeric
// field of processes, the customer, inventories and flow elements that is
// 0 (or absent) becomes not entered (nil). The one exception is
// "yield_factor_given": true, which marked a yield_factor of 0 as entered;
// that yield stays 0 (the flag itself is no longer part of the model).
// Non-zero values load unchanged, so a legacy file gives the same results
// as before. In an assessment result, "working_days_per_week": 0 becomes
// not entered, and a KPI target that is not 0 is kept. A KPI target of 0 is
// ambiguous in a legacy file: the original format (which had no
// "has_target") wrote 0 both for "no target" and for a real target of 0,
// so a 0 counts as entered only when "has_target": true marks it (as files
// from the intermediate format do), and is otherwise not entered.
//
// A value stream nested in a project or design result follows its own
// "format_version"; without one, the container's. Files are always written
// in the current format; loading and saving a legacy file converts it.
const FormatVersion = 2

// MarshalJSON writes vs with "format_version" (see FormatVersion).
func (vs ValueStream) MarshalJSON() ([]byte, error) {
	type plain ValueStream
	return json.Marshal(struct {
		FormatVersion int `json:"format_version"`
		plain
	}{FormatVersion, plain(vs)})
}

// UnmarshalJSON reads vs in the current or the legacy format (see
// FormatVersion).
func (vs *ValueStream) UnmarshalJSON(data []byte) error {
	return decodeStream(data, 0, vs)
}

// decodeStream decodes data into vs. A stream without its own
// format_version is read as format inherited (0: legacy).
func decodeStream(data []byte, inherited int, vs *ValueStream) error {
	type plain ValueStream
	var w struct {
		FormatVersion *int `json:"format_version"`
		plain
	}
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*vs = ValueStream(w.plain)
	version := inherited
	if w.FormatVersion != nil {
		version = *w.FormatVersion
	}
	if version < FormatVersion {
		return migrateLegacyStream(vs, data)
	}
	return nil
}

// decodeStreamRaw decodes an optional nested stream: nil for an absent or
// null value.
func decodeStreamRaw(raw json.RawMessage, inherited int) (*ValueStream, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	vs := &ValueStream{}
	if err := decodeStream(raw, inherited, vs); err != nil {
		return nil, err
	}
	return vs, nil
}

// migrateLegacyStream converts a stream read from a legacy file (data) to
// the current meaning: legacy 0 meant "not entered" (see FormatVersion).
func migrateLegacyStream(vs *ValueStream, data []byte) error {
	var legacy struct {
		Processes []struct {
			Parameters struct {
				YieldFactorGiven bool `json:"yield_factor_given"`
			} `json:"parameters"`
		} `json:"processes"`
	}
	if err := json.Unmarshal(data, &legacy); err != nil {
		return err
	}
	for i := range vs.Processes {
		p := &vs.Processes[i].Parameters
		zeroYield := i < len(legacy.Processes) && legacy.Processes[i].Parameters.YieldFactorGiven &&
			num(p.YieldFactor) == 0
		clearZeros(p)
		if zeroYield {
			p.YieldFactor = F(0)
		}
	}
	if vs.Customer != nil {
		clearZeros(vs.Customer)
	}
	for i := range vs.Inventory {
		clearZeros(&vs.Inventory[i])
	}
	for i := range vs.FlowElements {
		clearZeros(&vs.FlowElements[i])
	}
	return nil
}

// clearZeros sets every *float64 and *int field of the struct v points to
// that holds 0 to nil (legacy "not entered").
func clearZeros(v any) {
	s := reflect.ValueOf(v).Elem()
	for i := 0; i < s.NumField(); i++ {
		f := s.Field(i)
		if f.Kind() != reflect.Pointer || f.IsNil() || !f.CanSet() {
			continue
		}
		switch e := f.Elem(); e.Kind() {
		case reflect.Float64, reflect.Int:
			if e.IsZero() {
				f.Set(reflect.Zero(f.Type()))
			}
		}
	}
}

// MarshalJSON writes project with "format_version" (see FormatVersion).
func (project VSMProject) MarshalJSON() ([]byte, error) {
	type plain VSMProject
	return json.Marshal(struct {
		FormatVersion int `json:"format_version"`
		plain
	}{FormatVersion, plain(project)})
}

// UnmarshalJSON reads project in the current or the legacy format (see
// FormatVersion).
func (project *VSMProject) UnmarshalJSON(data []byte) error {
	type plain VSMProject
	var w struct {
		FormatVersion int `json:"format_version"`
		plain
		CurrentState json.RawMessage `json:"current_state"`
		FutureState  json.RawMessage `json:"future_state"`
		IdealState   json.RawMessage `json:"ideal_state"`
	}
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*project = VSMProject(w.plain)
	current, err := decodeStreamRaw(w.CurrentState, w.FormatVersion)
	if err != nil {
		return fmt.Errorf("current_state: %w", err)
	}
	if current != nil {
		project.CurrentState = *current
	}
	if project.FutureState, err = decodeStreamRaw(w.FutureState, w.FormatVersion); err != nil {
		return fmt.Errorf("future_state: %w", err)
	}
	if project.IdealState, err = decodeStreamRaw(w.IdealState, w.FormatVersion); err != nil {
		return fmt.Errorf("ideal_state: %w", err)
	}
	return nil
}

// MarshalJSON writes result with "format_version" (see FormatVersion).
func (result DesignResult) MarshalJSON() ([]byte, error) {
	type plain DesignResult
	return json.Marshal(struct {
		FormatVersion int `json:"format_version"`
		plain
	}{FormatVersion, plain(result)})
}

// UnmarshalJSON reads result in the current or the legacy format (see
// FormatVersion).
func (result *DesignResult) UnmarshalJSON(data []byte) error {
	type plain DesignResult
	var w struct {
		FormatVersion int `json:"format_version"`
		plain
		FutureState json.RawMessage `json:"future_state"`
		IdealState  json.RawMessage `json:"ideal_state"`
	}
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*result = DesignResult(w.plain)
	var err error
	if result.FutureState, err = decodeStreamRaw(w.FutureState, w.FormatVersion); err != nil {
		return fmt.Errorf("future_state: %w", err)
	}
	if result.IdealState, err = decodeStreamRaw(w.IdealState, w.FormatVersion); err != nil {
		return fmt.Errorf("ideal_state: %w", err)
	}
	return nil
}

// MarshalJSON writes result with "format_version" (see FormatVersion).
func (result AssessmentResult) MarshalJSON() ([]byte, error) {
	type plain AssessmentResult
	return json.Marshal(struct {
		FormatVersion int `json:"format_version"`
		plain
	}{FormatVersion, plain(result)})
}

// UnmarshalJSON reads result in the current or the legacy format (see
// FormatVersion): in a legacy file, working_days_per_week 0 is not entered,
// a KPI target that is not 0 is kept, and a target of 0 counts only with
// "has_target": true (see FormatVersion).
func (result *AssessmentResult) UnmarshalJSON(data []byte) error {
	type plain AssessmentResult
	var w struct {
		FormatVersion int `json:"format_version"`
		plain
	}
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*result = AssessmentResult(w.plain)
	if w.FormatVersion >= FormatVersion {
		return nil
	}
	if num(result.WorkingDaysPerWeek) == 0 {
		result.WorkingDaysPerWeek = nil
	}
	var legacy struct {
		KPIs map[string]struct {
			HasTarget bool `json:"has_target"`
		} `json:"kpis"`
	}
	if err := json.Unmarshal(data, &legacy); err != nil {
		return err
	}
	for name, m := range result.KPIs {
		if m.Target != nil && *m.Target == 0 && !legacy.KPIs[name].HasTarget {
			m.Target = nil
			result.KPIs[name] = m
		}
	}
	return nil
}

// Serializer provides JSON serialization/deserialization for VSM objects
type Serializer struct {
	validator *Validator
}

// NewSerializer creates a new VSM serializer
func NewSerializer() *Serializer {
	return &Serializer{
		validator: NewValidator(),
	}
}

// SerializeValueStream serializes a value stream to JSON
func (s *Serializer) SerializeValueStream(vs *ValueStream) ([]byte, error) {
	if vs == nil {
		return nil, errors.New("value stream cannot be nil")
	}

	// Validate before serialization
	if err := s.validator.validateValueStream(vs, false); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return json.MarshalIndent(vs, "", "  ")
}

// DeserializeValueStream deserializes a value stream from JSON. Stored values
// that disagree with the values computed from their parts are listed in the
// result's ConsistencyIssues (see CheckConsistency); they do not fail the
// load. Nor does a time_unit the model cannot convert (Validator rejects
// it): it is kept as entered and reported (see validateValueStream).
func (s *Serializer) DeserializeValueStream(data []byte) (*ValueStream, error) {
	vs, _, err := s.DeserializeValueStreamWithIssues(data)
	return vs, err
}

// DeserializeValueStreamWithIssues is DeserializeValueStream that also
// returns the consistency issues (the same list as the result's
// ConsistencyIssues).
func (s *Serializer) DeserializeValueStreamWithIssues(data []byte) (*ValueStream, []ConsistencyIssue, error) {
	var vs ValueStream
	if err := json.Unmarshal(data, &vs); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal value stream: %w", err)
	}

	// Validate after deserialization
	if err := s.validator.validateValueStream(&vs, false); err != nil {
		return nil, nil, fmt.Errorf("validation failed: %w", err)
	}

	vs.ConsistencyIssues = CheckConsistency(&vs)
	return &vs, vs.ConsistencyIssues, nil
}

// SerializeVSMProject serializes a VSM project to JSON
func (s *Serializer) SerializeVSMProject(project *VSMProject) ([]byte, error) {
	if project == nil {
		return nil, errors.New("VSM project cannot be nil")
	}

	// Validate before serialization
	if err := s.validator.validateVSMProject(project, false); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return json.MarshalIndent(project, "", "  ")
}

// DeserializeVSMProject deserializes a VSM project from JSON. The
// ConsistencyIssues of the current, future and ideal states are recomputed
// (see CheckConsistency); any list stored in the JSON is replaced.
func (s *Serializer) DeserializeVSMProject(data []byte) (*VSMProject, error) {
	var project VSMProject
	if err := json.Unmarshal(data, &project); err != nil {
		return nil, fmt.Errorf("failed to unmarshal VSM project: %w", err)
	}

	// Validate after deserialization
	if err := s.validator.validateVSMProject(&project, false); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	refreshProjectIssues(&project)
	return &project, nil
}

// refreshProjectIssues recomputes the ConsistencyIssues of each state of
// project.
func refreshProjectIssues(project *VSMProject) {
	refreshStreamIssues(&project.CurrentState, project.FutureState, project.IdealState)
}

// refreshStreamIssues recomputes the ConsistencyIssues of each non-nil
// stream.
func refreshStreamIssues(streams ...*ValueStream) {
	for _, vs := range streams {
		if vs != nil {
			vs.ConsistencyIssues = CheckConsistency(vs)
		}
	}
}

// SerializeAnalysisResult serializes analysis results to JSON
func (s *Serializer) SerializeAnalysisResult(result *AnalysisResult) ([]byte, error) {
	if result == nil {
		return nil, errors.New("analysis result cannot be nil")
	}
	return json.MarshalIndent(result, "", "  ")
}

// DeserializeAnalysisResult deserializes analysis results from JSON
func (s *Serializer) DeserializeAnalysisResult(data []byte) (*AnalysisResult, error) {
	var result AnalysisResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal analysis result: %w", err)
	}
	return &result, nil
}

// SerializeDesignResult serializes design results to JSON
func (s *Serializer) SerializeDesignResult(result *DesignResult) ([]byte, error) {
	if result == nil {
		return nil, errors.New("design result cannot be nil")
	}
	return json.MarshalIndent(result, "", "  ")
}

// DeserializeDesignResult deserializes design results from JSON. The
// ConsistencyIssues of its future and ideal states are recomputed (see
// CheckConsistency).
func (s *Serializer) DeserializeDesignResult(data []byte) (*DesignResult, error) {
	var result DesignResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal design result: %w", err)
	}
	refreshStreamIssues(result.FutureState, result.IdealState)
	return &result, nil
}

// SerializePlanningResult serializes planning results to JSON
func (s *Serializer) SerializePlanningResult(result *PlanningResult) ([]byte, error) {
	if result == nil {
		return nil, errors.New("planning result cannot be nil")
	}
	return json.MarshalIndent(result, "", "  ")
}

// DeserializePlanningResult deserializes planning results from JSON
func (s *Serializer) DeserializePlanningResult(data []byte) (*PlanningResult, error) {
	var result PlanningResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal planning result: %w", err)
	}
	return &result, nil
}

// SerializeAssessmentResult serializes assessment results to JSON
func (s *Serializer) SerializeAssessmentResult(result *AssessmentResult) ([]byte, error) {
	if result == nil {
		return nil, errors.New("assessment result cannot be nil")
	}
	return json.MarshalIndent(result, "", "  ")
}

// DeserializeAssessmentResult deserializes assessment results from JSON
func (s *Serializer) DeserializeAssessmentResult(data []byte) (*AssessmentResult, error) {
	var result AssessmentResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal assessment result: %w", err)
	}
	return &result, nil
}

// SaveToFile saves any serializable object to a JSON file
func (s *Serializer) SaveToFile(data interface{}, filename string) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	if err := os.WriteFile(filename, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// LoadFromFile loads any serializable object from a JSON file. When target
// is a *ValueStream, *VSMProject or *DesignResult, the ConsistencyIssues of
// each value stream in it are recomputed (see CheckConsistency).
func (s *Serializer) LoadFromFile(filename string, target interface{}) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("failed to unmarshal data: %w", err)
	}

	switch t := target.(type) {
	case *ValueStream:
		t.ConsistencyIssues = CheckConsistency(t)
	case *VSMProject:
		refreshProjectIssues(t)
	case *DesignResult:
		refreshStreamIssues(t.FutureState, t.IdealState)
	}
	return nil
}

// LoadValueStreamFromFile loads a value stream from a JSON file and returns
// it with its consistency issues (also in its ConsistencyIssues).
func (s *Serializer) LoadValueStreamFromFile(filename string) (*ValueStream, []ConsistencyIssue, error) {
	var vs ValueStream
	if err := s.LoadFromFile(filename, &vs); err != nil {
		return nil, nil, err
	}
	return &vs, vs.ConsistencyIssues, nil
}

// ExportValueStreamMap exports a value stream as a text-based map
// representation.
//
// Every number shown is the value the model computes (see
// Calculator.ResolveParametersInStream and CalculateKPIs), not a stored
// total or stored KPI, so the text never mixes resolved values with stale
// ones. Stored KPIs that the library does not compute are listed separately
// as stored. When stored values disagree with the computed ones, the map
// ends with the consistency issues (see CheckConsistency), and when values
// that stream totals need are missing, with the missing data (the KPIs that
// need them are not shown).
func (s *Serializer) ExportValueStreamMap(vs *ValueStream) (string, error) {
	if vs == nil {
		return "", errors.New("value stream cannot be nil")
	}
	calc := NewCalculator()
	kpis := calc.CalculateKPIs(vs)
	timeUnit := string(calc.ReferenceTimeUnit(vs.Processes))
	if timeUnit == "" {
		timeUnit = "time units"
	}

	mapStr := fmt.Sprintf("Value Stream Map: %s\n", vs.Name)
	mapStr += fmt.Sprintf("Product Family: %s\n", vs.ProductFamily)
	mapStr += fmt.Sprintf("State: %s\n", vs.State)
	mapStr += "═" + "═══════════════════════════════════════════════\n\n"

	// Customer
	if vs.Customer != nil {
		period := string(vs.Customer.DemandPeriod)
		if period == "" {
			period = "period"
		}
		mapStr += fmt.Sprintf("Customer: %s\n", vs.Customer.Name)
		mapStr += fmt.Sprintf("Demand: %s\n", optional(vs.Customer.Demand, "%.2f units/"+period))
		if w := vs.Customer.WorkingDaysPerWeek; w != nil {
			mapStr += fmt.Sprintf("Working Days: %g working days/week (inventory ranges are calendar time)\n", *w)
		}
		if a := vs.Customer.AvailableTime; a != nil {
			mapStr += fmt.Sprintf("Available Time: %.2f %s/%s\n", *a, timeUnit, period)
		}
		// Same takt the KPIs use: AvailableTime ÷ Demand, else stored TaktTime.
		var takt *float64
		if v, ok := kpis["customer_takt"]; ok {
			takt = &v
		}
		mapStr += fmt.Sprintf("Takt Time: %s\n\n", optional(takt, "%.2f "+timeUnit))
	}

	// Supplier
	if vs.Supplier != nil {
		mapStr += fmt.Sprintf("Supplier: %s\n", vs.Supplier.Name)
		mapStr += fmt.Sprintf("Delivery Time: %.2f time units\n\n", vs.Supplier.DeliveryTime)
	}

	// Processes
	mapStr += "Processes:\n"
	for i, process := range vs.Processes {
		// Resolved values: totals computed from their parts.
		params := calc.ResolveParametersInStream(process.Parameters, vs)
		unit := string(params.TimeUnit)
		if unit == "" {
			unit = timeUnit
		}
		mapStr += fmt.Sprintf("  %d. %s (%s)\n", i+1, process.Name, process.Type)
		mapStr += fmt.Sprintf("     Process Time: %s\n", optional(params.ProcessTime, "%.2f "+unit))
		mapStr += fmt.Sprintf("     Cycle Time: %s\n", optional(params.CycleTime, "%.2f "+unit))
		q := process.Parameters
		switch {
		case params.YieldFactor != nil:
			mapStr += fmt.Sprintf("     Yield Factor: %.2f%%\n", *params.YieldFactor*100)
		case defectsImpossible(q):
			mapStr += "     Yield Factor: unknown (scrap + rework above 100%)\n"
		case q.YieldFactor != nil || q.ScrapRate != nil || q.ReworkRate != nil:
			mapStr += "     Yield Factor: unknown (impossible quality values)\n"
		default:
			mapStr += "     Yield Factor: not given\n"
		}

		if r := process.Parameters.ScrapRate; r != nil {
			mapStr += fmt.Sprintf("     Scrap Rate: %.2f%%\n", *r*100)
		}
		if r := process.Parameters.ReworkRate; r != nil {
			mapStr += fmt.Sprintf("     Rework Rate: %.2f%%\n", *r*100)
		}
		mapStr += "\n"
	}

	// Inventory: range is quantity ÷ customer demand rate when that is
	// known; the time each inventory adds to the lead time is the one the
	// KPIs count (stream figures, totals.go).
	if len(vs.Inventory) > 0 {
		ctx := calc.contextOf(vs)
		figures := calc.figures(vs)
		mapStr += "Inventory:\n"
		for i, inv := range vs.Inventory {
			inf := figures.inventories[i]
			r := inv.Range
			if derived, ok := ctx.inventoryRange(inv); ok {
				r = &derived
			}
			if inf.counted {
				r = inf.time
			}
			rangeText := "range not given"
			switch {
			case r != nil:
				rangeText = fmt.Sprintf("%.2f %s range", *r, timeUnit)
			case inf.counted:
				rangeText = "range unknown"
			}
			mapStr += fmt.Sprintf("  %s: %s (%s)", inv.Name, optional(inv.Quantity, "%.2f units"), rangeText)
			if holder, linked := stockHolder(vs, inv); linked {
				if inf.counted {
					mapStr += fmt.Sprintf(" — stock of process %s, whose waiting time is not known, so this "+
						"range is counted", holder.ID)
				} else {
					mapStr += fmt.Sprintf(" — stock of process %s, counted in its waiting time", holder.ID)
				}
			}
			mapStr += "\n"
		}
		mapStr += "\n"
	}

	// Flow Elements
	if len(vs.FlowElements) > 0 {
		mapStr += "Flow Control Elements:\n"
		for _, element := range vs.FlowElements {
			capacity := "to be sized"
			if element.Capacity != nil {
				capacity = fmt.Sprintf("%.2f", *element.Capacity)
			}
			mapStr += fmt.Sprintf("  %s (%s): Capacity %s\n", element.ID, element.Type, capacity)
		}
		mapStr += "\n"
	}

	// KPIs (computed; sorted by name; ratios shown as percent)
	if len(kpis) > 0 {
		mapStr += "Key Performance Indicators:\n"
		for _, key := range sortedKeys(kpis) {
			mapStr += fmt.Sprintf("  %s: %s\n", key, formatKPIValue(key, kpis[key]))
		}
		mapStr += "\n"
	}
	other := map[string]float64{}
	for key, value := range vs.KPIs {
		if _, computed := kpis[key]; !computed {
			other[key] = value
		}
	}
	if len(other) > 0 {
		mapStr += "Other KPIs (as stored; not computed by the library):\n"
		for _, key := range sortedKeys(other) {
			mapStr += fmt.Sprintf("  %s: %s\n", key, formatKPIValue(key, other[key]))
		}
		mapStr += "\n"
	}

	// Consistency issues: contradictions, then missing data.
	var contradictions, missing []ConsistencyIssue
	for _, issue := range CheckConsistency(vs) {
		if issue.Kind == IssueMissingData {
			missing = append(missing, issue)
		} else {
			contradictions = append(contradictions, issue)
		}
	}
	if len(contradictions) > 0 {
		mapStr += fmt.Sprintf("Consistency issues (%d): stored values that disagree with the computed "+
			"values shown above, or are impossible\n", len(contradictions))
		for _, issue := range contradictions {
			mapStr += fmt.Sprintf("  - %s\n", issue.Message)
		}
		mapStr += "\n"
	}
	if len(missing) > 0 {
		mapStr += fmt.Sprintf("Missing data (%d): KPIs that need these values are not shown\n", len(missing))
		for _, issue := range missing {
			mapStr += fmt.Sprintf("  - %s\n", issue.Message)
		}
		mapStr += "\n"
	}

	return mapStr, nil
}

// optional formats *v with format, or "not given" when v is nil.
func optional(v *float64, format string) string {
	if v == nil {
		return "not given"
	}
	return fmt.Sprintf(format, *v)
}

// sortedKeys returns m's keys in order.
func sortedKeys(m map[string]float64) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// ImportFromCSV imports basic value stream data from CSV format
// This is a simplified implementation for basic process data
func (s *Serializer) ImportFromCSV(csvData string) (*ValueStream, error) {
	// This would implement CSV parsing for basic VSM data
	// For now, return an error indicating this feature needs implementation
	return nil, errors.New("CSV import not yet implemented - use JSON format instead")
}

// Validator provides validation for VSM objects
type Validator struct{}

// NewValidator creates a new VSM validator
func NewValidator() *Validator {
	return &Validator{}
}

// ValidateValueStream validates a value stream. A process time_unit the
// model cannot convert (see ValidateProcess) is an error.
func (v *Validator) ValidateValueStream(vs *ValueStream) error {
	return v.validateValueStream(vs, true)
}

// validateValueStream is ValidateValueStream; with checkUnits false an
// unsupported time_unit is not an error. The Serializer loads and saves
// such streams (main's format accepted any unit string): the unit is kept
// as entered, CheckConsistency reports it as a contradiction, and the
// process's times count as unknown in the stream totals.
func (v *Validator) validateValueStream(vs *ValueStream, checkUnits bool) error {
	if vs == nil {
		return errors.New("value stream cannot be nil")
	}

	if vs.ID == "" {
		return errors.New("value stream ID cannot be empty")
	}

	if vs.Name == "" {
		return errors.New("value stream name cannot be empty")
	}

	if len(vs.Processes) == 0 {
		return errors.New("value stream must have at least one process")
	}

	// Validate processes
	for i, process := range vs.Processes {
		if err := v.validateProcess(&process, checkUnits); err != nil {
			return fmt.Errorf("process %d validation failed: %w", i, err)
		}
	}

	if vs.Customer != nil && !workingDaysValid(vs.Customer.WorkingDaysPerWeek) {
		return fmt.Errorf("customer working_days_per_week %g must be between 1 and 7 (or not entered for 7)",
			*vs.Customer.WorkingDaysPerWeek)
	}

	// Validate inventory
	for i, inv := range vs.Inventory {
		if err := v.ValidateInventory(&inv); err != nil {
			return fmt.Errorf("inventory %d validation failed: %w", i, err)
		}
	}

	return nil
}

// ValidateProcess validates a process. A time_unit that is not supported
// (see isKnownTimeUnit; empty is the stream's reference unit) is an error.
func (v *Validator) ValidateProcess(process *Process) error {
	return v.validateProcess(process, true)
}

// validateProcess is ValidateProcess; with checkUnits false the time_unit
// is not checked (see validateValueStream).
func (v *Validator) validateProcess(process *Process, checkUnits bool) error {
	if process.ID == "" {
		return errors.New("process ID cannot be empty")
	}

	if process.Name == "" {
		return errors.New("process name cannot be empty")
	}

	if pt := process.Parameters.ProcessTime; pt != nil && *pt < 0 {
		return errors.New("process time cannot be negative")
	}

	if ct := process.Parameters.CycleTime; ct != nil && *ct < 0 {
		return errors.New("cycle time cannot be negative")
	}

	if u := process.Parameters.TimeUnit; checkUnits && !unitSupported(u) {
		return fmt.Errorf("time unit %q is not supported (use seconds, minutes, hours, days or weeks, "+
			"or leave it empty for the stream's reference unit)", u)
	}

	return nil
}

// ValidateInventory validates inventory
func (v *Validator) ValidateInventory(inv *Inventory) error {
	if inv.ID == "" {
		return errors.New("inventory ID cannot be empty")
	}

	if inv.Quantity != nil && *inv.Quantity < 0 {
		return errors.New("inventory quantity cannot be negative")
	}

	if inv.Range != nil && *inv.Range < 0 {
		return errors.New("inventory range cannot be negative")
	}

	return nil
}

// ValidateVSMProject validates a VSM project (see ValidateValueStream).
func (v *Validator) ValidateVSMProject(project *VSMProject) error {
	return v.validateVSMProject(project, true)
}

// validateVSMProject is ValidateVSMProject; checkUnits as for
// validateValueStream.
func (v *Validator) validateVSMProject(project *VSMProject, checkUnits bool) error {
	if project == nil {
		return errors.New("VSM project cannot be nil")
	}

	if project.ID == "" {
		return errors.New("project ID cannot be empty")
	}

	if project.Name == "" {
		return errors.New("project name cannot be empty")
	}

	if err := v.validateValueStream(&project.CurrentState, checkUnits); err != nil {
		return fmt.Errorf("current state validation failed: %w", err)
	}

	if project.FutureState != nil {
		if err := v.validateValueStream(project.FutureState, checkUnits); err != nil {
			return fmt.Errorf("future state validation failed: %w", err)
		}
	}

	if project.IdealState != nil {
		if err := v.validateValueStream(project.IdealState, checkUnits); err != nil {
			return fmt.Errorf("ideal state validation failed: %w", err)
		}
	}

	return nil
}
