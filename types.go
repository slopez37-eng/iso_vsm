// Package vsm implements ISO 22468:2020 Value Stream Management (VSM) standard
package vsm

import (
	"time"
)

// ProcessType represents the type of process according to ISO 22468:2020
type ProcessType string

const (
	ProcessTypeMaterial ProcessType = "material"
	ProcessTypeEnergy   ProcessType = "energy"
	ProcessTypeData     ProcessType = "data"
)

// FlowType represents the type of flow control
type FlowType string

const (
	FlowTypePush FlowType = "push"
	FlowTypePull FlowType = "pull"
)

// ValueStreamPhase represents the phases of VSM
type ValueStreamPhase string

const (
	PhaseAnalysis   ValueStreamPhase = "analysis"
	PhaseDesign     ValueStreamPhase = "design"
	PhasePlanning   ValueStreamPhase = "planning"
	PhaseAssessment ValueStreamPhase = "assessment"
	PhaseAdjustment ValueStreamPhase = "adjustment"
)

// TimeUnit represents different time units used in VSM calculations
type TimeUnit string

const (
	TimeUnitSeconds TimeUnit = "seconds"
	TimeUnitMinutes TimeUnit = "minutes"
	TimeUnitHours   TimeUnit = "hours"
	TimeUnitDays    TimeUnit = "days"
	TimeUnitWeeks   TimeUnit = "weeks"
)

// ProcessParameters contains all the parameters for a process according to Table A.2
type ProcessParameters struct {
	// Basic time parameters
	ProcessTime              float64 `json:"process_time"`               // PT - Process time
	CycleTime                float64 `json:"cycle_time"`                 // CT - Cycle time
	SetupTime                float64 `json:"setup_time"`                 // ST - Setup time
	Downtime                 float64 `json:"downtime"`                   // DT - Downtime
	IdleTime                 float64 `json:"idle_time"`                  // IT - Idle time
	WaitingTime              float64 `json:"waiting_time"`               // WT - Waiting time
	WaitingTimeProcess       float64 `json:"waiting_time_process"`       // WTp - Waiting time, process
	TransportTime            float64 `json:"transport_time"`             // TT - Transport time
	TransportRepetitionTime  float64 `json:"transport_repetition_time"`  // TRT - Transport repetition time
	TransportRepetitionCount int     `json:"transport_repetition_count"` // #TRep - Number of transport repetitions
	StorageTime              float64 `json:"storage_time"`               // ST_T - Storage time
	HandlingTime             float64 `json:"handling_time"`              // HT - Handling time
	AncillaryTime            float64 `json:"ancillary_time"`             // AT - Ancillary time
	TimeBetweenFailures      float64 `json:"time_between_failures"`      // TBF - Time between failures
	TimeToRepair             float64 `json:"time_to_repair"`             // TTR - Time to repair
	RepetitionTime           float64 `json:"repetition_time"`            // RT - Repetition time
	RepetitionCount          int     `json:"repetition_count"`           // #Rep - Number of repetitions

	// Value adding time parameters
	ValueAddingTime             float64 `json:"value_adding_time"`               // VAT - Value adding time
	NonValueAddingTime          float64 `json:"non_value_adding_time"`           // NVAT - Non-value adding time
	NecessaryNonValueAddingTime float64 `json:"necessary_non_value_adding_time"` // NNVAT - Necessary non-value adding time
	ValueAddingTimeProcess      float64 `json:"value_adding_time_process"`       // VATp - Value adding time, process

	// Quantity parameters
	CustomerDemand float64 `json:"customer_demand"` // CD - Customer demand
	OrderQuantity  float64 `json:"order_quantity"`  // PQ - Process quantity
	StockQuantity  float64 `json:"stock_quantity"`  // SQ - Stock quantity
	BatchSize      int     `json:"batch_size"`      // Batch size

	// Resource parameters
	EmployeeCount float64 `json:"employee_count"` // #EMP - Number of employees (FTE)
	ResourceCount int     `json:"resource_count"` // #RES - Number of resources
	FailureCount  int     `json:"failure_count"`  // #F - Number of failures

	// Quality parameters
	YieldFactor float64 `json:"yield_factor"` // YF - Yield factor
	ScrapRate   float64 `json:"scrap_rate"`   // SR - Scrap rate
	ReworkRate  float64 `json:"rework_rate"`  // RR - Rework rate

	// Derived parameters (calculated)
	LeadTime         float64 `json:"lead_time"`          // LT - Lead time
	OperatingTime    float64 `json:"operating_time"`     // OT - Operating time
	ProcessingTime   float64 `json:"processing_time"`    // PRT - Processing time
	RangeOfInventory float64 `json:"range_of_inventory"` // Range of inventory

	TimeUnit TimeUnit `json:"time_unit"`
}

// Process represents a process in the value stream
type Process struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Type        ProcessType       `json:"type"`
	Parameters  ProcessParameters `json:"parameters"`
	Description string            `json:"description,omitempty"`
	Symbol      string            `json:"symbol,omitempty"` // VSM symbol representation
}

// Customer represents the end customer in the value stream
type Customer struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Demand      float64 `json:"demand"`    // Customer demand per period
	TaktTime    float64 `json:"takt_time"` // Customer takt time
	Description string  `json:"description,omitempty"`
}

// Supplier represents a supplier in the value stream
type Supplier struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	DeliveryTime float64 `json:"delivery_time"` // Delivery time
	Description  string  `json:"description,omitempty"`
}

// Inventory represents inventory/stock in the value stream
type Inventory struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Quantity    float64 `json:"quantity"` // Number of products or range
	Range       float64 `json:"range"`    // Time period for inventory
	Type        string  `json:"type"`     // "stock", "warehouse", "supermarket", etc.
	Description string  `json:"description,omitempty"`
}

// FlowElement represents elements that control product flow
type FlowElement struct {
	ID             string   `json:"id"`
	Type           string   `json:"type"`                      // "fifo", "lifo", "kanban", etc.
	Capacity       float64  `json:"capacity"`                  // Process quantity or capacity
	ControlTickets []string `json:"control_tickets,omitempty"` // Kanban cards, etc.
	Description    string   `json:"description,omitempty"`
}

// ValueStreamState represents different states of the value stream
type ValueStreamState string

const (
	StateCurrent ValueStreamState = "current"
	StateFuture  ValueStreamState = "future"
	StateIdeal   ValueStreamState = "ideal"
)

// ValueStream represents the complete value stream
type ValueStream struct {
	ID            string           `json:"id"`
	Name          string           `json:"name"`
	State         ValueStreamState `json:"state"`
	ProductFamily string           `json:"product_family"`
	Description   string           `json:"description,omitempty"`

	// Core elements
	Customer  *Customer   `json:"customer,omitempty"`
	Supplier  *Supplier   `json:"supplier,omitempty"`
	Processes []Process   `json:"processes"`
	Inventory []Inventory `json:"inventory,omitempty"`

	// Flow control elements
	FlowElements []FlowElement `json:"flow_elements,omitempty"`

	// Analysis results
	KPIs map[string]float64 `json:"kpis,omitempty"` // Key Performance Indicators

	// Metadata
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Version   string    `json:"version"`
}

// ImprovementMeasure represents a measure for improvement in the catalogue
type ImprovementMeasure struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Responsible string    `json:"responsible"`
	Priority    int       `json:"priority"` // 1=high, 2=medium, 3=low
	Status      string    `json:"status"`   // "planned", "in_progress", "completed", "cancelled"
	SMART       SMARTGoal `json:"smart"`    // Specific, Measurable, Accepted, Realistic, Time-related
	Costs       Costs     `json:"costs,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// SMARTGoal represents SMART criteria for improvement measures
type SMARTGoal struct {
	Specific   string    `json:"specific"`
	Measurable string    `json:"measurable"`
	Accepted   string    `json:"accepted"`
	Realistic  string    `json:"realistic"`
	TimeBound  time.Time `json:"time_bound"`
}

// Costs represents cost information for improvement measures
type Costs struct {
	Investment    float64 `json:"investment"`
	Operational   float64 `json:"operational"`
	Benefits      float64 `json:"benefits"`
	PaybackPeriod float64 `json:"payback_period"`
}

// CatalogueOfMeasures contains all improvement measures
type CatalogueOfMeasures struct {
	ID            string               `json:"id"`
	ValueStreamID string               `json:"value_stream_id"`
	Measures      []ImprovementMeasure `json:"measures"`
	CreatedAt     time.Time            `json:"created_at"`
	UpdatedAt     time.Time            `json:"updated_at"`
}

// VSMProject represents a complete VSM project
type VSMProject struct {
	ID                  string               `json:"id"`
	Name                string               `json:"name"`
	Description         string               `json:"description"`
	CurrentState        ValueStream          `json:"current_state"`
	FutureState         *ValueStream         `json:"future_state,omitempty"`
	IdealState          *ValueStream         `json:"ideal_state,omitempty"`
	CatalogueOfMeasures *CatalogueOfMeasures `json:"catalogue_of_measures,omitempty"`
	CurrentPhase        ValueStreamPhase     `json:"current_phase"`
	Status              string               `json:"status"`
	CreatedAt           time.Time            `json:"created_at"`
	UpdatedAt           time.Time            `json:"updated_at"`
}
