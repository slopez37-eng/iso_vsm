package vsm

import (
	"errors"
	"fmt"
	"time"
)

// DesignResult contains the results of the value stream design phase
type DesignResult struct {
	ValueStreamID          string                  `json:"value_stream_id"`
	ImprovementPotentials  []ImprovementPotential  `json:"improvement_potentials"`
	IdealState             *ValueStream            `json:"ideal_state"`
	FutureState            *ValueStream            `json:"future_state"`
	DesignPrinciples       []DesignPrinciple       `json:"design_principles"`
	ContinuousImprovements []ContinuousImprovement `json:"continuous_improvements"`
	DesignedAt             time.Time               `json:"designed_at"`
}

// ImprovementPotential represents a potential improvement in the value stream
type ImprovementPotential struct {
	ID          string  `json:"id"`
	Type        string  `json:"type"`     // "bottleneck", "waste", "flow", "quality", etc.
	Location    string  `json:"location"` // Process or area where improvement applies
	Description string  `json:"description"`
	Priority    int     `json:"priority"` // 1=high, 2=medium, 3=low
	Impact      float64 `json:"impact"`   // Estimated improvement impact (0-1)
	Effort      float64 `json:"effort"`   // Estimated effort required (0-1)
}

// DesignPrinciple represents the 8 VSM design principles from Rother & Shook
type DesignPrinciple struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Applied     bool   `json:"applied"`
	Status      string `json:"status"` // "not_applicable", "planned", "implemented"
}

// ContinuousImprovement represents continuous improvement flashes
type ContinuousImprovement struct {
	ID          string    `json:"id"`
	ProcessID   string    `json:"process_id"`
	Description string    `json:"description"`
	Type        string    `json:"type"` // "kaizen", "quick_win", "major_change"
	Priority    int       `json:"priority"`
	Status      string    `json:"status"` // "identified", "planned", "implemented"
	CreatedAt   time.Time `json:"created_at"`
}

// Designer provides methods for designing improved value streams
type Designer struct {
	calculator *Calculator
	analyzer   *Analyzer
}

// NewDesigner creates a new VSM designer
func NewDesigner() *Designer {
	return &Designer{
		calculator: NewCalculator(),
		analyzer:   NewAnalyzer(),
	}
}

// DesignFutureState performs the complete design phase for a value stream
func (d *Designer) DesignFutureState(current *ValueStream, analysis *AnalysisResult) (*DesignResult, error) {
	if current == nil {
		return nil, errors.New("current value stream cannot be nil")
	}

	result := &DesignResult{
		ValueStreamID: current.ID,
		DesignedAt:    time.Now(),
	}

	// Identify improvement potentials
	potentials, err := d.IdentifyImprovementPotentials(current, analysis)
	if err != nil {
		return nil, fmt.Errorf("failed to identify improvement potentials: %w", err)
	}
	result.ImprovementPotentials = potentials

	// Create ideal state
	idealState, err := d.CreateIdealState(current)
	if err != nil {
		return nil, fmt.Errorf("failed to create ideal state: %w", err)
	}
	result.IdealState = idealState

	// Design future state based on ideal state and practical constraints
	futureState, err := d.DesignFutureStateFromIdeal(idealState, current)
	if err != nil {
		return nil, fmt.Errorf("failed to design future state: %w", err)
	}
	result.FutureState = futureState

	// Apply VSM design principles
	principles := d.ApplyDesignPrinciples(current, futureState)
	result.DesignPrinciples = principles

	// Identify continuous improvements
	improvements := d.IdentifyContinuousImprovements(current, analysis)
	result.ContinuousImprovements = improvements

	return result, nil
}

// IdentifyImprovementPotentials identifies potential improvements based on analysis
func (d *Designer) IdentifyImprovementPotentials(vs *ValueStream, analysis *AnalysisResult) ([]ImprovementPotential, error) {
	var potentials []ImprovementPotential

	// Based on bottlenecks
	for _, bottleneck := range analysis.Bottlenecks {
		potentials = append(potentials, ImprovementPotential{
			ID:          fmt.Sprintf("bottleneck_%s", bottleneck.ProcessID),
			Type:        "bottleneck",
			Location:    bottleneck.ProcessName,
			Description: fmt.Sprintf("Optimize bottleneck process: %s", bottleneck.Description),
			Priority:    d.calculatePriorityFromUtilization(bottleneck.Utilization),
			Impact:      bottleneck.Utilization - 0.7, // Improvement needed to reach 70% utilization
			Effort:      0.6,                          // Medium effort
		})
	}

	// Based on waste analysis
	for wasteType, amount := range analysis.WasteAnalysis.WasteByType {
		if amount > 0 {
			potentials = append(potentials, ImprovementPotential{
				ID:          fmt.Sprintf("waste_%s", wasteType),
				Type:        "waste",
				Location:    "Value Stream",
				Description: fmt.Sprintf("Reduce %s waste (%.2f time units)", wasteType, amount),
				Priority:    2,
				Impact:      d.calculateWasteImpact(wasteType),
				Effort:      0.4,
			})
		}
	}

	// Based on flow analysis
	if analysis.FlowAnalysis.FlowType == FlowTypePush {
		potentials = append(potentials, ImprovementPotential{
			ID:          "flow_push_to_pull",
			Type:        "flow",
			Location:    "Value Stream",
			Description: "Transition from push to pull system",
			Priority:    1,
			Impact:      0.8,
			Effort:      0.8,
		})
	}

	for _, interruption := range analysis.FlowAnalysis.FlowInterruptions {
		potentials = append(potentials, ImprovementPotential{
			ID:          fmt.Sprintf("flow_%s_%s", interruption.Type, interruption.Location),
			Type:        "flow",
			Location:    interruption.Location,
			Description: interruption.Description,
			Priority:    2,
			Impact:      interruption.Impact / 100, // Normalize impact
			Effort:      0.5,
		})
	}

	// Sort by priority and impact
	for i := range potentials {
		potentials[i].Priority = d.calculateOverallPriority(potentials[i])
	}

	return potentials, nil
}

// calculatePriorityFromUtilization calculates priority based on utilization
func (d *Designer) calculatePriorityFromUtilization(utilization float64) int {
	if utilization > 0.9 {
		return 1 // High priority
	} else if utilization > 0.8 {
		return 2 // Medium priority
	}
	return 3 // Low priority
}

// calculateWasteImpact calculates the impact of reducing specific waste types
func (d *Designer) calculateWasteImpact(wasteType string) float64 {
	switch wasteType {
	case "transportation":
		return 0.3
	case "inventory":
		return 0.4
	case "waiting":
		return 0.5
	case "defects":
		return 0.6
	case "over_processing":
		return 0.4
	case "over_production":
		return 0.3
	case "motion":
		return 0.2
	default:
		return 0.3
	}
}

// calculateOverallPriority calculates overall priority considering impact and effort
func (d *Designer) calculateOverallPriority(potential ImprovementPotential) int {
	// Simple priority calculation: high impact + low effort = high priority
	score := potential.Impact / potential.Effort

	if score > 1.5 {
		return 1
	} else if score > 0.8 {
		return 2
	}
	return 3
}

// CreateIdealState creates an ideal (waste-free) value stream state
func (d *Designer) CreateIdealState(current *ValueStream) (*ValueStream, error) {
	ideal := &ValueStream{
		ID:            current.ID + "_ideal",
		Name:          current.Name + " (Ideal State)",
		State:         StateIdeal,
		ProductFamily: current.ProductFamily,
		Description:   "Waste-free ideal state with minimal lead time",
		Customer:      current.Customer,
		Supplier:      current.Supplier,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	// In ideal state, all processes are perfectly balanced
	// Lead time = Value-adding time only (no waste)
	var idealProcesses []Process

	for _, process := range current.Processes {
		idealProcess := process
		idealProcess.ID = process.ID + "_ideal"
		idealProcess.Name = process.Name + " (Ideal)"

		// In ideal state: only value-adding time remains
		idealProcess.Parameters.ProcessTime = process.Parameters.ValueAddingTime
		idealProcess.Parameters.IdleTime = 0
		idealProcess.Parameters.WaitingTime = 0
		idealProcess.Parameters.TransportTime = 0
		idealProcess.Parameters.StorageTime = 0
		idealProcess.Parameters.NonValueAddingTime = 0
		idealProcess.Parameters.NecessaryNonValueAddingTime = 0
		idealProcess.Parameters.ScrapRate = 0
		idealProcess.Parameters.ReworkRate = 0
		idealProcess.Parameters.YieldFactor = 1.0

		idealProcesses = append(idealProcesses, idealProcess)
	}

	ideal.Processes = idealProcesses

	// No inventory in ideal state (continuous flow)
	ideal.Inventory = []Inventory{}

	// Calculate ideal KPIs
	ideal.KPIs = d.calculator.CalculateKPIs(ideal)

	return ideal, nil
}

// DesignFutureStateFromIdeal creates a practical future state based on the ideal state
func (d *Designer) DesignFutureStateFromIdeal(ideal, current *ValueStream) (*ValueStream, error) {
	future := &ValueStream{
		ID:            current.ID + "_future",
		Name:          current.Name + " (Future State)",
		State:         StateFuture,
		ProductFamily: current.ProductFamily,
		Description:   "Improved future state based on ideal state principles",
		Customer:      current.Customer,
		Supplier:      current.Supplier,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	// Apply VSM principles to create future state
	var futureProcesses []Process

	for _, process := range current.Processes {
		futureProcess := process
		futureProcess.ID = process.ID + "_future"
		futureProcess.Name = process.Name + " (Future)"

		// Reduce waste by 50-70% compared to current state
		futureProcess.Parameters.ProcessTime = process.Parameters.ProcessTime * 0.6
		futureProcess.Parameters.IdleTime = process.Parameters.IdleTime * 0.3
		futureProcess.Parameters.WaitingTime = process.Parameters.WaitingTime * 0.4
		futureProcess.Parameters.TransportTime = process.Parameters.TransportTime * 0.5
		futureProcess.Parameters.StorageTime = process.Parameters.StorageTime * 0.5
		futureProcess.Parameters.NonValueAddingTime = process.Parameters.NonValueAddingTime * 0.4
		futureProcess.Parameters.NecessaryNonValueAddingTime = process.Parameters.NecessaryNonValueAddingTime * 0.6
		futureProcess.Parameters.ScrapRate = process.Parameters.ScrapRate * 0.3
		futureProcess.Parameters.ReworkRate = process.Parameters.ReworkRate * 0.4

		// Improve yield factor
		if process.Parameters.YieldFactor < 0.95 {
			futureProcess.Parameters.YieldFactor = 0.95
		}

		futureProcesses = append(futureProcesses, futureProcess)
	}

	future.Processes = futureProcesses

	// Implement supermarket for pull system
	if len(current.Inventory) > 0 {
		supermarket := Inventory{
			ID:          "supermarket_1",
			Name:        "Supermarket",
			Quantity:    current.Customer.Demand * 0.1, // Small buffer
			Range:       0.5,                           // Half day coverage
			Type:        "supermarket",
			Description: "Pull system supermarket",
		}
		future.Inventory = []Inventory{supermarket}
	}

	// Add flow control elements
	future.FlowElements = []FlowElement{
		{
			ID:          "fifo_lane_1",
			Type:        "fifo",
			Capacity:    10,
			Description: "FIFO lane between processes",
		},
		{
			ID:             "kanban_1",
			Type:           "kanban",
			Capacity:       5,
			ControlTickets: []string{"ticket_1", "ticket_2", "ticket_3", "ticket_4", "ticket_5"},
			Description:    "Kanban control system",
		},
	}

	// Calculate future KPIs
	future.KPIs = d.calculator.CalculateKPIs(future)

	return future, nil
}

// ApplyDesignPrinciples applies the 8 VSM design principles from Rother & Shook
func (d *Designer) ApplyDesignPrinciples(current, future *ValueStream) []DesignPrinciple {
	principles := []DesignPrinciple{
		{
			ID:          "takt_time",
			Name:        "Takt time at pacemaker process",
			Description: "Customer takt time determines pace of pacemaker process",
			Applied:     d.checkTaktTimePrinciple(future),
			Status:      "planned",
		},
		{
			ID:          "supermarket_direct",
			Name:        "Supermarket or direct shipping",
			Description: "Use supermarket for upstream processes, direct shipping for downstream",
			Applied:     d.checkSupermarketPrinciple(future),
			Status:      "planned",
		},
		{
			ID:          "continuous_flow",
			Name:        "Continuous product flow",
			Description: "Minimize interruptions in product flow between processes",
			Applied:     d.checkContinuousFlowPrinciple(future),
			Status:      "planned",
		},
		{
			ID:          "pull_system",
			Name:        "Supermarket pull systems",
			Description: "Use pull systems to control production at supermarket locations",
			Applied:     d.checkPullSystemPrinciple(future),
			Status:      "planned",
		},
		{
			ID:          "pacemaker_definition",
			Name:        "Definition of pacemaker process",
			Description: "Clearly define which process sets the pace for the entire value stream",
			Applied:     d.checkPacemakerPrinciple(future),
			Status:      "planned",
		},
		{
			ID:          "leveling",
			Name:        "Levelling of product mix",
			Description: "Level product mix at pacemaker process to smooth production",
			Applied:     false, // Would need more detailed analysis
			Status:      "not_applicable",
		},
		{
			ID:          "pacemaker_release",
			Name:        "Release at pacemaker process",
			Description: "Release work at pacemaker process based on customer demand",
			Applied:     d.checkReleasePrinciple(future),
			Status:      "planned",
		},
		{
			ID:          "continuous_improvement",
			Name:        "Further process improvements",
			Description: "Continuous improvement of all processes using CIP",
			Applied:     true, // Always applicable
			Status:      "planned",
		},
	}

	// Update status based on application
	for i := range principles {
		if principles[i].Applied {
			principles[i].Status = "implemented"
		}
	}

	return principles
}

// checkTaktTimePrinciple checks if takt time principle is applied. Takt is
// the customer_takt KPI (AvailableTime ÷ Demand for the same demand period,
// else the stored TaktTime), compared with cycle times in the same
// reference time unit.
func (d *Designer) checkTaktTimePrinciple(vs *ValueStream) bool {
	takt, ok := d.calculator.CalculateKPIs(vs)["customer_takt"]
	if !ok || takt <= 0 {
		return false
	}
	unit := d.calculator.ReferenceTimeUnit(vs.Processes)
	// Check if any process has cycle time close to customer takt
	for _, process := range vs.Processes {
		cycleTime := d.calculator.normalizeTimes(process.Parameters, unit).CycleTime
		if cycleTime > 0 {
			ratio := cycleTime / takt
			if ratio >= 0.9 && ratio <= 1.1 { // Within 10% of takt time
				return true
			}
		}
	}
	return false
}

// checkSupermarketPrinciple checks if supermarket principle is applied
func (d *Designer) checkSupermarketPrinciple(vs *ValueStream) bool {
	for _, inv := range vs.Inventory {
		if inv.Type == "supermarket" {
			return true
		}
	}
	return false
}

// checkContinuousFlowPrinciple checks if continuous flow principle is applied
func (d *Designer) checkContinuousFlowPrinciple(vs *ValueStream) bool {
	// Check for low idle times and waiting times
	totalIdle := 0.0
	totalProcess := 0.0

	unit := d.calculator.ReferenceTimeUnit(vs.Processes)
	for _, process := range vs.Processes {
		params := d.calculator.normalizeTimes(process.Parameters, unit)
		totalIdle += d.calculator.effectiveIdleTime(params)
		totalProcess += d.calculator.effectiveProcessTime(params)
	}

	if totalProcess > 0 {
		idleRatio := totalIdle / totalProcess
		return idleRatio < 0.5 // Less than 50% idle time indicates good flow
	}
	return false
}

// checkPullSystemPrinciple checks if pull system principle is applied
func (d *Designer) checkPullSystemPrinciple(vs *ValueStream) bool {
	for _, element := range vs.FlowElements {
		if element.Type == "kanban" || element.Type == "supermarket" {
			return true
		}
	}
	return false
}

// checkPacemakerPrinciple checks if pacemaker principle is properly defined
func (d *Designer) checkPacemakerPrinciple(vs *ValueStream) bool {
	// Pacemaker should have the highest utilization or be explicitly defined
	maxUtilization := 0.0
	for _, process := range vs.Processes {
		_, utilization, ok := d.analyzer.CalculateProcessLoad(process, vs)
		if ok && utilization > maxUtilization {
			maxUtilization = utilization
		}
	}
	return maxUtilization > 0.8 // Pacemaker should be highly utilized
}

// checkReleasePrinciple checks if release principle is applied
func (d *Designer) checkReleasePrinciple(vs *ValueStream) bool {
	// Check if there's a clear release mechanism (kanban, signals, etc.)
	return len(vs.FlowElements) > 0
}

// IdentifyContinuousImprovements identifies specific continuous improvement opportunities
func (d *Designer) IdentifyContinuousImprovements(vs *ValueStream, analysis *AnalysisResult) []ContinuousImprovement {
	var improvements []ContinuousImprovement

	now := time.Now()

	// Generate improvements based on bottlenecks
	for _, bottleneck := range analysis.Bottlenecks {
		improvements = append(improvements, ContinuousImprovement{
			ID:        fmt.Sprintf("ci_bottleneck_%s", bottleneck.ProcessID),
			ProcessID: bottleneck.ProcessID,
			Description: fmt.Sprintf("Optimize bottleneck process '%s' to reduce utilization from %.1f%%",
				bottleneck.ProcessName, bottleneck.Utilization*100),
			Type:      "kaizen",
			Priority:  1,
			Status:    "identified",
			CreatedAt: now,
		})
	}

	// Generate improvements based on waste
	if analysis.WasteAnalysis.TotalWaste > 0 {
		improvements = append(improvements, ContinuousImprovement{
			ID:        "ci_waste_reduction",
			ProcessID: "", // System-wide
			Description: fmt.Sprintf("Implement 5S and standard work to reduce waste by %.1f%%",
				analysis.WasteAnalysis.WasteReductionPotential/analysis.WasteAnalysis.TotalWaste*100),
			Type:      "major_change",
			Priority:  2,
			Status:    "identified",
			CreatedAt: now,
		})
	}

	// Generate improvements based on flow issues
	for _, interruption := range analysis.FlowAnalysis.FlowInterruptions {
		improvements = append(improvements, ContinuousImprovement{
			ID:          fmt.Sprintf("ci_flow_%s", interruption.Type),
			ProcessID:   "", // Location-specific
			Description: interruption.Description,
			Type:        "quick_win",
			Priority:    2,
			Status:      "identified",
			CreatedAt:   now,
		})
	}

	return improvements
}
