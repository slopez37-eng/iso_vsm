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
	// ImprovementMetrics compares future with current KPIs (see
	// Calculator.CalculateImprovementMetrics). Only set when targets were
	// supplied.
	ImprovementMetrics map[string]float64 `json:"improvement_metrics,omitempty"`
	// Notes explains how the future state's numbers were produced.
	Notes []string `json:"notes,omitempty"`
	// ConsistencyIssues lists stored values in the current state that
	// disagree with the values computed from their parts (see
	// CheckConsistency). The future and ideal states use the computed values.
	ConsistencyIssues []ConsistencyIssue `json:"consistency_issues,omitempty"`
	DesignedAt        time.Time          `json:"designed_at"`
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

// noTargetsNote is reported when a future state is designed without targets.
const noTargetsNote = "No improvement targets supplied: the future state keeps the current " +
	"values, so its KPIs equal the current KPIs. Supply targets with " +
	"Designer.DesignFutureStateWithTargets to project improvements."

// DesignFutureState performs the complete design phase for a value stream
// without improvement targets.
//
// The future state keeps every current number (times, rates, inventory);
// it only adds structural recommendations (FIFO lane, kanban, supermarket
// flow elements, unsized). No reductions are invented, so its KPIs equal
// the current KPIs and DesignResult.Notes says so. To project improvements,
// call DesignFutureStateWithTargets with the targets you expect to reach.
func (d *Designer) DesignFutureState(current *ValueStream, analysis *AnalysisResult) (*DesignResult, error) {
	return d.DesignFutureStateWithTargets(current, analysis, nil)
}

// DesignFutureStateWithTargets performs the complete design phase, building
// the future state from the caller's targets (see DesignFutureStateFromTargets)
// and computing its KPIs with the calculator. With nil targets it behaves
// like DesignFutureState.
func (d *Designer) DesignFutureStateWithTargets(current *ValueStream, analysis *AnalysisResult, targets *FutureStateTargets) (*DesignResult, error) {
	if current == nil {
		return nil, errors.New("current value stream cannot be nil")
	}
	if analysis == nil {
		return nil, errors.New("analysis result cannot be nil")
	}

	result := &DesignResult{
		ValueStreamID:     current.ID,
		DesignedAt:        time.Now(),
		ConsistencyIssues: CheckConsistency(current),
	}
	if n := len(result.ConsistencyIssues); n > 0 {
		result.Notes = append(result.Notes, fmt.Sprintf(consistencyNoteFormat, n))
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

	// Design future state from the supplied targets
	futureState, err := d.DesignFutureStateFromTargets(current, targets)
	if err != nil {
		return nil, fmt.Errorf("failed to design future state: %w", err)
	}
	result.FutureState = futureState
	if targets == nil {
		result.Notes = append(result.Notes, noTargetsNote)
	} else {
		result.ImprovementMetrics = d.calculator.CalculateImprovementMetrics(current, futureState)
		result.Notes = append(result.Notes, "Future state numbers come from the supplied targets; "+
			"values without a target are unchanged.")
	}

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
		idealProcess.Parameters = d.calculator.idealParameters(process.Parameters, d.calculator.contextOf(current)).Clone()

		idealProcesses = append(idealProcesses, idealProcess)
	}

	ideal.Processes = idealProcesses

	// No inventory in ideal state (continuous flow)
	ideal.Inventory = []Inventory{}

	// Calculate ideal KPIs
	ideal.KPIs = d.calculator.CalculateKPIs(ideal)

	return ideal, nil
}

// idealParameters returns params reduced to value-adding time only, kept
// consistent (see ResolveParameters): process time = processing time = lead
// time = value adding time; setup, handling, waiting, ancillary, idle, stock,
// NVAT, NNVAT, scrap and rework that were known become 0 (those not entered
// stay not entered); repetition and transport repetition data is removed;
// yield is 1. A process without value adding time keeps no process, lead or
// processing time (nothing is known to remain). When processing time is
// computed from cycle time × order quantity ÷ resources (A.5), the cycle
// time is set so that it gives the value adding time.
func (c *Calculator) idealParameters(params ProcessParameters, ctx streamContext) ProcessParameters {
	r, _ := c.resolveParameters(params, ctx)
	vat := r.ValueAddingTime
	p := r
	p.ProcessTime, p.ProcessingTime, p.LeadTime = vat, vat, vat
	p.ValueAddingTime, p.ValueAddingTimeProcess = vat, vat
	for _, field := range []**float64{
		&p.SetupTime, &p.HandlingTime, &p.WaitingTimeProcess, &p.AncillaryTime,
		&p.IdleTime, &p.WaitingTime, &p.TransportTime, &p.StorageTime,
		&p.StockQuantity, &p.RangeOfInventory,
		&p.NonValueAddingTime, &p.NecessaryNonValueAddingTime,
		&p.ScrapRate, &p.ReworkRate,
	} {
		if *field != nil {
			*field = F(0)
		}
	}
	p.RepetitionTime, p.RepetitionCount = nil, nil
	p.TransportRepetitionTime, p.TransportRepetitionCount = nil, nil
	p.YieldFactor = F(1)
	if processingFormula(p) && positive(p.OrderQuantity) {
		p.CycleTime = F(num(vat) * float64(*p.ResourceCount) / *p.OrderQuantity)
	}
	return c.trimDerived(params, p, ctx)
}

// DesignFutureStateFromIdeal creates a future state with no projected
// improvements: it is DesignFutureStateFromTargets(current, nil). The ideal
// state is not used for numbers (kept for API compatibility); supply targets
// via DesignFutureStateFromTargets to project improvements.
func (d *Designer) DesignFutureStateFromIdeal(ideal, current *ValueStream) (*ValueStream, error) {
	return d.DesignFutureStateFromTargets(current, nil)
}

// DesignFutureStateFromTargets builds a future state from the current state
// and the caller's targets (see FutureStateTargets), then computes its KPIs
// with the calculator. Values without a target keep their current value; with
// nil targets the future state's numbers equal the current state's.
//
// Structural recommendations are added as unsized flow elements: a FIFO lane,
// a kanban loop and, when the current state holds inventory, a supermarket.
// Their capacity is not entered (to be sized by the team); they do not change
// the KPIs.
//
// The future state's processes and inventories share no pointers with the
// current state's (see ProcessParameters.Clone); its Customer and Supplier
// are the current state's.
//
// Returns an error if a target is invalid or names a process or inventory ID
// that is not in the current state.
func (d *Designer) DesignFutureStateFromTargets(current *ValueStream, targets *FutureStateTargets) (*ValueStream, error) {
	if current == nil {
		return nil, errors.New("current value stream cannot be nil")
	}
	if targets == nil {
		targets = &FutureStateTargets{}
	}
	if err := checkTargetIDs(current, targets); err != nil {
		return nil, err
	}

	description := "Future state with current values (no targets supplied, no projected improvements)"
	if targets.Default != nil || len(targets.Processes) > 0 || targets.InventoryDefault != nil || len(targets.Inventory) > 0 {
		description = "Future state projected from the supplied improvement targets"
	}

	future := &ValueStream{
		ID:            current.ID + "_future",
		Name:          current.Name + " (Future State)",
		State:         StateFuture,
		ProductFamily: current.ProductFamily,
		Description:   description,
		Customer:      current.Customer,
		Supplier:      current.Supplier,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	var futureProcesses []Process
	for _, process := range current.Processes {
		futureProcess := process
		futureProcess.ID = process.ID + "_future"
		futureProcess.Name = process.Name + " (Future)"

		var own *ProcessTargets
		if t, ok := targets.Processes[process.ID]; ok {
			own = &t
		}
		params, err := d.calculator.applyProcessTargets(process.Parameters, own, targets.Default, d.calculator.contextOf(current))
		if err != nil {
			return nil, fmt.Errorf("process %s: %w", process.ID, err)
		}
		futureProcess.Parameters = params.Clone()

		futureProcesses = append(futureProcesses, futureProcess)
	}
	future.Processes = futureProcesses

	for _, inv := range current.Inventory {
		var own *InventoryTargets
		if t, ok := targets.Inventory[inv.ID]; ok {
			own = &t
		}
		def := targets.InventoryDefault
		if holder, linked := stockHolder(current, inv); linked {
			// The stock of a process (see Inventory.ProcessID): it follows
			// the process's future stock_quantity. Stream-wide inventory
			// defaults do not apply to it.
			if own != nil {
				return nil, fmt.Errorf("inventory %s: it is the stock of process %s (process_id); "+
					"target that process's stock_quantity or waiting_time instead", inv.ID, holder.ID)
			}
			// In either stored form (quantity, or range alone), and down to
			// zero when the process's stock is targeted away.
			own, def = nil, nil
			futureStock := num(futureProcesses[processIndex(current, holder.ID)].Parameters.StockQuantity)
			currentStock := *holder.Parameters.StockQuantity
			ctx := d.calculator.contextOf(current)
			_, rateKnown := ctx.demandRate(ctx.unit)
			switch {
			case inv.Quantity != nil || (inv.Range != nil && rateKnown):
				// With the demand rate known, a range-only inventory's
				// range follows from the quantity (see applyInventoryTargets).
				own = &InventoryTargets{Quantity: AbsoluteTarget(futureStock)}
			case inv.Range != nil && currentStock > 0:
				// Range alone, demand rate unknown: the range is
				// proportional to the stock.
				own = &InventoryTargets{Range: AbsoluteTarget(*inv.Range * futureStock / currentStock)}
			case inv.Range != nil && futureStock == 0:
				own = &InventoryTargets{Range: AbsoluteTarget(0)}
			}
		}
		futureInv, err := d.calculator.applyInventoryTargets(inv, own, def, d.calculator.contextOf(current))
		if err != nil {
			return nil, fmt.Errorf("inventory %s: %w", inv.ID, err)
		}
		futureInv = futureInv.Clone()
		if i := processIndex(current, inv.ProcessID); i >= 0 {
			futureInv.ProcessID = futureProcesses[i].ID
		}
		future.Inventory = append(future.Inventory, futureInv)
	}

	// Structural recommendations (unsized: capacity left to the team)
	future.FlowElements = []FlowElement{
		{
			ID:          "fifo_lane_1",
			Type:        "fifo",
			Description: "Recommended FIFO lane between processes; capacity to be sized",
		},
		{
			ID:          "kanban_1",
			Type:        "kanban",
			Description: "Recommended kanban control loop; number of cards to be sized",
		},
	}
	if len(current.Inventory) > 0 {
		future.FlowElements = append(future.FlowElements, FlowElement{
			ID:   "supermarket_1",
			Type: "supermarket",
			Description: "Recommended pull supermarket in place of push inventory; " +
				"size from demand and replenishment time",
		})
	}

	// Calculate future KPIs
	future.KPIs = d.calculator.CalculateKPIs(future)

	return future, nil
}

// processIndex returns the index of the process with ID id in vs, or -1.
func processIndex(vs *ValueStream, id string) int {
	if id == "" {
		return -1
	}
	for i, p := range vs.Processes {
		if p.ID == id {
			return i
		}
	}
	return -1
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
		ct := d.calculator.normalizeTimes(process.Parameters, unit).CycleTime
		if positive(ct) {
			ratio := *ct / takt
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
	for _, element := range vs.FlowElements {
		if element.Type == "supermarket" {
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

	ctx := d.calculator.contextOf(vs)
	for _, process := range vs.Processes {
		params := d.calculator.resolvedIn(process.Parameters, ctx)
		totalIdle += num(params.IdleTime)
		totalProcess += num(params.ProcessTime)
	}

	_, inventory := d.calculator.inventoryTimes(vs)
	totalIdle += inventory

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
