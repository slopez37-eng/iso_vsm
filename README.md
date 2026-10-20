# ISO 22468:2020 Value Stream Management (VSM) Go SDK

## Document Information

**Document Title**: ISO 22468:2020 VSM Go SDK Developer Guide  
**Document Version**: 1.0  
**Document Date**: December 2025  
**Software Version**: v1.0.0  
**Compliance Standard**: ISO/IEC/IEEE 26514:2017  

## Table of Contents

| Section | Topic | Description |
|---------|-------|-------------|
| **1** | **[Introduction](#1-introduction)** | Document purpose, scope, audience, and requirements |
| **1.1** | **[Purpose](#11-purpose)** | Goals and objectives of this SDK |
| **1.2** | **[Scope](#12-scope)** | What is covered and what is not |
| **1.3** | **[Target Audience](#13-target-audience)** | Who this documentation is for |
| **1.4** | **[Prerequisites](#14-prerequisites)** | Required knowledge and software |
| **2** | **[Concept of Operations](#2-concept-of-operations)** | Understanding VSM and SDK architecture |
| **2.1** | **[Value Stream Management Overview](#21-value-stream-management-overview)** | VSM methodology explanation |
| **2.2** | **[SDK Architecture](#22-sdk-architecture)** | How the SDK components work together |
| **2.3** | **[Core Components](#23-core-components)** | Key data structures and calculations |
| **3** | **[Procedures](#3-procedures)** | Step-by-step usage instructions |
| **3.1** | **[Installation](#31-installation)** | How to install the SDK |
| **3.2** | **[Getting Started](#32-getting-started)** | Basic setup and first value stream |
| **3.3** | **[Basic Usage](#33-basic-usage)** | Essential operations and workflows |
| **3.4** | **[Advanced Usage](#34-advanced-usage)** | Complex scenarios and optimization |
| **4** | **[Reference Information](#4-reference-information)** | Detailed technical specifications |
| **4.1** | **[API Reference](#41-api-reference)** | Complete API documentation |
| **4.2** | **[Calculation Procedures](#42-calculation-procedures)** | KPI formulas and algorithms |
| **4.3** | **[VSM Symbols](#43-vsm-symbols)** | Standard symbols and usage |
| **4.4** | **[Serialization](#44-serialization)** | Data persistence and formats |
| **5** | **[Troubleshooting](#5-troubleshooting)** | Common issues and solutions |
| **6** | **[Glossary](#6-glossary)** | Key terms and definitions |
| **7** | **[Standards Compliance](#7-standards-compliance)** | ISO compliance and validation |
| **8** | **[Contributing](#8-contributing)** | How to contribute to the project |
| **9** | **[License](#9-license)** | Legal information and permissions |
| **10** | **[References](#10-references)** | Standards and bibliography |

### Quick Navigation Guide

| **Getting Started** | **API Reference** | **Troubleshooting** |
|-------------------|------------------|-------------------|
| [Installation](#31-installation) | [API Reference](#41-api-reference) | [Troubleshooting](#5-troubleshooting) |
| [Getting Started](#32-getting-started) | [Calculations](#42-calculation-procedures) | [Common Issues](#5-troubleshooting) |
| [Basic Usage](#33-basic-usage) | [Symbols](#43-vsm-symbols) | [Error Solutions](#5-troubleshooting) |
| [Advanced Usage](#34-advanced-usage) | [Serialization](#44-serialization) | [Help & Support](#5-troubleshooting) |

### Document Statistics
- **Total Sections**: 10 main sections
- **Total Subsections**: 14 subsections
- **Code Examples**: 25+ practical examples
- **Compliance**: ISO/IEC/IEEE 26514:2017

<a name="1-introduction"></a>
## 1. Introduction

### 1.1 Purpose

This document provides comprehensive guidance for developers implementing Value Stream Management (VSM) solutions using the ISO 22468:2020 VSM Go SDK. The SDK offers a complete Go implementation of the ISO 22468:2020 standard, enabling systematic analysis, design, planning, and assessment of value streams in manufacturing and service processes.

The primary purposes of this SDK are to:
- Provide standardized VSM implementation compliant with ISO 22468:2020
- Enable automated calculation of key performance indicators (KPIs)
- Support complete VSM workflow from current state analysis to future state implementation
- Facilitate waste identification and reduction opportunities

### 1.2 Scope

This documentation covers:
- Installation and setup of the VSM Go SDK
- Core concepts and architecture of the SDK
- Step-by-step procedures for VSM implementation
- API reference for all SDK components
- Calculation procedures and formulas from ISO 22468:2020 Annex A
- Troubleshooting common issues and error conditions

This documentation does **not** cover:
- General Go programming language concepts
- ISO 22468:2020 standard interpretation (refer to the standard itself)
- Lean manufacturing principles beyond VSM scope
- Third-party Go libraries used by the SDK

### 1.3 Target Audience

This guide is intended for:
- **Software Developers**: Go developers implementing VSM solutions
- **Process Engineers**: Engineers requiring automated VSM analysis tools
- **System Architects**: Architects designing manufacturing execution systems
- **Quality Assurance Specialists**: QA professionals validating VSM implementations

**Assumed Knowledge**:
- Basic proficiency in Go programming language
- Understanding of manufacturing processes and value streams
- Familiarity with lean manufacturing concepts
- Knowledge of JSON data structures

### 1.4 Prerequisites

Before using this SDK, ensure you have:

**Software Requirements**:
- Go 1.19 or later
- Operating system: Windows, macOS, or Linux

**Knowledge Prerequisites**:
- Go programming fundamentals (variables, functions, structs, interfaces)
- Basic understanding of JSON serialization
- Familiarity with manufacturing process terminology
- Understanding of value stream mapping concepts

<a name="2-concept-of-operations"></a>
## 2. Concept of Operations

### 2.1 Value Stream Management Overview

Value Stream Management (VSM) is a lean methodology for analyzing and improving the flow of materials and information required to bring a product or service to a customer. The methodology focuses on identifying and eliminating waste while optimizing value-adding activities.

The VSM process consists of four main phases:

1. **Analysis Phase**: Mapping the current state and identifying bottlenecks and waste
2. **Design Phase**: Creating future state designs with improvement opportunities
3. **Planning Phase**: Developing implementation plans with specific measures
4. **Assessment Phase**: Monitoring performance and managing continuous improvement

### 2.2 SDK Architecture

The SDK implements the complete VSM methodology through four main components:

- **Analyzer**: Performs current state analysis including bottleneck detection and waste analysis
- **Designer**: Creates future state designs and identifies improvement potentials
- **Planner**: Develops implementation plans with SMART (Specific, Measurable, Achievable, Relevant, Time-bound) measures
- **Assessor**: Monitors performance against targets and manages PDCA (Plan-Do-Check-Act) cycles

### 2.3 Core Components

#### Value Stream Model
The central data structure representing a complete value stream with:
- Processes (material, energy, data transformations)
- Inventory points and buffers
- Customer and supplier interfaces
- Flow controls and information flows

#### Process Parameters
All parameters defined in ISO 22468:2020 Annex A:
- Process Time (PT) = Processing Time + Setup Time + Handling Time + Waiting Time + Administrative Time
- Cycle Time (CT): Time between process completions
- Lead Time (LT) = Value-Added Time + Non-Value-Added Time + Necessary Non-Value-Added Time
- Yield Factor (YF) = 1 - (Scrap Rate + Rework Rate)

#### KPI Calculations
Automated calculation of key performance indicators:
- Value-Added Ratio = VAT / LT
- Process Efficiency = VAT / PT
- Customer Takt Time = Operating Time / Customer Demand
- Inventory Turns = Customer Demand / Stock Quantity

<a name="3-procedures"></a>
## 3. Procedures

<a name="31-installation"></a>
### 3.1 Installation

To install the VSM Go SDK in your Go project:

1. Ensure you have Go 1.19 or later installed
2. Open a terminal in your project directory
3. Run the following command:

```bash
go get github.com/iso22468/vsm-go-sdk
```

4. Import the package in your Go code:

```go
import vsm "github.com/iso22468/vsm-go-sdk"
```

**Expected Result**: The SDK is now available in your Go module dependencies.

<a name="32-getting-started"></a>
### 3.2 Getting Started

Follow these steps to create your first VSM analysis:

#### Step 1: Import Required Packages
```go
package main

import (
    "fmt"
    "log"
    vsm "github.com/iso22468/vsm-go-sdk"
)
```

#### Step 2: Create a Value Stream
```go
func main() {
    // Define customer requirements
    customer := &vsm.Customer{
        ID:       "customer_1",
        Name:     "Manufacturing Corp",
        Demand:   vsm.F(1000), // units per period
        TaktTime: vsm.F(5.0),  // time units per unit
    }

    // Create value stream
    vs := &vsm.ValueStream{
        ID:            "production_line_1",
        Name:          "Main Production Line",
        State:         vsm.StateCurrent,
        ProductFamily: "Widget Family A",
        Customer:      customer,
    }

    fmt.Printf("Created value stream: %s\n", vs.Name)
}
```

#### Step 3: Add Processes

Every numeric parameter is optional. Fill one in with `vsm.F(v)` (or
`vsm.I(n)` for counts); a field left out is "not entered", and `vsm.F(0)` is
an entered zero (a setup time of 0, a stock of 0, a yield of 0). In JSON a
value that was not entered is omitted; files written before
`"format_version": 2` read a 0 as "not entered" (see serialization.go).

```go
// Add manufacturing processes
processes := []vsm.Process{
    {
        ID:   "receiving",
        Name: "Receiving",
        Type: vsm.ProcessTypeMaterial,
        Parameters: vsm.ProcessParameters{
            ProcessTime:    vsm.F(30),   // minutes
            CycleTime:      vsm.F(5),    // minutes
            YieldFactor:    vsm.F(0.98), // 98% yield
            TimeUnit:       vsm.TimeUnitMinutes,
            SetupTime:      vsm.F(10),
            WaitingTime:    vsm.F(15),
        },
    },
    {
        ID:   "assembly",
                Name: "Assembly",
                Type: vsm.ProcessTypeMaterial,
                Parameters: vsm.ProcessParameters{
            ProcessTime:    vsm.F(45),
            CycleTime:      vsm.F(8),
            YieldFactor:    vsm.F(0.95),
            TimeUnit:       vsm.TimeUnitMinutes,
            SetupTime:      vsm.F(20),
            WaitingTime:    vsm.F(10),
        },
    },
}

vs.Processes = processes
```

#### Step 4: Perform Analysis
```go
// Create analyzer and analyze current state
analyzer := vsm.NewAnalyzer()
analysis, err := analyzer.AnalyzeCurrentState(vs)
if err != nil {
    log.Fatalf("Analysis failed: %v", err)
}

// Display results
fmt.Printf("Bottlenecks found: %d\n", len(analysis.Bottlenecks))
fmt.Printf("Waste reduction potential: %.1f%%\n",
    analysis.WasteAnalysis.WasteReductionPotential)
```

<a name="33-basic-usage"></a>
### 3.3 Basic Usage

#### Calculating KPIs
```go
calculator := vsm.NewCalculator()
kpis, err := calculator.CalculateKPIs(vs)
if err != nil {
    log.Fatalf("KPI calculation failed: %v", err)
}

fmt.Printf("Value-added ratio: %.2f%%\n", kpis["value_added_ratio"]*100)
fmt.Printf("Process efficiency: %.2f%%\n", kpis["process_efficiency"]*100)
```

#### Working with Inventory
```go
inventory := []vsm.Inventory{
    {
        ID:          "raw_materials",
        Name:        "Raw Materials Buffer",
        Type:        vsm.InventoryTypeWIP,
        Quantity:    vsm.F(500),
        TurnRate:    12, // turns per year
        HoldingCost: 0.15, // 15% of value per year
    },
}

vs.Inventory = inventory
```

<a name="34-advanced-usage"></a>
### 3.4 Advanced Usage

#### Future State Design
```go
// Analyze current state first
    analysis, _ := analyzer.AnalyzeCurrentState(vs)

// Create designer and design future state
designer := vsm.NewDesigner()
project := &vsm.Project{
    ID:          "improvement_project_1",
    Name:        "Lean Transformation Initiative",
    Objectives:  []string{"Reduce lead time by 30%", "Increase efficiency by 25%"},
    Timeframe:   "6 months",
    Budget:      50000.0,
}

design, err := designer.DesignFutureState(analysis, project)
if err != nil {
    log.Fatalf("Future state design failed: %v", err)
}

fmt.Printf("Identified %d improvement potentials\n", len(design.ImprovementPotentials))
```

#### Implementation Planning
```go
planner := vsm.NewPlanner()
plan, err := planner.PlanImplementation(design, project)
if err != nil {
    log.Fatalf("Planning failed: %v", err)
}

fmt.Printf("Generated %d SMART measures\n", len(plan.CatalogueOfMeasures.Measures))
fmt.Printf("Workshop scheduled for: %s\n", plan.WorkshopPlan.Date)
```

#### Performance Assessment
```go
assessor := vsm.NewAssessor()

// Define performance targets
targets := map[string]float64{
    "value_added_ratio":    0.30, // 30%
    "process_efficiency":   0.60, // 60%
    "on_time_delivery":     0.95, // 95%
}

// Historical performance data
history := []vsm.PerformanceData{
    {
        Timestamp: time.Now().AddDate(0, -1, 0), // 1 month ago
        KPIs: map[string]float64{
            "value_added_ratio": 0.22,
            "process_efficiency": 0.45,
        },
    },
}

assessment, err := assessor.AssessValueStream(vs, targets, history)
if err != nil {
    log.Fatalf("Assessment failed: %v", err)
}

fmt.Printf("Performance index: %.2f/100\n", assessment.PerformanceIndex*100)
for _, action := range assessment.ActionItems {
    fmt.Printf("Recommended action: %s\n", action.Description)
}
```

<a name="4-reference-information"></a>
## 4. Reference Information

<a name="41-api-reference"></a>
### 4.1 API Reference

#### Core Types

##### ValueStream
The main data structure representing a complete value stream.

```go
type ValueStream struct {
    ID            string       // Unique identifier
    Name          string       // Display name
    State         State        // Current, Future, or Ideal
    ProductFamily string       // Product family name
    Customer      *Customer    // Customer requirements
    Supplier      *Supplier    // Supplier information (optional)
    Processes     []Process    // Manufacturing processes
    Inventory     []Inventory  // Inventory points
    FlowControls  []FlowControl // Push/pull controls
    CreatedAt     time.Time    // Creation timestamp
    UpdatedAt     time.Time    // Last update timestamp
}
```

##### Process
Represents a manufacturing or service process.

```go
type Process struct {
    ID         string            // Unique identifier
    Name       string            // Process name
    Type       ProcessType       // Material, Energy, or Data
    Parameters ProcessParameters // Process timing and quality parameters
    Inputs     []string          // Input material/process IDs
    Outputs    []string          // Output material/process IDs
    Resources  []Resource        // Required resources
}
```

##### ProcessParameters
Contains all timing and quality parameters as defined in ISO 22468:2020 Annex A.

```go
type ProcessParameters struct {
    // Time components (all in TimeUnit)
    ProcessTime    float64   // PRT: Processing time
    SetupTime      float64   // ST: Setup/changeover time
    HandlingTime   float64   // HT: Material handling time
    WaitingTime    float64   // WTp: Process waiting time
    AdminTime      float64   // AT: Administrative time
    CycleTime      float64   // CT: Time between completions

    // Quality parameters
    YieldFactor    float64   // YF: 1 - (scrap + rework rates)
    ScrapRate      float64   // SR: Scrap percentage
    ReworkRate     float64   // RR: Rework percentage

    // Additional parameters
    BatchSize      int       // BS: Process batch size
    TimeUnit       TimeUnit  // Unit for time measurements
    Availability   float64   // OA: Operational availability (0.0-1.0)
}
```

#### Main Components

None of the components has configuration options; create each with its `New…` function. The listings below show the main public methods.

##### Analyzer
Performs current state analysis and bottleneck detection.

```go
func NewAnalyzer() *Analyzer
func (a *Analyzer) AnalyzeCurrentState(vs *ValueStream) (*AnalysisResult, error)
func (a *Analyzer) IdentifyBottlenecks(vs *ValueStream) ([]BottleneckInfo, error)
func (a *Analyzer) AnalyzeIdleTimes(vs *ValueStream) (*IdleTimeAnalysis, error)
func (a *Analyzer) AnalyzeWaste(vs *ValueStream) (*WasteAnalysis, error)
func (a *Analyzer) AnalyzeFlow(vs *ValueStream) (*FlowAnalysis, error)
func (a *Analyzer) CalculateProcessLoad(process Process, vs *ValueStream) (capacity, utilization float64, ok bool)
func (a *Analyzer) GenerateRecommendations(result *AnalysisResult) []string
func (a *Analyzer) SelectProductFamily(products []ProductFamily) (*ProductFamily, error)
```

##### Designer
Creates ideal and future state designs and improvement potentials.

```go
func NewDesigner() *Designer
func (d *Designer) DesignFutureState(current *ValueStream, analysis *AnalysisResult) (*DesignResult, error)
func (d *Designer) DesignFutureStateWithTargets(current *ValueStream, analysis *AnalysisResult, targets *FutureStateTargets) (*DesignResult, error)
func (d *Designer) DesignFutureStateWithInputs(current *ValueStream, analysis *AnalysisResult, targets *FutureStateTargets, inputs map[string]MeasureInput) (*DesignResult, error)
func (d *Designer) CreateIdealState(current *ValueStream) (*ValueStream, error)
func (d *Designer) DesignFutureStateFromIdeal(ideal, current *ValueStream) (*ValueStream, error)
func (d *Designer) DesignFutureStateFromTargets(current *ValueStream, targets *FutureStateTargets) (*ValueStream, error)
func (d *Designer) IdentifyImprovementPotentials(vs *ValueStream, analysis *AnalysisResult) ([]ImprovementPotential, error)
func (d *Designer) PrioritizeImprovementPotentials(potentials []ImprovementPotential, inputs map[string]MeasureInput) ([]ImprovementPotential, error)
func (d *Designer) IdentifyContinuousImprovements(vs *ValueStream, analysis *AnalysisResult) []ContinuousImprovement
func (d *Designer) ApplyDesignPrinciples(current, future *ValueStream) []DesignPrinciple
```

##### Planner
Develops implementation plans, catalogues of measures and workshops.

```go
func NewPlanner() *Planner
func (p *Planner) PlanImplementation(design *DesignResult, project *VSMProject) (*PlanningResult, error)
func (p *Planner) PlanImplementationWithInputs(design *DesignResult, project *VSMProject, inputs *PlanningInputs) (*PlanningResult, error)
func (p *Planner) CreateCatalogueOfMeasures(design *DesignResult, project *VSMProject) (*CatalogueOfMeasures, error)
func (p *Planner) PlanWorkshop(design *DesignResult, project *VSMProject) (*WorkshopPlan, error)
func (p *Planner) CreateImplementationPlan(catalogue *CatalogueOfMeasures) (*ImplementationPlan, error)
func (p *Planner) AssessRisks(design *DesignResult, implementation *ImplementationPlan) []Risk
```

##### Assessor
Assesses performance against targets; action items come back inside the assessment result.

```go
func NewAssessor() *Assessor
func (a *Assessor) AssessValueStream(vs *ValueStream, targets map[string]float64, history []AssessmentResult) (*AssessmentResult, error)
func (a *Assessor) CompareAssessments(current, previous *AssessmentResult) map[string]float64
```

##### Calculator
Performs the KPI calculations of ISO 22468:2020 Annex A.

```go
func NewCalculator() *Calculator
func (c *Calculator) CalculateKPIs(vs *ValueStream) map[string]float64
func (c *Calculator) CalculateImprovementMetrics(current, future *ValueStream) map[string]float64
func (c *Calculator) ResolveParameters(params ProcessParameters) ProcessParameters
func (c *Calculator) ResolveParametersInStream(params ProcessParameters, vs *ValueStream) ProcessParameters
func (c *Calculator) ValidateProcessParameters(params ProcessParameters) []error
func (c *Calculator) CalculateProcessTime(params ProcessParameters) float64
func (c *Calculator) CalculateLeadTime(params ProcessParameters) float64
func (c *Calculator) CalculateValueAddedRatio(valueAddingTime, leadTime float64) float64
```

The Calculator also offers one method per Annex A formula (for example `CalculateCustomerTakt`, `CalculateYieldFactor`, `CalculateInventoryTime`, `ConvertTimeUnit`).

<a name="42-calculation-procedures"></a>
### 4.2 Calculation Procedures

The SDK implements all calculation procedures from ISO 22468:2020 Annex A. All calculations follow the exact formulas specified in the standard.

#### Basic Time Calculations

**Process Time (PT)**  
Total time required to complete a process cycle:
```
PT = PRT + ST + HT + WTp + AT
```
Where:
- PRT = Processing Time (actual transformation time)
- ST = Setup Time (changeover/setup time)
- HT = Handling Time (material movement time)
- WTp = Process Waiting Time (queue time within process)
- AT = Administrative Time (documentation/reporting time)

**Lead Time (LT)**  
Total time from customer order to delivery:
```
LT = VAT + NVAT + NNVAT
```
Where:
- VAT = Value-Added Time (actual processing time)
- NVAT = Non-Value-Added Time (waste time, can be eliminated)
- NNVAT = Necessary Non-Value-Added Time (required waste, e.g., safety buffers)

**Value-Added Ratio (VAR)**  
Percentage of time spent on value-adding activities:
```
VAR = VAT ÷ LT × 100%
```

#### Key Performance Indicators

**Process Efficiency (PE)**  
Efficiency of individual processes:
```
PE = VAT ÷ PT × 100%
```

**Customer Takt Time (CTT)**  
Rate at which customers require product:
```
CTT = Operating Time ÷ Customer Demand
```

**Inventory Turns (IT)**  
How often inventory is replenished:
```
IT = Customer Demand ÷ Average Inventory Quantity
```

**On-Time In-Full (OTIF)**  
Delivery performance metric:
```
OTIF = (Delivered on Time × Delivered In Full) ÷ Total Orders × 100%
```

**Yield Factor (YF)**  
Quality performance indicator:
```
YF = 1 - (Scrap Rate + Rework Rate)
```

**Flow Efficiency (FE)**  
Measure of process flow smoothness:
```
FE = VAT ÷ LT × 100%
```

#### Advanced Calculations

**Total Value Stream Efficiency (TVSE)**  
Overall efficiency across the entire value stream:
```
TVSE = Σ(VAT) ÷ Σ(LT) × 100%
```

**Process Capacity Utilization (PCU)**  
How effectively process capacity is used:
```
PCU = (Actual Output × Standard Time) ÷ Available Time × 100%
```

**Bottleneck Detection Index (BDI)**  
Identifies constraining processes:
```
BDI = (Process Cycle Time × Customer Demand) ÷ Process Capacity
```
Values > 1.0 indicate potential bottlenecks.

<a name="43-vsm-symbols"></a>
### 4.3 VSM Symbols

The SDK includes standard VSM symbols for mapping and visualization as defined in ISO 22468:2020.

#### Symbol Categories

**Process Symbols**:
- `process_box`: Standard process box for material transformation
- `data_box`: Data/information processing
- `shipment`: Customer/supplier shipment process
- `inspection`: Quality inspection point

**Inventory Symbols**:
- `inventory_triangle`: Inventory/buffer storage
- `supermarket`: Controlled inventory (kanban)
- `fifo_lane`: First-in-first-out lane

**Information Flow Symbols**:
- `information_flow`: Electronic information flow
- `manual_info_flow`: Manual information transfer
- `production_control`: Production scheduling signal
- `mrp_erp`: MRP/ERP system signal

**Material Flow Symbols**:
- `push_arrow`: Push system flow (forecast-driven)
- `pull_arrow`: Pull system flow (demand-driven)
- `go_see_production`: Go-see production scheduling

#### Usage Examples

```go
// Get specific symbol information
symbol, exists := vsm.GetSymbol("process_box")
if exists {
    fmt.Printf("Symbol: %s - %s\n", symbol.Code, symbol.Description)
    fmt.Printf("Category: %s\n", symbol.Category)
}

// Get all symbols in a category
processSymbols := vsm.GetSymbolsByCategory(vsm.CategoryProcess)
for _, sym := range processSymbols {
    fmt.Printf("%s: %s\n", sym.Code, sym.Name)
}

// Check if symbol exists
if vsm.SymbolExists("push_arrow") {
    fmt.Println("Push arrow symbol is available")
}
```

#### Symbol Properties

Each symbol contains:
- `Code`: Unique identifier (string)
- `Name`: Display name (string)
- `Description`: Detailed description (string)
- `Category`: Symbol category (Process, Inventory, Flow, Information)
- `Standard`: Compliance standard reference
- `VisualRepresentation`: ASCII art or Unicode symbol

<a name="44-serialization"></a>
### 4.4 Serialization

The SDK provides comprehensive serialization support for saving and loading value stream data in JSON format.

#### Serializer Component

```go
type Serializer struct {
    // Configuration options
    PrettyPrint      bool          // Enable pretty-printed JSON
    IncludeMetadata  bool          // Include timestamps and version info
    Compression      CompressionType // JSON, GZIP, or NONE
    Validation       bool          // Validate data during serialization
}
```

#### Basic Serialization

**Serialize to JSON string**:
```go
serializer := vsm.NewSerializer()

// Serialize value stream to JSON
jsonData, err := serializer.SerializeValueStream(valueStream)
if err != nil {
    log.Fatalf("Serialization failed: %v", err)
}

fmt.Printf("Serialized data length: %d bytes\n", len(jsonData))
```

**Deserialize from JSON string**:
```go
// Deserialize value stream from JSON
loadedVS, err := serializer.DeserializeValueStream(jsonData)
if err != nil {
    log.Fatalf("Deserialization failed: %v", err)
}

fmt.Printf("Loaded value stream: %s\n", loadedVS.Name)
```

#### File Operations

**Save to file**:
```go
// Save value stream to JSON file
err = serializer.SaveToFile(valueStream, "production_line.json")
if err != nil {
    log.Fatalf("Save failed: %v", err)
}

fmt.Println("Value stream saved to production_line.json")
```

**Load from file**:
```go
// Load value stream from JSON file
loadedVS, err := serializer.LoadFromFile("production_line.json")
if err != nil {
    log.Fatalf("Load failed: %v", err)
}

fmt.Printf("Loaded value stream with %d processes\n", len(loadedVS.Processes))
```

#### Advanced Features

**Batch operations**:
```go
// Serialize multiple value streams
streams := []*vsm.ValueStream{vs1, vs2, vs3}
batchData, err := serializer.SerializeBatch(streams)

// Deserialize multiple value streams
loadedStreams, err := serializer.DeserializeBatch(batchData)
```

**Validation during serialization**:
```go
serializer.Validation = true

// This will validate data integrity before serialization
validatedData, err := serializer.SerializeValueStream(valueStream)
if err != nil {
    fmt.Printf("Validation error: %v\n", err)
}
```

**Compressed serialization**:
```go
serializer.Compression = vsm.CompressionGZIP

// Save compressed data
err = serializer.SaveToFile(valueStream, "compressed_vsm.json.gz")
```

<a name="5-troubleshooting"></a>
## 5. Troubleshooting

This section provides solutions to common issues encountered when using the VSM Go SDK.

### Common Issues and Solutions

#### Issue: "go get github.com/iso22468/vsm-go-sdk" fails

**Symptoms**:
- Command returns network error
- "module not found" error
- Proxy/firewall blocking access

**Solutions**:

1. **Check network connectivity**:
   ```bash
   ping github.com
   ```

2. **Configure Go proxy** (if behind corporate firewall):
   ```bash
   go env -w GOPROXY=https://proxy.golang.org,direct
   ```

3. **Use VPN** if corporate network blocks GitHub

4. **Manual download** (fallback option):
   - Download the repository as ZIP from GitHub
   - Place in `$GOPATH/src/github.com/iso22468/`
   - Run `go mod tidy`

#### Issue: Import errors in Go code

**Symptoms**:
- "cannot find package" error
- IDE shows red squiggles on import statement

**Solutions**:

1. **Verify module installation**:
   ```bash
   go list -m github.com/iso22468/vsm-go-sdk
   ```

2. **Clean module cache**:
   ```bash
   go clean -modcache
   go get github.com/iso22468/vsm-go-sdk@latest
   ```

3. **Check Go version compatibility**:
   ```bash
   go version
   ```
   Ensure Go 1.19 or later is installed.

#### Issue: Analysis returns empty results

**Symptoms**:
- `analyzer.AnalyzeCurrentState()` returns empty bottleneck list
- KPI calculations return zero values
- No errors reported but no meaningful output

**Solutions**:

1. **Check value stream completeness**:
   ```go
   // Ensure processes have valid parameters
   for _, process := range vs.Processes {
       if process.Parameters.ProcessTime <= 0 {
           fmt.Printf("Process %s has invalid process time\n", process.ID)
       }
   }
   ```

2. **Verify customer demand is set**:
   ```go
   if vs.Customer == nil || vs.Customer.Demand <= 0 {
       fmt.Println("Customer demand not properly configured")
   }
   ```

3. **Check process connections**:
   - Ensure process inputs/outputs are properly linked
   - Verify inventory points are correctly positioned

#### Issue: Serialization fails

**Symptoms**:
- `SerializeValueStream()` returns error
- JSON file is corrupted or unreadable

**Solutions**:

1. **Enable validation**:
   ```go
   serializer := vsm.NewSerializer()
   serializer.Validation = true

   data, err := serializer.SerializeValueStream(vs)
   if err != nil {
       fmt.Printf("Validation failed: %v\n", err)
   }
   ```

2. **Check for circular references**:
   - Ensure no circular dependencies in process flows
   - Verify inventory references are valid

3. **Handle special characters**:
   - Use UTF-8 encoding for all strings
   - Escape special characters in process names

#### Issue: Memory usage grows during analysis

**Symptoms**:
- Application consumes excessive memory
- Performance degrades with large value streams
- Out of memory errors

**Solutions**:

1. **Optimize analyzer configuration**:
   ```go
   analyzer := vsm.NewAnalyzer()
   analyzer.SensitivityThreshold = 0.8 // Reduce sensitivity for large datasets
   ```

2. **Process in batches**:
   ```go
   // Analyze processes in chunks
   chunkSize := 50
   for i := 0; i < len(vs.Processes); i += chunkSize {
       end := i + chunkSize
       if end > len(vs.Processes) {
           end = len(vs.Processes)
       }

       partialVS := &vsm.ValueStream{
           Processes: vs.Processes[i:end],
           // ... other fields
       }

       partialAnalysis, _ := analyzer.AnalyzeCurrentState(partialVS)
       // Process partial results
   }
   ```

3. **Use streaming serialization** for large datasets

#### Issue: KPI calculations are incorrect

**Symptoms**:
- Calculated ratios don't match expectations
- Negative values where positive expected
- Results don't align with manual calculations

**Solutions**:

1. **Verify time units consistency**:
   ```go
   // Ensure all processes use the same time unit
   for _, process := range vs.Processes {
       if process.Parameters.TimeUnit != vsm.TimeUnitMinutes {
           fmt.Printf("Inconsistent time unit in process %s\n", process.ID)
       }
   }
   ```

2. **Check parameter ranges**:
   - Process times should be positive
   - Yield factors should be between 0.0 and 1.0
   - Efficiency ratios should not exceed 1.0

3. **Validate against known benchmarks**:
   - Compare with manual calculations for small examples
   - Use `example_vsm_workflow.go` as reference

### Error Codes and Meanings

| Error Code | Description | Solution |
|------------|-------------|----------|
| `ERR_INVALID_PROCESS_PARAMS` | Process parameters are invalid | Check all required parameters are set and positive |
| `ERR_MISSING_CUSTOMER_DATA` | Customer information incomplete | Ensure customer demand and takt time are specified |
| `ERR_CIRCULAR_DEPENDENCY` | Processes have circular references | Review process flow and remove circular dependencies |
| `ERR_SERIALIZATION_FAILED` | Data serialization failed | Enable validation and check data integrity |
| `ERR_CALCULATION_OVERFLOW` | Mathematical calculation overflow | Reduce dataset size or adjust calculation precision |

### Performance Tuning

#### Memory Optimization
```go
// For large value streams, disable unnecessary features
analyzer := vsm.NewAnalyzer()
analyzer.IncludeWasteAnalysis = false // Disable detailed waste analysis

designer := vsm.NewDesigner()
designer.MaxImprovements = 10 // Limit improvement suggestions
```

#### CPU Optimization
```go
// Use concurrent processing for multiple value streams
var wg sync.WaitGroup
results := make(chan *vsm.AnalysisResult, len(valueStreams))

for _, vs := range valueStreams {
    wg.Add(1)
    go func(stream *vsm.ValueStream) {
        defer wg.Done()
        analyzer := vsm.NewAnalyzer()
        result, _ := analyzer.AnalyzeCurrentState(stream)
        results <- result
    }(vs)
}

wg.Wait()
close(results)
```

### Getting Help

If you encounter issues not covered here:

1. Check the `example_vsm_workflow.go` file for complete examples
2. Review the ISO 22468:2020 standard for methodology questions
3. Search existing issues in the project repository
4. Create a new issue with:
   - Go version and operating system
   - Complete error messages
   - Minimal code example reproducing the issue
   - Expected vs. actual behavior

<a name="6-glossary"></a>
## 6. Glossary

| Term | Definition |
|------|------------|
| **Value Stream** | The sequence of activities required to design, produce, and deliver a product or service to a customer |
| **Value Stream Mapping (VSM)** | A lean methodology for analyzing and improving value streams by identifying waste and bottlenecks |
| **Value-Added Time (VAT)** | Time spent on activities that directly increase the value of the product from the customer's perspective |
| **Non-Value-Added Time (NVAT)** | Time spent on activities that do not add value but are currently necessary (e.g., safety procedures) |
| **Necessary Non-Value-Added Time (NNVAT)** | Time spent on activities that do not add value and can be eliminated (pure waste) |
| **Process Time (PT)** | Total time required to complete one cycle of a process, including all components |
| **Cycle Time (CT)** | Time between completion of consecutive units in a process |
| **Lead Time (LT)** | Total time from customer order receipt to product delivery |
| **Takt Time** | Rate at which customers require product delivery (Available Time ÷ Customer Demand) |
| **Bottleneck** | A process that limits the overall capacity of the value stream |
| **Yield Factor (YF)** | Measure of process quality (1 - Scrap Rate - Rework Rate) |
| **Push System** | Production based on forecasts and schedules, regardless of actual customer demand |
| **Pull System** | Production triggered by actual customer demand signals |
| **Kaizen** | Continuous improvement methodology focusing on small, incremental changes |
| **Just-in-Time (JIT)** | Production philosophy of producing only what is needed, when needed |
| **Kanban** | Visual signaling system used in pull systems to control production |
| **Single Minute Exchange of Dies (SMED)** | Methodology for reducing setup/changeover times |
| **Overall Equipment Effectiveness (OEE)** | Measure of manufacturing productivity (Availability × Performance × Quality) |
| **On-Time In-Full (OTIF)** | Delivery performance metric combining timeliness and completeness |
| **Process Efficiency** | Ratio of value-added time to total process time |
| **Flow Efficiency** | Ratio of value-added time to total lead time |
| **Inventory Turns** | Number of times inventory is replenished per period |
| **Work-in-Progress (WIP)** | Inventory between processes in the value stream |
| **First-In-First-Out (FIFO)** | Inventory management method ensuring oldest items are used first |
| **Supermarket** | Controlled inventory point in pull systems |
| **Pacemaker Process** | The process that sets the pace for the entire value stream |
| **Heijunka** | Production leveling to smooth workflow and reduce variability |
| **Jidoka** | Quality control method where machines stop when abnormalities occur |
| **Muda** | Japanese term for waste; any activity that consumes resources but creates no value |
| **Mura** | Japanese term for unevenness or inconsistency in workflow |
| **Muri** | Japanese term for overburdening people or equipment |

<a name="7-standards-compliance"></a>
## 7. Standards Compliance

This SDK is fully compliant with ISO 22468:2020 Value Stream Management standard:

### ISO 22468:2020 Compliance Matrix

| Standard Section | Implementation Status | Description |
|------------------|---------------------|-------------|
| **Core Methodology** | ✅ Fully Implemented | Complete VSM 4-phase methodology (Analysis, Design, Planning, Assessment) |
| **Annex A - Calculations** | ✅ Fully Implemented | All calculation procedures for KPIs, process parameters, and performance metrics |
| **Annex B - Data Boxes** | ✅ Fully Implemented | Standard data box formats for process information and inventory |
| **Annex C - Symbols** | ✅ Fully Implemented | Complete set of VSM symbols for mapping and visualization |
| **7 Types of Waste (Muda)** | ✅ Fully Implemented | Transportation, Inventory, Motion, Waiting, Over-processing, Over-production, Defects |
| **8 Design Principles** | ✅ Fully Implemented | Takt time, Supermarket, Continuous flow, Pull systems, Pacemaker, Levelling, Release, Continuous improvement |

### Rother & Shook Design Principles

The SDK implements all 8 design principles from "Learning to See":

1. **Takt Time**: Calculate and use customer takt time to pace production
2. **Supermarket**: Implement controlled inventory points for pull systems
3. **Continuous Flow**: Design processes for smooth, uninterrupted flow
4. **Pull Systems**: Use demand signals to control production
5. **Pacemaker**: Establish a pacemaker process to control the entire value stream
6. **Levelling (Heijunka)**: Level production mix and volume to reduce variability
7. **Release**: Control work release to prevent overburdening
8. **Continuous Improvement**: Establish mechanisms for ongoing kaizen activities

### Calculation Accuracy

All calculations follow the exact formulas specified in ISO 22468:2020 Annex A:
- Process time components (PRT, ST, HT, WTp, AT)
- Lead time decomposition (VAT, NVAT, NNVAT)
- Performance ratios and efficiency metrics
- Quality and yield calculations
- Inventory and flow metrics

### Validation and Testing

The SDK includes comprehensive validation:
- Parameter range checking
- Formula accuracy verification
- Data integrity validation
- Cross-reference validation
- Standards compliance testing

<a name="8-contributing"></a>
## 8. Contributing

We welcome contributions to the VSM Go SDK! This section outlines the process for contributing to the project.

### Ways to Contribute

- **Bug Reports**: Report issues via GitHub issues
- **Feature Requests**: Suggest new features or improvements
- **Code Contributions**: Submit pull requests with bug fixes or enhancements
- **Documentation**: Improve documentation, examples, or tutorials
- **Testing**: Add test cases or improve test coverage

### Development Setup

1. **Fork and Clone**:
   ```bash
   git clone https://github.com/your-username/iso22468-vsm-go-sdk.git
   cd iso22468-vsm-go-sdk
   ```

2. **Install Dependencies**:
   ```bash
   go mod download
   ```

3. **Run Tests**:
   ```bash
   go test ./...
   ```

4. **Build Examples**:
   ```bash
   go build ./example_vsm_workflow.go
   ```

### Contribution Guidelines

#### Code Standards
- Follow standard Go formatting (`go fmt`)
- Use meaningful variable and function names
- Add comments for complex algorithms
- Include unit tests for new functionality
- Maintain backward compatibility where possible

#### Documentation Standards
- Update README.md for API changes
- Add examples for new features
- Include inline code documentation
- Update this contributing guide as needed

#### Testing Requirements
- All tests must pass: `go test ./...`
- Minimum 80% code coverage for new code
- Include integration tests for complex features
- Test edge cases and error conditions

#### ISO 22468:2020 Compliance
- Maintain compliance with the VSM standard
- Validate calculations against Annex A formulas
- Preserve backward compatibility with existing VSM models
- Document any deviations from the standard

### Pull Request Process

1. **Create a Branch**:
   ```bash
   git checkout -b feature/your-feature-name
   ```

2. **Make Changes**:
   - Implement your feature or fix
   - Add/update tests
   - Update documentation

3. **Run Quality Checks**:
   ```bash
   go vet ./...
   go fmt ./...
   go test ./... -cover
   ```

4. **Commit Changes**:
   ```bash
   git commit -m "Add feature: brief description"
   ```

5. **Push and Create PR**:
   ```bash
   git push origin feature/your-feature-name
   ```
   Then create a pull request on GitHub

### Issue Reporting

When reporting issues, please include:
- Go version (`go version`)
- Operating system and version
- Steps to reproduce the issue
- Expected vs. actual behavior
- Code snippets or example files
- Error messages and stack traces

### Code of Conduct

This project follows a code of conduct to ensure a welcoming environment for all contributors. Please:
- Be respectful and inclusive
- Focus on constructive feedback
- Help newcomers learn and contribute
- Maintain professional communication

<a name="9-license"></a>
## 9. License

This project implements the ISO 22468:2020 Value Stream Management standard and is licensed under the MIT License.

**MIT License**

Copyright (c) 2025 VSM Go SDK Contributors

Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the "Software"), to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

<a name="10-references"></a>
## 10. References

### Primary Standards
- **ISO 22468:2020** - Value stream management (VSM)
  - International Organization for Standardization
  - Geneva, Switzerland

### Key Literature
- **Rother, M. & Shook, J. (1999)** - *Learning to See: Value Stream Mapping to Add Value and Eliminate Muda*
  - Lean Enterprise Institute
  - Brookline, MA, USA

- **Womack, J. P. & Jones, D. T. (1996)** - *Lean Thinking: Banish Waste and Create Wealth in Your Corporation*
  - Free Press
  - New York, NY, USA

- **Rother, M. (2010)** - *Toyota Kata: Managing People for Improvement, Adaptiveness and Superior Results*
  - McGraw-Hill
  - New York, NY, USA

### Related Standards
- **ISO 9001:2015** - Quality management systems
- **ISO 14001:2015** - Environmental management systems
- **ISO 45001:2018** - Occupational health and safety management systems

### Online Resources
- [Lean Enterprise Institute](https://www.lean.org/) - Lean manufacturing resources
- [International Organization for Standardization](https://www.iso.org/) - ISO standards
- [Go Programming Language](https://golang.org/) - Go language documentation

---

**Document History**
- v1.0 (December 2025) - Initial ISO/IEC/IEEE 26514 compliant documentation
