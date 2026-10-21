package vsm

import "errors"

// StateComparison is the relative assessment of clause 4.5.2: the shares of
// value adding (process) and non-value adding (idle) time in the actual and
// target states, and the comparison figures ω_PT, ω_IT and ω_LT. Times are in
// the actual state's time unit.
type StateComparison struct {
	TimeUnit                  TimeUnit   `json:"time_unit"`
	ActualProcessTime         float64    `json:"actual_process_time"`
	ActualIdleTime            float64    `json:"actual_idle_time"`
	ActualLeadTime            float64    `json:"actual_lead_time"`
	TargetProcessTime         float64    `json:"target_process_time"`
	TargetIdleTime            float64    `json:"target_idle_time"`
	TargetLeadTime            float64    `json:"target_lead_time"`
	ActualValueAddingShare    float64    `json:"actual_value_adding_share"`     // S_VA,ACT
	ActualNonValueAddingShare float64    `json:"actual_non_value_adding_share"` // S_NVA,ACT
	TargetValueAddingShare    float64    `json:"target_value_adding_share"`     // S_VA,TAR
	TargetNonValueAddingShare float64    `json:"target_non_value_adding_share"` // S_NVA,TAR
	Figures                   Comparison `json:"comparison"`
}

// CompareStates compares an actual (current) and a target (future) state by
// clause 4.5.2. Both need a known lead time that is not zero.
func CompareStates(actual, target Result) (StateComparison, error) {
	if !actual.Totals.LeadTime.Known {
		return StateComparison{}, errors.New("vsm: the actual state's total process time or idle time is unknown")
	}
	if !target.Totals.LeadTime.Known {
		return StateComparison{}, errors.New("vsm: the target state's total process time or idle time is unknown")
	}
	if actual.Totals.LeadTime.Value == 0 || target.Totals.LeadTime.Value == 0 {
		return StateComparison{}, errors.New("vsm: a lead time of zero has no shares")
	}
	convert := func(v Value) (float64, error) { return Convert(v.Value, target.TimeUnit, actual.TimeUnit) }
	tpt, err := convert(target.Totals.ProcessTime)
	if err != nil {
		return StateComparison{}, err
	}
	tit, _ := convert(target.Totals.IdleTime)
	tlt, _ := convert(target.Totals.LeadTime)
	apt, ait := actual.Totals.ProcessTime.Value, actual.Totals.IdleTime.Value
	// Shares have no unit, so each state's own KPIs serve as they are.
	return StateComparison{
		TimeUnit:          actual.TimeUnit,
		ActualProcessTime: apt, ActualIdleTime: ait, ActualLeadTime: actual.Totals.LeadTime.Value,
		TargetProcessTime: tpt, TargetIdleTime: tit, TargetLeadTime: tlt,
		ActualValueAddingShare:    actual.KPIs.ValueAddingShare.Value,
		ActualNonValueAddingShare: actual.KPIs.NonValueAddingShare.Value,
		TargetValueAddingShare:    target.KPIs.ValueAddingShare.Value,
		TargetNonValueAddingShare: target.KPIs.NonValueAddingShare.Value,
		Figures:                   Compare(apt, ait, tpt, tit),
	}, nil
}
