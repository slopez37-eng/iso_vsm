package vsm

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// PlanningResult contains the results of the value stream planning phase
type PlanningResult struct {
	ValueStreamID       string               `json:"value_stream_id"`
	CatalogueOfMeasures *CatalogueOfMeasures `json:"catalogue_of_measures"`
	WorkshopPlan        *WorkshopPlan        `json:"workshop_plan"`
	ImplementationPlan  *ImplementationPlan  `json:"implementation_plan"`
	RiskAssessment      []Risk               `json:"risk_assessment"`
	PlannedAt           time.Time            `json:"planned_at"`
}

// WorkshopPlan represents the plan for a value stream workshop
type WorkshopPlan struct {
	ID           string                `json:"id"`
	Title        string                `json:"title"`
	Date         time.Time             `json:"date"`
	Duration     time.Duration         `json:"duration"`
	Participants []WorkshopParticipant `json:"participants"`
	Agenda       []WorkshopAgendaItem  `json:"agenda"`
	Goals        []string              `json:"goals"`
	Materials    []string              `json:"materials"`
	Status       string                `json:"status"` // "planned", "in_progress", "completed", "cancelled"
	CreatedAt    time.Time             `json:"created_at"`
}

// WorkshopParticipant represents a participant in the workshop
type WorkshopParticipant struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Role       string `json:"role"`
	Department string `json:"department"`
	Required   bool   `json:"required"`
	Confirmed  bool   `json:"confirmed"`
}

// WorkshopAgendaItem represents an item in the workshop agenda
type WorkshopAgendaItem struct {
	ID          string        `json:"id"`
	Time        time.Duration `json:"time"`
	Title       string        `json:"title"`
	Description string        `json:"description"`
	Facilitator string        `json:"facilitator"`
	Duration    time.Duration `json:"duration"`
	Materials   []string      `json:"materials,omitempty"`
}

// ImplementationPlan represents the implementation plan for measures
type ImplementationPlan struct {
	ID                   string                `json:"id"`
	Phases               []ImplementationPhase `json:"phases"`
	Milestones           []Milestone           `json:"milestones"`
	ResourceRequirements []ResourceRequirement `json:"resource_requirements"`
	Timeline             Timeline              `json:"timeline"`
	Status               string                `json:"status"`
	CreatedAt            time.Time             `json:"created_at"`
}

// ImplementationPhase represents a phase in the implementation
type ImplementationPhase struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	StartDate   time.Time `json:"start_date"`
	EndDate     time.Time `json:"end_date"`
	Measures    []string  `json:"measures"` // IDs of measures in this phase
	Status      string    `json:"status"`   // "planned", "in_progress", "completed"
}

// Milestone represents a milestone in the implementation
type Milestone struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Date        time.Time `json:"date"`
	Completed   bool      `json:"completed"`
	MeasureID   string    `json:"measure_id,omitempty"` // Associated measure
}

// ResourceRequirement represents resources needed for implementation
type ResourceRequirement struct {
	ID          string  `json:"id"`
	Type        string  `json:"type"` // "personnel", "equipment", "budget", "training"
	Description string  `json:"description"`
	Quantity    float64 `json:"quantity"`
	Timeframe   string  `json:"timeframe"` // "immediate", "short_term", "long_term"
	Status      string  `json:"status"`    // "planned", "allocated", "available"
}

// Timeline represents the overall implementation timeline
type Timeline struct {
	StartDate     time.Time     `json:"start_date"`
	EndDate       time.Time     `json:"end_date"`
	Duration      time.Duration `json:"duration"`
	CriticalPath  []string      `json:"critical_path"`  // IDs of critical measures
	KeyMilestones []string      `json:"key_milestones"` // IDs of key milestones
}

// Risk represents a risk in the implementation
type Risk struct {
	ID          string  `json:"id"`
	Description string  `json:"description"`
	Probability float64 `json:"probability"` // 0-1
	Impact      float64 `json:"impact"`      // 0-1
	Severity    float64 `json:"severity"`    // Probability × Impact
	Mitigation  string  `json:"mitigation"`
	Status      string  `json:"status"` // "identified", "mitigated", "occurred"
}

// Planner provides methods for planning value stream improvements
type Planner struct{}

// NewPlanner creates a new VSM planner
func NewPlanner() *Planner {
	return &Planner{}
}

// PlanImplementation creates a comprehensive implementation plan
func (p *Planner) PlanImplementation(design *DesignResult, project *VSMProject) (*PlanningResult, error) {
	if design == nil || project == nil {
		return nil, errors.New("design result and project cannot be nil")
	}

	result := &PlanningResult{
		ValueStreamID: design.ValueStreamID,
		PlannedAt:     time.Now(),
	}

	// Create catalogue of measures
	catalogue, err := p.CreateCatalogueOfMeasures(design, project)
	if err != nil {
		return nil, fmt.Errorf("failed to create catalogue of measures: %w", err)
	}
	result.CatalogueOfMeasures = catalogue

	// Plan workshop
	workshop, err := p.PlanWorkshop(design, project)
	if err != nil {
		return nil, fmt.Errorf("failed to plan workshop: %w", err)
	}
	result.WorkshopPlan = workshop

	// Create implementation plan
	implementation, err := p.CreateImplementationPlan(catalogue)
	if err != nil {
		return nil, fmt.Errorf("failed to create implementation plan: %w", err)
	}
	result.ImplementationPlan = implementation

	// Assess risks
	risks := p.AssessRisks(design, implementation)
	result.RiskAssessment = risks

	return result, nil
}

// CreateCatalogueOfMeasures creates a SMART catalogue of improvement measures
func (p *Planner) CreateCatalogueOfMeasures(design *DesignResult, project *VSMProject) (*CatalogueOfMeasures, error) {
	catalogue := &CatalogueOfMeasures{
		ID:            fmt.Sprintf("catalogue_%s", design.ValueStreamID),
		ValueStreamID: design.ValueStreamID,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	// Convert improvement potentials to SMART measures
	for _, potential := range design.ImprovementPotentials {
		measure := p.convertPotentialToMeasure(potential, design.DesignedAt)
		catalogue.Measures = append(catalogue.Measures, measure)
	}

	// Add measures from continuous improvements
	for _, improvement := range design.ContinuousImprovements {
		measure := p.convertImprovementToMeasure(improvement, design.DesignedAt)
		catalogue.Measures = append(catalogue.Measures, measure)
	}

	// Sort measures by priority
	sort.Slice(catalogue.Measures, func(i, j int) bool {
		return catalogue.Measures[i].Priority < catalogue.Measures[j].Priority
	})

	return catalogue, nil
}

// convertPotentialToMeasure converts an improvement potential to a SMART measure
func (p *Planner) convertPotentialToMeasure(potential ImprovementPotential, baseDate time.Time) ImprovementMeasure {
	measure := ImprovementMeasure{
		ID:          potential.ID,
		Title:       p.generateMeasureTitle(potential),
		Description: potential.Description,
		Responsible: "TBD", // To be determined
		Priority:    potential.Priority,
		Status:      "planned",
		SMART: SMARTGoal{
			Specific:   p.generateSpecificGoal(potential),
			Measurable: p.generateMeasurableGoal(potential),
			Accepted:   "Agreed by cross-functional team",
			Realistic:  p.generateRealisticGoal(potential),
			TimeBound:  baseDate.AddDate(0, 1, 0), // 1 month from design date
		},
		Costs: Costs{
			Investment:  p.estimateInvestment(potential),
			Operational: p.estimateOperationalCosts(potential),
			Benefits:    p.estimateBenefits(potential),
		},
		CreatedAt: baseDate,
		UpdatedAt: baseDate,
	}

	measure.Costs.PaybackPeriod = p.calculatePaybackPeriod(measure.Costs)

	return measure
}

// convertImprovementToMeasure converts a continuous improvement to a measure
func (p *Planner) convertImprovementToMeasure(improvement ContinuousImprovement, baseDate time.Time) ImprovementMeasure {
	potential := ImprovementPotential{
		Type:        improvement.Type,
		Location:    improvement.ProcessID,
		Description: improvement.Description,
		Priority:    improvement.Priority,
	}

	measure := p.convertPotentialToMeasure(potential, baseDate)
	measure.ID = improvement.ID

	return measure
}

// generateMeasureTitle generates a clear title for the measure
func (p *Planner) generateMeasureTitle(potential ImprovementPotential) string {
	switch potential.Type {
	case "bottleneck":
		return fmt.Sprintf("Optimize %s Process", potential.Location)
	case "waste":
		return fmt.Sprintf("Reduce %s Waste", potential.Location)
	case "flow":
		return fmt.Sprintf("Improve Flow at %s", potential.Location)
	case "quality":
		return fmt.Sprintf("Improve Quality for %s", potential.Location)
	default:
		return fmt.Sprintf("Implement %s Improvement", potential.Type)
	}
}

// generateSpecificGoal generates a specific goal description
func (p *Planner) generateSpecificGoal(potential ImprovementPotential) string {
	switch potential.Type {
	case "bottleneck":
		return fmt.Sprintf("Reduce process cycle time at %s by implementing capacity optimization", potential.Location)
	case "waste":
		return fmt.Sprintf("Eliminate identified waste at %s through targeted improvement actions", potential.Location)
	case "flow":
		return fmt.Sprintf("Implement pull system controls at %s to reduce waiting times", potential.Location)
	default:
		return fmt.Sprintf("Achieve specific improvement target for %s", potential.Type)
	}
}

// generateMeasurableGoal generates a measurable goal description
func (p *Planner) generateMeasurableGoal(potential ImprovementPotential) string {
	switch potential.Type {
	case "bottleneck":
		return "Reduce process utilization by 20% while maintaining output"
	case "waste":
		return "Reduce waste by 30% as measured by time or cost savings"
	case "flow":
		return "Reduce idle time by 25% and improve flow efficiency by 15%"
	default:
		return "Achieve measurable improvement of 20% in key performance indicators"
	}
}

// generateRealisticGoal generates a realistic goal description
func (p *Planner) generateRealisticGoal(potential ImprovementPotential) string {
	switch potential.Effort {
	case 0.8:
		return "Requires significant resources but achievable with cross-functional team"
	case 0.6:
		return "Moderate resource requirements, achievable within current capabilities"
	case 0.4:
		return "Low resource requirements, quick win opportunity"
	default:
		return "Realistic with available resources and timeline"
	}
}

// estimateInvestment estimates investment costs
func (p *Planner) estimateInvestment(potential ImprovementPotential) float64 {
	// Simplified estimation based on effort and impact
	baseCost := 10000.0 // Base cost in currency units
	return baseCost * potential.Effort * (1 + potential.Impact)
}

// estimateOperationalCosts estimates operational costs
func (p *Planner) estimateOperationalCosts(potential ImprovementPotential) float64 {
	// Ongoing costs after implementation
	return p.estimateInvestment(potential) * 0.1 // 10% of investment as annual operational cost
}

// estimateBenefits estimates benefits
func (p *Planner) estimateBenefits(potential ImprovementPotential) float64 {
	// Benefits based on impact
	baseBenefit := 25000.0 // Base benefit in currency units
	return baseBenefit * potential.Impact
}

// calculatePaybackPeriod calculates payback period in months
func (p *Planner) calculatePaybackPeriod(costs Costs) float64 {
	if costs.Operational <= 0 {
		return 0
	}
	return costs.Investment / costs.Operational * 12 // Months
}

// PlanWorkshop creates a workshop plan for discussing the value stream plan
func (p *Planner) PlanWorkshop(design *DesignResult, project *VSMProject) (*WorkshopPlan, error) {
	workshop := &WorkshopPlan{
		ID:        fmt.Sprintf("workshop_%s", design.ValueStreamID),
		Title:     fmt.Sprintf("Value Stream Workshop: %s", project.Name),
		Date:      time.Now().AddDate(0, 0, 14), // 2 weeks from now
		Duration:  time.Duration(8) * time.Hour, // 8 hours
		Status:    "planned",
		CreatedAt: time.Now(),
		Goals: []string{
			"Review current state analysis",
			"Discuss future state design",
			"Agree on implementation priorities",
			"Assign responsibilities and timelines",
		},
		Materials: []string{
			"Current state value stream map",
			"Future state value stream map",
			"Catalogue of measures",
			"KPI dashboard",
			"Whiteboard and markers",
		},
	}

	// Define participants
	workshop.Participants = []WorkshopParticipant{
		{
			ID:         "facilitator",
			Name:       "VSM Facilitator",
			Role:       "Workshop Facilitator",
			Department: "Lean Office",
			Required:   true,
			Confirmed:  false,
		},
		{
			ID:         "manager_ops",
			Name:       "Operations Manager",
			Role:       "Decision Maker",
			Department: "Operations",
			Required:   true,
			Confirmed:  false,
		},
		{
			ID:         "supervisor_prod",
			Name:       "Production Supervisor",
			Role:       "Process Expert",
			Department: "Production",
			Required:   true,
			Confirmed:  false,
		},
		{
			ID:         "engineer_lean",
			Name:       "Lean Engineer",
			Role:       "Technical Expert",
			Department: "Engineering",
			Required:   false,
			Confirmed:  false,
		},
	}

	// Define agenda
	startTime := time.Duration(9) * time.Hour // 9 AM start
	workshop.Agenda = []WorkshopAgendaItem{
		{
			ID:          "welcome",
			Time:        startTime,
			Title:       "Welcome and Introduction",
			Description: "Workshop objectives, agenda review, and introductions",
			Facilitator: "VSM Facilitator",
			Duration:    time.Duration(30) * time.Minute,
		},
		{
			ID:          "current_state",
			Time:        startTime + time.Duration(30)*time.Minute,
			Title:       "Current State Review",
			Description: "Present current state analysis, bottlenecks, and waste",
			Facilitator: "VSM Facilitator",
			Duration:    time.Duration(90) * time.Minute,
			Materials:   []string{"Current state map", "KPI data"},
		},
		{
			ID:          "future_state",
			Time:        startTime + time.Duration(120)*time.Minute,
			Title:       "Future State Design",
			Description: "Review future state design and improvement potentials",
			Facilitator: "Lean Engineer",
			Duration:    time.Duration(90) * time.Minute,
			Materials:   []string{"Future state map", "Design principles"},
		},
		{
			ID:          "break",
			Time:        startTime + time.Duration(210)*time.Minute,
			Title:       "Break",
			Description: "Coffee break and informal discussion",
			Duration:    time.Duration(30) * time.Minute,
		},
		{
			ID:          "implementation",
			Time:        startTime + time.Duration(240)*time.Minute,
			Title:       "Implementation Planning",
			Description: "Review catalogue of measures and create action plan",
			Facilitator: "Operations Manager",
			Duration:    time.Duration(120) * time.Minute,
			Materials:   []string{"Catalogue of measures", "Action plan template"},
		},
		{
			ID:          "next_steps",
			Time:        startTime + time.Duration(360)*time.Minute,
			Title:       "Next Steps and Follow-up",
			Description: "Agree on next steps, responsibilities, and follow-up actions",
			Facilitator: "VSM Facilitator",
			Duration:    time.Duration(60) * time.Minute,
		},
	}

	return workshop, nil
}

// CreateImplementationPlan creates a phased implementation plan
func (p *Planner) CreateImplementationPlan(catalogue *CatalogueOfMeasures) (*ImplementationPlan, error) {
	plan := &ImplementationPlan{
		ID:        fmt.Sprintf("impl_%s", catalogue.ID),
		Status:    "planned",
		CreatedAt: time.Now(),
	}

	// Group measures by priority and create phases
	highPriority := []string{}
	mediumPriority := []string{}
	lowPriority := []string{}

	for _, measure := range catalogue.Measures {
		switch measure.Priority {
		case 1:
			highPriority = append(highPriority, measure.ID)
		case 2:
			mediumPriority = append(mediumPriority, measure.ID)
		case 3:
			lowPriority = append(lowPriority, measure.ID)
		}
	}

	now := time.Now()

	// Create implementation phases
	plan.Phases = []ImplementationPhase{
		{
			ID:          "phase_1",
			Name:        "Quick Wins",
			Description: "Implement high-impact, low-effort improvements",
			StartDate:   now.AddDate(0, 0, 7),  // Start in 1 week
			EndDate:     now.AddDate(0, 0, 30), // Complete in 1 month
			Measures:    highPriority,
			Status:      "planned",
		},
		{
			ID:          "phase_2",
			Name:        "Core Improvements",
			Description: "Implement medium-priority improvements",
			StartDate:   now.AddDate(0, 0, 31), // Start after phase 1
			EndDate:     now.AddDate(0, 2, 0),  // Complete in 2 months total
			Measures:    mediumPriority,
			Status:      "planned",
		},
		{
			ID:          "phase_3",
			Name:        "Advanced Improvements",
			Description: "Implement complex, long-term improvements",
			StartDate:   now.AddDate(0, 2, 1), // Start after phase 2
			EndDate:     now.AddDate(0, 6, 0), // Complete in 6 months total
			Measures:    lowPriority,
			Status:      "planned",
		},
	}

	// Create milestones
	plan.Milestones = []Milestone{
		{
			ID:          "milestone_quick_wins",
			Name:        "Quick Wins Completed",
			Description: "All high-priority measures implemented",
			Date:        now.AddDate(0, 0, 30),
			Completed:   false,
		},
		{
			ID:          "milestone_mid_term",
			Name:        "Mid-term Review",
			Description: "Review progress after 3 months",
			Date:        now.AddDate(0, 3, 0),
			Completed:   false,
		},
		{
			ID:          "milestone_full_implementation",
			Name:        "Full Implementation",
			Description: "All measures implemented and stable",
			Date:        now.AddDate(0, 6, 0),
			Completed:   false,
		},
	}

	// Define resource requirements
	plan.ResourceRequirements = []ResourceRequirement{
		{
			ID:          "resource_personnel",
			Type:        "personnel",
			Description: "Cross-functional improvement team",
			Quantity:    4,
			Timeframe:   "immediate",
			Status:      "planned",
		},
		{
			ID:          "resource_budget",
			Type:        "budget",
			Description: "Implementation budget",
			Quantity:    50000, // Currency units
			Timeframe:   "short_term",
			Status:      "planned",
		},
		{
			ID:          "resource_training",
			Type:        "training",
			Description: "Lean training for team members",
			Quantity:    40, // Hours
			Timeframe:   "immediate",
			Status:      "planned",
		},
	}

	// Define timeline
	plan.Timeline = Timeline{
		StartDate:    now.AddDate(0, 0, 7),
		EndDate:      now.AddDate(0, 6, 0),
		Duration:     time.Duration(24*30*6) * time.Hour, // 6 months in hours
		CriticalPath: highPriority,
		KeyMilestones: []string{
			"milestone_quick_wins",
			"milestone_mid_term",
			"milestone_full_implementation",
		},
	}

	return plan, nil
}

// AssessRisks assesses risks in the implementation
func (p *Planner) AssessRisks(design *DesignResult, implementation *ImplementationPlan) []Risk {
	risks := []Risk{
		{
			ID:          "risk_resistance",
			Description: "Employee resistance to change",
			Probability: 0.6,
			Impact:      0.7,
			Mitigation:  "Comprehensive change management and communication plan",
			Status:      "identified",
		},
		{
			ID:          "risk_resource_shortage",
			Description: "Insufficient resources allocated",
			Probability: 0.4,
			Impact:      0.8,
			Mitigation:  "Detailed resource planning and stakeholder buy-in",
			Status:      "identified",
		},
		{
			ID:          "risk_technical_issues",
			Description: "Technical difficulties during implementation",
			Probability: 0.3,
			Impact:      0.6,
			Mitigation:  "Pilot testing and phased implementation approach",
			Status:      "identified",
		},
		{
			ID:          "risk_scope_creep",
			Description: "Scope expansion beyond planned measures",
			Probability: 0.5,
			Impact:      0.4,
			Mitigation:  "Strict change control and scope management",
			Status:      "identified",
		},
	}

	// Calculate severity for each risk
	for i := range risks {
		risks[i].Severity = risks[i].Probability * risks[i].Impact
	}

	// Sort by severity (highest first)
	sort.Slice(risks, func(i, j int) bool {
		return risks[i].Severity > risks[j].Severity
	})

	return risks
}
