package vsm

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
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

// DeserializeValueStream deserializes a value stream from JSON
func (s *Serializer) DeserializeValueStream(data []byte) (*ValueStream, error) {
	var vs ValueStream
	if err := json.Unmarshal(data, &vs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal value stream: %w", err)
	}

	// Validate after deserialization
	if err := s.validator.ValidateValueStream(&vs); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return &vs, nil
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

// LoadFromFile loads any serializable object from a JSON file
func (s *Serializer) LoadFromFile(filename string, target interface{}) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("failed to unmarshal data: %w", err)
	}

	return nil
}

// ExportValueStreamMap exports a value stream as a text-based map representation
func (s *Serializer) ExportValueStreamMap(vs *ValueStream) (string, error) {
	if vs == nil {
		return "", errors.New("value stream cannot be nil")
	}

	mapStr := fmt.Sprintf("Value Stream Map: %s\n", vs.Name)
	mapStr += fmt.Sprintf("Product Family: %s\n", vs.ProductFamily)
	mapStr += fmt.Sprintf("State: %s\n", vs.State)
	mapStr += "═" + "═══════════════════════════════════════════════\n\n"

	// Customer
	if vs.Customer != nil {
		calc := NewCalculator()
		timeUnit := string(calc.ReferenceTimeUnit(vs.Processes))
		if timeUnit == "" {
			timeUnit = "time units"
		}
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
		if v, ok := calc.CalculateKPIs(vs)["customer_takt"]; ok {
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
		mapStr += fmt.Sprintf("  %d. %s (%s)\n", i+1, process.Name, process.Type)
		mapStr += fmt.Sprintf("     Process Time: %.2f %s\n", process.Parameters.ProcessTime, process.Parameters.TimeUnit)
		mapStr += fmt.Sprintf("     Cycle Time: %.2f %s\n", process.Parameters.CycleTime, process.Parameters.TimeUnit)
		mapStr += fmt.Sprintf("     Yield Factor: %.2f%%\n", process.Parameters.YieldFactor*100)

		if process.Parameters.ScrapRate > 0 {
			mapStr += fmt.Sprintf("     Scrap Rate: %.2f%%\n", process.Parameters.ScrapRate*100)
		}
		if process.Parameters.ReworkRate > 0 {
			mapStr += fmt.Sprintf("     Rework Rate: %.2f%%\n", process.Parameters.ReworkRate*100)
		}
		mapStr += "\n"
	}

	// Inventory
	if len(vs.Inventory) > 0 {
		mapStr += "Inventory:\n"
		for _, inv := range vs.Inventory {
			mapStr += fmt.Sprintf("  %s: %.2f units (%.2f time units range)\n", inv.Name, inv.Quantity, inv.Range)
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

	// KPIs
	if len(vs.KPIs) > 0 {
		mapStr += "Key Performance Indicators:\n"
		for key, value := range vs.KPIs {
			mapStr += fmt.Sprintf("  %s: %.4f\n", key, value)
		}
		mapStr += "\n"
	}

	return mapStr, nil
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
