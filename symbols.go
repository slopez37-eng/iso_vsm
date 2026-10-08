package vsm

// SymbolCategory represents the category of VSM symbols
type SymbolCategory string

const (
	CategoryProcess SymbolCategory = "process"
	CategoryFlow    SymbolCategory = "flow"
	CategoryControl SymbolCategory = "control"
)

// Symbol represents a VSM symbol according to ISO 22468:2020 Table A.1
type Symbol struct {
	Category    SymbolCategory `json:"category"`
	Code        string         `json:"code"`        // Symbol code/representation
	Name        string         `json:"name"`        // Symbol name
	Description string         `json:"description"` // Description of what the symbol represents
	Position    string         `json:"position"`    // Position in value stream diagram
}

// VSMSymbols contains all standard VSM symbols
var VSMSymbols = map[string]Symbol{
	// Process symbols
	"process": {
		Category:    CategoryProcess,
		Code:        "[]",
		Name:        "Process",
		Description: "Material-, energy- or data-driven process",
		Position:    "Main process flow",
	},
	"customer": {
		Category:    CategoryProcess,
		Code:        "@",
		Name:        "Customer",
		Description: "End customer or external source",
		Position:    "Top right",
	},
	"supplier": {
		Category:    CategoryProcess,
		Code:        "A",
		Name:        "Supplier",
		Description: "External source or supplier",
		Position:    "Top left",
	},
	"business_process": {
		Category:    CategoryProcess,
		Code:        "☐",
		Name:        "Business Process",
		Description: "Business process with communication means",
		Position:    "Process flow",
	},
	"shared_process": {
		Category:    CategoryProcess,
		Code:        "[[]]",
		Name:        "Shared Process",
		Description: "Process with shared resources",
		Position:    "Process flow",
	},
	"data_box": {
		Category:    CategoryProcess,
		Code:        "□",
		Name:        "Data Box",
		Description: "Process parameters and data",
		Position:    "Within process symbol",
	},
	"repetitive_process": {
		Category:    CategoryProcess,
		Code:        "[—]",
		Name:        "Repetitive Process",
		Description: "Process with repetitions",
		Position:    "Above process symbol",
	},
	"bottleneck": {
		Category:    CategoryProcess,
		Code:        "X",
		Name:        "Bottleneck",
		Description: "Bottleneck process",
		Position:    "Above process symbol",
	},
	"operator": {
		Category:    CategoryProcess,
		Code:        "👤",
		Name:        "Operator",
		Description: "Number of operators (FTE)",
		Position:    "Within process symbol",
	},
	"resources": {
		Category:    CategoryProcess,
		Code:        "⚙",
		Name:        "Resources",
		Description: "Machines, tools, equipment",
		Position:    "Within process symbol",
	},

	// Flow symbols
	"push_flow": {
		Category:    CategoryFlow,
		Code:        "→",
		Name:        "Push Flow",
		Description: "Product flow controlled by upstream processes",
		Position:    "Between processes",
	},
	"pull_flow": {
		Category:    CategoryFlow,
		Code:        "←",
		Name:        "Pull Flow",
		Description: "Product flow controlled by downstream processes",
		Position:    "Between processes",
	},
	"external_flow": {
		Category:    CategoryFlow,
		Code:        "⇒",
		Name:        "External Product Flow",
		Description: "Shipments, external logistics",
		Position:    "Between external entities",
	},
	"fifo_lane": {
		Category:    CategoryFlow,
		Code:        "□",
		Name:        "FIFO Lane",
		Description: "First-in-first-out flow control",
		Position:    "Between processes",
	},
	"lifo_lane": {
		Category:    CategoryFlow,
		Code:        "▽",
		Name:        "LIFO Lane",
		Description: "Last-in-first-out flow control",
		Position:    "Between processes",
	},
	"jit_delivery": {
		Category:    CategoryFlow,
		Code:        "🚚",
		Name:        "JIT Delivery",
		Description: "Just-in-time delivery",
		Position:    "Between processes",
	},
	"jis_delivery": {
		Category:    CategoryFlow,
		Code:        "🚛",
		Name:        "JIS Delivery",
		Description: "Just-in-sequence delivery",
		Position:    "Between processes",
	},
	"inventory": {
		Category:    CategoryFlow,
		Code:        "△",
		Name:        "Inventory",
		Description: "Stock or inventory triangle",
		Position:    "Between processes",
	},
	"warehouse": {
		Category:    CategoryFlow,
		Code:        "□",
		Name:        "Warehouse",
		Description: "Organized warehouse",
		Position:    "Between processes",
	},
	"supermarket": {
		Category:    CategoryFlow,
		Code:        "▨",
		Name:        "Supermarket",
		Description: "Pull system supermarket",
		Position:    "Between processes",
	},
	"withdrawal": {
		Category:    CategoryFlow,
		Code:        "⇄",
		Name:        "Withdrawal",
		Description: "Product withdrawal from supermarket",
		Position:    "Between processes",
	},
	"truck": {
		Category:    CategoryFlow,
		Code:        "🚛",
		Name:        "Truck Transport",
		Description: "Truck transport",
		Position:    "Between processes",
	},
	"other_transport": {
		Category:    CategoryFlow,
		Code:        "🚐",
		Name:        "Other Transport",
		Description: "Other transport means",
		Position:    "Between processes",
	},
	"milkrun": {
		Category:    CategoryFlow,
		Code:        "🔄",
		Name:        "Milkrun",
		Description: "Milkrun delivery concept",
		Position:    "Between processes",
	},
	"express_delivery": {
		Category:    CategoryFlow,
		Code:        "⚡",
		Name:        "Express Delivery",
		Description: "Exceptional delivery with reduced time",
		Position:    "Between processes",
	},
	"cross_dock": {
		Category:    CategoryFlow,
		Code:        "⊞",
		Name:        "Cross-Dock",
		Description: "Cross-docking warehouse",
		Position:    "Between processes",
	},
	"external_warehouse": {
		Category:    CategoryFlow,
		Code:        "⊟",
		Name:        "External Warehouse",
		Description: "External warehouse",
		Position:    "Between processes",
	},

	// Control symbols
	"manual_info_flow": {
		Category:    CategoryControl,
		Code:        "📄",
		Name:        "Manual Information Flow",
		Description: "Manual information flow (phone, fax, mail)",
		Position:    "Above processes",
	},
	"electronic_info_flow": {
		Category:    CategoryControl,
		Code:        "💻",
		Name:        "Electronic Information Flow",
		Description: "Electronic information flow (EDI, systems)",
		Position:    "Above processes",
	},
	"data_set": {
		Category:    CategoryControl,
		Code:        "📋",
		Name:        "Data Set",
		Description: "Documents, lists, labels",
		Position:    "Above processes",
	},
	"synchronization": {
		Category:    CategoryControl,
		Code:        "🔗",
		Name:        "Synchronization",
		Description: "Synchronization between main and ancillary routes",
		Position:    "Above processes",
	},
	"production_kanban": {
		Category:    CategoryControl,
		Code:        "📊",
		Name:        "Production Kanban",
		Description: "Production control ticket",
		Position:    "Above processes",
	},
	"withdrawal_kanban": {
		Category:    CategoryControl,
		Code:        "📈",
		Name:        "Withdrawal Kanban",
		Description: "Withdrawal control ticket",
		Position:    "Above processes",
	},
	"signal_kanban": {
		Category:    CategoryControl,
		Code:        "📉",
		Name:        "Signal Kanban",
		Description: "Signal kanban for production",
		Position:    "Above processes",
	},
	"load_levelling": {
		Category:    CategoryControl,
		Code:        "⚖",
		Name:        "Load Levelling",
		Description: "Load levelling / production sequencing",
		Position:    "Above processes",
	},
	"gosee_planning": {
		Category:    CategoryControl,
		Code:        "👁",
		Name:        "Go-See Planning",
		Description: "Manual observation of process flow",
		Position:    "Above processes",
	},
	"order_backlog": {
		Category:    CategoryControl,
		Code:        "📚",
		Name:        "Order Backlog",
		Description: "Order backlog inventory",
		Position:    "Above processes",
	},
	"improvement_flash": {
		Category:    CategoryControl,
		Code:        "💡",
		Name:        "Improvement Flash",
		Description: "Continuous improvement flash",
		Position:    "Above processes",
	},
}

// GetSymbol returns the symbol for a given symbol code
func GetSymbol(code string) (Symbol, bool) {
	symbol, exists := VSMSymbols[code]
	return symbol, exists
}

// GetSymbolsByCategory returns all symbols for a given category
func GetSymbolsByCategory(category SymbolCategory) []Symbol {
	var symbols []Symbol
	for _, symbol := range VSMSymbols {
		if symbol.Category == category {
			symbols = append(symbols, symbol)
		}
	}
	return symbols
}

// GetAllSymbols returns all available VSM symbols
func GetAllSymbols() map[string]Symbol {
	return VSMSymbols
}
