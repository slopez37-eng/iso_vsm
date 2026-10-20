package vsm

// SchemaVersion is the version of the JSON format written by this package.
const SchemaVersion = 1

// F returns a pointer to v. Optional numbers in the model are pointers: nil
// means "not entered", while a pointer to 0 is an entered zero.
func F(v float64) *float64 { return &v }

// State says which state of the value stream a map describes (clauses 4.2.4,
// 4.3.3 and 4.3.4).
type State string

const (
	StateCurrent State = "current"
	StateIdeal   State = "ideal"
	StateFuture  State = "future"
)

// ProcessType is one of the three cases of Figure A.1.
type ProcessType string

const (
	ProcessManual    ProcessType = "manual"    // case A: manual, material- or energy-related
	ProcessAutomated ProcessType = "automated" // case B: automated, material- or energy-related
	ProcessData      ProcessType = "data"      // case C: data-related
)

// ValueStream is one value stream map of a product family.
//
// All time values in the stream, including the customer's operating time and
// period length, are in TimeUnit. Rates (scrap, yield, rework) are fractions.
type ValueStream struct {
	SchemaVersion int      `json:"schema_version"`
	Name          string   `json:"name"`
	State         State    `json:"state"`
	TimeUnit      TimeUnit `json:"time_unit"`
	Description   string   `json:"description,omitempty"`

	ProductFamily *ProductFamily `json:"product_family,omitempty"`
	Customer      Customer       `json:"customer"`
	Suppliers     []Supplier     `json:"suppliers,omitempty"`

	// Processes are listed in flow order, from the supplier to the customer.
	Processes []Process `json:"processes"`
	// Idles are the idle segments between processes (storage, transport and
	// waiting, A.2).
	Idles []Idle `json:"idles,omitempty"`

	InformationFlows []InformationFlow `json:"information_flows,omitempty"`
	Flashes          []Flash           `json:"improvement_flashes,omitempty"`
	Guidelines       []GuidelineStatus `json:"design_guidelines,omitempty"`

	// Stream-level counts for the key performance indicators of A.3.3.
	Deliveries *Deliveries `json:"deliveries,omitempty"`
	Quality    *Quality    `json:"quality,omitempty"`
	Inventory  *Turnover   `json:"inventory_turnover,omitempty"`
}

// ProductFamily records the selected product family (clause 4.2.2). The
// selection itself is a judgement the standard leaves to the team.
type ProductFamily struct {
	Name      string   `json:"name"`
	Variants  []string `json:"variants,omitempty"`
	Rationale string   `json:"rationale,omitempty"`
}

// Customer is the end customer and the review period that demand and
// operating time refer to.
type Customer struct {
	Name string `json:"name"`
	// Period names the period under review, for example "year" or "day".
	Period string `json:"period,omitempty"`
	// Demand is the customer demand per period (CD).
	Demand *float64 `json:"demand,omitempty"`
	// OperatingTime is the operating time per period (OT), in the stream's
	// time unit.
	OperatingTime *float64 `json:"operating_time,omitempty"`
	// PeriodLength is the calendar length of the period, in the stream's time
	// unit. It converts a range of inventory (A.7, in periods) to time.
	PeriodLength *float64 `json:"period_length,omitempty"`
}

// Supplier is an external source (Table A.1).
type Supplier struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// DeliveryFrequency is free text such as "2x per week".
	DeliveryFrequency string `json:"delivery_frequency,omitempty"`
}

// Process is one process step with its data box parameters (Table A.2).
// Every number is optional; leave it nil when it was not collected.
type Process struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Type      ProcessType `json:"type,omitempty"`
	Shared    bool        `json:"shared,omitempty"`    // shared process / shared resources
	Pacemaker bool        `json:"pacemaker,omitempty"` // designated pacemaker process (3.7)

	Operators *float64 `json:"operators,omitempty"` // #EMP, in FTE
	Resources *float64 `json:"resources,omitempty"` // #RES

	ProcessTime    *float64  `json:"process_time,omitempty"`    // PT
	CycleTime      *float64  `json:"cycle_time,omitempty"`      // CT, observed interval between products leaving the whole process
	SetupTime      *float64  `json:"setup_time,omitempty"`      // ST
	ProcessingTime *float64  `json:"processing_time,omitempty"` // PRT
	HandlingTime   *float64  `json:"handling_time,omitempty"`   // HT
	WaitingTime    *float64  `json:"waiting_time,omitempty"`    // WTp, waiting within the process
	Downtime       *float64  `json:"downtime,omitempty"`        // DT
	TimesToRepair  []float64 `json:"times_to_repair,omitempty"` // TTR of each failure
	AncillaryTime  *float64  `json:"ancillary_time,omitempty"`  // AT, parallel to PT and not part of it
	TransportTime  *float64  `json:"transport_time,omitempty"`  // TTp, transport within the process

	RepetitionTime  *float64 `json:"repetition_time,omitempty"`  // RT
	Repetitions     *float64 `json:"repetitions,omitempty"`      // #Rep.
	ProcessQuantity *float64 `json:"process_quantity,omitempty"` // PQ

	TimeBetweenFailures *float64 `json:"time_between_failures,omitempty"` // TBF
	Failures            *float64 `json:"failures,omitempty"`              // #F per period

	BatchSize *float64 `json:"batch_size,omitempty"`
	Variants  *float64 `json:"variants,omitempty"`

	ScrapRate   *float64 `json:"scrap_rate,omitempty"`   // SR
	YieldFactor *float64 `json:"yield_factor,omitempty"` // YF
	ReworkRate  *float64 `json:"rework_rate,omitempty"`  // RR

	ValueShares
}

// ValueShares splits a process or idle time into value adding (VAT),
// non-value adding (NVAT) and necessary non-value adding (NNVAT) time.
type ValueShares struct {
	ValueAdding             *float64 `json:"value_adding_time,omitempty"`
	NonValueAdding          *float64 `json:"non_value_adding_time,omitempty"`
	NecessaryNonValueAdding *float64 `json:"necessary_non_value_adding_time,omitempty"`
}

// Idle is the idle time between two points of the flow: storage, transport
// and waiting (A.2). From and To name processes or suppliers; leave To empty
// for the segment that ends at the customer.
type Idle struct {
	ID   string `json:"id"`
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`

	IdleTime *float64 `json:"idle_time,omitempty"` // IT

	TransportTime           *float64 `json:"transport_time,omitempty"`            // TT
	TransportRepetitions    *float64 `json:"transport_repetitions,omitempty"`     // #TRep.
	TransportRepetitionTime *float64 `json:"transport_repetition_time,omitempty"` // TRT

	WaitingTime   *float64 `json:"waiting_time,omitempty"`   // WT
	WaitingOrders *float64 `json:"waiting_orders,omitempty"` // SQ as waiting orders, for A.11

	StorageTime *float64    `json:"storage_time,omitempty"` // STT
	Inventories []Inventory `json:"inventories,omitempty"`

	// Idle time adds no value from the customer's view (clause 4.5.2), so
	// only the non-value adding split is recorded.
	NonValueAdding          *float64 `json:"non_value_adding_time,omitempty"`
	NecessaryNonValueAdding *float64 `json:"necessary_non_value_adding_time,omitempty"`
}

// InventoryKind is the Table A.1 symbol an inventory point is drawn with.
type InventoryKind string

const (
	InventoryStock             InventoryKind = "stock"
	InventoryWarehouse         InventoryKind = "warehouse"
	InventorySupermarket       InventoryKind = "supermarket"
	InventoryExternalWarehouse InventoryKind = "external_warehouse"
	InventoryFIFOLane          InventoryKind = "fifo_lane"
	InventoryLIFOLane          InventoryKind = "lifo_lane"
	InventoryOrderBacklog      InventoryKind = "order_backlog"
)

// Inventory is a stock or warehouse in an idle segment. Its range (R) counts
// towards storage time (A.9).
type Inventory struct {
	ID       string        `json:"id,omitempty"`
	Kind     InventoryKind `json:"kind"`
	Quantity *float64      `json:"quantity,omitempty"` // SQ, pieces
	Range    *float64      `json:"range,omitempty"`    // R, in the stream's time unit
}

// FlowKind is a Table A.1 information flow symbol.
type FlowKind string

const (
	FlowManual           FlowKind = "manual"
	FlowElectronic       FlowKind = "electronic"
	FlowProductionTicket FlowKind = "production_control_ticket"
	FlowWithdrawalTicket FlowKind = "withdrawal_control_ticket"
	FlowGoSee            FlowKind = "gosee"
)

// InformationFlow links two elements of the map (customer, supplier,
// process or planning).
type InformationFlow struct {
	From        string   `json:"from"`
	To          string   `json:"to"`
	Kind        FlowKind `json:"kind"`
	Description string   `json:"description,omitempty"`
}

// Waste is one of the seven types of waste that clause 4.3.2 refers to.
type Waste string

const (
	WasteOverproduction Waste = "overproduction"
	WasteWaiting        Waste = "waiting"
	WasteTransport      Waste = "transport"
	WasteOverprocessing Waste = "overprocessing"
	WasteInventory      Waste = "inventory"
	WasteMotion         Waste = "motion"
	WasteDefects        Waste = "defects"
)

// Wastes lists the seven types of waste.
var Wastes = []Waste{WasteOverproduction, WasteWaiting, WasteTransport, WasteOverprocessing, WasteInventory, WasteMotion, WasteDefects}

// Flash is a continuous improvement flash: an improvement potential drawn on
// the map (clause 4.3.2).
type Flash struct {
	Number      int     `json:"number"`
	Target      string  `json:"target,omitempty"` // ID of the process or idle it sits on
	Description string  `json:"description"`
	Wastes      []Waste `json:"wastes,omitempty"`
}

// Deliveries counts deliveries to the end customer for OTIF (A.14).
type Deliveries struct {
	Total          float64 `json:"total"`
	CompleteOnTime float64 `json:"complete_on_time"`
}

// Quality counts products of the stream for scrap rate and yield factor.
type Quality struct {
	Total     float64 `json:"total"`
	Defective float64 `json:"defective"`
}

// Turnover holds usage and average stock per period for inventory turns
// (A.13).
type Turnover struct {
	Usage        float64 `json:"usage"`
	AverageStock float64 `json:"average_stock"`
}

// Guideline is one of the eight design guidelines of clause 4.3.4.
type Guideline string

const (
	GuidelineTaktAtPacemaker    Guideline = "takt_at_pacemaker"
	GuidelineSupermarketOrShip  Guideline = "supermarket_or_direct_shipping"
	GuidelineContinuousFlow     Guideline = "continuous_flow"
	GuidelineSupermarketPull    Guideline = "supermarket_pull"
	GuidelinePacemaker          Guideline = "define_pacemaker"
	GuidelineLevelMix           Guideline = "level_product_mix"
	GuidelineReleaseAtPacemaker Guideline = "release_at_pacemaker"
	GuidelineCIP                Guideline = "continuous_improvement"
)

// Guidelines lists the eight design guidelines in the standard's order.
var Guidelines = []Guideline{
	GuidelineTaktAtPacemaker, GuidelineSupermarketOrShip, GuidelineContinuousFlow, GuidelineSupermarketPull,
	GuidelinePacemaker, GuidelineLevelMix, GuidelineReleaseAtPacemaker, GuidelineCIP,
}

// GuidelineStatus records how a future state addresses one design guideline.
// Whether a guideline is met is the team's judgement, not a calculation.
type GuidelineStatus struct {
	Guideline Guideline `json:"guideline"`
	Addressed bool      `json:"addressed"`
	Note      string    `json:"note,omitempty"`
}
