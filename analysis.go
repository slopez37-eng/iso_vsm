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

// IdleTimeAnalysis contains analysis of idle times across the value stream
type IdleTimeAnalysis struct {
	TotalIdleTime      float64            `json:"total_idle_time"`
	IdleTimeByProcess  map[string]float64 `json:"idle_time_by_process"`
	IdleTimeBreakdown  IdleTimeBreakdown  `json:"idle_time_breakdown"`
	IdleTimeEfficiency float64            `json:"idle_time_efficiency"` // Share of productive time, as a fraction (0-1)
}

// IdleTimeBreakdown breaks down idle time by category
type IdleTimeBreakdown struct {
	TransportTime float64 `json:"transport_time"`
	WaitingTime   float64 `json:"waiting_time"`
	StorageTime   float64 `json:"storage_time"`
}

// WasteAnalysis contains analysis of different types of waste (Muda)
type WasteAnalysis struct {
	TotalWaste              float64            `json:"total_waste"`
	WasteByType             map[string]float64 `json:"waste_by_type"` // 7 types of waste
	WasteByProcess          map[string]float64 `json:"waste_by_process"`
	WasteReductionPotential float64            `json:"waste_reduction_potential"`
}

// FlowAnalysis contains analysis of product and information flow
type FlowAnalysis struct {
	FlowType               FlowType           `json:"flow_type"` // Push or Pull
	FlowEfficiency         float64            `json:"flow_efficiency"`
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
	if n := len(result.ConsistencyIssues); n > 0 {
		result.Notes = append(result.Notes, fmt.Sprintf(consistencyNoteFormat, n))
	}

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
const consistencyNoteFormat = "%d stored value(s) disagree with the values computed from their parts; " +
	"calculations use the computed values (see consistency_issues)"

// hasLoadInputs reports whether the customer data needed for utilization is set.
func (a *Analyzer) hasLoadInputs(vs *ValueStream) bool {
	c := vs.Customer
	return c != nil && c.Demand > 0 && c.AvailableTime > 0 && c.DemandPeriod.IsValid()
}

// CalculateProcessLoad returns a process's capacity and utilization for the
// customer's demand period.
//
//	capacity    = AvailableTime ÷ CT          (units per DemandPeriod)
//	utilization = Demand × CT ÷ AvailableTime (fraction; = CT ÷ takt)
//
// Demand and AvailableTime are both per Customer.DemandPeriod, so capacity and
// demand are compared over the same period. CT is the process's CycleTime,
// converted to the reference time unit (Calculator.ReferenceTimeUnit), which
// is the unit AvailableTime is given in. Utilization is not capped, so a value
// above 1 means demand exceeds capacity.
//
// ok is false (and nothing is assumed) when the customer, Demand,
// DemandPeriod, AvailableTime or the process's CycleTime is missing.
func (a *Analyzer) CalculateProcessLoad(process Process, vs *ValueStream) (capacity, utilization float64, ok bool) {
	if vs == nil || !a.hasLoadInputs(vs) {
		return 0, 0, false
	}
	unit := a.calculator.ReferenceTimeUnit(vs.Processes)
	cycleTime := a.calculator.resolvedIn(process.Parameters, unit).CycleTime
	if cycleTime <= 0 {
		return 0, 0, false
	}
	available := vs.Customer.AvailableTime
	return available / cycleTime, vs.Customer.Demand * cycleTime / available, true
}

// AnalyzeIdleTimes analyzes idle times across all processes
func (a *Analyzer) AnalyzeIdleTimes(vs *ValueStream) (*IdleTimeAnalysis, error) {
	analysis := &IdleTimeAnalysis{
		IdleTimeByProcess: make(map[string]float64),
	}

	var totalIdleTime float64
	var totalProcessTime float64

	breakdown := IdleTimeBreakdown{}

	// Express every process in one time unit before summing (see
	// Calculator.ReferenceTimeUnit), with totals resolved from their parts
	// (see Calculator.ResolveParameters).
	unit := a.calculator.ReferenceTimeUnit(vs.Processes)
	for _, process := range vs.Processes {
		params := a.calculator.resolvedIn(process.Parameters, unit)
		idleTime := params.IdleTime
		processTime := params.ProcessTime

		analysis.IdleTimeByProcess[process.ID] = idleTime
		totalIdleTime += idleTime
		totalProcessTime += processTime

		// Accumulate breakdown
		breakdown.TransportTime += params.TransportTime
		breakdown.WaitingTime += params.WaitingTime
		breakdown.StorageTime += params.StorageTime
	}

	analysis.TotalIdleTime = totalIdleTime
	analysis.IdleTimeBreakdown = breakdown

	// Calculate idle time efficiency
	totalTime := totalIdleTime + totalProcessTime
	if totalTime > 0 {
		analysis.IdleTimeEfficiency = totalProcessTime / totalTime
	}

	return analysis, nil
}

// AnalyzeWaste analyzes the 7 types of waste (Muda) in the value stream
func (a *Analyzer) AnalyzeWaste(vs *ValueStream) (*WasteAnalysis, error) {
	analysis := &WasteAnalysis{
		WasteByType:    make(map[string]float64),
		WasteByProcess: make(map[string]float64),
	}

	// 7 Types of Waste according to Lean:
	// 1. Transportation, 2. Inventory, 3. Motion, 4. Waiting, 5. Over-processing, 6. Over-production, 7. Defects

	// Resolved values in the reference time unit (see Calculator.ResolveParameters).
	unit := a.calculator.ReferenceTimeUnit(vs.Processes)
	for _, process := range vs.Processes {
		params := a.calculator.resolvedIn(process.Parameters, unit)

		// Transportation waste
		transportWaste := params.TransportTime
		analysis.WasteByType["transportation"] += transportWaste

		// Inventory waste (waiting time can indicate inventory issues)
		inventoryWaste := params.WaitingTime * 0.5 // Assume 50% of waiting is inventory-related
		analysis.WasteByType["inventory"] += inventoryWaste

		// Motion waste (handling time inefficiencies)
		motionWaste := params.HandlingTime * 0.3 // Assume 30% of handling time is waste
		analysis.WasteByType["motion"] += motionWaste

		// Waiting waste
		waitingWaste := params.WaitingTime * 0.5 // Remaining waiting time
		analysis.WasteByType["waiting"] += waitingWaste

		// Over-processing waste (non-value adding activities)
		overProcessingWaste := params.NonValueAddingTime
		analysis.WasteByType["over_processing"] += overProcessingWaste

		// Defects waste (scrap and rework)
		defectsWaste := params.ScrapRate + params.ReworkRate
		if params.ProcessTime > 0 {
			defectsWaste *= params.ProcessTime
		}
		analysis.WasteByType["defects"] += defectsWaste

		// Over-production waste (excess capacity utilization); only when
		// utilization can be computed (see CalculateProcessLoad).
		_, utilization, loadKnown := a.CalculateProcessLoad(process, vs)
		underUtilized := loadKnown && utilization < 0.7
		if underUtilized {
			overProductionWaste := (1 - utilization) * params.OperatingTime
			analysis.WasteByType["over_production"] += overProductionWaste
		}

		// Total waste for this process
		processWaste := transportWaste + inventoryWaste + motionWaste + waitingWaste +
			overProcessingWaste + defectsWaste
		if underUtilized {
			processWaste += (1 - utilization) * params.OperatingTime
		}

		analysis.WasteByProcess[process.ID] = processWaste
		analysis.TotalWaste += processWaste
	}

	// Estimate waste reduction potential (typically 20-50% can be eliminated)
	analysis.WasteReductionPotential = analysis.TotalWaste * 0.35 // 35% potential reduction

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

	// Resolved values in the reference time unit (see Calculator.ResolveParameters).
	unit := a.calculator.ReferenceTimeUnit(vs.Processes)
	resolved := make([]ProcessParameters, len(vs.Processes))
	for i, process := range vs.Processes {
		resolved[i] = a.calculator.resolvedIn(process.Parameters, unit)
	}

	for _, params := range resolved {
		// Look for supermarket indicators (pull system)
		if params.WaitingTime < params.ProcessTime*0.1 {
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

	// Analyze flow efficiency (ratio of value-adding time to total lead time)
	totalVAT := a.calculator.CalculateValueAddingTime(vs.Processes)
	totalLeadTime := a.calculator.CalculateOverallLeadTime(vs.Processes)

	if totalLeadTime > 0 {
		analysis.FlowEfficiency = totalVAT / totalLeadTime
	}

	// Identify flow interruptions
	for i, process := range vs.Processes {
		params := resolved[i]

		// Check for quality issues (high scrap/rework rates)
		if params.ScrapRate > 0.05 || params.ReworkRate > 0.1 {
			analysis.FlowInterruptions = append(analysis.FlowInterruptions, FlowInterruption{
				Type:        "quality",
				Location:    process.Name,
				Frequency:   params.ScrapRate + params.ReworkRate, // Defect share, as a fraction
				Impact:      params.Downtime,
				Description: fmt.Sprintf("High defect rate (%.1f%%) causing flow interruptions", (params.ScrapRate+params.ReworkRate)*100),
			})
		}

		// Check for capacity bottlenecks
		if i < len(vs.Processes)-1 {
			nextProcess := vs.Processes[i+1]
			next := resolved[i+1]
			if params.ProcessTime > next.ProcessTime*1.5 {
				analysis.FlowInterruptions = append(analysis.FlowInterruptions, FlowInterruption{
					Type:        "capacity",
					Location:    fmt.Sprintf("%s → %s", process.Name, nextProcess.Name),
					Frequency:   1.0, // Continuous
					Impact:      params.ProcessTime - next.ProcessTime,
					Description: "Capacity mismatch causing bottlenecks",
				})
			}
		}

		// Check for inventory issues (high waiting times)
		if params.WaitingTime > params.ProcessTime {
			// Waiting-to-process ratio; with no process time recorded the
			// ratio is undefined, so report 0 rather than +Inf.
			frequency := 0.0
			if params.ProcessTime > 0 {
				frequency = params.WaitingTime / params.ProcessTime
			}
			analysis.FlowInterruptions = append(analysis.FlowInterruptions, FlowInterruption{
				Type:        "inventory",
				Location:    process.Name,
				Frequency:   frequency,
				Impact:      params.WaitingTime,
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

	// Idle time recommendations
	if result.IdleTimeAnalysis.IdleTimeEfficiency < 0.5 {
		recommendations = append(recommendations,
			"High idle time detected. Consider implementing continuous flow or pull systems")
	}

	// Waste recommendations
	if result.WasteAnalysis.TotalWaste > 0 {
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

	if result.FlowAnalysis.FlowEfficiency < 0.3 {
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
