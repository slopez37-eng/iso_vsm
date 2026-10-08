package vsm

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Planning uses only numbers the user entered (see PlanningInputs) or that
// the calculator computes from the value streams. Money, priorities, due
// dates, risk scores and resource quantities that were not entered are not
// estimated: they are nil (omitted in JSON) and a note says so.

// PlanningResult contains the results of the value stream planning phase
type PlanningResult struct {
	ValueStreamID       string               `json:"value_stream_id"`
	CatalogueOfMeasures *CatalogueOfMeasures `json:"catalogue_of_measures"`
	WorkshopPlan        *WorkshopPlan        `json:"workshop_plan"`
	ImplementationPlan  *ImplementationPlan  `json:"implementation_plan"`
	RiskAssessment      []Risk               `json:"risk_assessment"`
	// CostTotals adds up the measures' entered costs (see CostTotals for
	// the rule on missing amounts).
	CostTotals CostTotals `json:"cost_totals"`
	// Notes lists what was not entered and so not estimated.
	Notes     []string  `json:"notes,omitempty"`
	PlannedAt time.Time `json:"planned_at"`
}

// WorkshopPlan represents the plan for a value stream workshop
type WorkshopPlan struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// Date is the entered workshop date (PlanningInputs.WorkshopDate); nil
	// (omitted) = not scheduled.
	Date *time.Time `json:"date,omitempty"`
	// Duration is the sum of the agenda items' durations.
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

// WorkshopAgendaItem represents an item in the workshop agenda. The agenda
// is a suggested template: Time is the item's start as an offset from the
// start of the workshop (the first item is at 0), and the durations are
// suggestions to adjust.
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
	ID     string                `json:"id"`
	Phases []ImplementationPhase `json:"phases"`
	// UnprioritizedMeasures are the IDs of measures without a priority
	// (impact or effort rating not entered); they are in no phase.
	UnprioritizedMeasures []string              `json:"unprioritized_measures,omitempty"`
	Milestones            []Milestone           `json:"milestones"`
	ResourceRequirements  []ResourceRequirement `json:"resource_requirements"`
	Timeline              Timeline              `json:"timeline"`
	Notes                 []string              `json:"notes,omitempty"`
	Status                string                `json:"status"`
	CreatedAt             time.Time             `json:"created_at"`
}

// ImplementationPhase is the measures of one priority (phase 1: priority
// 1, and so on). Phases run one after another: the first non-empty phase
// starts on the entered start date, each later one when the previous
// non-empty phase ends, and a phase ends on the latest due date of its
// measures. A date is nil (omitted) when what it needs was not entered
// (no start date, or a measure without a due date); an empty phase has no
// dates.
type ImplementationPhase struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	StartDate   *time.Time `json:"start_date,omitempty"`
	EndDate     *time.Time `json:"end_date,omitempty"`
	Measures    []string   `json:"measures"` // IDs of measures in this phase
	Status      string     `json:"status"`   // "planned", "in_progress", "completed"
}

// Milestone represents a milestone in the implementation. Date is nil
// (omitted) when not known from the entered dates.
type Milestone struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Date        *time.Time `json:"date,omitempty"`
	Completed   bool       `json:"completed"`
	MeasureID   string     `json:"measure_id,omitempty"` // Associated measure
}

// ResourceRequirement represents resources needed for implementation.
// Quantity is nil (omitted) when not entered; the budget's quantity is the
// total entered investment (see CostTotals), in currency units.
type ResourceRequirement struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"` // "personnel", "equipment", "budget", "training"
	Description string   `json:"description"`
	Quantity    *float64 `json:"quantity,omitempty"`
	Timeframe   string   `json:"timeframe"` // "immediate", "short_term", "long_term"
	Status      string   `json:"status"`    // "planned", "allocated", "available"
}

// Timeline represents the overall implementation timeline: from the
// entered start date to the latest measure due date (when every measure
// has one). Unknown parts are nil (omitted).
type Timeline struct {
	StartDate     *time.Time     `json:"start_date,omitempty"`
	EndDate       *time.Time     `json:"end_date,omitempty"`
	Duration      *time.Duration `json:"duration,omitempty"`
	CriticalPath  []string       `json:"critical_path"`  // IDs of priority-1 measures
	KeyMilestones []string       `json:"key_milestones"` // IDs of key milestones
}

// Risk represents a risk in the implementation. Probability and Impact
// (fractions 0–1) are as entered (see RiskInput); Severity = Probability ×
// Impact when both are entered. Unassessed risks have them nil (omitted)
// and a Note.
type Risk struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Probability *float64 `json:"probability,omitempty"`
	Impact      *float64 `json:"impact,omitempty"`
	Severity    *float64 `json:"severity,omitempty"`
	Mitigation  string   `json:"mitigation"`
	Status      string   `json:"status"` // "identified", "mitigated", "occurred"
	Note        string   `json:"note,omitempty"`
}

// Planner provides methods for planning value stream improvements
type Planner struct{}

// NewPlanner creates a new VSM planner
func NewPlanner() *Planner {
	return &Planner{}
}

// PlanImplementation creates an implementation plan without user inputs:
// it is PlanImplementationWithInputs(design, project, nil). Nothing is
// prioritised, costed, dated or risk-scored beyond what the design result
// already carries (ratings entered at design time).
func (p *Planner) PlanImplementation(design *DesignResult, project *VSMProject) (*PlanningResult, error) {
	return p.PlanImplementationWithInputs(design, project, nil)
}

// PlanImplementationWithInputs creates a comprehensive implementation plan
// from the design, the project's current state and the user's inputs (see
// PlanningInputs; nil = nothing entered).
//
// Returns an error for an invalid input (a rating outside 1–5, a negative
// amount, a risk probability or impact outside 0–1, an ID that names no
// measure or risk) or dates that contradict each other (a due date before
// the start date, or before the phase it falls in starts).
func (p *Planner) PlanImplementationWithInputs(design *DesignResult, project *VSMProject, inputs *PlanningInputs) (*PlanningResult, error) {
	if design == nil || project == nil {
		return nil, errors.New("design result and project cannot be nil")
	}
	if inputs == nil {
		inputs = &PlanningInputs{}
	}

	result := &PlanningResult{
		ValueStreamID: design.ValueStreamID,
		PlannedAt:     time.Now(),
	}

	catalogue, err := p.createCatalogue(design, project, inputs)
	if err != nil {
		return nil, fmt.Errorf("failed to create catalogue of measures: %w", err)
	}
	result.CatalogueOfMeasures = catalogue
	result.CostTotals = calculateCostTotals(catalogue.Measures)

	result.WorkshopPlan = p.planWorkshop(design, project, inputs.WorkshopDate)

	implementation, err := p.createImplementationPlan(catalogue, inputs.StartDate)
	if err != nil {
		return nil, fmt.Errorf("failed to create implementation plan: %w", err)
	}
	result.ImplementationPlan = implementation

	risks, err := p.assessRisks(inputs.Risks)
	if err != nil {
		return nil, fmt.Errorf("failed to assess risks: %w", err)
	}
	result.RiskAssessment = risks

	result.Notes = planningNotes(result, inputs)
	return result, nil
}

// planningNotes lists what was not entered and so not estimated.
func planningNotes(r *PlanningResult, inputs *PlanningInputs) []string {
	var notes []string
	unrated, undated := 0, 0
	for _, m := range r.CatalogueOfMeasures.Measures {
		if m.Priority == 0 {
			unrated++
		}
		if m.SMART.TimeBound == nil {
			undated++
		}
	}
	n := len(r.CatalogueOfMeasures.Measures)
	if unrated > 0 {
		notes = append(notes, fmt.Sprintf("%d of %d measures are not prioritised (impact or effort "+
			"rating not entered); they keep their original order and are in no phase.", unrated, n))
	}
	if missing := len(r.CostTotals.MissingInputs); missing > 0 {
		notes = append(notes, fmt.Sprintf("%d of %d measures have costs not entered: their payback and "+
			"the totals that need them are not estimated.", missing, n))
	}
	if undated > 0 {
		notes = append(notes, fmt.Sprintf("%d of %d measures have no due date: phase and timeline "+
			"dates that need them are not set.", undated, n))
	}
	if inputs.StartDate == nil {
		notes = append(notes, "No start date entered: phase and timeline start dates are not set.")
	}
	if inputs.WorkshopDate == nil {
		notes = append(notes, "No workshop date entered: the workshop is not scheduled.")
	}
	notes = append(notes, "The workshop agenda and participant roles are a suggested template.")
	unassessed := 0
	for _, risk := range r.RiskAssessment {
		if risk.Severity == nil {
			unassessed++
		}
	}
	if unassessed > 0 {
		notes = append(notes, fmt.Sprintf("%d of %d risks are not assessed (probability or impact not "+
			"entered); they are listed after the assessed risks.", unassessed, len(r.RiskAssessment)))
	}
	return notes
}

// CreateCatalogueOfMeasures creates a SMART catalogue of improvement
// measures without user inputs (see PlanImplementationWithInputs).
func (p *Planner) CreateCatalogueOfMeasures(design *DesignResult, project *VSMProject) (*CatalogueOfMeasures, error) {
	if design == nil || project == nil {
		return nil, errors.New("design result and project cannot be nil")
	}
	return p.createCatalogue(design, project, &PlanningInputs{})
}

// createCatalogue converts the design's improvement potentials and
// continuous improvements to measures (same IDs), applies the inputs, and
// orders the measures: prioritised measures by priority (ties keep their
// order), then the measures that are not prioritised, in their original
// order.
func (p *Planner) createCatalogue(design *DesignResult, project *VSMProject, inputs *PlanningInputs) (*CatalogueOfMeasures, error) {
	catalogue := &CatalogueOfMeasures{
		ID:            fmt.Sprintf("catalogue_%s", design.ValueStreamID),
		ValueStreamID: design.ValueStreamID,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	known := map[string]bool{}
	for _, potential := range design.ImprovementPotentials {
		known[potential.ID] = true
	}
	for _, improvement := range design.ContinuousImprovements {
		known[improvement.ID] = true
	}
	if err := validateMeasureInputs(inputs.Measures, known, "measure (improvement potential or continuous improvement)"); err != nil {
		return nil, err
	}
	if inputs.StartDate != nil {
		for _, id := range sortedInputIDs(inputs.Measures) {
			if due := inputs.Measures[id].DueDate; due != nil && due.Before(*inputs.StartDate) {
				return nil, fmt.Errorf("measure %q is due on %s, before the start date %s",
					id, due.Format(time.DateOnly), inputs.StartDate.Format(time.DateOnly))
			}
		}
	}

	current := &project.CurrentState
	for _, potential := range design.ImprovementPotentials {
		catalogue.Measures = append(catalogue.Measures,
			p.convertPotentialToMeasure(potential, inputs.Measures[potential.ID], current, design.FutureState, design.DesignedAt))
	}
	for _, improvement := range design.ContinuousImprovements {
		catalogue.Measures = append(catalogue.Measures,
			p.convertImprovementToMeasure(improvement, inputs.Measures[improvement.ID], current, design.FutureState, design.DesignedAt))
	}

	sort.SliceStable(catalogue.Measures, func(i, j int) bool {
		return priorityRank(catalogue.Measures[i].Priority) < priorityRank(catalogue.Measures[j].Priority)
	})
	return catalogue, nil
}

// priorityRank orders priorities 1, 2, 3, then not prioritised (0).
func priorityRank(priority int) int {
	if priority == 0 {
		return math.MaxInt
	}
	return priority
}

// convertPotentialToMeasure converts an improvement potential to a SMART
// measure with the user's input for it. Ratings entered in the input
// replace the potential's; without them, the potential's ratings (entered
// at design time) are used.
func (p *Planner) convertPotentialToMeasure(potential ImprovementPotential, in MeasureInput, current, future *ValueStream, baseDate time.Time) ImprovementMeasure {
	impact, effort, priority, note := rate(in, potential.ImpactRating, potential.EffortRating)
	responsible := in.Responsible
	if responsible == "" {
		responsible = "TBD" // To be determined
	}
	var due *time.Time
	if in.DueDate != nil {
		d := *in.DueDate
		due = &d
	}
	return ImprovementMeasure{
		ID:           potential.ID,
		Title:        p.generateMeasureTitle(potential),
		Description:  potential.Description,
		Responsible:  responsible,
		Priority:     priority,
		ImpactRating: impact,
		EffortRating: effort,
		PriorityNote: note,
		ProcessID:    potential.ProcessID,
		Status:       "planned",
		SMART: SMARTGoal{
			Specific:   p.generateSpecificGoal(potential),
			Measurable: measurableGoal(current, future, potential.ProcessID),
			Accepted:   acceptedGoal(in.Responsible),
			Realistic:  realisticGoal(effort),
			TimeBound:  due,
		},
		Costs:     measureCosts(in),
		CreatedAt: baseDate,
		UpdatedAt: baseDate,
	}
}

// convertImprovementToMeasure converts a continuous improvement to a measure
func (p *Planner) convertImprovementToMeasure(improvement ContinuousImprovement, in MeasureInput, current, future *ValueStream, baseDate time.Time) ImprovementMeasure {
	potential := ImprovementPotential{
		ID:           improvement.ID,
		Type:         improvement.Type,
		Location:     improvement.ProcessID,
		ProcessID:    improvement.ProcessID,
		Description:  improvement.Description,
		ImpactRating: improvement.ImpactRating,
		EffortRating: improvement.EffortRating,
	}
	return p.convertPotentialToMeasure(potential, in, current, future, baseDate)
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

// acceptedGoal states who accepts the measure (the entered responsible
// person), or that nobody is recorded.
func acceptedGoal(responsible string) string {
	if responsible == "" {
		return "Not recorded: no responsible person entered"
	}
	return "To be accepted by " + responsible
}

// realisticGoal states the entered effort rating, or that it was not rated.
func realisticGoal(effort *int) string {
	if effort == nil {
		return "Not assessed: effort rating not entered"
	}
	return fmt.Sprintf("Effort rated %d of %d (entered)", *effort, RatingMax)
}

// goalMetric is one value a measurable goal compares between the current
// state and the targeted future state (nil = unknown).
type goalMetric struct {
	name            string
	current, future *float64
	ratio           bool // a fraction, shown as percent
}

// measurableGoal states, for the measure's process (or the whole value
// stream when processID is empty or not in current), each value's change
// from the current state to the future state designed from the user's
// targets, computed by the calculator: cycle time and lead time of the
// process, or total lead time and value added ratio of the stream. A value
// the targets do not change says "target not set"; no change is invented.
func measurableGoal(current, future *ValueStream, processID string) string {
	if current == nil {
		return "Target not set: no current state"
	}
	calc := NewCalculator()
	unit := calc.ReferenceTimeUnit(current.Processes)
	unitName := string(unit)
	if unitName == "" {
		unitName = "time units"
	}

	var metrics []goalMetric
	if i := processIndex(current, processID); i >= 0 {
		name := current.Processes[i].Name
		cur, curOK := calc.resolvedIn(current.Processes[i].Parameters, calc.contextOf(current))
		var fut ProcessParameters
		futOK := false
		if future != nil {
			if j := processIndex(future, processID+"_future"); j >= 0 {
				fut, futOK = calc.resolvedIn(future.Processes[j].Parameters, calc.contextOf(future))
				if from := calc.ReferenceTimeUnit(future.Processes); futOK && from != unit && isKnownTimeUnit(from) && isKnownTimeUnit(unit) {
					fut = calc.normalizeTimes(fut, unit)
				}
			}
		}
		pick := func(p ProcessParameters, ok bool, get func(ProcessParameters) *float64) *float64 {
			if !ok {
				return nil
			}
			return get(p)
		}
		cycle := func(p ProcessParameters) *float64 { return p.CycleTime }
		lead := func(p ProcessParameters) *float64 { return p.LeadTime }
		metrics = []goalMetric{
			{name: "cycle time at " + name, current: pick(cur, curOK, cycle), future: pick(fut, futOK, cycle)},
			{name: "lead time at " + name, current: pick(cur, curOK, lead), future: pick(fut, futOK, lead)},
		}
	} else {
		curKPIs := calc.kpisInUnit(current, unit)
		var futKPIs map[string]float64
		if future != nil {
			futKPIs = calc.kpisInUnit(future, unit)
		}
		get := func(kpis map[string]float64, name string) *float64 {
			if v, ok := kpis[name]; ok {
				return F(v)
			}
			return nil
		}
		metrics = []goalMetric{
			{name: "total lead time", current: get(curKPIs, "total_lead_time"), future: get(futKPIs, "total_lead_time")},
			{name: "value added ratio", current: get(curKPIs, "value_added_ratio"), future: get(futKPIs, "value_added_ratio"), ratio: true},
		}
	}

	parts := make([]string, 0, len(metrics))
	for _, m := range metrics {
		parts = append(parts, describeGoal(m, unitName, future != nil))
	}
	s := strings.Join(parts, "; ")
	return strings.ToUpper(s[:1]) + s[1:]
}

// describeGoal states one metric's change from current to future.
func describeGoal(m goalMetric, unit string, haveFuture bool) string {
	show := func(v float64) string {
		if m.ratio {
			return formatNumber(v*100) + "%"
		}
		return formatNumber(v) + " " + unit
	}
	switch {
	case !haveFuture:
		return m.name + ": target not set (no future state)"
	case m.current != nil && m.future != nil && sameAmount(*m.current, *m.future):
		return fmt.Sprintf("%s: target not set (current %s)", m.name, show(*m.current))
	case m.current != nil && m.future != nil:
		verb := "reduce"
		if *m.future > *m.current {
			verb = "increase"
		}
		s := fmt.Sprintf("%s %s from %s to %s", verb, m.name, show(*m.current), show(*m.future))
		if !m.ratio && *m.current != 0 {
			s += fmt.Sprintf(" (%+.1f%%)", (*m.future-*m.current) / *m.current * 100)
		}
		return s
	case m.future != nil:
		return fmt.Sprintf("%s: target %s (current value unknown)", m.name, show(*m.future))
	case m.current != nil:
		return fmt.Sprintf("%s: future value unknown (current %s)", m.name, show(*m.current))
	default:
		return m.name + ": current value unknown, target not set"
	}
}

// sameAmount reports whether a and b are equal up to float rounding (a
// value no target changed is computed the same way in both states).
func sameAmount(a, b float64) bool {
	return math.Abs(a-b) <= boundEpsilon*math.Max(1, math.Abs(b))
}

// formatNumber formats v with at most 4 decimals and no trailing zeros.
func formatNumber(v float64) string {
	return strconv.FormatFloat(math.Round(v*1e4)/1e4, 'f', -1, 64)
}

// calculatePaybackPeriod returns the payback period in months for costs
// (see Costs): months is nil when not estimated (an amount not entered) or
// when the measure never pays back (never is then true).
func (p *Planner) calculatePaybackPeriod(costs Costs) (months *float64, never bool) {
	return calculatePayback(costs)
}

// PlanWorkshop creates a workshop plan for discussing the value stream
// plan, not scheduled (see PlanImplementationWithInputs for the date).
func (p *Planner) PlanWorkshop(design *DesignResult, project *VSMProject) (*WorkshopPlan, error) {
	if design == nil || project == nil {
		return nil, errors.New("design result and project cannot be nil")
	}
	return p.planWorkshop(design, project, nil), nil
}

// planWorkshop creates the workshop plan on the entered date (nil = not
// scheduled). The agenda is a suggested template; the duration is the sum
// of its items.
func (p *Planner) planWorkshop(design *DesignResult, project *VSMProject, date *time.Time) *WorkshopPlan {
	workshop := &WorkshopPlan{
		ID:        fmt.Sprintf("workshop_%s", design.ValueStreamID),
		Title:     fmt.Sprintf("Value Stream Workshop: %s", project.Name),
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
	if date != nil {
		d := *date
		workshop.Date = &d
	}

	// Suggested participant roles
	workshop.Participants = []WorkshopParticipant{
		{ID: "facilitator", Name: "VSM Facilitator", Role: "Workshop Facilitator", Department: "Lean Office", Required: true},
		{ID: "manager_ops", Name: "Operations Manager", Role: "Decision Maker", Department: "Operations", Required: true},
		{ID: "supervisor_prod", Name: "Production Supervisor", Role: "Process Expert", Department: "Production", Required: true},
		{ID: "engineer_lean", Name: "Lean Engineer", Role: "Technical Expert", Department: "Engineering"},
	}

	// Suggested agenda; each item starts when the previous one ends.
	workshop.Agenda = []WorkshopAgendaItem{
		{ID: "welcome", Title: "Welcome and Introduction",
			Description: "Workshop objectives, agenda review, and introductions",
			Facilitator: "VSM Facilitator", Duration: 30 * time.Minute},
		{ID: "current_state", Title: "Current State Review",
			Description: "Present current state analysis, bottlenecks, and waste",
			Facilitator: "VSM Facilitator", Duration: 90 * time.Minute,
			Materials: []string{"Current state map", "KPI data"}},
		{ID: "future_state", Title: "Future State Design",
			Description: "Review future state design and improvement potentials",
			Facilitator: "Lean Engineer", Duration: 90 * time.Minute,
			Materials: []string{"Future state map", "Design principles"}},
		{ID: "break", Title: "Break",
			Description: "Coffee break and informal discussion", Duration: 30 * time.Minute},
		{ID: "implementation", Title: "Implementation Planning",
			Description: "Review catalogue of measures, enter ratings, costs and due dates, and create action plan",
			Facilitator: "Operations Manager", Duration: 120 * time.Minute,
			Materials: []string{"Catalogue of measures", "Action plan template"}},
		{ID: "next_steps", Title: "Next Steps and Follow-up",
			Description: "Agree on next steps, responsibilities, and follow-up actions",
			Facilitator: "VSM Facilitator", Duration: 60 * time.Minute},
	}
	var offset time.Duration
	for i := range workshop.Agenda {
		workshop.Agenda[i].Time = offset
		offset += workshop.Agenda[i].Duration
	}
	workshop.Duration = offset

	return workshop
}

// CreateImplementationPlan creates a phased implementation plan without a
// start date (see ImplementationPhase for how dates are set). Returns an
// error when a measure's due date falls before its phase starts.
func (p *Planner) CreateImplementationPlan(catalogue *CatalogueOfMeasures) (*ImplementationPlan, error) {
	if catalogue == nil {
		return nil, errors.New("catalogue cannot be nil")
	}
	return p.createImplementationPlan(catalogue, nil)
}

// createImplementationPlan groups the measures by priority into phases and
// dates them from start and the measures' due dates.
func (p *Planner) createImplementationPlan(catalogue *CatalogueOfMeasures, start *time.Time) (*ImplementationPlan, error) {
	plan := &ImplementationPlan{
		ID:        fmt.Sprintf("impl_%s", catalogue.ID),
		Status:    "planned",
		CreatedAt: time.Now(),
	}

	byPriority := map[int][]ImprovementMeasure{}
	for _, measure := range catalogue.Measures {
		if measure.Priority == 0 {
			plan.UnprioritizedMeasures = append(plan.UnprioritizedMeasures, measure.ID)
			continue
		}
		byPriority[measure.Priority] = append(byPriority[measure.Priority], measure)
	}

	plan.Phases = []ImplementationPhase{
		{ID: "phase_1", Name: "Quick Wins", Description: "Implement priority 1 measures (impact rated above effort)", Status: "planned"},
		{ID: "phase_2", Name: "Core Improvements", Description: "Implement priority 2 measures (impact rated equal to effort)", Status: "planned"},
		{ID: "phase_3", Name: "Advanced Improvements", Description: "Implement priority 3 measures (effort rated above impact)", Status: "planned"},
	}
	chain := cloneTime(start)
	for i := range plan.Phases {
		phase := &plan.Phases[i]
		measures := byPriority[i+1]
		phase.Measures = []string{}
		for _, m := range measures {
			phase.Measures = append(phase.Measures, m.ID)
		}
		if len(measures) == 0 {
			continue
		}
		phase.StartDate = cloneTime(chain)
		if phase.StartDate != nil {
			for _, m := range measures {
				if due := m.SMART.TimeBound; due != nil && due.Before(*phase.StartDate) {
					return nil, fmt.Errorf("measure %q (priority %d) is due on %s, before %s starts on %s; "+
						"phases run one after another, so change its due date or ratings",
						m.ID, m.Priority, due.Format(time.DateOnly), phase.ID, phase.StartDate.Format(time.DateOnly))
				}
			}
		}
		phase.EndDate = latestDueDate(measures)
		chain = cloneTime(phase.EndDate)
	}

	plan.Timeline = Timeline{
		StartDate:     cloneTime(start),
		EndDate:       latestDueDate(catalogue.Measures),
		CriticalPath:  append([]string{}, plan.Phases[0].Measures...),
		KeyMilestones: []string{"milestone_quick_wins", "milestone_mid_term", "milestone_full_implementation"},
	}
	if plan.Timeline.StartDate != nil && plan.Timeline.EndDate != nil {
		d := plan.Timeline.EndDate.Sub(*plan.Timeline.StartDate)
		plan.Timeline.Duration = &d
	}

	plan.Milestones = []Milestone{
		{ID: "milestone_quick_wins", Name: "Quick Wins Completed",
			Description: "All priority 1 measures implemented", Date: cloneTime(plan.Phases[0].EndDate)},
		{ID: "milestone_mid_term", Name: "Mid-term Review",
			Description: "Review progress (date to be set by the team)"},
		{ID: "milestone_full_implementation", Name: "Full Implementation",
			Description: "All measures implemented and stable", Date: cloneTime(plan.Timeline.EndDate)},
	}

	totals := calculateCostTotals(catalogue.Measures)
	budget := "Implementation budget: total entered investment"
	if totals.Investment == nil {
		budget = "Implementation budget: not estimated (investment not entered for every measure)"
	}
	plan.ResourceRequirements = []ResourceRequirement{
		{ID: "resource_personnel", Type: "personnel",
			Description: "Cross-functional improvement team (size to be set by the team)",
			Timeframe:   "immediate", Status: "planned"},
		{ID: "resource_budget", Type: "budget", Description: budget,
			Quantity: totals.Investment, Timeframe: "short_term", Status: "planned"},
		{ID: "resource_training", Type: "training",
			Description: "Lean training for team members (hours to be set by the team)",
			Timeframe:   "immediate", Status: "planned"},
	}

	if len(plan.UnprioritizedMeasures) > 0 {
		plan.Notes = append(plan.Notes, fmt.Sprintf("%d measures are not prioritised and are in no phase: "+
			"enter their impact and effort ratings.", len(plan.UnprioritizedMeasures)))
	}
	if start == nil || plan.Timeline.EndDate == nil {
		plan.Notes = append(plan.Notes, "Phase and timeline dates are set only from the entered start "+
			"date and due dates; dates that need a value not entered are not set.")
	}
	return plan, nil
}

// latestDueDate returns the latest due date of measures, or nil when there
// are none or any has no due date.
func latestDueDate(measures []ImprovementMeasure) *time.Time {
	if len(measures) == 0 {
		return nil
	}
	var latest *time.Time
	for _, m := range measures {
		due := m.SMART.TimeBound
		if due == nil {
			return nil
		}
		if latest == nil || due.After(*latest) {
			latest = due
		}
	}
	return cloneTime(latest)
}

func cloneTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	v := *t
	return &v
}

// riskChecklist is the list of typical implementation risks the plan
// asks the team to assess (IDs for RiskInput).
func riskChecklist() []Risk {
	return []Risk{
		{ID: "risk_resistance", Description: "Employee resistance to change",
			Mitigation: "Comprehensive change management and communication plan", Status: "identified"},
		{ID: "risk_resource_shortage", Description: "Insufficient resources allocated",
			Mitigation: "Detailed resource planning and stakeholder buy-in", Status: "identified"},
		{ID: "risk_technical_issues", Description: "Technical difficulties during implementation",
			Mitigation: "Pilot testing and phased implementation approach", Status: "identified"},
		{ID: "risk_scope_creep", Description: "Scope expansion beyond planned measures",
			Mitigation: "Strict change control and scope management", Status: "identified"},
	}
}

// AssessRisks returns the risk checklist, not assessed (see
// PlanImplementationWithInputs to enter probability and impact).
func (p *Planner) AssessRisks(design *DesignResult, implementation *ImplementationPlan) []Risk {
	risks, _ := p.assessRisks(nil)
	return risks
}

// assessRisks returns the risk checklist with the entered probability and
// impact; severity = probability × impact when both are entered. Assessed
// risks come first, by severity (highest first); unassessed risks follow in
// checklist order.
func (p *Planner) assessRisks(inputs map[string]RiskInput) ([]Risk, error) {
	risks := riskChecklist()
	known := map[string]bool{}
	for _, r := range risks {
		known[r.ID] = true
	}
	if err := validateRiskInputs(inputs, known); err != nil {
		return nil, err
	}
	for i := range risks {
		in := inputs[risks[i].ID]
		risks[i].Probability, risks[i].Impact = cloneFloat(in.Probability), cloneFloat(in.Impact)
		if in.Probability != nil && in.Impact != nil {
			risks[i].Severity = F(*in.Probability * *in.Impact)
		} else {
			risks[i].Note = "Not assessed: probability and impact not entered"
			if in.Probability != nil || in.Impact != nil {
				risks[i].Note = "Not assessed: probability or impact not entered"
			}
		}
	}
	sort.SliceStable(risks, func(i, j int) bool {
		a, b := risks[i].Severity, risks[j].Severity
		if a == nil || b == nil {
			return a != nil && b == nil
		}
		return *a > *b
	})
	return risks, nil
}
