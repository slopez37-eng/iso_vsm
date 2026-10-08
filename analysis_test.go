package vsm

import (
	"math"
	"testing"
)

func TestAnalyzeIdleTimesMixedUnits(t *testing.T) {
	a := NewAnalyzer()
	got, err := a.AnalyzeIdleTimes(mixedUnitStream())
	if err != nil {
		t.Fatal(err)
	}
	// Minutes (see mixedUnitStream): idle 10 + 6 + 6 = 22, process 2 + 1 + 3 = 6.
	if !approx(got.TotalIdleTime, 22) {
		t.Errorf("total idle = %v, want 22", got.TotalIdleTime)
	}
	if !approx(got.IdleTimeEfficiency, 6.0/28) {
		t.Errorf("idle efficiency = %v, want %v", got.IdleTimeEfficiency, 6.0/28)
	}
}

func TestAnalyzeFlowNoInfWhenProcessTimeZero(t *testing.T) {
	a := NewAnalyzer()
	vs := &ValueStream{Processes: []Process{{Name: "P", Parameters: ProcessParameters{WaitingTime: 10}}}}
	got, err := a.AnalyzeFlow(vs)
	if err != nil {
		t.Fatal(err)
	}
	for _, fi := range got.FlowInterruptions {
		if math.IsInf(fi.Frequency, 0) || math.IsNaN(fi.Frequency) {
			t.Errorf("interruption %q has non-finite frequency %v", fi.Type, fi.Frequency)
		}
	}
}

func TestAnalyzeFlowEfficiencyMatchesKPI(t *testing.T) {
	vs := mixedUnitStream()
	got, err := NewAnalyzer().AnalyzeFlow(vs)
	if err != nil {
		t.Fatal(err)
	}
	if want := NewCalculator().CalculateKPIs(vs)["value_added_ratio"]; !approx(got.FlowEfficiency, want) {
		t.Errorf("flow efficiency = %v, want value_added_ratio %v", got.FlowEfficiency, want)
	}
}

func TestPaybackPeriod(t *testing.T) {
	p := NewPlanner()
	tests := []struct {
		name  string
		costs Costs
		want  float64
	}{
		// 12,000 ÷ (25,000 − 1,000) per year × 12 = 6 months
		{"positive net benefit", Costs{Investment: 12000, Operational: 1000, Benefits: 25000}, 6},
		{"never pays back", Costs{Investment: 12000, Operational: 5000, Benefits: 4000}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := p.calculatePaybackPeriod(tt.costs); !approx(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContinuousFlowPrincipleNormalizesUnits(t *testing.T) {
	// In minutes: idle 1 + 1 = 2, process 10 + 1 = 11 -> ratio 0.18 (< 0.5).
	// Summing raw numbers would give 61 / 70 = 0.87 and the wrong answer.
	vs := &ValueStream{Processes: []Process{
		{Parameters: ProcessParameters{TimeUnit: TimeUnitMinutes, ProcessTime: 10, IdleTime: 1}},
		{Parameters: ProcessParameters{TimeUnit: TimeUnitSeconds, ProcessTime: 60, IdleTime: 60}},
	}}
	if !NewDesigner().checkContinuousFlowPrinciple(vs) {
		t.Error("expected continuous flow principle to be detected")
	}
}
