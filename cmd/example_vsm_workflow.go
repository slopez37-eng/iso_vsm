package main

import (
	"fmt"
	"log"
	"time"

	vsm "github.com/iso22468/vsm-go-sdk"
)

func main() {
	fmt.Println("=== ISO 22468:2020 Value Stream Management (VSM) SDK Example ===")
	fmt.Println("This example demonstrates a complete VSM workflow for a manufacturing process")
	fmt.Println()

	// Step 1: Create a VSM project and define the current state
	project := createVSMProject()
	fmt.Printf("✓ Created VSM project: %s\n", project.Name)

	// Step 2: Analyze the current state
	analysisResult := analyzeCurrentState(project)
	fmt.Println("✓ Completed current state analysis")

	// Step 3: Design future state improvements
	designResult := designFutureState(project, analysisResult)
	fmt.Println("✓ Completed future state design")

	// Step 4: Plan implementation
	planningResult := planImplementation(designResult, project)
	fmt.Println("✓ Completed implementation planning")

	// Step 5: Assess performance and demonstrate PDCA cycle
	assessmentResult := assessPerformance(project)
	fmt.Println("✓ Completed performance assessment")

	// Step 6: Serialize and save results
	serializeResults(project, analysisResult, designResult, planningResult, assessmentResult)
	fmt.Println("✓ Serialized and saved all results")

	// Step 7: Display summary
	displaySummary(analysisResult, designResult, planningResult, assessmentResult)

	fmt.Println("\n=== VSM Workflow Complete ===")
	fmt.Println("The SDK successfully implemented ISO 22468:2020 VSM methodology")
}

// createVSMProject creates a sample VSM project for a burger production process
func createVSMProject() *vsm.VSMProject {
	// Create customer
	customer := &vsm.Customer{
		ID:            "customer_1",
		Name:          "Restaurant Chain",
		Demand:        vsm.F(200),          // 200 burgers per day
		DemandPeriod:  vsm.DemandPeriodDay, // demand and available time are per day
		AvailableTime: vsm.F(480),          // minutes per day (one 8 hour shift)
		TaktTime:      vsm.F(2.4),          // minutes per burger (480 / 200)
		Description:   "Main customer for burger products",
	}

	// Create supplier
	supplier := &vsm.Supplier{
		ID:           "supplier_1",
		Name:         "Food Ingredients Supplier",
		DeliveryTime: 2 * 24 * 60, // 2 days in minutes
		Description:  "Supplier of burger ingredients",
	}

	// Create processes for burger production
	processes := []vsm.Process{
		{
			ID:   "process_1",
			Name: "Receive Ingredients",
			Type: vsm.ProcessTypeMaterial,
			Parameters: vsm.ProcessParameters{
				ProcessTime:                 vsm.F(30), // 30 minutes
				CycleTime:                   vsm.F(30), // 30 minutes
				SetupTime:                   vsm.F(15), // 15 minutes
				ProcessingTime:              vsm.F(15), // processing; process time = processing + setup (A.2)
				EmployeeCount:               vsm.F(1),  // 1 employee
				ResourceCount:               vsm.I(1),  // 1 workstation
				TimeUnit:                    vsm.TimeUnitMinutes,
				ValueAddingTime:             vsm.F(25),   // 25 minutes value-adding
				NonValueAddingTime:          vsm.F(5),    // 5 minutes non-value-adding
				NecessaryNonValueAddingTime: vsm.F(0),    // No necessary non-value-adding time
				YieldFactor:                 vsm.F(0.98), // 98% yield
				ScrapRate:                   vsm.F(0.02), // 2% scrap
				ReworkRate:                  vsm.F(0.00), // 0% rework
			},
			Description: "Receive and inspect incoming ingredients",
		},
		{
			ID:   "process_2",
			Name: "Prepare Patties",
			Type: vsm.ProcessTypeMaterial,
			Parameters: vsm.ProcessParameters{
				ProcessTime:                 vsm.F(45),  // 45 minutes
				CycleTime:                   vsm.F(2.5), // 2.5 minutes per patty
				SetupTime:                   vsm.F(10),  // 10 minutes
				ProcessingTime:              vsm.F(35),  // processing; process time = processing + setup (A.2)
				EmployeeCount:               vsm.F(2),   // 2 employees
				ResourceCount:               vsm.I(1),   // 1 patty machine
				TimeUnit:                    vsm.TimeUnitMinutes,
				ValueAddingTime:             vsm.F(40),   // 40 minutes value-adding
				NonValueAddingTime:          vsm.F(5),    // 5 minutes non-value-adding
				NecessaryNonValueAddingTime: vsm.F(0),    // No necessary non-value-adding time
				YieldFactor:                 vsm.F(0.95), // 95% yield
				ScrapRate:                   vsm.F(0.03), // 3% scrap
				ReworkRate:                  vsm.F(0.02), // 2% rework
			},
			Description: "Mix and form burger patties",
		},
		{
			ID:   "process_3",
			Name: "Grill Patties",
			Type: vsm.ProcessTypeMaterial,
			Parameters: vsm.ProcessParameters{
				ProcessTime:                 vsm.F(60), // 60 minutes
				CycleTime:                   vsm.F(5),  // 5 minutes per patty
				SetupTime:                   vsm.F(5),  // 5 minutes
				ProcessingTime:              vsm.F(55), // processing; process time = processing + setup (A.2)
				EmployeeCount:               vsm.F(1),  // 1 grill operator
				ResourceCount:               vsm.I(1),  // 1 grill
				TimeUnit:                    vsm.TimeUnitMinutes,
				ValueAddingTime:             vsm.F(55),   // 55 minutes value-adding
				NonValueAddingTime:          vsm.F(5),    // 5 minutes non-value-adding
				NecessaryNonValueAddingTime: vsm.F(0),    // No necessary non-value-adding time
				YieldFactor:                 vsm.F(0.97), // 97% yield
				ScrapRate:                   vsm.F(0.02), // 2% scrap
				ReworkRate:                  vsm.F(0.01), // 1% rework
			},
			Description: "Grill burger patties to perfection",
		},
		{
			ID:   "process_4",
			Name: "Assemble Burgers",
			Type: vsm.ProcessTypeMaterial,
			Parameters: vsm.ProcessParameters{
				ProcessTime:                 vsm.F(30),  // 30 minutes
				CycleTime:                   vsm.F(1.5), // 1.5 minutes per burger
				SetupTime:                   vsm.F(5),   // 5 minutes
				ProcessingTime:              vsm.F(25),  // processing; process time = processing + setup (A.2)
				EmployeeCount:               vsm.F(3),   // 3 assembly workers
				ResourceCount:               vsm.I(1),   // 1 assembly station
				TimeUnit:                    vsm.TimeUnitMinutes,
				ValueAddingTime:             vsm.F(28),   // 28 minutes value-adding
				NonValueAddingTime:          vsm.F(2),    // 2 minutes non-value-adding
				NecessaryNonValueAddingTime: vsm.F(0),    // No necessary non-value-adding time
				YieldFactor:                 vsm.F(0.99), // 99% yield
				ScrapRate:                   vsm.F(0.01), // 1% scrap
				ReworkRate:                  vsm.F(0.00), // 0% rework
			},
			Description: "Assemble complete burgers with toppings",
		},
		{
			ID:   "process_5",
			Name: "Package and Ship",
			Type: vsm.ProcessTypeMaterial,
			Parameters: vsm.ProcessParameters{
				ProcessTime:                 vsm.F(45), // 45 minutes
				CycleTime:                   vsm.F(2),  // 2 minutes per order
				SetupTime:                   vsm.F(10), // 10 minutes
				ProcessingTime:              vsm.F(35), // processing; process time = processing + setup (A.2)
				EmployeeCount:               vsm.F(2),  // 2 packaging workers
				ResourceCount:               vsm.I(1),  // 1 packaging station
				TimeUnit:                    vsm.TimeUnitMinutes,
				ValueAddingTime:             vsm.F(40),   // 40 minutes value-adding
				NonValueAddingTime:          vsm.F(5),    // 5 minutes non-value-adding
				NecessaryNonValueAddingTime: vsm.F(0),    // No necessary non-value-adding time
				YieldFactor:                 vsm.F(0.98), // 98% yield
				ScrapRate:                   vsm.F(0.02), // 2% scrap
				ReworkRate:                  vsm.F(0.00), // 0% rework
			},
			Description: "Package burgers and prepare for shipping",
		},
	}

	// Create inventory elements
	inventory := []vsm.Inventory{
		{
			ID:          "inventory_1",
			Name:        "Raw Ingredients",
			Quantity:    vsm.F(400),         // burger ingredient kits (same unit as customer demand)
			Range:       vsm.F(2 * 24 * 60), // 2 days in minutes: 400 kits ÷ 200 per day
			Type:        "warehouse",
			Description: "Raw ingredients warehouse",
		},
		{
			ID:          "inventory_2",
			Name:        "Finished Patties",
			Quantity:    vsm.F(50),     // patties
			Range:       vsm.F(6 * 60), // 6 hours in minutes: 50 ÷ 200 per day = 0.25 day
			Type:        "supermarket",
			Description: "Grilled patties supermarket",
		},
	}

	// Create current state value stream
	currentState := &vsm.ValueStream{
		ID:            "vsm_burger_production_current",
		Name:          "Burger Production - Current State",
		State:         vsm.StateCurrent,
		ProductFamily: "Burgers",
		Description:   "Current state of burger production value stream",
		Customer:      customer,
		Supplier:      supplier,
		Processes:     processes,
		Inventory:     inventory,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Version:       "1.0",
	}

	// Calculate KPIs for current state
	calculator := vsm.NewCalculator()
	currentState.KPIs = calculator.CalculateKPIs(currentState)

	// Create VSM project
	project := &vsm.VSMProject{
		ID:           "project_burger_production",
		Name:         "Burger Production VSM Project",
		Description:  "Complete VSM analysis and improvement project for burger production",
		CurrentState: *currentState,
		CurrentPhase: vsm.PhaseAnalysis,
		Status:       "active",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	return project
}

// analyzeCurrentState performs comprehensive analysis of the current value stream
func analyzeCurrentState(project *vsm.VSMProject) *vsm.AnalysisResult {
	analyzer := vsm.NewAnalyzer()

	// Perform current state analysis
	analysisResult, err := analyzer.AnalyzeCurrentState(&project.CurrentState)
	if err != nil {
		log.Fatalf("Failed to analyze current state: %v", err)
	}

	return analysisResult
}

// designFutureState creates future state design based on analysis
func designFutureState(project *vsm.VSMProject, analysis *vsm.AnalysisResult) *vsm.DesignResult {
	designer := vsm.NewDesigner()

	// Future-state targets the team expects to reach (example values):
	// grill setup time down 60%, raw ingredient stock halved.
	targets := &vsm.FutureStateTargets{
		Processes: map[string]vsm.ProcessTargets{
			"process_3": {SetupTime: vsm.ReductionTarget(60)},
		},
		Inventory: map[string]vsm.InventoryTargets{
			"inventory_1": {Quantity: vsm.ReductionTarget(50)},
		},
	}

	// Design future state improvements, with the team's impact and effort
	// ratings (1 = lowest, 5 = highest) for two of the potentials; the
	// others stay unprioritised.
	designResult, err := designer.DesignFutureStateWithInputs(&project.CurrentState, analysis, targets, exampleMeasureInputs())
	if err != nil {
		log.Fatalf("Failed to design future state: %v", err)
	}

	return designResult
}

// exampleMeasureInputs are example values the team entered for two
// measures (keyed by measure ID): ratings, money in currency units, a due
// date and an owner. Measures without inputs are reported as not
// prioritised and not costed.
func exampleMeasureInputs() map[string]vsm.MeasureInput {
	due := time.Now().AddDate(0, 2, 0)
	return map[string]vsm.MeasureInput{
		"bottleneck_process_3": {
			ImpactRating: vsm.I(4), EffortRating: vsm.I(2),
			Investment: vsm.F(12000), AnnualBenefit: vsm.F(25000), AnnualOperatingCost: vsm.F(1000),
			DueDate: &due, Responsible: "Grill team lead",
		},
		"waste_inventory": {
			ImpactRating: vsm.I(3), EffortRating: vsm.I(3),
			Investment: vsm.F(5000), AnnualBenefit: vsm.F(8000), AnnualOperatingCost: vsm.F(500),
		},
	}
}

// planImplementation creates implementation plan
func planImplementation(design *vsm.DesignResult, project *vsm.VSMProject) *vsm.PlanningResult {
	planner := vsm.NewPlanner()

	start := time.Now().AddDate(0, 0, 7)
	workshop := time.Now().AddDate(0, 0, 3)
	inputs := &vsm.PlanningInputs{
		Measures:     exampleMeasureInputs(),
		StartDate:    &start,
		WorkshopDate: &workshop,
		Risks: map[string]vsm.RiskInput{
			"risk_resistance": {Probability: vsm.F(0.5), Impact: vsm.F(0.6)},
		},
	}

	// Create implementation plan
	planningResult, err := planner.PlanImplementationWithInputs(design, project, inputs)
	if err != nil {
		log.Fatalf("Failed to plan implementation: %v", err)
	}

	return planningResult
}

// assessPerformance performs performance assessment
func assessPerformance(project *vsm.VSMProject) *vsm.AssessmentResult {
	assessor := vsm.NewAssessor()

	// Define KPI targets
	targets := map[string]float64{
		"value_added_ratio":  0.25, // Target 25% value-added time
		"process_efficiency": 0.50, // Target 50% process efficiency
		// Turns are per the customer's demand period (here per day): 0.5
		// turns a day is 2 days of stock, about 180 turns a year at 7
		// working days a week. The current state holds 450 units at 200 a
		// day, 0.44 turns a day.
		"inventory_turns":      0.5,
		"average_yield_factor": 0.95, // Target 95% yield
	}

	// Assess current performance
	assessmentResult, err := assessor.AssessValueStream(&project.CurrentState, targets, nil)
	if err != nil {
		log.Fatalf("Failed to assess performance: %v", err)
	}

	return assessmentResult
}

// serializeResults saves all results to JSON files
func serializeResults(project *vsm.VSMProject, analysis *vsm.AnalysisResult,
	design *vsm.DesignResult, planning *vsm.PlanningResult, assessment *vsm.AssessmentResult) {

	serializer := vsm.NewSerializer()

	// Save project
	if err := serializer.SaveToFile(project, "vsm_project.json"); err != nil {
		log.Printf("Failed to save project: %v", err)
	}

	// Save analysis results
	if err := serializer.SaveToFile(analysis, "analysis_result.json"); err != nil {
		log.Printf("Failed to save analysis: %v", err)
	}

	// Save design results
	if err := serializer.SaveToFile(design, "design_result.json"); err != nil {
		log.Printf("Failed to save design: %v", err)
	}

	// Save planning results
	if err := serializer.SaveToFile(planning, "planning_result.json"); err != nil {
		log.Printf("Failed to save planning: %v", err)
	}

	// Save assessment results
	if err := serializer.SaveToFile(assessment, "assessment_result.json"); err != nil {
		log.Printf("Failed to save assessment: %v", err)
	}

	// Export value stream map
	if project.FutureState != nil {
		mapStr, err := serializer.ExportValueStreamMap(project.FutureState)
		if err != nil {
			log.Printf("Failed to export value stream map: %v", err)
		} else {
			if err := serializer.SaveToFile(mapStr, "value_stream_map.txt"); err != nil {
				log.Printf("Failed to save value stream map: %v", err)
			}
		}
	}
}

// displaySummary shows a summary of all results
func displaySummary(analysis *vsm.AnalysisResult, design *vsm.DesignResult,
	planning *vsm.PlanningResult, assessment *vsm.AssessmentResult) {

	fmt.Println("\n=== VSM Analysis Summary ===")

	// Analysis Summary
	fmt.Printf("Bottlenecks Found: %d\n", len(analysis.Bottlenecks))
	// Waste and flow efficiency are nil when data they need is missing
	// (listed in the consistency issues as missing data).
	if w := analysis.WasteAnalysis; w.TotalWaste != nil && *w.TotalWaste > 0 && w.WasteReductionPotential != nil {
		fmt.Printf("Waste Reduction Potential: %.1f%%\n", *w.WasteReductionPotential / *w.TotalWaste * 100)
	} else {
		fmt.Println("Waste Reduction Potential: unknown")
	}
	if e := analysis.FlowAnalysis.FlowEfficiency; e != nil {
		fmt.Printf("Flow Efficiency: %.1f%%\n", *e*100)
	} else {
		fmt.Println("Flow Efficiency: unknown")
	}
	fmt.Printf("Consistency Issues (current / future state): %d / %d\n",
		len(analysis.ConsistencyIssues), len(vsm.CheckConsistency(design.FutureState)))

	// Design Summary
	fmt.Printf("Improvement Potentials: %d\n", len(design.ImprovementPotentials))
	fmt.Printf("Design Principles Applied: %d/%d\n", countAppliedPrinciples(design.DesignPrinciples), len(design.DesignPrinciples))
	fmt.Printf("Continuous Improvements: %d\n", len(design.ContinuousImprovements))

	// Planning Summary
	fmt.Printf("Measures in Catalogue: %d\n", len(planning.CatalogueOfMeasures.Measures))
	fmt.Printf("Implementation Phases: %d\n", len(planning.ImplementationPlan.Phases))
	fmt.Printf("Risks Identified: %d\n", len(planning.RiskAssessment))
	// Priority, payback and goal come only from entered values; the rest
	// is reported as not prioritised / not estimated.
	for _, m := range planning.CatalogueOfMeasures.Measures[:min(3, len(planning.CatalogueOfMeasures.Measures))] {
		priority := "not prioritised"
		if m.Priority > 0 {
			priority = fmt.Sprintf("priority %d", m.Priority)
		}
		payback := m.Costs.Note
		switch {
		case m.Costs.PaybackMonths != nil:
			payback = fmt.Sprintf("payback %.1f months", *m.Costs.PaybackMonths)
		case m.Costs.NeverPaysBack:
			payback = "never pays back"
		}
		fmt.Printf("  Measure %s: %s; %s\n    Goal: %s\n", m.ID, priority, payback, m.SMART.Measurable)
	}
	if inv := planning.CostTotals.Investment; inv != nil {
		fmt.Printf("Total Investment: %.0f\n", *inv)
	} else {
		fmt.Printf("Total Investment: not estimated (%d measures without costs entered)\n",
			len(planning.CostTotals.MissingInputs))
	}

	// Assessment Summary
	fmt.Printf("Performance Index: %.2f\n", assessment.PerformanceIndex)
	fmt.Printf("KPIs Assessed: %d\n", len(assessment.KPIs))
	fmt.Printf("Action Items: %d\n", len(assessment.ActionItems))
	fmt.Printf("Recommendations: %d\n", len(assessment.Recommendations))
}

func countAppliedPrinciples(principles []vsm.DesignPrinciple) int {
	count := 0
	for _, principle := range principles {
		if principle.Applied {
			count++
		}
	}
	return count
}
