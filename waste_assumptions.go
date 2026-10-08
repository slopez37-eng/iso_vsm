package vsm

import "fmt"

// WasteAssumptions are the user's own estimates that split measured times
// into waste (ValueStream.WasteAssumptions). The model does not guess them:
// a share that is not entered leaves the waste it splits not estimated
// (nil or left out of the result, with a note naming the field to enter).
//
// Every field is a fraction between 0 and 1. Optional (nil = not entered);
// 0 is an entered value. A value outside 0–1 is reported by
// CheckConsistency (kind "contradiction") and treated as not entered.
type WasteAssumptions struct {
	// InventoryShareOfWaiting is the share of waiting time (process waiting
	// time and the time of inventories counted on their own) that is
	// inventory waste; the rest (1 − share) is waiting waste.
	InventoryShareOfWaiting *float64 `json:"inventory_share_of_waiting,omitempty"`
	// WasteShareOfHandling is the share of handling time that is motion
	// waste.
	WasteShareOfHandling *float64 `json:"waste_share_of_handling,omitempty"`
	// ReductionPotential is the share of the total waste that the user
	// expects can be removed (WasteAnalysis.WasteReductionPotential).
	ReductionPotential *float64 `json:"reduction_potential,omitempty"`
}

// Rules for the user-entered assumptions (ConsistencyIssue.Rule).
const (
	ruleAssumptionFraction = "waste assumptions are fractions between 0 and 1 (not entered: the waste they split is not estimated)"
	ruleInfoFlowQuality    = "information_flow_quality is a rating between 0 and 1 (not entered: not estimated)"
)

// Field names of the assumptions (JSON), as used in notes and issues.
const (
	fieldInventoryShare     = "waste_assumptions.inventory_share_of_waiting"
	fieldHandlingShare      = "waste_assumptions.waste_share_of_handling"
	fieldReductionPotential = "waste_assumptions.reduction_potential"
	fieldInfoFlowQuality    = "information_flow_quality"
)

// validFraction returns *p when it is entered and within 0–1, and false
// otherwise (not entered, or out of range: reported by CheckConsistency).
func validFraction(p *float64) (float64, bool) {
	if p == nil || outOfBounds(*p, true) {
		return 0, false
	}
	return *p, true
}

// assumption returns the field of vs's waste assumptions that get selects,
// when entered and valid.
func assumption(vs *ValueStream, get func(*WasteAssumptions) *float64) (float64, bool) {
	if vs == nil || vs.WasteAssumptions == nil {
		return 0, false
	}
	return validFraction(get(vs.WasteAssumptions))
}

// assumptionIssues reports user-entered fractions of vs that are outside
// 0–1 (kind IssueContradiction is set by CheckConsistency).
func assumptionIssues(vs *ValueStream) []ConsistencyIssue {
	type entry struct {
		field, rule string
		value       *float64
	}
	var entries []entry
	if wa := vs.WasteAssumptions; wa != nil {
		entries = append(entries,
			entry{fieldInventoryShare, ruleAssumptionFraction, wa.InventoryShareOfWaiting},
			entry{fieldHandlingShare, ruleAssumptionFraction, wa.WasteShareOfHandling},
			entry{fieldReductionPotential, ruleAssumptionFraction, wa.ReductionPotential})
	}
	entries = append(entries, entry{fieldInfoFlowQuality, ruleInfoFlowQuality, vs.InformationFlowQuality})

	var issues []ConsistencyIssue
	for _, e := range entries {
		if e.value == nil || !outOfBounds(*e.value, true) {
			continue
		}
		issues = append(issues, ConsistencyIssue{
			Element: "value_stream", ID: vs.ID, Field: e.field, StoredValue: *e.value, ComputedValue: 1,
			Rule: e.rule,
			Message: fmt.Sprintf("value stream: %s %g is outside 0–1; it is treated as not entered",
				e.field, *e.value),
		})
	}
	return issues
}
