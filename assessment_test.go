package vsm

import (
	"math"
	"testing"
	"time"
)

// history builds assessments with one KPI taking the given values in order.
func history(kpi string, values ...float64) []AssessmentResult {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	h := make([]AssessmentResult, len(values))
	for i, v := range values {
		h[i] = AssessmentResult{
			AssessmentDate: base.AddDate(0, 0, 7*i),
			KPIs:           map[string]KPIMeasure{kpi: {Name: kpi, Value: v}},
		}
	}
	return h
}

func TestGetKPIDirection(t *testing.T) {
	tests := map[string]KPIDirection{
		"total_lead_time":    LowerIsBetter,
		"Lead Time":          LowerIsBetter,
		"waiting-time":       LowerIsBetter,
		"range_of_inventory": LowerIsBetter,
		"scrap_rate":         LowerIsBetter,
		"WIP":                LowerIsBetter,
		"value_added_ratio":  HigherIsBetter,
		"flow_efficiency":    HigherIsBetter,
		"OTIF":               HigherIsBetter,
		"first_pass_yield":   HigherIsBetter,
		"inventory_turns":    HigherIsBetter,
		"customer_takt":      DirectionUnknown,
		"made_up_kpi":        DirectionUnknown,
	}
	for name, want := range tests {
		if got := GetKPIDirection(name); got != want {
			t.Errorf("GetKPIDirection(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestAnalyzeTrendDirection(t *testing.T) {
	a := NewAssessor()
	tests := []struct {
		name       string
		kpi        string
		values     []float64
		want       string
		wantChange float64
		changeNil  bool
	}{
		{"lead time falling improves", "total_lead_time", []float64{100, 90, 80}, "improving", -0.2, false},
		{"lead time rising declines", "total_lead_time", []float64{80, 100}, "declining", 0.25, false},
		{"flow efficiency rising improves", "value_added_ratio", []float64{0.1, 0.2}, "improving", 1, false},
		{"flow efficiency falling declines", "value_added_ratio", []float64{0.2, 0.1}, "declining", -0.5, false},
		{"small change is stable", "total_lead_time", []float64{100, 102}, "stable", 0.02, false},
		{"unknown kpi reports change only", "customer_takt", []float64{2, 3}, "increasing", 0.5, false},
		{"unknown kpi falling", "made_up_kpi", []float64{2, 1}, "decreasing", -0.5, false},
		// first value 0: relative change undefined, label follows the sign
		{"from zero rising", "scrap_rate", []float64{0, 0.02}, "declining", 0, true},
		{"from zero to zero", "scrap_rate", []float64{0, 0}, "stable", 0, false},
		// negative start: sign must follow the actual movement
		{"negative start", "made_up_kpi", []float64{-10, -5}, "increasing", 0.5, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, change := a.analyzeTrend(tt.kpi, history(tt.kpi, tt.values...))
			if got != tt.want {
				t.Errorf("trend = %q, want %q", got, tt.want)
			}
			if tt.changeNil {
				if change != nil {
					t.Errorf("change = %v, want nil", *change)
				}
				return
			}
			if change == nil || math.IsNaN(*change) || math.IsInf(*change, 0) || !approx(*change, tt.wantChange) {
				t.Errorf("change = %v, want %v", change, tt.wantChange)
			}
		})
	}
}

func TestAssessValueStreamTrendForLeadTime(t *testing.T) {
	vs := mixedUnitStream()
	lt := NewCalculator().CalculateKPIs(vs)["total_lead_time"]
	// Lead time fell from 2× to the current value: improving, not declining.
	h := history("total_lead_time", 2*lt, lt)
	got, err := NewAssessor().AssessValueStream(vs, nil, h)
	if err != nil {
		t.Fatal(err)
	}
	m := got.KPIs["total_lead_time"]
	if m.Trend != "improving" || m.Direction != LowerIsBetter {
		t.Errorf("lead time trend = %q (%q), want improving (lower_is_better)", m.Trend, m.Direction)
	}
}

func TestTrendRecommendationRespectsDirection(t *testing.T) {
	a := NewAssessor()
	falling := &AssessmentResult{PerformanceIndex: 1, Trends: []TrendAnalysis{
		{KPIName: "total_lead_time", Direction: LowerIsBetter, Slope: -1},
	}}
	if recs := a.generateAssessmentRecommendations(falling); len(recs) != 0 {
		t.Errorf("falling lead time should not be flagged: %v", recs)
	}
	rising := &AssessmentResult{PerformanceIndex: 1, Trends: []TrendAnalysis{
		{KPIName: "total_lead_time", Direction: LowerIsBetter, Slope: 1},
	}}
	if recs := a.generateAssessmentRecommendations(rising); len(recs) != 1 {
		t.Errorf("rising lead time should be flagged once: %v", recs)
	}
}

func TestRatioKPIsLabelledRatio(t *testing.T) {
	got, err := NewAssessor().AssessValueStream(mixedUnitStream(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"value_added_ratio", "process_efficiency", "average_yield_factor"} {
		m, ok := got.KPIs[name]
		if !ok {
			t.Fatalf("missing KPI %s", name)
		}
		if m.Unit != "ratio" || m.Value <= 0 || m.Value > 1 {
			t.Errorf("%s = %v %q, want a fraction labelled ratio", name, m.Value, m.Unit)
		}
	}
}

// Lead time 100 → 90 → 80, weekly: the forecast for next week is 70, from
// the latest point, even when history is passed newest first.
func TestTrendForecastFromLatest(t *testing.T) {
	h := history("total_lead_time", 100, 90, 80)
	reversed := []AssessmentResult{h[2], h[1], h[0]}
	for name, hist := range map[string][]AssessmentResult{"oldest first": h, "newest first": reversed} {
		got, err := NewAssessor().AssessValueStream(mixedUnitStream(), nil, hist)
		if err != nil {
			t.Fatal(err)
		}
		var trend *TrendAnalysis
		for i := range got.Trends {
			if got.Trends[i].KPIName == "total_lead_time" {
				trend = &got.Trends[i]
			}
		}
		if trend == nil {
			t.Fatalf("%s: no lead time trend", name)
		}
		if !approx(trend.Forecast, 70) {
			t.Errorf("%s: forecast = %v, want 70", name, trend.Forecast)
		}
		if pts := trend.DataPoints; !pts[0].Date.Before(pts[len(pts)-1].Date) {
			t.Errorf("%s: data points not oldest first", name)
		}
		if !approx(trend.Confidence, 1) {
			t.Errorf("%s: confidence = %v, want 1 (perfect straight line, falling)", name, trend.Confidence)
		}
		if trend.Period != "weekly" {
			t.Errorf("%s: period = %q, want weekly", name, trend.Period)
		}
		if m := got.KPIs["total_lead_time"]; m.Trend != "improving" {
			t.Errorf("%s: KPI trend = %q, want improving", name, m.Trend)
		}
	}
}
