package vsm

import (
	"errors"
	"fmt"
	"sort"
	"strings"
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

// ImprovementPotential represents a potential improvement in the value stream.
//
// Its ID is stable (built from what the analysis found, e.g.
// "bottleneck_<process ID>", "waste_<type>", "flow_push_to_pull"), so
// ratings can be entered against it (see MeasureInput). Impact and effort
// are never estimated: they are the user's ratings, and without both the
// potential is not prioritised (Priority 0, PriorityNote says so).
type ImprovementPotential struct {
	ID          string `json:"id"`
	Type        string `json:"type"`     // "bottleneck", "waste", "flow", "quality", etc.
	Location    string `json:"location"` // Process or area where improvement applies
	ProcessID   string `json:"process_id,omitempty"`
	Description string `json:"description"`
	// Priority is 1=high, 2=medium, 3=low from the ratings (see
	// RatingPriority); 0 (omitted) = not prioritised.
	Priority     int    `json:"priority,omitempty"`
	ImpactRating *int   `json:"impact_rating,omitempty"` // entered, 1 (lowest) to 5 (highest)
	EffortRating *int   `json:"effort_rating,omitempty"` // entered, 1 (lowest) to 5 (highest)
	PriorityNote string `json:"priority_note,omitempty"`
}

// DesignPrinciple represents the 8 VSM design principles from Rother & Shook
type DesignPrinciple struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Applied     bool   `json:"applied"`
	Status      string `json:"status"` // "not_applicable", "planned", "implemented"
}

// ContinuousImprovement represents continuous improvement flashes. Like
// ImprovementPotential, its priority comes only from entered ratings
// (keyed by its stable ID, e.g. "ci_bottleneck_<process ID>").
type ContinuousImprovement struct {
	ID           string    `json:"id"`
	ProcessID    string    `json:"process_id"`
	Description  string    `json:"description"`
	Type         string    `json:"type"`               // "kaizen", "quick_win", "major_change"
	Priority     int       `json:"priority,omitempty"` // 1-3 from ratings; 0 = not prioritised
	ImpactRating *int      `json:"impact_rating,omitempty"`
	EffortRating *int      `json:"effort_rating,omitempty"`
	PriorityNote string    `json:"priority_note,omitempty"`
	Status       string    `json:"status"` // "identified", "planned", "implemented"
	CreatedAt    time.Time `json:"created_at"`
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
// like DesignFutureState. Improvement potentials are not prioritised (see
// DesignFutureStateWithInputs).
func (d *Designer) DesignFutureStateWithTargets(current *ValueStream, analysis *AnalysisResult, targets *FutureStateTargets) (*DesignResult, error) {
	return d.DesignFutureStateWithInputs(current, analysis, targets, nil)
}

// notPrioritisedNote is reported when some potentials or continuous
// improvements have no impact and effort ratings.
const notPrioritisedNote = "%d of %d improvement potentials and continuous improvements are not " +
	"prioritised: their impact and effort ratings (1 to 5) were not entered. They keep the order " +
	"the analysis found them in; enter ratings by ID (MeasureInput) to prioritise them."

// DesignFutureStateWithInputs is DesignFutureStateWithTargets that also
// applies the user's impact and effort ratings, keyed by improvement
// potential or continuous improvement ID (see MeasureInput; only the
// ratings are used here). Rated items get a priority (see RatingPriority);
// items without both ratings are not prioritised (Priority 0, with a
// note). Returns an error for a rating outside 1–5 or an input ID that is
// neither a potential nor a continuous improvement.
func (d *Designer) DesignFutureStateWithInputs(current *ValueStream, analysis *AnalysisResult, targets *FutureStateTargets, inputs map[string]MeasureInput) (*DesignResult, error) {
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
	result.Notes = append(result.Notes, issueNotes(result.ConsistencyIssues)...)

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

	// Apply the user's ratings (validated against both kinds of ID)
	known := map[string]bool{}
	for _, p := range potentials {
		known[p.ID] = true
	}
	for _, ci := range improvements {
		known[ci.ID] = true
	}
	if err := validateMeasureInputs(inputs, known, "improvement potential or continuous improvement"); err != nil {
		return nil, err
	}
	for i := range result.ImprovementPotentials {
		p := &result.ImprovementPotentials[i]
		p.ImpactRating, p.EffortRating, p.Priority, p.PriorityNote = rate(inputs[p.ID], nil, nil)
	}
	prioritizeContinuousImprovements(improvements, inputs)
	result.ContinuousImprovements = improvements

	unrated, total := 0, len(result.ImprovementPotentials)+len(improvements)
	for _, p := range result.ImprovementPotentials {
		if p.Priority == 0 {
			unrated++
		}
	}
	for _, ci := range improvements {
		if ci.Priority == 0 {
			unrated++
		}
	}
	if unrated > 0 {
		result.Notes = append(result.Notes, fmt.Sprintf(notPrioritisedNote, unrated, total))
	}

	return result, nil
}

// IdentifyImprovementPotentials identifies potential improvements based on
// analysis, in a fixed order (bottlenecks, waste types in sorted order,
// push-to-pull, flow interruptions). They are not prioritised: impact and
// effort are never estimated (see PrioritizeImprovementPotentials to apply
// the user's ratings).
func (d *Designer) IdentifyImprovementPotentials(vs *ValueStream, analysis *AnalysisResult) ([]ImprovementPotential, error) {
	if analysis == nil {
		return nil, errors.New("analysis result cannot be nil")
	}
	var potentials []ImprovementPotential

	// Based on bottlenecks
	for _, bottleneck := range analysis.Bottlenecks {
		potentials = append(potentials, ImprovementPotential{
			ID:          fmt.Sprintf("bottleneck_%s", bottleneck.ProcessID),
			Type:        "bottleneck",
			Location:    bottleneck.ProcessName,
			ProcessID:   bottleneck.ProcessID,
			Description: fmt.Sprintf("Optimize bottleneck process: %s", bottleneck.Description),
		})
	}

	// Based on waste analysis (sorted, so the order is stable)
	wasteTypes := make([]string, 0, len(analysis.WasteAnalysis.WasteByType))
	for wasteType := range analysis.WasteAnalysis.WasteByType {
		wasteTypes = append(wasteTypes, wasteType)
	}
	sort.Strings(wasteTypes)
	for _, wasteType := range wasteTypes {
		if amount := analysis.WasteAnalysis.WasteByType[wasteType]; amount > 0 {
			potentials = append(potentials, ImprovementPotential{
				ID:          fmt.Sprintf("waste_%s", wasteType),
				Type:        "waste",
				Location:    "Value Stream",
				Description: fmt.Sprintf("Reduce %s waste (%.2f time units)", wasteType, amount),
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
		})
	}

	for _, interruption := range analysis.FlowAnalysis.FlowInterruptions {
		potentials = append(potentials, ImprovementPotential{
			ID:          fmt.Sprintf("flow_%s_%s", interruption.Type, interruption.Location),
			Type:        "flow",
			Location:    interruption.Location,
			ProcessID:   processIDOf(vs, interruption.Location),
			Description: interruption.Description,
		})
	}

	for i := range potentials {
		_, _, potentials[i].Priority, potentials[i].PriorityNote = rate(MeasureInput{}, nil, nil)
	}
	return potentials, nil
}

// processIDOf returns the ID of the process of vs whose ID or name is
// location, or "".
func processIDOf(vs *ValueStream, location string) string {
	if vs == nil {
		return ""
	}
	for _, p := range vs.Processes {
		if p.ID == location || p.Name == location {
			return p.ID
		}
	}
	return ""
}

// PrioritizeImprovementPotentials returns a copy of potentials with the
// user's impact and effort ratings applied (inputs keyed by potential ID;
// other MeasureInput fields are not used here). A potential with both
// ratings gets the priority RatingPriority gives; one without stays not
// prioritised (Priority 0, PriorityNote says which rating is missing). The
// order is not changed. Returns an error for a rating outside 1–5, or an
// input whose ID is not one of the potentials.
func (d *Designer) PrioritizeImprovementPotentials(potentials []ImprovementPotential, inputs map[string]MeasureInput) ([]ImprovementPotential, error) {
	known := map[string]bool{}
	for _, p := range potentials {
		known[p.ID] = true
	}
	if err := validateMeasureInputs(inputs, known, "improvement potential"); err != nil {
		return nil, err
	}
	out := append([]ImprovementPotential(nil), potentials...)
	for i := range out {
		p := &out[i]
		p.ImpactRating, p.EffortRating, p.Priority, p.PriorityNote = rate(inputs[p.ID], nil, nil)
	}
	return out, nil
}

// prioritizeContinuousImprovements applies the ratings to improvements as
// PrioritizeImprovementPotentials does (inputs already validated).
func prioritizeContinuousImprovements(improvements []ContinuousImprovement, inputs map[string]MeasureInput) {
	for i := range improvements {
		ci := &improvements[i]
		ci.ImpactRating, ci.EffortRating, ci.Priority, ci.PriorityNote = rate(inputs[ci.ID], nil, nil)
	}
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
// yield is 1. When processing time is computed from cycle time × order
// quantity ÷ resources (A.5), the cycle time is set so that it gives the
// value adding time.
//
// A process whose value adding time is unknown (not entered, or
// impossible) has no known ideal time: its ideal process, processing,
// cycle and lead times are unknown, never 0. Its times and stock are then
// left not entered (an ideal 0 for setup, idle time, NVAT or stock would,
// by the blank-parts rule of ResolveParameters, make the blank process and
// lead time 0), so CheckConsistency reports the process's lead time and
// value adding time as missing data, and the stream totals that need them
// are not computed. Its yield is still 1.
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
	if vat == nil {
		for _, field := range []**float64{
			&p.CycleTime, &p.SetupTime, &p.HandlingTime, &p.WaitingTimeProcess, &p.AncillaryTime,
			&p.IdleTime, &p.WaitingTime, &p.TransportTime, &p.StorageTime,
			&p.StockQuantity, &p.RangeOfInventory,
			&p.NonValueAddingTime, &p.NecessaryNonValueAddingTime,
		} {
			*field = nil
		}
	} else if processingFormula(p) && positive(p.OrderQuantity) {
		resources, _ := resourceCount(p)
		p.CycleTime = F(*vat * resources / *p.OrderQuantity)
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
// Returns an error if a target is invalid, names a process or inventory ID
// that is not in the current state, or would make the future state
// contradict itself where the current state does not (for example an idle
// time target below the range of an inventory that is the process's stock;
// target the process's stock_quantity to move that inventory with it).
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
		// The user's waste assumptions and information-flow rating carry
		// over unchanged: without targets the future state knows exactly
		// what the current state knows.
		WasteAssumptions:       current.WasteAssumptions.Clone(),
		InformationFlowQuality: cloneFloat(current.InformationFlowQuality),
		CreatedAt:              time.Now(),
		UpdatedAt:              time.Now(),
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

	// Targets that are consistent process by process can still contradict
	// across elements (an inventory linked to a process, see
	// Inventory.ProcessID): reject a contradiction the current state does
	// not have.
	if err := newContradiction(current, future); err != nil {
		return nil, err
	}

	// Calculate future KPIs
	future.KPIs = d.calculator.CalculateKPIs(future)

	return future, nil
}

// newContradiction returns an error naming the first contradiction (see
// CheckConsistency) in future that current does not have. Future process
// IDs are the current ones + "_future"; inventory IDs are the same.
func newContradiction(current, future *ValueStream) error {
	key := func(issue ConsistencyIssue) string {
		id := issue.ID
		if issue.Element == "process" {
			id = strings.TrimSuffix(id, "_future")
		}
		return issue.Element + "\x00" + id + "\x00" + issue.Field
	}
	had := map[string]bool{}
	for _, issue := range CheckConsistency(current) {
		had[key(issue)] = true
	}
	for _, issue := range CheckConsistency(future) {
		if issue.Kind == IssueContradiction && !had[key(issue)] {
			return fmt.Errorf("these targets would make the value stream inconsistent: %s", issue.Message)
		}
	}
	return nil
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
	ctx := d.calculator.contextOf(vs)
	// Check if any process has cycle time close to customer takt
	for _, process := range vs.Processes {
		params, ok := d.calculator.resolvedIn(process.Parameters, ctx)
		if !ok {
			continue // unsupported time unit: cannot be compared with takt
		}
		if ct := params.CycleTime; positive(ct) {
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

// checkContinuousFlowPrinciple checks if continuous flow principle is
// applied: the stream's idle time (processes and inventory points, the
// stream figures CalculateKPIs uses) is less than half its process time.
// Not applied when either total is unknown.
func (d *Designer) checkContinuousFlowPrinciple(vs *ValueStream) bool {
	f := d.calculator.figures(vs)
	if f.idleTime == nil || f.processTime == nil || *f.processTime <= 0 {
		return false
	}
	return *f.idleTime / *f.processTime < 0.5 // Less than 50% idle time indicates good flow
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

	// Generate improvements based on bottlenecks. Priorities come only
	// from entered ratings (see DesignFutureStateWithInputs): none here.
	for _, bottleneck := range analysis.Bottlenecks {
		improvements = append(improvements, ContinuousImprovement{
			ID:        fmt.Sprintf("ci_bottleneck_%s", bottleneck.ProcessID),
			ProcessID: bottleneck.ProcessID,
			Description: fmt.Sprintf("Optimize bottleneck process '%s' to reduce utilization from %.1f%%",
				bottleneck.ProcessName, bottleneck.Utilization*100),
			Type:      "kaizen",
			Status:    "identified",
			CreatedAt: now,
		})
	}

	// Generate improvements based on waste (only when the waste is known)
	if w := analysis.WasteAnalysis; w.TotalWaste != nil && *w.TotalWaste > 0 && w.WasteReductionPotential != nil {
		improvements = append(improvements, ContinuousImprovement{
			ID:        "ci_waste_reduction",
			ProcessID: "", // System-wide
			Description: fmt.Sprintf("Implement 5S and standard work to reduce waste by %.1f%%",
				*w.WasteReductionPotential / *w.TotalWaste * 100),
			Type:      "major_change",
			Status:    "identified",
			CreatedAt: now,
		})
	}

	// Generate improvements based on flow issues. The ID names the
	// location too, so two interruptions of one type get distinct IDs.
	for _, interruption := range analysis.FlowAnalysis.FlowInterruptions {
		improvements = append(improvements, ContinuousImprovement{
			ID:          fmt.Sprintf("ci_flow_%s_%s", interruption.Type, interruption.Location),
			ProcessID:   processIDOf(vs, interruption.Location),
			Description: interruption.Description,
			Type:        "quick_win",
			Status:      "identified",
			CreatedAt:   now,
		})
	}

	for i := range improvements {
		_, _, improvements[i].Priority, improvements[i].PriorityNote = rate(MeasureInput{}, nil, nil)
	}
	return improvements
}
