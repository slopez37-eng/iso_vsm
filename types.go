// Package vsm implements ISO 22468:2020 Value Stream Management (VSM) standard
package vsm

import (
	"reflect"
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

// ProcessParameters contains all the parameters for a process according to
// Table A.2.
//
// Every numeric field is optional: nil means "not entered", and any value,
// including 0, is a value that was entered (a setup time of 0, a stock of 0,
// a yield of 0 are real measurements). Use F and I to fill them in literals,
// e.g. ProcessParameters{SetupTime: vsm.F(0), ResourceCount: vsm.I(2)}.
// In JSON a field that was not entered is omitted (see serialization.go for
// how files written before format_version 2 are read).
type ProcessParameters struct {
	// Basic time parameters
	ProcessTime              *float64 `json:"process_time,omitempty"`               // PT - Process time
	CycleTime                *float64 `json:"cycle_time,omitempty"`                 // CT - Cycle time
	SetupTime                *float64 `json:"setup_time,omitempty"`                 // ST - Setup time
	Downtime                 *float64 `json:"downtime,omitempty"`                   // DT - Downtime
	IdleTime                 *float64 `json:"idle_time,omitempty"`                  // IT - Idle time
	WaitingTime              *float64 `json:"waiting_time,omitempty"`               // WT - Waiting time
	WaitingTimeProcess       *float64 `json:"waiting_time_process,omitempty"`       // WTp - Waiting time, process
	TransportTime            *float64 `json:"transport_time,omitempty"`             // TT - Transport time
	TransportRepetitionTime  *float64 `json:"transport_repetition_time,omitempty"`  // TRT - Transport repetition time
	TransportRepetitionCount *int     `json:"transport_repetition_count,omitempty"` // #TRep - Number of transport repetitions
	StorageTime              *float64 `json:"storage_time,omitempty"`               // ST_T - Storage time
	HandlingTime             *float64 `json:"handling_time,omitempty"`              // HT - Handling time
	AncillaryTime            *float64 `json:"ancillary_time,omitempty"`             // AT - Ancillary time
	TimeBetweenFailures      *float64 `json:"time_between_failures,omitempty"`      // TBF - Time between failures
	TimeToRepair             *float64 `json:"time_to_repair,omitempty"`             // TTR - Time to repair
	RepetitionTime           *float64 `json:"repetition_time,omitempty"`            // RT - Repetition time
	RepetitionCount          *int     `json:"repetition_count,omitempty"`           // #Rep - Number of repetitions

	// Value adding time parameters
	ValueAddingTime             *float64 `json:"value_adding_time,omitempty"`               // VAT - Value adding time
	NonValueAddingTime          *float64 `json:"non_value_adding_time,omitempty"`           // NVAT - Non-value adding time
	NecessaryNonValueAddingTime *float64 `json:"necessary_non_value_adding_time,omitempty"` // NNVAT - Necessary non-value adding time
	ValueAddingTimeProcess      *float64 `json:"value_adding_time_process,omitempty"`       // VATp - Value adding time, process

	// Quantity parameters
	CustomerDemand *float64 `json:"customer_demand,omitempty"` // CD - Customer demand
	OrderQuantity  *float64 `json:"order_quantity,omitempty"`  // PQ - Process quantity
	StockQuantity  *float64 `json:"stock_quantity,omitempty"`  // SQ - Stock quantity
	BatchSize      *int     `json:"batch_size,omitempty"`      // Batch size

	// Resource parameters
	EmployeeCount *float64 `json:"employee_count,omitempty"` // #EMP - Number of employees (FTE)
	// ResourceCount (#RES) is the number of parallel resources: not
	// entered counts as 1; 0 means no capacity (processing time and
	// utilization unknown, see ResolveParameters).
	ResourceCount *int `json:"resource_count,omitempty"`
	FailureCount  *int `json:"failure_count,omitempty"` // #F - Number of failures

	// Quality parameters
	YieldFactor *float64 `json:"yield_factor,omitempty"` // YF - Yield factor
	ScrapRate   *float64 `json:"scrap_rate,omitempty"`   // SR - Scrap rate
	ReworkRate  *float64 `json:"rework_rate,omitempty"`  // RR - Rework rate

	// Derived parameters (calculated)
	LeadTime         *float64 `json:"lead_time,omitempty"`          // LT - Lead time
	OperatingTime    *float64 `json:"operating_time,omitempty"`     // OT - Operating time
	ProcessingTime   *float64 `json:"processing_time,omitempty"`    // PRT - Processing time
	RangeOfInventory *float64 `json:"range_of_inventory,omitempty"` // Range of inventory

	TimeUnit TimeUnit `json:"time_unit"`
}

// F returns a pointer to v, for filling an optional numeric field that was
// entered (including 0): ProcessParameters{SetupTime: vsm.F(0)}.
func F(v float64) *float64 { return &v }

// I is F for the optional integer fields (counts).
func I(v int) *int { return &v }

// Clone returns a copy of p that shares no pointers with p, so changing a
// value in one (*copy.SetupTime = 3) does not change the other. Results
// that are new states (resolved parameters, designed future and ideal
// states) are returned as clones.
func (p ProcessParameters) Clone() ProcessParameters {
	cloneOptionals(&p)
	return p
}

// Clone returns a copy of c that shares no pointers with c.
func (c Customer) Clone() Customer {
	cloneOptionals(&c)
	return c
}

// Clone returns a copy of inv that shares no pointers with inv.
func (inv Inventory) Clone() Inventory {
	cloneOptionals(&inv)
	return inv
}

// cloneOptionals replaces every non-nil *float64 and *int field of the
// struct v points to with a pointer to a copy of its value.
func cloneOptionals(v any) {
	s := reflect.ValueOf(v).Elem()
	for i := 0; i < s.NumField(); i++ {
		f := s.Field(i)
		if f.Kind() != reflect.Pointer || f.IsNil() || !f.CanSet() {
			continue
		}
		if k := f.Elem().Kind(); k == reflect.Float64 || k == reflect.Int {
			c := reflect.New(f.Elem().Type())
			c.Elem().Set(f.Elem())
			f.Set(c)
		}
	}
}

// num returns *p, or 0 when p is nil (not entered). It is for sums and
// display where a missing value adds nothing; presence checks use p != nil.
func num(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

// positive reports whether p is entered and above 0: the condition for a
// value used as a divisor or rate (a demand of 0 has no rate), not a
// presence check.
func positive(p *float64) bool { return p != nil && *p > 0 }

// Process represents a process in the value stream
type Process struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Type        ProcessType       `json:"type"`
	Parameters  ProcessParameters `json:"parameters"`
	Description string            `json:"description,omitempty"`
	Symbol      string            `json:"symbol,omitempty"` // VSM symbol representation
}

// DemandPeriod is the calendar period that Customer.Demand and
// Customer.AvailableTime both refer to.
type DemandPeriod string

const (
	DemandPeriodDay   DemandPeriod = "day"
	DemandPeriodWeek  DemandPeriod = "week"
	DemandPeriodMonth DemandPeriod = "month"
	DemandPeriodYear  DemandPeriod = "year"
)

// IsValid reports whether p is one of the defined demand periods.
func (p DemandPeriod) IsValid() bool {
	switch p {
	case DemandPeriodDay, DemandPeriodWeek, DemandPeriodMonth, DemandPeriodYear:
		return true
	}
	return false
}

// Customer represents the end customer in the value stream
type Customer struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Demand is the customer demand per DemandPeriod. Optional (nil = not
	// entered); a demand of 0 is entered but gives no demand rate, so
	// nothing that divides by it (takt, inventory time, turns) is computed.
	Demand *float64 `json:"demand,omitempty"`
	// TaktTime is the stored customer takt time (optional). When
	// AvailableTime and Demand are both entered, takt is computed from them
	// instead (see Calculator.CalculateKPIs).
	TaktTime *float64 `json:"takt_time,omitempty"`
	// DemandPeriod is the period Demand and AvailableTime are counted over
	// (day, week, month or year). Optional, but process capacity and
	// utilization are only computed when it is set (see
	// Analyzer.CalculateProcessLoad).
	DemandPeriod DemandPeriod `json:"demand_period,omitempty"`
	// AvailableTime is the available operating time per DemandPeriod (e.g.
	// one shift of 480 for a "day" period in minutes), in the value stream's
	// reference time unit (see Calculator.ReferenceTimeUnit).
	// Optional: when set together with Demand, takt = AvailableTime / Demand,
	// and with DemandPeriod also set, utilization = Demand × cycle time /
	// (AvailableTime × resource count), equivalently cycle time / (takt ×
	// resource count); see Analyzer.CalculateProcessLoad.
	AvailableTime *float64 `json:"available_time,omitempty"`
	// WorkingDaysPerWeek is how many days a week the plant works (1 to 7,
	// fractions allowed, e.g. 5.5). Optional: absent (nil) means 7. It
	// matters only for a "day" DemandPeriod: Demand is then per working day,
	// so stock ÷ daily demand is working days of cover, which the model
	// turns into calendar (elapsed) time as working days × 7 ÷
	// WorkingDaysPerWeek (an average; no calendar dates). A week is always
	// 7 calendar days. See Calculator.CalculateCustomerInventoryTime.
	WorkingDaysPerWeek *float64 `json:"working_days_per_week,omitempty"`
	Description        string   `json:"description,omitempty"`
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
	ID   string `json:"id"`
	Name string `json:"name"`
	// Quantity is the number of products (same unit as Customer.Demand).
	// Optional: nil = not entered; 0 = an empty inventory (no stock, no
	// inventory time).
	Quantity *float64 `json:"quantity,omitempty"`
	// Range is the time the quantity lasts at customer demand, in the
	// reference time unit. Optional (nil = not entered); when the quantity
	// and the customer demand rate are known, the range is computed from
	// them instead and a stored range is only checked. A quantity of 0
	// lasts 0 with or without a demand rate (a stored range other than 0 is
	// then a contradiction).
	Range *float64 `json:"range,omitempty"`
	Type  string   `json:"type"` // "stock", "warehouse", "supermarket", etc.
	// ProcessID (optional) names the process whose stock_quantity is this
	// same stock. The stock is then counted once in the lead time, through
	// that process's waiting time, and quantity must equal the process's
	// stock_quantity (CheckConsistency reports it when not). A process's
	// stock is one inventory: when several inventories link to the same
	// process, only the first (in map order) counts, and the others are
	// reported. Without it, the inventory is a separate stock and its range
	// adds to the lead time.
	// When the process has no waiting time to carry the stock's time (no
	// customer demand rate, and no waiting_time, idle_time or lead_time
	// stored that includes it), the inventory's range is the only measure
	// of it and is counted instead; the stock is still counted once
	// (inventory turns). An empty inventory (quantity or
	// range entered as 0) linked to a process without stock is consistent;
	// one with nothing entered is missing data (its time and stock are
	// unknown, see CheckConsistency). See Calculator.CalculateKPIs.
	ProcessID   string `json:"process_id,omitempty"`
	Description string `json:"description,omitempty"`
}

// FlowElement represents elements that control product flow
type FlowElement struct {
	ID             string   `json:"id"`
	Type           string   `json:"type"`                      // "fifo", "lifo", "kanban", etc.
	Capacity       *float64 `json:"capacity,omitempty"`        // Process quantity or capacity; nil = not sized yet
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

	// ConsistencyIssues lists stored values that disagree with the values
	// computed from their parts (see CheckConsistency). It is output only:
	// DeserializeValueStream, DeserializeVSMProject, DeserializeDesignResult
	// and LoadFromFile recompute it on every load (a stored list is
	// replaced), and calculations ignore it.
	ConsistencyIssues []ConsistencyIssue `json:"consistency_issues,omitempty"`

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
