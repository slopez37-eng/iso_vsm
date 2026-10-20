package vsm

import (
	"fmt"
	"strings"
)

// KPIDirection says which way a KPI has to move to count as better.
type KPIDirection string

const (
	HigherIsBetter   KPIDirection = "higher_is_better"
	LowerIsBetter    KPIDirection = "lower_is_better"
	DirectionUnknown KPIDirection = "unknown"
)

// kpiDirections is the built-in direction table. Keys are normalised with
// normalizeKPIName. Names not listed here are DirectionUnknown: the library
// does not guess.
var kpiDirections = map[string]KPIDirection{
	// Lower is better: times, inventory, losses.
	"lead_time":                LowerIsBetter,
	"total_lead_time":          LowerIsBetter,
	"process_time":             LowerIsBetter,
	"total_process_time":       LowerIsBetter,
	"idle_time":                LowerIsBetter,
	"total_idle_time":          LowerIsBetter,
	"waiting_time":             LowerIsBetter,
	"total_waiting_time":       LowerIsBetter,
	"transport_time":           LowerIsBetter,
	"storage_time":             LowerIsBetter,
	"setup_time":               LowerIsBetter,
	"changeover_time":          LowerIsBetter,
	"downtime":                 LowerIsBetter,
	"inventory":                LowerIsBetter,
	"total_inventory":          LowerIsBetter,
	"stock":                    LowerIsBetter,
	"stock_quantity":           LowerIsBetter,
	"range_of_inventory":       LowerIsBetter,
	"inventory_range":          LowerIsBetter,
	"wip":                      LowerIsBetter,
	"work_in_process":          LowerIsBetter,
	"work_in_progress":         LowerIsBetter,
	"scrap_rate":               LowerIsBetter,
	"rework_rate":              LowerIsBetter,
	"defect_rate":              LowerIsBetter,
	"non_value_adding_time":    LowerIsBetter,
	"total_non_value_add_time": LowerIsBetter,

	// Higher is better: efficiency, delivery, quality, turns.
	"value_added_ratio":    HigherIsBetter,
	"flow_efficiency":      HigherIsBetter,
	"process_efficiency":   HigherIsBetter,
	"otif":                 HigherIsBetter,
	"on_time_in_full":      HigherIsBetter,
	"availability":         HigherIsBetter,
	"oee":                  HigherIsBetter,
	"yield":                HigherIsBetter,
	"yield_factor":         HigherIsBetter,
	"average_yield_factor": HigherIsBetter,
	"first_pass_yield":     HigherIsBetter,
	"fpy":                  HigherIsBetter,
	"inventory_turns":      HigherIsBetter,
}

// kpiNameReplacer turns the spaces and hyphens of a KPI name into underscores.
var kpiNameReplacer = strings.NewReplacer(" ", "_", "-", "_")

// normalizeKPIName lower-cases a KPI name and turns spaces and hyphens into
// underscores, so "Lead Time" and "lead-time" match "lead_time".
func normalizeKPIName(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	return kpiNameReplacer.Replace(n)
}

// GetKPIDirection returns whether a higher or lower value of the named KPI
// is better, from the built-in table. Unlisted KPIs (for example
// customer_takt or total_value_adding_time, which have no better direction
// on their own) return DirectionUnknown.
func GetKPIDirection(name string) KPIDirection {
	if d, ok := kpiDirections[normalizeKPIName(name)]; ok {
		return d
	}
	return DirectionUnknown
}

// kpiUnits labels the KPIs CalculateKPIs produces. Ratios are stored as
// fractions (0.25 means 25%) and labelled "ratio"; they are converted to
// percent only when formatted for people (see formatKPIValue).
var kpiUnits = map[string]string{
	"customer_takt":           "time",
	"total_lead_time":         "time",
	"total_value_adding_time": "time",
	"value_added_ratio":       "ratio",
	"process_efficiency":      "ratio",
	"inventory_turns":         "turns",
	"average_yield_factor":    "ratio",
}

// kpiUnit returns the unit label for a KPI, or "unit" when unknown.
func kpiUnit(name string) string {
	if unit, ok := kpiUnits[name]; ok {
		return unit
	}
	return "unit"
}

// kpiUnitIn is kpiUnit for an assessment with time unit unit: time KPIs take
// the assessment's unit when it has one.
func kpiUnitIn(name string, unit TimeUnit) string {
	if u := kpiUnit(name); u != "time" || unit == "" {
		return u
	}
	return string(unit)
}

// kpiDescriptions describes the KPIs CalculateKPIs produces.
var kpiDescriptions = map[string]string{
	"customer_takt":           "Time interval corresponding to customer demand",
	"total_lead_time":         "Total time from order to delivery",
	"total_value_adding_time": "Total time adding value to product",
	"value_added_ratio":       "Ratio of value-adding time to total lead time",
	"process_efficiency":      "Efficiency of process operations",
	"inventory_turns":         "Number of times inventory turns over",
	"average_yield_factor":    "Average yield factor across processes",
}

// kpiDescription returns the description of a KPI, or a generic one when
// unknown.
func kpiDescription(name string) string {
	if desc, ok := kpiDescriptions[name]; ok {
		return desc
	}
	return "Key performance indicator"
}

// formatKPIValue formats a KPI value for human-readable text: ratio KPIs
// (stored as fractions) are shown as percent, everything else as a number.
func formatKPIValue(name string, value float64) string {
	if kpiUnit(name) == "ratio" {
		return fmt.Sprintf("%.1f%%", value*100)
	}
	return fmt.Sprintf("%.4f", value)
}
