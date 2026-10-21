package vsm

import "errors"

// StateComparison is the relative assessment of clause 4.5.2: the shares of
// value adding (process) and non-value adding (idle) time in the actual and
// target states, and the comparison figures ω_PT, ω_IT and ω_LT. Times are in
// the actual state's time unit.
type StateComparison struct {
	TimeUnit                  TimeUnit          `json:"time_unit"`
	ActualProcessTime         float64           `json:"actual_process_time"`
	ActualIdleTime            float64           `json:"actual_idle_time"`
	ActualLeadTime            float64           `json:"actual_lead_time"`
	TargetProcessTime         float64           `json:"target_process_time"`
	TargetIdleTime            float64           `json:"target_idle_time"`
	TargetLeadTime            float64           `json:"target_lead_time"`
	ActualValueAddingShare    float64           `json:"actual_value_adding_share"`     // S_VA,ACT
	ActualNonValueAddingShare float64           `json:"actual_non_value_adding_share"` // S_NVA,ACT
	TargetValueAddingShare    float64           `json:"target_value_adding_share"`     // S_VA,TAR
	TargetNonValueAddingShare float64           `json:"target_non_value_adding_share"` // S_NVA,TAR
	Figures                   ComparisonFigures `json:"comparison"`
}

// ComparisonFigures holds ω_PT, ω_IT and ω_LT of clause 4.5.2. A figure is
// unknown when its actual-state time is 0, for example ω_IT when the current
// state has no idle time.
type ComparisonFigures struct {
	ProcessTime Value `json:"omega_pt"` // ω_PT
	IdleTime    Value `json:"omega_it"` // ω_IT
	LeadTime    Value `json:"omega_lt"` // ω_LT
}

// CompareStates compares an actual (current) and a target (future) state by
// clause 4.5.2. Both need a known total process time and idle time and a
// non-zero lead time.
func CompareStates(actual, target Result) (StateComparison, error) {
	if !actual.Totals.ProcessTime.Known || !actual.Totals.IdleTime.Known {
		return StateComparison{}, errors.New("vsm: the actual state's total process time or idle time is unknown")
	}
	if !target.Totals.ProcessTime.Known || !target.Totals.IdleTime.Known {
		return StateComparison{}, errors.New("vsm: the target state's total process time or idle time is unknown")
	}
	tpt, err := Convert(target.Totals.ProcessTime.Value, target.TimeUnit, actual.TimeUnit)
	if err != nil {
		return StateComparison{}, err
	}
	tit, _ := Convert(target.Totals.IdleTime.Value, target.TimeUnit, actual.TimeUnit)
	apt, ait := actual.Totals.ProcessTime.Value, actual.Totals.IdleTime.Value

	c := StateComparison{
		TimeUnit:          actual.TimeUnit,
		ActualProcessTime: apt, ActualIdleTime: ait, ActualLeadTime: apt + ait,
		TargetProcessTime: tpt, TargetIdleTime: tit, TargetLeadTime: tpt + tit,
	}
	if c.ActualLeadTime == 0 || c.TargetLeadTime == 0 {
		return StateComparison{}, errors.New("vsm: a lead time of zero has no shares")
	}
	c.ActualValueAddingShare, _ = ValueAddingShare(apt, c.ActualLeadTime)
	c.ActualNonValueAddingShare, _ = NonValueAddingShare(ait, c.ActualLeadTime)
	c.TargetValueAddingShare, _ = ValueAddingShare(tpt, c.TargetLeadTime)
	c.TargetNonValueAddingShare, _ = NonValueAddingShare(tit, c.TargetLeadTime)
	ratio := func(target, actual float64) Value {
		v, err := divide(target, actual)
		if err != nil {
			return Value{}
		}
		return Value{Value: v, Known: true, Source: "4.5.2"}
	}
	c.Figures = ComparisonFigures{
		ProcessTime: ratio(tpt, apt),
		IdleTime:    ratio(tit, ait),
		LeadTime:    ratio(c.TargetLeadTime, c.ActualLeadTime),
	}
	return c, nil
}
