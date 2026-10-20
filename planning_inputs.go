package vsm

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"time"
)

// User-entered planning inputs. The library does not estimate money,
// priorities, dates or risk scores: each comes from these inputs, and what
// is not entered is reported as not estimated (nil, with a note), never
// filled with a default.

// Rating scale for impact and effort: whole numbers from RatingMin
// (lowest) to RatingMax (highest).
const (
	RatingMin = 1
	RatingMax = 5
)

// MeasureInput is what the user entered for one improvement potential,
// continuous improvement or catalogue measure, keyed by its ID (see
// ImprovementPotential.ID and ContinuousImprovement.ID; the catalogue
// measure has the same ID). Every field is optional.
type MeasureInput struct {
	// ImpactRating and EffortRating are on the 1 (lowest) to 5 (highest)
	// scale. With both entered the measure gets a priority (see
	// RatingPriority); otherwise it is not prioritised.
	ImpactRating *int `json:"impact_rating,omitempty"`
	EffortRating *int `json:"effort_rating,omitempty"`
	// Investment is the one-off cost of the measure, in currency units, 0
	// or more.
	Investment *float64 `json:"investment,omitempty"`
	// AnnualBenefit is the saving or extra income per year once the measure
	// is in place, in currency units, 0 or more.
	AnnualBenefit *float64 `json:"annual_benefit,omitempty"`
	// AnnualOperatingCost is the running cost per year the measure adds, in
	// currency units, 0 or more (enter 0 when there is none: not entered is
	// unknown, and the payback is then not estimated).
	AnnualOperatingCost *float64 `json:"annual_operating_cost,omitempty"`
	// DueDate is when the measure is to be done (SMART "time-bound").
	DueDate *time.Time `json:"due_date,omitempty"`
	// Responsible is the person or role who owns the measure.
	Responsible string `json:"responsible,omitempty"`
}

// RiskInput is the user's assessment of one risk of the implementation
// plan (keyed by Risk.ID): probability and impact as fractions 0 to 1.
type RiskInput struct {
	Probability *float64 `json:"probability,omitempty"`
	Impact      *float64 `json:"impact,omitempty"`
}

// PlanningInputs are the user-entered values the planner uses (see
// Planner.PlanImplementationWithInputs). All optional.
type PlanningInputs struct {
	// Measures holds the ratings, money, due date and owner per measure ID.
	Measures map[string]MeasureInput `json:"measures,omitempty"`
	// StartDate is when implementation starts (first phase start).
	StartDate *time.Time `json:"start_date,omitempty"`
	// WorkshopDate is when the value stream workshop takes place.
	WorkshopDate *time.Time `json:"workshop_date,omitempty"`
	// Risks holds the probability and impact per risk ID (see
	// Planner.AssessRisks for the IDs of the risk checklist).
	Risks map[string]RiskInput `json:"risks,omitempty"`
}

// RatingPriority returns the priority of a measure from its entered impact
// and effort ratings: 1 (high) when impact is rated above effort, 2
// (medium) when they are equal, 3 (low) when effort is rated above impact.
// ok is false (priority 0, not prioritised) when either rating is not
// entered or is outside RatingMin–RatingMax.
func RatingPriority(impact, effort *int) (priority int, ok bool) {
	if !ratingValid(impact) || !ratingValid(effort) {
		return 0, false
	}
	switch {
	case *impact > *effort:
		return 1, true
	case *impact == *effort:
		return 2, true
	default:
		return 3, true
	}
}

func ratingValid(r *int) bool { return r != nil && *r >= RatingMin && *r <= RatingMax }

// rate returns the ratings, priority and priority note from in, or from
// the fallback ratings (entered earlier, e.g. on a design result) when in
// has neither rating. A fallback outside 1–5 (from a hand-edited file)
// counts as not entered.
func rate(in MeasureInput, fallbackImpact, fallbackEffort *int) (impact, effort *int, priority int, note string) {
	impact, effort = in.ImpactRating, in.EffortRating
	if impact == nil && effort == nil {
		impact, effort = fallbackImpact, fallbackEffort
	}
	impact, effort = clonePtr(impact), clonePtr(effort)
	if !ratingValid(impact) {
		impact = nil
	}
	if !ratingValid(effort) {
		effort = nil
	}
	if p, ok := RatingPriority(impact, effort); ok {
		return impact, effort, p, ""
	}
	var missing []string
	if impact == nil {
		missing = append(missing, "impact")
	}
	if effort == nil {
		missing = append(missing, "effort")
	}
	return impact, effort, 0, "Not prioritised: " + strings.Join(missing, " and ") + " rating not entered"
}

// validateMeasureInputs returns an error for an input outside its range
// (rating not 1–5, negative or non-finite money) or keyed by an ID not in
// known (what names the IDs, e.g. "improvement potential or continuous
// improvement").
func validateMeasureInputs(inputs map[string]MeasureInput, known map[string]bool, what string) error {
	for _, id := range slices.Sorted(maps.Keys(inputs)) {
		in := inputs[id]
		if !known[id] {
			return fmt.Errorf("measure input %q: no %s has this ID", id, what)
		}
		for _, r := range []struct {
			name string
			v    *int
		}{{"impact_rating", in.ImpactRating}, {"effort_rating", in.EffortRating}} {
			if r.v != nil && !ratingValid(r.v) {
				return fmt.Errorf("measure input %q: %s %d must be a whole number from %d to %d",
					id, r.name, *r.v, RatingMin, RatingMax)
			}
		}
		for _, m := range []struct {
			name string
			v    *float64
		}{{"investment", in.Investment}, {"annual_benefit", in.AnnualBenefit}, {"annual_operating_cost", in.AnnualOperatingCost}} {
			if m.v != nil && (*m.v < 0 || math.IsNaN(*m.v) || math.IsInf(*m.v, 0)) {
				return fmt.Errorf("measure input %q: %s %v must be a finite amount of 0 or more", id, m.name, *m.v)
			}
		}
	}
	return nil
}

// measureCosts returns the costs of a measure from its entered amounts,
// with the payback computed (see Costs).
func measureCosts(in MeasureInput) Costs {
	costs := Costs{
		Investment:          clonePtr(in.Investment),
		AnnualBenefit:       clonePtr(in.AnnualBenefit),
		AnnualOperatingCost: clonePtr(in.AnnualOperatingCost),
	}
	costs.PaybackMonths, costs.NeverPaysBack = calculatePayback(costs)
	costs.Note = costsNote(costs)
	return costs
}

// calculatePayback returns the payback period in months, investment ÷
// (annual benefit − annual operating cost) × 12. never is true when the
// annual net benefit is 0 or less (the measure never pays back). Both are
// zero values (months nil, never false: not estimated) when an amount is
// not entered.
func calculatePayback(c Costs) (months *float64, never bool) {
	if c.Investment == nil || c.AnnualBenefit == nil || c.AnnualOperatingCost == nil {
		return nil, false
	}
	net := *c.AnnualBenefit - *c.AnnualOperatingCost
	if net <= 0 {
		return nil, true
	}
	return F(*c.Investment / net * 12), false
}

// costsNote says which amounts were not entered (and so why the payback is
// not estimated), or that the measure never pays back.
func costsNote(c Costs) string {
	var missing []string
	if c.Investment == nil {
		missing = append(missing, "investment")
	}
	if c.AnnualBenefit == nil {
		missing = append(missing, "annual benefit")
	}
	if c.AnnualOperatingCost == nil {
		missing = append(missing, "annual operating cost")
	}
	switch {
	case len(missing) > 0:
		return "Payback not estimated: " + strings.Join(missing, ", ") + " not entered"
	case c.NeverPaysBack:
		return "Never pays back: the annual benefit does not exceed the annual operating cost"
	}
	return ""
}

// CostTotals adds up the entered costs of every measure in the catalogue.
//
// Rule: a total is known only when every measure has that amount entered
// (an amount not entered is unknown, never 0), so a total is never a
// partial sum. MissingInputs lists the measures with at least one of the
// three amounts not entered. The payback is computed from the three totals
// as for one measure (see Costs). With no measures the totals are 0 and the
// payback is not estimated.
type CostTotals struct {
	Investment          *float64 `json:"investment,omitempty"`
	AnnualBenefit       *float64 `json:"annual_benefit,omitempty"`
	AnnualOperatingCost *float64 `json:"annual_operating_cost,omitempty"`
	PaybackMonths       *float64 `json:"payback_months,omitempty"`
	NeverPaysBack       bool     `json:"never_pays_back,omitempty"`
	MissingInputs       []string `json:"missing_inputs,omitempty"`
	Note                string   `json:"note,omitempty"`
}

// calculateCostTotals returns the cost totals of measures (see CostTotals).
func calculateCostTotals(measures []ImprovementMeasure) CostTotals {
	var inv, ben, op sum
	var t CostTotals
	for _, m := range measures {
		inv.add(m.Costs.Investment)
		ben.add(m.Costs.AnnualBenefit)
		op.add(m.Costs.AnnualOperatingCost)
		if m.Costs.Investment == nil || m.Costs.AnnualBenefit == nil || m.Costs.AnnualOperatingCost == nil {
			t.MissingInputs = append(t.MissingInputs, m.ID)
		}
	}
	if len(measures) == 0 {
		t.Investment, t.AnnualBenefit, t.AnnualOperatingCost = F(0), F(0), F(0)
		t.Note = "No measures in the catalogue: nothing to invest, payback not estimated"
		return t
	}
	t.Investment, t.AnnualBenefit, t.AnnualOperatingCost = inv.value(), ben.value(), op.value()
	t.PaybackMonths, t.NeverPaysBack = calculatePayback(Costs{
		Investment: t.Investment, AnnualBenefit: t.AnnualBenefit, AnnualOperatingCost: t.AnnualOperatingCost,
	})
	switch {
	case len(t.MissingInputs) > 0:
		t.Note = fmt.Sprintf("Totals not estimated where a measure's amount is not entered "+
			"(%d of %d measures have missing amounts); totals are never partial sums",
			len(t.MissingInputs), len(measures))
	case t.NeverPaysBack:
		t.Note = "Never pays back: the total annual benefit does not exceed the total annual operating cost"
	}
	return t
}

// validateRiskInputs returns an error for a probability or impact outside
// 0–1 or a risk ID not in known.
func validateRiskInputs(inputs map[string]RiskInput, known map[string]bool) error {
	for _, id := range slices.Sorted(maps.Keys(inputs)) {
		if !known[id] {
			return fmt.Errorf("risk input %q: no risk in the checklist has this ID", id)
		}
		in := inputs[id]
		for _, f := range []struct {
			name string
			v    *float64
		}{{"probability", in.Probability}, {"impact", in.Impact}} {
			if f.v != nil && !(*f.v >= 0 && *f.v <= 1) {
				return fmt.Errorf("risk input %q: %s %v must be between 0 and 1", id, f.name, *f.v)
			}
		}
	}
	return nil
}

// legacyCosts reports whether data is costs written by the earlier
// planner, which made every amount up from fixed base amounts (it always
// wrote "operational", "benefits" and "payback_period").
func legacyCosts(data []byte) bool {
	var w struct {
		Operational   *json.RawMessage `json:"operational"`
		Benefits      *json.RawMessage `json:"benefits"`
		PaybackPeriod *json.RawMessage `json:"payback_period"`
	}
	if json.Unmarshal(data, &w) != nil {
		return false
	}
	return w.Operational != nil || w.Benefits != nil || w.PaybackPeriod != nil
}

// UnmarshalJSON reads costs; legacy costs (see legacyCosts) load as not
// entered, with a note.
func (c *Costs) UnmarshalJSON(data []byte) error {
	type plain Costs
	var w plain
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	if legacyCosts(data) {
		*c = Costs{Note: "Payback not estimated: costs in this file were estimated by an " +
			"earlier version from fixed amounts, not entered; enter them as measure inputs"}
		return nil
	}
	*c = Costs(w)
	return nil
}

// Loading potentials, continuous improvements and measures: the priority
// is derived from the stored ratings (see RatingPriority), so it is
// recomputed on load. A priority stored without both ratings (files from
// the earlier version, which made priorities up from fixed scores) loads
// as not prioritised.

// UnmarshalJSON reads p, recomputing its priority from its ratings.
func (p *ImprovementPotential) UnmarshalJSON(data []byte) error {
	type plain ImprovementPotential
	var w plain
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*p = ImprovementPotential(w)
	p.ImpactRating, p.EffortRating, p.Priority, p.PriorityNote = rate(MeasureInput{}, p.ImpactRating, p.EffortRating)
	return nil
}

// UnmarshalJSON reads ci, recomputing its priority from its ratings.
func (ci *ContinuousImprovement) UnmarshalJSON(data []byte) error {
	type plain ContinuousImprovement
	var w plain
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*ci = ContinuousImprovement(w)
	ci.ImpactRating, ci.EffortRating, ci.Priority, ci.PriorityNote = rate(MeasureInput{}, ci.ImpactRating, ci.EffortRating)
	return nil
}

// UnmarshalJSON reads m, recomputing its priority from its ratings. A
// measure written by the earlier planner (legacy costs, see legacyCosts)
// also had a made-up due date (design date + 1 month): it loads as not
// entered.
func (m *ImprovementMeasure) UnmarshalJSON(data []byte) error {
	type plain ImprovementMeasure
	var w struct {
		plain
		Costs json.RawMessage `json:"costs"`
	}
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*m = ImprovementMeasure(w.plain)
	if len(w.Costs) > 0 {
		if err := json.Unmarshal(w.Costs, &m.Costs); err != nil {
			return err
		}
		if legacyCosts(w.Costs) {
			m.SMART.TimeBound = nil
		}
	}
	m.ImpactRating, m.EffortRating, m.Priority, m.PriorityNote = rate(MeasureInput{}, m.ImpactRating, m.EffortRating)
	return nil
}

// MarshalJSON writes result with "format_version" (see FormatVersion).
func (result PlanningResult) MarshalJSON() ([]byte, error) {
	type plain PlanningResult
	return json.Marshal(struct {
		FormatVersion int `json:"format_version"`
		plain
	}{FormatVersion, plain(result)})
}
