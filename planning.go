package vsm

import (
	"cmp"
	"math"
	"slices"
)

// MeasureStatus is the implementation status of a measure, which clause 4.4.2
// requires to be noted.
type MeasureStatus string

const (
	StatusPlanned    MeasureStatus = "planned"
	StatusInProgress MeasureStatus = "in_progress"
	StatusDone       MeasureStatus = "done"
	StatusCancelled  MeasureStatus = "cancelled"
)

// MeasureStatuses lists every MeasureStatus.
var MeasureStatuses = []MeasureStatus{StatusPlanned, StatusInProgress, StatusDone, StatusCancelled}

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
	// Responsible is the person who accepted the measure.
	Responsible string `json:"responsible,omitempty"`
	Department  string `json:"department,omitempty"`
	// Deadline is a date in YYYY-MM-DD form (time-related).
	Deadline string        `json:"deadline,omitempty"`
	Status   MeasureStatus `json:"status"`
	// Flashes lists the numbers of the improvement flashes this measure acts on.
	Flashes []int `json:"flashes,omitempty"`
	// Cost and Benefit feed the cost-benefit prioritisation of clause 4.5.2.
	// Use one currency and period for every measure.
	Cost    *float64 `json:"cost,omitempty"`
	Benefit *float64 `json:"benefit,omitempty"`
}

// SMARTGaps lists the SMART criteria the measure's fields do not yet show.
// "Realistic" is a judgement and is never reported.
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
	}
	if m.Deadline == "" {
		gaps = append(gaps, "time-related: no deadline")
	} else if !isDate(m.Deadline) {
		gaps = append(gaps, "time-related: deadline is not a YYYY-MM-DD date")
	}
	return gaps
}

// BenefitCostRatio returns benefit ÷ cost. It is unknown when either is
// missing, and +Inf for a positive benefit at zero cost.
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

// Prioritize returns the measure numbers ordered by the cost-benefit analysis
// of clause 4.5.2: highest benefit ÷ cost first, then higher net benefit.
// Measures without both figures come last, in catalogue order. Done and
// cancelled measures are left out.
func Prioritize(measures []Measure) []int {
	open := slices.DeleteFunc(slices.Clone(measures), func(m Measure) bool {
		return m.Status == StatusDone || m.Status == StatusCancelled
	})
	slices.SortStableFunc(open, func(a, b Measure) int {
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
		if c := cmp.Compare(rb.Value, ra.Value); c != 0 {
			return c
		}
		return cmp.Compare(*b.Benefit-*b.Cost, *a.Benefit-*a.Cost)
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
