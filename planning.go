package vsm

import (
	"cmp"
	"math"
	"slices"
)

// MeasureStatus is the implementation status of a measure, which clause 4.4.2
// requires to be noted. The first five stages are the catalogue key of Annex
// B.2.4.
type MeasureStatus string

const (
	StatusProblemDetected           MeasureStatus = "problem_detected"
	StatusResponsibilitiesClarified MeasureStatus = "responsibilities_clarified"
	StatusMeasureChecked            MeasureStatus = "measure_checked" // measure created and checked
	StatusImplementing              MeasureStatus = "implementing"
	StatusImplemented               MeasureStatus = "implemented" // implemented and reviewed
	StatusCancelled                 MeasureStatus = "cancelled"
)

// MeasureStatuses lists every MeasureStatus.
var MeasureStatuses = []MeasureStatus{StatusProblemDetected, StatusResponsibilitiesClarified, StatusMeasureChecked,
	StatusImplementing, StatusImplemented, StatusCancelled}

// Period is a length of time that a measure's benefit is counted in.
type Period string

const (
	PeriodDay   Period = "day"
	PeriodWeek  Period = "week"
	PeriodMonth Period = "month"
	PeriodYear  Period = "year"
)

// Periods lists every Period.
var Periods = []Period{PeriodDay, PeriodWeek, PeriodMonth, PeriodYear}

// TargetRule says how a measured KPI is checked against a measure's target.
type TargetRule string

const (
	TargetAtMost  TargetRule = "at_most"  // measured value is at or below the target
	TargetAtLeast TargetRule = "at_least" // measured value is at or above the target
	TargetEqual   TargetRule = "equal"    // measured value is within Tolerance of the target
)

// TargetRules lists every TargetRule.
var TargetRules = []TargetRule{TargetAtMost, TargetAtLeast, TargetEqual}

// Measure is one entry of the catalogue of measures for improvement (clause
// 4.4.2), defined by the SMART method.
type Measure struct {
	Number int `json:"number"`
	// Potential is the improvement potential addressed, such as "stock quantity".
	Potential string `json:"potential"`
	// Description states the measure and its goal (specific).
	Description string `json:"description"`
	// Metric and Target make the goal measurable, e.g. "scrap rate" and 0.02.
	Metric string   `json:"metric,omitempty"`
	Target *float64 `json:"target,omitempty"`
	// KPI names the indicator that Target is checked against in the monitoring
	// snapshots. A time target is in the current state's time unit.
	KPI KPIName `json:"kpi,omitempty"`
	// TargetRule says how the measured KPI is checked against Target. Without
	// it, each KPI uses this library's default direction, which is not the
	// standard's.
	TargetRule TargetRule `json:"target_rule,omitempty"`
	// Tolerance is the allowed distance from Target for TargetEqual, in the unit
	// of Target (a time target is in the current state's time unit). Absent means
	// 0. It cannot be negative.
	Tolerance *float64 `json:"tolerance,omitempty"`
	// Responsible is the person responsible for the measure.
	Responsible string `json:"responsible,omitempty"`
	// Accepted records that the responsible person has accepted the measure.
	Accepted bool `json:"accepted,omitempty"`
	// Realistic records that the measure is judged realistic.
	Realistic  bool   `json:"realistic,omitempty"`
	Department string `json:"department,omitempty"`
	// Deadline is a date in YYYY-MM-DD form (time-related).
	Deadline string        `json:"deadline,omitempty"`
	Status   MeasureStatus `json:"status"`
	// Flashes lists the numbers of the improvement flashes this measure acts on.
	Flashes []int `json:"flashes,omitempty"`
	// Cost and Benefit feed the cost-benefit prioritisation of clause 4.5.2.
	// Use one currency for every measure. Cost is a one-off amount. Benefit is
	// earned once per BenefitPeriod; when BenefitPeriod is empty, Benefit is a
	// total.
	Cost          *float64 `json:"cost,omitempty"`
	Benefit       *float64 `json:"benefit,omitempty"`
	BenefitPeriod Period   `json:"benefit_period,omitempty"`
}

// SMARTGaps lists the SMART criteria the measure's fields do not yet show. The
// acceptance and realism of a measure are recorded as entered, as the user's
// judgement; until they are recorded as true, they are listed as gaps.
func (m Measure) SMARTGaps() []string {
	var gaps []string
	if m.Description == "" {
		gaps = append(gaps, "specific: no description of the measure and goal")
	}
	if m.Metric == "" || m.Target == nil {
		gaps = append(gaps, "measurable: no metric and target")
	}
	if m.Responsible == "" {
		gaps = append(gaps, "accepted: no responsible person")
	} else if !m.Accepted {
		gaps = append(gaps, "accepted: not confirmed")
	}
	if !m.Realistic {
		gaps = append(gaps, "realistic: not confirmed")
	}
	if m.Deadline == "" {
		gaps = append(gaps, "time-related: no deadline")
	} else if !isDate(m.Deadline) {
		gaps = append(gaps, "time-related: deadline is not a YYYY-MM-DD date")
	}
	return gaps
}

// BenefitCostRatio returns benefit ÷ cost. It is unknown when either is
// missing or negative, and at 0 ÷ 0; it is +Inf for a positive benefit at zero
// cost.
func (m Measure) BenefitCostRatio() Value {
	if m.Cost == nil || m.Benefit == nil || *m.Cost < 0 || *m.Benefit < 0 {
		return Value{}
	}
	if *m.Cost == 0 {
		if *m.Benefit == 0 {
			return Value{}
		}
		return knownValue(math.Inf(1), "4.5.2")
	}
	return knownValue(*m.Benefit / *m.Cost, "4.5.2")
}

// Difference returns benefit − cost, the net benefit. Cost is one-off and
// Benefit is per BenefitPeriod when that is set, so the two are compared as
// entered. It is unknown unless both are entered and not negative.
func (m Measure) Difference() Value {
	if m.Cost == nil || m.Benefit == nil || *m.Cost < 0 || *m.Benefit < 0 {
		return Value{}
	}
	return knownValue(*m.Benefit-*m.Cost, "4.5.2")
}

// paybackSource marks payback as derived from the catalogue. ISO 22468 defines
// no payback period.
const paybackSource = "payback (Cost ÷ Benefit per period)"

// Payback returns Cost ÷ Benefit, counted in BenefitPeriod: the number of
// periods the benefit takes to repay the cost. It is unknown unless
// BenefitPeriod is a valid period and both figures are entered and not
// negative. It is 0 at zero cost and +Inf at zero benefit. ISO 22468 defines
// no payback period, so this is derived from the entered figures.
func (m Measure) Payback() Value {
	if !slices.Contains(Periods, m.BenefitPeriod) || m.Cost == nil || m.Benefit == nil || *m.Cost < 0 || *m.Benefit < 0 {
		return Value{}
	}
	if *m.Cost == 0 {
		return knownValue(0, paybackSource)
	}
	if *m.Benefit == 0 {
		return knownValue(math.Inf(1), paybackSource)
	}
	return knownValue(*m.Cost / *m.Benefit, paybackSource)
}

// isOpen reports whether the measure is still to be done. Implemented and
// cancelled measures are not.
func (m Measure) isOpen() bool {
	return m.Status != StatusImplemented && m.Status != StatusCancelled
}

// Prioritize returns the measure numbers ordered by the cost-benefit analysis
// of clause 4.5.2: highest net benefit (benefit − cost) first, then highest
// benefit ÷ cost. Measures without both figures come last, in catalogue order.
// Implemented and cancelled measures are left out. The standard fixes no
// formula; this order follows the priorities of the worked example in Table B.5.
func Prioritize(measures []Measure) []int {
	open := slices.DeleteFunc(slices.Clone(measures), func(m Measure) bool {
		return !m.isOpen()
	})
	slices.SortStableFunc(open, func(a, b Measure) int {
		da, db := a.Difference(), b.Difference()
		if da.Known != db.Known {
			if da.Known {
				return -1
			}
			return 1
		}
		if !da.Known {
			return 0
		}
		if c := cmp.Compare(db.Value, da.Value); c != 0 {
			return c
		}
		// Equal net benefit: a measure with no ratio (0 ÷ 0) comes after one with a ratio.
		ra, rb := a.BenefitCostRatio(), b.BenefitCostRatio()
		if ra.Known != rb.Known {
			if ra.Known {
				return -1
			}
			return 1
		}
		if !ra.Known {
			return 0
		}
		return cmp.Compare(rb.Value, ra.Value)
	})
	order := make([]int, len(open))
	for i, m := range open {
		order[i] = m.Number
	}
	return order
}

// Workshop records the communication of the catalogue to the responsible
// employees (clause 4.4.3), which concludes "plan".
type Workshop struct {
	Date         string   `json:"date,omitempty"` // YYYY-MM-DD
	Participants []string `json:"participants,omitempty"`
	Risks        []string `json:"risks,omitempty"`
	Agreed       bool     `json:"agreed"`
}
