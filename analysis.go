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
// the reference time unit. Waste is estimated from the times the lead time
// counts (the stream figures, totals.go), so the same stock gives the same
// waste however it is drawn (process stock, linked or separate inventory).
// The totals are nil (omitted in JSON) when unknown: when the stream lead
// time is unknown, or a process with scrap or rework has no known process
// time (CheckConsistency names the missing value).
type WasteAnalysis struct {
	TotalWaste *float64 `json:"total_waste,omitempty"`
	// WasteByType holds the 7 types of waste; empty when TotalWaste is
	// unknown.
	WasteByType map[string]float64 `json:"waste_by_type"`
	// WasteByProcess is each process's waste, for the processes whose
	// waste is known.
	WasteByProcess map[string]float64 `json:"waste_by_process"`
	// WasteByInventory is the waste of each inventory point counted on its
	// own (its time, split between inventory and waiting waste), when known.
	WasteByInventory        map[string]float64 `json:"waste_by_inventory,omitempty"`
	WasteReductionPotential *float64           `json:"waste_reduction_potential,omitempty"`
}

// FlowAnalysis contains analysis of product and information flow
type FlowAnalysis struct {
	FlowType FlowType `json:"flow_type"` // Push or Pull
	// FlowEfficiency is value adding time ÷ lead time: the
	// value_added_ratio KPI, with the same validity. nil (omitted in JSON)
	// when unknown: a total unknown, lead time 0, or a value adding time
	// larger than its process or lead time.
	FlowEfficiency         *float64           `json:"flow_efficiency,omitempty"`
	FlowInterruptions      []FlowInterruption `json:"flow_interruptions"`
	InformationFlowQuality float64            `json:"information_flow_quality"`
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

	// Identify bottlenecks
	bottlenecks, err := a.IdentifyBottlenecks(vs)
	if err != nil {
		return nil, fmt.Errorf("failed to identify bottlenecks: %w", err)
	}
	result.Bottlenecks = bottlenecks

	// Analyze idle times
	idleAnalysis, err := a.AnalyzeIdleTimes(vs)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze idle times: %w", err)
	}
	result.IdleTimeAnalysis = *idleAnalysis

	// Analyze waste
	wasteAnalysis, err := a.AnalyzeWaste(vs)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze waste: %w", err)
	}
	result.WasteAnalysis = *wasteAnalysis

	// Analyze flow
	flowAnalysis, err := a.AnalyzeFlow(vs)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze flow: %w", err)
	}
	result.FlowAnalysis = *flowAnalysis

	// Calculate KPIs
	result.KPIs = a.calculator.CalculateKPIs(vs)

	// Generate recommendations
	result.Recommendations = a.GenerateRecommendations(result)

	if len(vs.Processes) > 0 && !a.hasLoadInputs(vs) {
		result.Notes = append(result.Notes, missingLoadInputsNote)
	}

	result.ConsistencyIssues = CheckConsistency(vs)
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
	var bottlenecks []BottleneckInfo

	// Calculate utilization for each process
	for _, process := range vs.Processes {
		capacity, utilization, ok := a.CalculateProcessLoad(process, vs)
		if !ok {
			continue
		}

		bottleneckLevel := "low"
		if utilization > 0.9 {
			bottleneckLevel = "high"
		} else if utilization > 0.7 {
			bottleneckLevel = "medium"
		}

		if utilization > 0.7 { // Consider processes with >70% utilization as potential bottlenecks
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
	}

	// Sort by utilization (highest first)
	sort.Slice(bottlenecks, func(i, j int) bool {
		return bottlenecks[i].Utilization > bottlenecks[j].Utilization
	})

	return bottlenecks, nil
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
	var contradictions, missing int
	for _, issue := range issues {
		if issue.Kind == IssueMissingData {
			missing++
		} else {
			contradictions++
		}
	}
	var notes []string
	if contradictions > 0 {
		notes = append(notes, fmt.Sprintf(consistencyNoteFormat, contradictions))
	}
	if missing > 0 {
		notes = append(notes, fmt.Sprintf(missingDataNoteFormat, missing))
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
// #RES is the process's ResourceCount (not entered, or 0 or less, counts as 1): resources
// working in parallel multiply capacity, as in processing time = CT × PQ ÷
// #RES (A.5).
//
// Demand and AvailableTime are both per Customer.DemandPeriod, so capacity and
// demand are compared over the same period. CT is the process's CycleTime,
// converted to the reference time unit (Calculator.ReferenceTimeUnit), which
// is the unit AvailableTime is given in. Utilization is not capped, so a value
// above 1 means demand exceeds capacity.
//
// ok is false (and nothing is assumed) when the customer, Demand,
// DemandPeriod, AvailableTime or the process's CycleTime is not entered, or
// Demand, AvailableTime or CycleTime is 0, or the process's time unit is not
// supported.
func (a *Analyzer) CalculateProcessLoad(process Process, vs *ValueStream) (capacity, utilization float64, ok bool) {
	if vs == nil || !a.hasLoadInputs(vs) {
		return 0, 0, false
	}
	ctx := a.calculator.contextOf(vs)
	params, unitOK := a.calculator.resolvedIn(process.Parameters, ctx)
	ct := params.CycleTime
	if !unitOK || !positive(ct) {
		return 0, 0, false
	}
	cycleTime := *ct
	resources := float64(max(inum(process.Parameters.ResourceCount), 1))
	available := *vs.Customer.AvailableTime * resources
	return available / cycleTime, *vs.Customer.Demand * cycleTime / available, true
}

// AnalyzeIdleTimes analyzes idle times across all processes and inventory
// points, from the stream figures CalculateKPIs uses (see IdleTimeAnalysis).
func (a *Analyzer) AnalyzeIdleTimes(vs *ValueStream) (*IdleTimeAnalysis, error) {
	f := a.calculator.figures(vs)
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
	return analysis, nil
}

// wasteKinds are the 7 types of waste, in the order they are added.
var wasteKinds = []string{"transportation", "inventory", "motion", "waiting", "over_processing", "defects", "over_production"}

// AnalyzeWaste analyzes the 7 types of waste (Muda) in the value stream
// (see WasteAnalysis).
func (a *Analyzer) AnalyzeWaste(vs *ValueStream) (*WasteAnalysis, error) {
	analysis := &WasteAnalysis{
		WasteByType:    make(map[string]float64),
		WasteByProcess: make(map[string]float64),
	}

	// 7 Types of Waste according to Lean:
	// 1. Transportation, 2. Inventory, 3. Motion, 4. Waiting, 5. Over-processing, 6. Over-production, 7. Defects

	// The times the lead time counts, in the reference unit (totals.go).
	f := a.calculator.figures(vs)
	known := f.leadTime != nil
	byType := map[string]float64{}
	var total float64
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
			// Waiting time: half counted as inventory waste, half as waiting.
			"inventory": num(params.WaitingTime) * 0.5,
			"waiting":   num(params.WaitingTime) * 0.5,
			// Motion waste (handling time inefficiencies)
			"motion": num(params.HandlingTime) * 0.3, // Assume 30% of handling time is waste
			// Over-processing waste (non-value adding activities)
			"over_processing": num(params.NonValueAddingTime),
			// Defects waste: the defective share of the process time.
			"defects": defectShare * num(params.ProcessTime),
		}
		// Over-production waste (excess capacity utilization); only when
		// utilization can be computed (see CalculateProcessLoad).
		if _, utilization, loadKnown := a.CalculateProcessLoad(process, vs); loadKnown && utilization < 0.7 {
			waste["over_production"] = (1 - utilization) * num(params.OperatingTime)
		}

		var processWaste float64
		for _, kind := range wasteKinds {
			byType[kind] += waste[kind]
			processWaste += waste[kind]
		}
		analysis.WasteByProcess[process.ID] = processWaste
		total += processWaste
	}

	// Inventory points counted on their own: their time is waiting time,
	// split like a process's waiting time.
	for i, inf := range f.inventories {
		if !inf.counted {
			continue // the stock of a process, in its waiting time
		}
		if inf.time == nil {
			known = false
			continue
		}
		byType["inventory"] += *inf.time * 0.5
		byType["waiting"] += *inf.time * 0.5
		if analysis.WasteByInventory == nil {
			analysis.WasteByInventory = map[string]float64{}
		}
		analysis.WasteByInventory[vs.Inventory[i].ID] += *inf.time
		total += *inf.time
	}

	if known {
		analysis.WasteByType = byType
		analysis.TotalWaste = F(total)
		// Estimate waste reduction potential (typically 20-50% can be eliminated)
		analysis.WasteReductionPotential = F(total * 0.35) // 35% potential reduction
	}
	return analysis, nil
}

// AnalyzeFlow analyzes the product and information flow
func (a *Analyzer) AnalyzeFlow(vs *ValueStream) (*FlowAnalysis, error) {
	analysis := &FlowAnalysis{
		FlowInterruptions: []FlowInterruption{},
	}

	// Determine flow type (simplified analysis)
	pushIndicators := 0
	pullIndicators := 0

	// Resolved values in the reference time unit (the stream figures,
	// totals.go). A process whose time unit is not supported is left out:
	// its times cannot be compared with the others.
	f := a.calculator.figures(vs)
	resolved := make([]ProcessParameters, len(vs.Processes))
	for i, pf := range f.processes {
		resolved[i] = pf.params
	}

	for i, params := range resolved {
		if !f.processes[i].unitOK {
			continue
		}
		// Look for supermarket indicators (pull system)
		if num(params.WaitingTime) < num(params.ProcessTime)*0.1 {
			pullIndicators++
		} else {
			pushIndicators++
		}
	}

	if pullIndicators > pushIndicators {
		analysis.FlowType = FlowTypePull
	} else {
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
		params := resolved[i]
		scrap, rework := num(params.ScrapRate), num(params.ReworkRate)
		processTime, waiting := num(params.ProcessTime), num(params.WaitingTime)

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
		if i < len(vs.Processes)-1 && f.processes[i+1].unitOK {
			nextProcess := vs.Processes[i+1]
			next := num(resolved[i+1].ProcessTime)
			if processTime > next*1.5 {
				analysis.FlowInterruptions = append(analysis.FlowInterruptions, FlowInterruption{
					Type:        "capacity",
					Location:    fmt.Sprintf("%s → %s", process.Name, nextProcess.Name),
					Frequency:   1.0, // Continuous
					Impact:      processTime - next,
					Description: "Capacity mismatch causing bottlenecks",
				})
			}
		}

		// Check for inventory issues (high waiting times)
		if waiting > processTime {
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

	// Analyze information flow quality (simplified)
	infoFlowQuality := 0.8 // Assume good unless we have data to suggest otherwise
	if len(vs.FlowElements) == 0 {
		infoFlowQuality = 0.5 // Poor information flow if no flow controls
	}
	analysis.InformationFlowQuality = infoFlowQuality

	return analysis, nil
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

	// Waste recommendations (only when the waste is known)
	if w := result.WasteAnalysis.TotalWaste; w != nil && *w > 0 {
		if waste, exists := result.WasteAnalysis.WasteByType["inventory"]; exists && waste > 0 {
			recommendations = append(recommendations,
				"Reduce inventory waste by implementing just-in-time delivery")
		}
		if waste, exists := result.WasteAnalysis.WasteByType["waiting"]; exists && waste > 0 {
			recommendations = append(recommendations,
				"Reduce waiting time waste by balancing process capacities")
		}
		if waste, exists := result.WasteAnalysis.WasteByType["defects"]; exists && waste > 0 {
			recommendations = append(recommendations,
				"Implement quality improvement measures to reduce defect waste")
		}
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
