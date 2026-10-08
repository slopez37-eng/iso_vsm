package vsm

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
)

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
	if err := s.validator.ValidateValueStream(vs); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return json.MarshalIndent(vs, "", "  ")
}

// DeserializeValueStream deserializes a value stream from JSON. Stored values
// that disagree with the values computed from their parts are listed in the
// result's ConsistencyIssues (see CheckConsistency); they do not fail the
// load.
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
	if err := s.validator.ValidateValueStream(&vs); err != nil {
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
	if err := s.validator.ValidateVSMProject(project); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return json.MarshalIndent(project, "", "  ")
}

// DeserializeVSMProject deserializes a VSM project from JSON
func (s *Serializer) DeserializeVSMProject(data []byte) (*VSMProject, error) {
	var project VSMProject
	if err := json.Unmarshal(data, &project); err != nil {
		return nil, fmt.Errorf("failed to unmarshal VSM project: %w", err)
	}

	// Validate after deserialization
	if err := s.validator.ValidateVSMProject(&project); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return &project, nil
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

// DeserializeDesignResult deserializes design results from JSON
func (s *Serializer) DeserializeDesignResult(data []byte) (*DesignResult, error) {
	var result DesignResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal design result: %w", err)
	}
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
// is a *ValueStream or *VSMProject, the ConsistencyIssues of each value
// stream are filled in (see CheckConsistency).
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
		for _, vs := range []*ValueStream{&t.CurrentState, t.FutureState, t.IdealState} {
			if vs != nil {
				vs.ConsistencyIssues = CheckConsistency(vs)
			}
		}
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
// ends with the consistency issues (see CheckConsistency).
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
		mapStr += fmt.Sprintf("Demand: %.2f units/%s\n", vs.Customer.Demand, period)
		if vs.Customer.AvailableTime > 0 {
			mapStr += fmt.Sprintf("Available Time: %.2f %s/%s\n", vs.Customer.AvailableTime, timeUnit, period)
		}
		// Same takt the KPIs use: AvailableTime ÷ Demand, else stored TaktTime.
		takt := vs.Customer.TaktTime
		if v, ok := kpis["customer_takt"]; ok {
			takt = v
		}
		mapStr += fmt.Sprintf("Takt Time: %.2f %s\n\n", takt, timeUnit)
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
		mapStr += fmt.Sprintf("     Process Time: %.2f %s\n", params.ProcessTime, unit)
		mapStr += fmt.Sprintf("     Cycle Time: %.2f %s\n", params.CycleTime, unit)
		if yieldKnown(process.Parameters) {
			mapStr += fmt.Sprintf("     Yield Factor: %.2f%%\n", params.YieldFactor*100)
		} else {
			mapStr += "     Yield Factor: not given\n"
		}

		if process.Parameters.ScrapRate > 0 {
			mapStr += fmt.Sprintf("     Scrap Rate: %.2f%%\n", process.Parameters.ScrapRate*100)
		}
		if process.Parameters.ReworkRate > 0 {
			mapStr += fmt.Sprintf("     Rework Rate: %.2f%%\n", process.Parameters.ReworkRate*100)
		}
		mapStr += "\n"
	}

	// Inventory: range is quantity ÷ customer demand rate when that is known.
	if len(vs.Inventory) > 0 {
		ctx := calc.contextOf(vs)
		mapStr += "Inventory:\n"
		for _, inv := range vs.Inventory {
			invRange := inv.Range
			if r, ok := ctx.inventoryRange(inv); ok {
				invRange = r
			}
			mapStr += fmt.Sprintf("  %s: %.2f units (%.2f %s range)\n", inv.Name, inv.Quantity, invRange, timeUnit)
		}
		mapStr += "\n"
	}

	// Flow Elements
	if len(vs.FlowElements) > 0 {
		mapStr += "Flow Control Elements:\n"
		for _, element := range vs.FlowElements {
			mapStr += fmt.Sprintf("  %s (%s): Capacity %.2f\n", element.ID, element.Type, element.Capacity)
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

	// Consistency issues
	if issues := CheckConsistency(vs); len(issues) > 0 {
		mapStr += fmt.Sprintf("Consistency issues (%d): stored values that disagree with the computed "+
			"values shown above\n", len(issues))
		for _, issue := range issues {
			mapStr += fmt.Sprintf("  - %s\n", issue.Message)
		}
		mapStr += "\n"
	}

	return mapStr, nil
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

// ValidateValueStream validates a value stream
func (v *Validator) ValidateValueStream(vs *ValueStream) error {
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
		if err := v.ValidateProcess(&process); err != nil {
			return fmt.Errorf("process %d validation failed: %w", i, err)
		}
	}

	// Validate inventory
	for i, inv := range vs.Inventory {
		if err := v.ValidateInventory(&inv); err != nil {
			return fmt.Errorf("inventory %d validation failed: %w", i, err)
		}
	}

	return nil
}

// ValidateProcess validates a process
func (v *Validator) ValidateProcess(process *Process) error {
	if process.ID == "" {
		return errors.New("process ID cannot be empty")
	}

	if process.Name == "" {
		return errors.New("process name cannot be empty")
	}

	if process.Parameters.ProcessTime < 0 {
		return errors.New("process time cannot be negative")
	}

	if process.Parameters.CycleTime < 0 {
		return errors.New("cycle time cannot be negative")
	}

	return nil
}

// ValidateInventory validates inventory
func (v *Validator) ValidateInventory(inv *Inventory) error {
	if inv.ID == "" {
		return errors.New("inventory ID cannot be empty")
	}

	if inv.Quantity < 0 {
		return errors.New("inventory quantity cannot be negative")
	}

	if inv.Range < 0 {
		return errors.New("inventory range cannot be negative")
	}

	return nil
}

// ValidateVSMProject validates a VSM project
func (v *Validator) ValidateVSMProject(project *VSMProject) error {
	if project == nil {
		return errors.New("VSM project cannot be nil")
	}

	if project.ID == "" {
		return errors.New("project ID cannot be empty")
	}

	if project.Name == "" {
		return errors.New("project name cannot be empty")
	}

	if err := v.ValidateValueStream(&project.CurrentState); err != nil {
		return fmt.Errorf("current state validation failed: %w", err)
	}

	if project.FutureState != nil {
		if err := v.ValidateValueStream(project.FutureState); err != nil {
			return fmt.Errorf("future state validation failed: %w", err)
		}
	}

	if project.IdealState != nil {
		if err := v.ValidateValueStream(project.IdealState); err != nil {
			return fmt.Errorf("ideal state validation failed: %w", err)
		}
	}

	return nil
}
