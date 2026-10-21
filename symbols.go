package vsm

// SymbolCategory is a category of Table A.1.
type SymbolCategory string

const (
	CategoryProcesses       SymbolCategory = "processes"
	CategoryProductFlow     SymbolCategory = "product_flow"
	CategoryPlanningControl SymbolCategory = "planning_control"
	CategoryGeneral         SymbolCategory = "general"
)

// Symbol is one value stream symbol of Table A.1. Information summarises the
// table's "additional information / position" column in our own words.
type Symbol struct {
	ID          string         `json:"id"`
	Category    SymbolCategory `json:"category"`
	Term        string         `json:"term"`
	Information string         `json:"information,omitempty"`
}

// Symbols is the catalogue of Table A.1, in the table's order.
var Symbols = []Symbol{
	{"process", CategoryProcesses, "process", "material-, energy- or data-driven process"},
	{"customer", CategoryProcesses, "customer process", "end customer; top right of the map. A customer plant uses the external source symbol"},
	{"supplier", CategoryProcesses, "supplier process / external source", "supplier or customer plant; top left of the map. A supplier that is also a customer uses the customer symbol"},
	{"business_process", CategoryProcesses, "business process, indirect area", "details communication means such as telephone, mail or IT system; optionally number of operators"},
	{"shared_process", CategoryProcesses, "shared process / shared resources", "process symbol with double frame"},
	{"operators", CategoryProcesses, "operators", "number of operators or FTE, inside the process symbol"},
	{"resources", CategoryProcesses, "resources", "machines, area, tools and so on, inside the process symbol"},
	{"data_box", CategoryProcesses, "data box", "selected process parameters for the process type, inside the process symbol"},
	{"repetitive_process", CategoryProcesses, "repetitive process", "number of repetitions, above the process symbol"},
	{"bottleneck", CategoryProcesses, "bottleneck", "above the process symbol"},
	{"push_flow", CategoryProductFlow, "push product flow", "product flow controlled by upstream processes"},
	{"external_flow", CategoryProductFlow, "external product flow", "shipments and external logistics"},
	{"fifo_lifo_lane", CategoryProductFlow, "FIFO or LIFO lane", "process flow control; shows the process quantity"},
	{"jit_jis_delivery", CategoryProductFlow, "just-in-time or just-in-sequence delivery", "process flow control; shows the process quantity"},
	{"stock", CategoryProductFlow, "stock", "inventory triangle; shows number of products or range"},
	{"warehouse", CategoryProductFlow, "warehouse", "organised warehouse or safety stock; shows number of products or range"},
	{"supermarket", CategoryProductFlow, "supermarket", "pull system element; shows number of products or range"},
	{"withdrawal", CategoryProductFlow, "withdrawal", "pull system element"},
	{"truck_transport", CategoryProductFlow, "truck transport", "shows delivery frequency"},
	{"other_transport", CategoryProductFlow, "other transport means", "shows delivery frequency"},
	{"distribution_center", CategoryProductFlow, "distribution center / hub / cross-dock", "external logistics and distribution"},
	{"external_warehouse", CategoryProductFlow, "external warehouse", "external storage and distribution; shows number of products or range"},
	{"milk_run", CategoryProductFlow, "milk run", "delivery concept"},
	{"express_delivery", CategoryProductFlow, "express delivery", "exceptional delivery with shorter delivery time"},
	{"manual_information", CategoryPlanningControl, "manual information flow", "for example list, label or document"},
	{"electronic_information", CategoryPlanningControl, "electronic information flow", "for example telephone, fax, mail or EDI"},
	{"information", CategoryPlanningControl, "information", "detail as data set, document or list"},
	{"synchronization", CategoryPlanningControl, "synchronization", "links main and ancillary routes of branched value streams"},
	{"production_control_ticket", CategoryPlanningControl, "production control-ticket", "control from supermarket to the upstream process; shows release unit"},
	{"withdrawal_control_ticket", CategoryPlanningControl, "withdrawal control-ticket", "control from downstream process to supermarket, shaded; shows release unit"},
	{"batch_control_ticket", CategoryPlanningControl, "control-ticket in batch size", "collection of production control-tickets"},
	{"load_levelling", CategoryPlanningControl, "load levelling", "balancing of the product mix"},
	{"gosee", CategoryPlanningControl, "GoSee production planning", "manual observation of the process flow"},
	{"order_backlog", CategoryPlanningControl, "order backlog", "for business processes, like the inventory triangle for production"},
	{"improvement_flash", CategoryGeneral, "continuous improvement flash", "with description and number, optionally coloured"},
}

// SymbolByID returns the Table A.1 symbol with the given id.
func SymbolByID(id string) (Symbol, bool) {
	for _, s := range Symbols {
		if s.ID == id {
			return s, true
		}
	}
	return Symbol{}, false
}
