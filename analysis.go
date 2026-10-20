package vsm

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// AnalysisResult contains the results of value stream analysis
type AnalysisResult struct {
	ValueStreamID    string             `json:"value_stream_id"`
	Bottlenecks      []BottleneckInfo   `json:"bottlenecks"`
	IdleTimeAnalysis IdleTimeAnalysis   `json:"idle_time_analysis"`
	WasteAnalysis    WasteAnalysis      `json:"waste_analysis"`
	FlowAnalysis     FlowAnalysis       `json:"flow_analysis"`
	KPIs             map[string]float64 `json:"kpis"`
	Recommendations  []string           `json:"recommendations"`
	// Notes explains analysis steps that were skipped for missing inputs.
	Notes []string `json:"notes,omitempty"`
	// ConsistencyIssues lists stored values that disagree with the values
	// computed from their parts (see CheckConsistency). The analysis uses
	// the computed values.
	ConsistencyIssues []ConsistencyIssue `json:"consistency_issues,omitempty"`
	AnalyzedAt        time.Time          `json:"analyzed_at"`
}

// BottleneckInfo contains information about a bottleneck process
type BottleneckInfo struct {
	ProcessID   string `json:"process_id"`
	ProcessName string `json:"process_name"`
	// Capacity is units the process can make per CapacityPeriod
	// (Customer.AvailableTime ÷ cycle time).
	Capacity float64 `json:"capacity"`
	// CapacityPeriod is the demand period Capacity is counted over.
	CapacityPeriod DemandPeriod `json:"capacity_period,omitempty"`
	// Utilization is demand ÷ capacity as a fraction (1.0 = fully loaded).
	// It is not capped: above 1.0 means demand exceeds capacity.
	Utilization     float64 `json:"utilization"`
	BottleneckLevel string  `json:"bottleneck_level"` // "high", "medium", "low"
	Description     string  `json:"description"`
}

// IdleTimeAnalysis contains analysis of idle times across the value stream.
//
// The numbers are the stream figures CalculateKPIs uses (totals.go): a
// total is nil (omitted in JSON) when an element on the map cannot give its
// part, and CheckConsistency names the missing value (Kind
// IssueMissingData).
type IdleTimeAnalysis struct {
	// TotalIdleTime is the stream's idle time: each process's idle time
	// (0 for a process whose lead time is its process time alone) plus the
	// time of each inventory point counted on its own. nil when unknown.
	TotalIdleTime *float64 `json:"total_idle_time,omitempty"`
	// IdleTimeByProcess is each process's idle time, for the processes
	// whose idle time is known.
	IdleTimeByProcess map[string]float64 `json:"idle_time_by_process"`
	// IdleTimeByInventory is the waiting time of each inventory point counted
	// on its own (see Inventory.ProcessID), keyed by inventory ID, when
	// known. It is part of TotalIdleTime and of IdleTimeBreakdown.WaitingTime.
	IdleTimeByInventory map[string]float64 `json:"idle_time_by_inventory,omitempty"`
	// IdleTimeBreakdown splits TotalIdleTime into its entered (or
	// computed) parts; an idle time given only as a total is not split, so
	// the parts can add up to less. nil when TotalIdleTime is unknown.
	IdleTimeBreakdown *IdleTimeBreakdown `json:"idle_time_breakdown,omitempty"`
	// IdleTimeEfficiency is process time ÷ (process time + idle time), the
	// share of the lead time spent in processes, as a fraction (0-1): the
	// process_efficiency KPI. nil when unknown.
	IdleTimeEfficiency *float64 `json:"idle_time_efficiency,omitempty"`
}

// IdleTimeBreakdown breaks down idle time by category
type IdleTimeBreakdown struct {
	TransportTime float64 `json:"transport_time"`
	WaitingTime   float64 `json:"waiting_time"`
	StorageTime   float64 `json:"storage_time"`
}

// WasteAnalysis contains analysis of different types of waste (Muda), in
// the reference time unit. Waste is taken from the times the lead time
// counts (the stream figures, totals.go), so the same stock gives the same
// waste however it is drawn (process stock, linked or separate inventory).
//
// Measured waste is reported as is: transport time, non-value adding time,
// the defective share (scrap + rework rate) of process time, and the
// waiting time (process waiting time plus the time of inventories counted
// on their own, WaitingAndInventoryWaste). Splits the model cannot measure
// come from the user (ValueStream.WasteAssumptions) and are not estimated
// without them: the inventory / waiting split of waiting time, the motion
// share of handling time, and the reduction potential. Notes names the
// assumption to enter for each.
//
// Nothing is reported (TotalWaste and WaitingAndInventoryWaste nil,
// WasteByType empty) when the stream lead time is unknown, or a process
// with scrap or rework has no known process time (CheckConsistency names
// the missing value).
type WasteAnalysis struct {
	// TotalWaste is the sum of the 7 types. nil (omitted in JSON) when
	// unknown, including when motion or over-production waste is not
	// estimated (see WasteByType). It does not need the inventory /
	// waiting split: the two add up to WaitingAndInventoryWaste.
	TotalWaste *float64 `json:"total_waste,omitempty"`
	// WasteByType holds the 7 types of waste, each only when it is known:
	// "inventory" and "waiting" need inventory_share_of_waiting (unless
	// there is no waiting time), "motion" needs waste_share_of_handling
	// (unless there is no handling time), and "over_production" needs the
	// operating_time of each process whose utilization is below 0.7.
	// Empty when the stream figures are unknown.
	WasteByType map[string]float64 `json:"waste_by_type"`
	// WaitingAndInventoryWaste is the measured waiting time that inventory
	// and waiting waste split between them (they add up to it). nil when
	// the stream figures are unknown.
	WaitingAndInventoryWaste *float64 `json:"waiting_and_inventory_waste,omitempty"`
	// WasteByProcess is each process's waste, for the processes whose
	// waste is known.
	WasteByProcess map[string]float64 `json:"waste_by_process"`
	// WasteByInventory is the waste of each inventory point counted on its
	// own (its time: inventory plus waiting waste), when known.
	WasteByInventory map[string]float64 `json:"waste_by_inventory,omitempty"`
	// WasteReductionPotential is TotalWaste × the user's
	// reduction_potential (waste_assumptions). nil when either is unknown.
	WasteReductionPotential *float64 `json:"waste_reduction_potential,omitempty"`
	// Notes names the user input to enter for waste that is not estimated.
	Notes []string `json:"notes,omitempty"`
}

// FlowAnalysis contains analysis of product and information flow
type FlowAnalysis struct {
	// FlowType is push or pull, judged from the processes whose waiting
	// time and process time are both known (see AnalyzeFlow). Empty
	// (omitted in JSON) when no process has them: not determined.
	FlowType FlowType `json:"flow_type,omitempty"`
	// FlowEfficiency is value adding time ÷ lead time: the
	// value_added_ratio KPI, with the same validity. nil (omitted in JSON)
	// when unknown: a total unknown, lead time 0, or a value adding time
	// larger than its process or lead time.
	FlowEfficiency    *float64           `json:"flow_efficiency,omitempty"`
	FlowInterruptions []FlowInterruption `json:"flow_interruptions"`
	// InformationFlowQuality is the user's rating of the information flow
	// (ValueStream.InformationFlowQuality, 0–1). nil (omitted in JSON) when
	// not entered or outside 0–1: not estimated.
	InformationFlowQuality *float64 `json:"information_flow_quality,omitempty"`
	// Notes explains what is not determined and which input it needs.
	Notes []string `json:"notes,omitempty"`
}

// FlowInterruption represents an interruption in the flow
type FlowInterruption struct {
	Type        string  `json:"type"`      // "stockout", "quality", "capacity", etc.
	Location    string  `json:"location"`  // Process or connection point
	Frequency   float64 `json:"frequency"` // How often it occurs: defect share as a fraction for "quality", waiting ÷ process time for "inventory", 1 for "capacity"
	Impact      float64 `json:"impact"`    // Time or cost impact
	Description string  `json:"description"`
}

// Analyzer provides methods for analyzing value streams
type Analyzer struct {
	calculator *Calculator
}

// NewAnalyzer creates a new VSM analyzer
func NewAnalyzer() *Analyzer {
	return &Analyzer{
		calculator: NewCalculator(),
	}
}

// AnalyzeCurrentState performs a comprehensive analysis of the current value stream state
func (a *Analyzer) AnalyzeCurrentState(vs *ValueStream) (*AnalysisResult, error) {
	if vs == nil {
		return nil, errors.New("value stream cannot be nil")
	}

	result := &AnalysisResult{
		ValueStreamID: vs.ID,
		AnalyzedAt:    time.Now(),
		KPIs:          make(map[string]float64),
	}

	// Every step reads the same stream figures (totals.go).
	f := a.calculator.figures(vs)

	// Identify bottlenecks
	result.Bottlenecks = a.identifyBottlenecks(vs, f)

	// Analyze idle times
	idleAnalysis := analyzeIdleTimes(vs, f)
	result.IdleTimeAnalysis = *idleAnalysis

	// Analyze waste
	wasteAnalysis := a.analyzeWaste(vs, f)
	result.WasteAnalysis = *wasteAnalysis

	// Analyze flow
	flowAnalysis := analyzeFlow(vs, f)
	result.FlowAnalysis = *flowAnalysis

	// Calculate KPIs
	result.KPIs = a.calculator.calculateKPIs(vs, f)

	// Generate recommendations
	result.Recommendations = a.GenerateRecommendations(result)

	if len(vs.Processes) > 0 && !a.hasLoadInputs(vs) {
		result.Notes = append(result.Notes, missingLoadInputsNote)
	}
	result.Notes = append(result.Notes, wasteAnalysis.Notes...)
	result.Notes = append(result.Notes, flowAnalysis.Notes...)

	result.ConsistencyIssues = checkConsistency(vs, f)
	result.Notes = append(result.Notes, issueNotes(result.ConsistencyIssues)...)

	return result, nil
}

// IdentifyBottlenecks identifies bottleneck processes in the value stream.
//
// Utilization needs Customer.Demand, Customer.DemandPeriod,
// Customer.AvailableTime and each process's CycleTime (see
// CalculateProcessLoad). Processes whose utilization cannot be computed are
// skipped, so with those inputs missing the result is empty.
func (a *Analyzer) IdentifyBottlenecks(vs *ValueStream) ([]BottleneckInfo, error) {
	return a.identifyBottlenecks(vs, a.calculator.figures(vs)), nil
}

// identifyBottlenecks is IdentifyBottlenecks for vs whose stream figures
// are f.
func (a *Analyzer) identifyBottlenecks(vs *ValueStream, f streamFigures) []BottleneckInfo {
	var bottlenecks []BottleneckInfo

	// Calculate utilization for each process
	for i, process := range vs.Processes {
		capacity, utilization, ok := a.processLoad(vs, f, i)
		// Consider processes with >70% utilization as potential bottlenecks
		if !ok || utilization <= 0.7 {
			continue
		}
		bottleneckLevel := "medium"
		if utilization > 0.9 {
			bottleneckLevel = "high"
		}
		bottlenecks = append(bottlenecks, BottleneckInfo{
			ProcessID:       process.ID,
			ProcessName:     process.Name,
			Capacity:        capacity,
			CapacityPeriod:  vs.Customer.DemandPeriod,
			Utilization:     utilization,
			BottleneckLevel: bottleneckLevel,
			Description:     fmt.Sprintf("Process %s has %.1f%% utilization", process.Name, utilization*100),
		})
	}

	// Sort by utilization (highest first)
	sort.Slice(bottlenecks, func(i, j int) bool {
		return bottlenecks[i].Utilization > bottlenecks[j].Utilization
	})

	return bottlenecks
}

// missingLoadInputsNote is reported when utilization cannot be computed.
const missingLoadInputsNote = "Process capacity and utilization not computed: set customer demand, " +
	"demand_period and available_time (available time per demand period, in the reference time unit), " +
	"and each process's cycle_time"

// consistencyNoteFormat is reported when the input contradicts itself.
const consistencyNoteFormat = "%d stored value(s) disagree with the values computed from their parts, or are " +
	"impossible; calculations use the computed values (see consistency_issues, kind \"contradiction\")"

// missingDataNoteFormat is reported when stream totals cannot be computed.
const missingDataNoteFormat = "%d value(s) needed for stream totals are missing; the totals and KPIs that " +
	"need them are not computed (see consistency_issues, kind \"missing_data\")"

// issueNotes returns the notes for issues: one counting contradictions,
// one counting missing data.
func issueNotes(issues []ConsistencyIssue) []string {
	contradictions, missing := splitIssues(issues)
	var notes []string
	if len(contradictions) > 0 {
		notes = append(notes, fmt.Sprintf(consistencyNoteFormat, len(contradictions)))
	}
	if len(missing) > 0 {
		notes = append(notes, fmt.Sprintf(missingDataNoteFormat, len(missing)))
	}
	return notes
}

// hasLoadInputs reports whether the customer data needed for utilization is set.
func (a *Analyzer) hasLoadInputs(vs *ValueStream) bool {
	c := vs.Customer
	return c != nil && positive(c.Demand) && positive(c.AvailableTime) && c.DemandPeriod.IsValid()
}

// CalculateProcessLoad returns a process's capacity and utilization for the
// customer's demand period.
//
//	capacity    = AvailableTime × #RES ÷ CT          (units per DemandPeriod)
//	utilization = Demand × CT ÷ (AvailableTime × #RES) (fraction; = CT ÷ (takt × #RES))
//
// #RES is the process's ResourceCount: resources working in parallel
// multiply capacity, as in processing time = CT × PQ ÷ #RES (A.5). Not
// entered, it counts as 1 (a process runs on at least one resource).
// Entered as 0 the process has no capacity, so its utilization is
// undefined: ok is false (CheckConsistency reports the 0 when there is
// demand; a negative count is impossible and reported too).
//
// Demand and AvailableTime are both per Customer.DemandPeriod, so capacity and
// demand are compared over the same period. CT is the process's CycleTime,
// converted to the reference time unit (Calculator.ReferenceTimeUnit), which
// is the unit AvailableTime is given in. Utilization is not capped, so a value
// above 1 means demand exceeds capacity.
//
// ok is false (and nothing is assumed) when the customer, Demand,
// DemandPeriod, AvailableTime or the process's CycleTime is not entered, or
// Demand, AvailableTime or CycleTime is 0, ResourceCount is entered as 0 or
// less, or the process's time unit is not supported.
func (a *Analyzer) CalculateProcessLoad(process Process, vs *ValueStream) (capacity, utilization float64, ok bool) {
	if vs == nil || !a.hasLoadInputs(vs) {
		return 0, 0, false
	}
	params, unitOK := a.calculator.resolvedIn(process.Parameters, a.calculator.contextOf(vs))
	effective, known := cycleTimePerResource(params, unitOK, process.Parameters)
	return load(vs.Customer, effective, known)
}

// processLoad is CalculateProcessLoad for process i of vs, whose stream
// figures are f.
func (a *Analyzer) processLoad(vs *ValueStream, f streamFigures, i int) (capacity, utilization float64, ok bool) {
	if vs == nil || !a.hasLoadInputs(vs) {
		return 0, 0, false
	}
	effective, known := effectiveCycleTime(vs, f, i)
	return load(vs.Customer, effective, known)
}

// load returns capacity and utilization at an effective cycle time (see
// CalculateProcessLoad) for cust, which has the load inputs
// (hasLoadInputs); ok is false when the cycle time is unknown or 0.
func load(cust *Customer, effective float64, known bool) (capacity, utilization float64, ok bool) {
	if !known || effective <= 0 {
		return 0, 0, false
	}
	available := *cust.AvailableTime
	return available / effective, *cust.Demand * effective / available, true
}

// effectiveCycleTime returns the cycle time per resource of process i of
// vs, whose stream figures are f (see cycleTimePerResource).
func effectiveCycleTime(vs *ValueStream, f streamFigures, i int) (float64, bool) {
	pf := f.processes[i]
	return cycleTimePerResource(pf.params, pf.unitOK, vs.Processes[i].Parameters)
}

// cycleTimePerResource returns a process's cycle time per resource, CT ÷
// #RES, in the reference time unit: the time between two units leaving
// the process, the one figure that capacity (available time ÷ CT ÷ #RES,
// see CalculateProcessLoad) depends on. resolved are its parameters
// resolved in the reference unit (unitOK as from resolvedIn), entered its
// parameters as entered. ok is false when it is unknown: cycle_time not
// entered or impossible, resource_count entered as 0 or less (see
// resourceCount), or a time unit that is not supported. A cycle_time
// entered as 0 gives 0.
func cycleTimePerResource(resolved ProcessParameters, unitOK bool, entered ProcessParameters) (float64, bool) {
	resources, resourcesOK := resourceCount(entered)
	if !unitOK || !resourcesOK || resolved.CycleTime == nil {
		return 0, false
	}
	return *resolved.CycleTime / resources, true
}

// AnalyzeIdleTimes analyzes idle times across all processes and inventory
// points, from the stream figures CalculateKPIs uses (see IdleTimeAnalysis).
func (a *Analyzer) AnalyzeIdleTimes(vs *ValueStream) (*IdleTimeAnalysis, error) {
	return analyzeIdleTimes(vs, a.calculator.figures(vs)), nil
}

// analyzeIdleTimes is AnalyzeIdleTimes for vs whose stream figures are f.
func analyzeIdleTimes(vs *ValueStream, f streamFigures) *IdleTimeAnalysis {
	analysis := &IdleTimeAnalysis{
		IdleTimeByProcess: make(map[string]float64),
		TotalIdleTime:     f.idleTime,
	}
	for i, pf := range f.processes {
		if pf.idle != nil {
			analysis.IdleTimeByProcess[vs.Processes[i].ID] = *pf.idle
		}
	}
	for i, inf := range f.inventories {
		if inf.counted && inf.time != nil {
			if analysis.IdleTimeByInventory == nil {
				analysis.IdleTimeByInventory = map[string]float64{}
			}
			analysis.IdleTimeByInventory[vs.Inventory[i].ID] += *inf.time
		}
	}
	if f.idleTime != nil {
		analysis.IdleTimeBreakdown = &IdleTimeBreakdown{
			TransportTime: *f.transport, WaitingTime: *f.waiting, StorageTime: *f.storage,
		}
	}
	if e, ok := f.processEfficiency(); ok {
		analysis.IdleTimeEfficiency = F(e)
	}
	return analysis
}

// wasteKinds are the 7 types of waste, in the order they are added.
var wasteKinds = []string{"transportation", "inventory", "motion", "waiting", "over_processing", "defects", "over_production"}

// AnalyzeWaste analyzes the 7 types of waste (Muda) in the value stream
// (see WasteAnalysis).
func (a *Analyzer) AnalyzeWaste(vs *ValueStream) (*WasteAnalysis, error) {
	return a.analyzeWaste(vs, a.calculator.figures(vs)), nil
}

// analyzeWaste is AnalyzeWaste for vs whose stream figures are f.
func (a *Analyzer) analyzeWaste(vs *ValueStream, f streamFigures) *WasteAnalysis {
	analysis := &WasteAnalysis{
		WasteByType:    make(map[string]float64),
		WasteByProcess: make(map[string]float64),
	}

	// 7 Types of Waste according to Lean:
	// 1. Transportation, 2. Inventory, 3. Motion, 4. Waiting, 5. Over-processing, 6. Over-production, 7. Defects

	// The user's splits (waste_assumptions), when entered and within 0–1.
	invShare, invShareOK := assumption(vs, func(w *WasteAssumptions) *float64 { return w.InventoryShareOfWaiting })
	handShare, handShareOK := assumption(vs, func(w *WasteAssumptions) *float64 { return w.WasteShareOfHandling })
	reduction, reductionOK := assumption(vs, func(w *WasteAssumptions) *float64 { return w.ReductionPotential })

	// The times the lead time counts, in the reference unit (totals.go).
	known := f.leadTime != nil
	// byType sums the types that need no split; motionKnown and
	// overProductionKnown are false when a process's part of them is not
	// estimated.
	byType := map[string]float64{}
	motionKnown, overProductionKnown := true, true
	var waiting float64 // process waiting time + inventories counted on their own
	for i, process := range vs.Processes {
		pf := f.processes[i]
		params := pf.params
		defectShare := num(params.ScrapRate) + num(params.ReworkRate)
		if !pf.unitOK || pf.leadTime == nil || (defectShare > 0 && params.ProcessTime == nil) {
			known = false
			continue
		}

		waste := map[string]float64{
			"transportation": num(params.TransportTime),
			// Over-processing waste (non-value adding activities)
			"over_processing": num(params.NonValueAddingTime),
			// Defects waste: the defective share of the process time.
			"defects": defectShare * num(params.ProcessTime),
		}
		processKnown := true
		// Motion waste: the user's share of the handling time.
		switch handling := num(params.HandlingTime); {
		case handling == 0:
		case handShareOK:
			waste["motion"] = handling * handShare
		default:
			motionKnown, processKnown = false, false
		}
		// Over-production waste (unused capacity × operating time); only
		// when utilization can be computed (see CalculateProcessLoad).
		if _, utilization, loadKnown := a.processLoad(vs, f, i); loadKnown && utilization < overProductionUtilization {
			if params.OperatingTime != nil {
				waste["over_production"] = (1 - utilization) * *params.OperatingTime
			} else {
				overProductionKnown, processKnown = false, false
			}
		}

		// Inventory and waiting waste add up to the waiting time, however
		// it is split.
		processWaste := num(params.WaitingTime)
		waiting += processWaste
		for kind, amount := range waste {
			byType[kind] += amount
			processWaste += amount
		}
		if processKnown {
			analysis.WasteByProcess[process.ID] = processWaste
		}
	}

	// Inventory points counted on their own: their time is waiting time.
	for i, inf := range f.inventories {
		if !inf.counted {
			continue // the stock of a process, in its waiting time
		}
		if inf.time == nil {
			known = false
			continue
		}
		if analysis.WasteByInventory == nil {
			analysis.WasteByInventory = map[string]float64{}
		}
		analysis.WasteByInventory[vs.Inventory[i].ID] += *inf.time
		waiting += *inf.time
	}

	if !known {
		return analysis
	}
	analysis.WaitingAndInventoryWaste = F(waiting)
	total := waiting
	for _, kind := range []string{"transportation", "over_processing", "defects"} {
		analysis.WasteByType[kind] = byType[kind]
		total += byType[kind]
	}
	switch {
	case waiting == 0:
		analysis.WasteByType["inventory"], analysis.WasteByType["waiting"] = 0, 0
	case invShareOK:
		analysis.WasteByType["inventory"] = waiting * invShare
		analysis.WasteByType["waiting"] = waiting * (1 - invShare)
	default:
		analysis.Notes = append(analysis.Notes, noteInventoryShare)
	}
	if motionKnown {
		analysis.WasteByType["motion"] = byType["motion"]
		total += byType["motion"]
	} else {
		analysis.Notes = append(analysis.Notes, noteHandlingShare)
	}
	if overProductionKnown {
		analysis.WasteByType["over_production"] = byType["over_production"]
		total += byType["over_production"]
	} else {
		analysis.Notes = append(analysis.Notes, noteOperatingTime)
	}
	if !motionKnown || !overProductionKnown {
		return analysis
	}
	analysis.TotalWaste = F(total)
	if reductionOK {
		analysis.WasteReductionPotential = F(total * reduction)
	} else {
		analysis.Notes = append(analysis.Notes, noteReduction)
	}
	return analysis
}

// overProductionUtilization is the utilization below which a process's
// unused share of operating time counts as over-production waste.
const overProductionUtilization = 0.7

// Notes for waste that is not estimated (WasteAnalysis.Notes).
var (
	noteInventoryShare = "Inventory and waiting waste not split: enter " + fieldInventoryShare +
		" (the share of waiting time that is inventory waste, 0–1); their measured total is " +
		"waiting_and_inventory_waste"
	noteHandlingShare = "Motion waste not estimated: enter " + fieldHandlingShare +
		" (the share of handling time that is waste, 0–1); total_waste is not computed without it"
	noteOperatingTime = "Over-production waste not estimated: enter operating_time for each process whose " +
		"utilization is below 0.7; total_waste is not computed without it"
	noteReduction = "Waste reduction potential not estimated: enter " + fieldReductionPotential +
		" (the share of total waste you expect to remove, 0–1)"
)

// AnalyzeFlow analyzes the product and information flow.
//
// The flow type is judged from the processes whose waiting time and process
// time are both known (see knownWaiting): a waiting time below 10% of the
// process time counts toward pull, otherwise toward push, and the side with
// more processes wins (a tie is push). A process with either value unknown
// counts toward neither; with none known the flow type is not determined
// (empty, with a note). Information flow quality is the user's rating
// (ValueStream.InformationFlowQuality), not estimated when not entered.
//
// A capacity interruption is reported between neighbouring processes when
// the first one's effective cycle time (CT ÷ #RES, see
// CalculateProcessLoad) is more than 1.5 times the next one's: it supplies
// the next process more slowly than that process can work. Process time is
// not compared, as it includes the batch (order quantity) and setup. An
// excess-inventory interruption is reported when a process's waiting time
// is longer than its process time. Each comparison needs both values known
// (an entered 0 is known); a value that is unknown is not compared (it is
// reported as missing data by CheckConsistency where a stream total needs
// it).
func (a *Analyzer) AnalyzeFlow(vs *ValueStream) (*FlowAnalysis, error) {
	return analyzeFlow(vs, a.calculator.figures(vs)), nil
}

// analyzeFlow is AnalyzeFlow for vs whose stream figures are f.
func analyzeFlow(vs *ValueStream, f streamFigures) *FlowAnalysis {
	analysis := &FlowAnalysis{
		FlowInterruptions: []FlowInterruption{},
	}

	// Determine flow type (simplified analysis)
	pushIndicators := 0
	pullIndicators := 0

	// Resolved values in the reference time unit (the stream figures,
	// totals.go). A process whose time unit is not supported is left out:
	// its times cannot be compared with the others.
	for _, pf := range f.processes {
		params := pf.params
		waiting, ok := knownWaiting(pf)
		if !ok || params.ProcessTime == nil {
			continue // not known: counts toward neither side
		}
		// Look for supermarket indicators (pull system)
		if waiting < *params.ProcessTime*pullWaitingShare {
			pullIndicators++
		} else {
			pushIndicators++
		}
	}

	switch {
	case pullIndicators+pushIndicators == 0:
		analysis.Notes = append(analysis.Notes, noteFlowType)
	case pullIndicators > pushIndicators:
		analysis.FlowType = FlowTypePull
	default:
		analysis.FlowType = FlowTypePush
	}

	// Flow efficiency: value adding time ÷ lead time, known exactly when
	// the value_added_ratio KPI is.
	if e, ok := f.flowEfficiency(); ok {
		analysis.FlowEfficiency = F(e)
	}

	// Identify flow interruptions
	for i, process := range vs.Processes {
		if !f.processes[i].unitOK {
			continue
		}
		params := f.processes[i].params
		scrap, rework := num(params.ScrapRate), num(params.ReworkRate)

		// Check for quality issues (high scrap/rework rates)
		if scrap > 0.05 || rework > 0.1 {
			analysis.FlowInterruptions = append(analysis.FlowInterruptions, FlowInterruption{
				Type:        "quality",
				Location:    process.Name,
				Frequency:   scrap + rework, // Defect share, as a fraction
				Impact:      num(params.Downtime),
				Description: fmt.Sprintf("High defect rate (%.1f%%) causing flow interruptions", (scrap+rework)*100),
			})
		}

		// Check for capacity bottlenecks
		if i < len(vs.Processes)-1 {
			nextProcess := vs.Processes[i+1]
			this, thisOK := effectiveCycleTime(vs, f, i)
			next, nextOK := effectiveCycleTime(vs, f, i+1)
			if thisOK && nextOK && exceeds(this, next*1.5) {
				analysis.FlowInterruptions = append(analysis.FlowInterruptions, FlowInterruption{
					Type:      "capacity",
					Location:  fmt.Sprintf("%s → %s", process.Name, nextProcess.Name),
					Frequency: 1.0, // Continuous
					Impact:    this - next,
					Description: fmt.Sprintf("Capacity mismatch causing bottlenecks: effective cycle time "+
						"(cycle time ÷ resources) %g, next process %g", this, next),
				})
			}
		}

		// Check for inventory issues (high waiting times)
		if params.ProcessTime != nil && params.WaitingTime != nil && *params.WaitingTime > *params.ProcessTime {
			processTime, waiting := *params.ProcessTime, *params.WaitingTime
			// Waiting-to-process ratio; with no process time recorded the
			// ratio is undefined, so report 0 rather than +Inf.
			frequency := 0.0
			if processTime > 0 {
				frequency = waiting / processTime
			}
			analysis.FlowInterruptions = append(analysis.FlowInterruptions, FlowInterruption{
				Type:        "inventory",
				Location:    process.Name,
				Frequency:   frequency,
				Impact:      waiting,
				Description: "Excess inventory causing waiting times",
			})
		}
	}

	// Information flow quality: the user's rating, when entered and valid.
	if q, ok := validFraction(vs.InformationFlowQuality); ok {
		analysis.InformationFlowQuality = F(q)
	} else {
		analysis.Notes = append(analysis.Notes, noteInfoFlowQuality)
	}

	return analysis
}

// pullWaitingShare: a process whose waiting time is below this share of its
// process time counts as a pull indicator (AnalyzeFlow).
const pullWaitingShare = 0.1

// Notes for flow results that are not determined (FlowAnalysis.Notes).
const (
	noteFlowType = "Push/pull flow type not determined: no process has both waiting_time (or a known idle " +
		"time of 0) and process_time known"
	noteInfoFlowQuality = "Information flow quality not estimated: enter " + fieldInfoFlowQuality +
		" (your rating of the information flow, 0–1)"
)

// knownWaiting returns the waiting time of a process as the stream counts
// it, and false when it is unknown: waiting_time entered or derived, or 0
// when the process's idle time is known to be 0. An idle time given only as
// a total above 0 does not say how much of it is waiting.
func knownWaiting(pf processFigures) (float64, bool) {
	switch {
	case !pf.unitOK:
		return 0, false
	case pf.params.WaitingTime != nil:
		return *pf.params.WaitingTime, true
	case pf.idle != nil && *pf.idle == 0:
		return 0, true
	}
	return 0, false
}

// GenerateRecommendations generates improvement recommendations based on analysis
func (a *Analyzer) GenerateRecommendations(result *AnalysisResult) []string {
	var recommendations []string

	// Bottleneck recommendations
	if len(result.Bottlenecks) > 0 {
		recommendations = append(recommendations,
			"Address bottleneck processes by increasing capacity or balancing workload")
		for _, bottleneck := range result.Bottlenecks {
			if bottleneck.BottleneckLevel == "high" {
				recommendations = append(recommendations,
					fmt.Sprintf("High priority: Optimize process '%s' (%.1f%% utilization)",
						bottleneck.ProcessName, bottleneck.Utilization*100))
			}
		}
	}

	// Idle time recommendations (only when the efficiency is known)
	if e := result.IdleTimeAnalysis.IdleTimeEfficiency; e != nil && *e < 0.5 {
		recommendations = append(recommendations,
			"High idle time detected. Consider implementing continuous flow or pull systems")
	}

	// Waste recommendations (only for the waste types that are known)
	byType := result.WasteAnalysis.WasteByType
	if waste, exists := byType["inventory"]; exists && waste > 0 {
		recommendations = append(recommendations,
			"Reduce inventory waste by implementing just-in-time delivery")
	}
	if waste, exists := byType["waiting"]; exists && waste > 0 {
		recommendations = append(recommendations,
			"Reduce waiting time waste by balancing process capacities")
	}
	if _, split := byType["inventory"]; !split {
		if w := result.WasteAnalysis.WaitingAndInventoryWaste; w != nil && *w > 0 {
			recommendations = append(recommendations,
				"Reduce waiting and inventory time by balancing process capacities and implementing just-in-time delivery")
		}
	}
	if waste, exists := byType["defects"]; exists && waste > 0 {
		recommendations = append(recommendations,
			"Implement quality improvement measures to reduce defect waste")
	}

	// Flow recommendations
	if result.FlowAnalysis.FlowType == FlowTypePush {
		recommendations = append(recommendations,
			"Consider transitioning from push to pull system for better flow control")
	}

	if e := result.FlowAnalysis.FlowEfficiency; e != nil && *e < 0.3 {
		recommendations = append(recommendations,
			"Low flow efficiency indicates need for value stream optimization")
	}

	// KPI-based recommendations
	if varRatio, exists := result.KPIs["value_added_ratio"]; exists && varRatio < 0.1 {
		recommendations = append(recommendations,
			"Very low value-added ratio. Focus on eliminating non-value-adding activities")
	}

	return recommendations
}

// SelectProductFamily provides guidance for selecting a representative product family
func (a *Analyzer) SelectProductFamily(products []ProductFamily) (*ProductFamily, error) {
	if len(products) == 0 {
		return nil, errors.New("no product families provided")
	}

	// Calculate weighted score for each product family
	// Criteria: sales volume (40%), strategic importance (30%), demand stability (30%)
	var bestFamily *ProductFamily
	bestScore := -1.0

	for i := range products {
		family := &products[i]
		score := family.SalesVolume*0.4 + family.StrategicImportance*0.3 + family.DemandStability*0.3

		if score > bestScore {
			bestScore = score
			bestFamily = family
		}
	}

	return bestFamily, nil
}

// ProductFamily represents a product family for selection
type ProductFamily struct {
	ID                  string  `json:"id"`
	Name                string  `json:"name"`
	SalesVolume         float64 `json:"sales_volume"`         // 0-1 scale
	StrategicImportance float64 `json:"strategic_importance"` // 0-1 scale
	DemandStability     float64 `json:"demand_stability"`     // 0-1 scale
	VariantCount        int     `json:"variant_count"`
	Description         string  `json:"description,omitempty"`
}
