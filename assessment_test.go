package vsm

import (
	"math"
	"strings"
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

// The same 60-minute stream assessed in minutes, then in seconds, is not a
// change: history is converted to the current unit before trends are read.
func TestTrendsNormaliseUnitsAndPeriods(t *testing.T) {
	a := NewAssessor()
	stream := func(unit TimeUnit, factor float64) *ValueStream {
		return &ValueStream{ID: "vs", Processes: []Process{{ID: "p", Parameters: ProcessParameters{
			ProcessTime: 20 * factor, IdleTime: 40 * factor, ValueAddingTime: 10 * factor, TimeUnit: unit}}}}
	}
	week := 7 * 24 * time.Hour
	first, err := a.AssessValueStream(stream(TimeUnitMinutes, 1), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.TimeUnit != TimeUnitMinutes || first.KPIs["total_lead_time"].Unit != "minutes" {
		t.Errorf("unit not recorded: %q / %q", first.TimeUnit, first.KPIs["total_lead_time"].Unit)
	}
	first.AssessmentDate = first.AssessmentDate.Add(-2 * week)
	second, err := a.AssessValueStream(stream(TimeUnitSeconds, 60), nil, []AssessmentResult{*first})
	if err != nil {
		t.Fatal(err)
	}
	second.AssessmentDate = second.AssessmentDate.Add(-week)
	third, err := a.AssessValueStream(stream(TimeUnitSeconds, 60), nil, []AssessmentResult{*first, *second})
	if err != nil {
		t.Fatal(err)
	}
	if m := third.KPIs["total_lead_time"]; m.Trend != "stable" || m.Value != 3600 {
		t.Errorf("lead time 60 min → 3600 s: trend %q value %v, want stable 3600", m.Trend, m.Value)
	}
	for _, tr := range third.Trends {
		if tr.KPIName == "total_lead_time" && (!approx(tr.DataPoints[0].Value, 3600) || !approx(tr.Forecast, 3600)) {
			t.Errorf("trend points %v forecast %v, want 3600 s throughout", tr.DataPoints, tr.Forecast)
		}
	}
	if d := a.CompareAssessments(second, first)["total_lead_time_change"]; d != 0 {
		t.Errorf("CompareAssessments change = %v, want 0", d)
	}

	// Inventory turns: 2 per day, then the same stock and demand stated per
	// week (14 per week) is not an improvement.
	turns := func(period DemandPeriod, demand float64) *ValueStream {
		vs := stream(TimeUnitMinutes, 1)
		vs.Customer = &Customer{ID: "c", Demand: demand, DemandPeriod: period}
		vs.Inventory = []Inventory{{ID: "i", Quantity: 50}}
		return vs
	}
	daily, err := a.AssessValueStream(turns(DemandPeriodDay, 100), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if daily.DemandPeriod != DemandPeriodDay {
		t.Errorf("demand period not recorded: %q", daily.DemandPeriod)
	}
	daily.AssessmentDate = daily.AssessmentDate.Add(-week)
	weekly, err := a.AssessValueStream(turns(DemandPeriodWeek, 700), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if m := weekly.KPIs["inventory_turns"]; m.Value != 14 {
		t.Errorf("weekly turns = %v, want 14", m.Value)
	}
	daily2 := *daily
	daily2.AssessmentDate = daily.AssessmentDate.Add(-week)
	weekly, err = a.AssessValueStream(turns(DemandPeriodWeek, 700), nil, []AssessmentResult{daily2, *daily})
	if err != nil {
		t.Fatal(err)
	}
	for _, tr := range weekly.Trends {
		if tr.KPIName == "inventory_turns" && !approx(tr.DataPoints[0].Value, 14) {
			t.Errorf("turns history %v, want 14 per week", tr.DataPoints)
		}
	}

	// Old history without units counts as the current unit.
	old := history("total_lead_time", 3600, 3600)
	got, err := a.AssessValueStream(stream(TimeUnitSeconds, 60), nil, old)
	if err != nil {
		t.Fatal(err)
	}
	if m := got.KPIs["total_lead_time"]; m.Trend != "stable" {
		t.Errorf("unitless history: trend %q, want stable", m.Trend)
	}
}

// Status comes from meeting the target in the KPI's better direction.
func TestKPIStatusUsesTargetAndDirection(t *testing.T) {
	a := NewAssessor()
	tests := []struct {
		kpi           string
		value, target float64
		want          string
	}{
		// Lower is better: at or below target is met.
		{"total_lead_time", 90, 100, StatusExcellent},
		{"total_lead_time", 100, 100, StatusExcellent},
		{"total_lead_time", 105, 100, StatusGood},
		{"total_lead_time", 120, 100, StatusWarning},
		{"total_lead_time", 150, 100, StatusCritical},
		{"scrap_rate", 0, 0, StatusExcellent},
		{"scrap_rate", 0.01, 0, StatusCritical},
		// Higher is better: at or above target is met.
		{"value_added_ratio", 0.3, 0.25, StatusExcellent},
		{"value_added_ratio", 0.25, 0.25, StatusExcellent},
		{"value_added_ratio", 0.24, 0.25, StatusGood},
		{"value_added_ratio", 0.2, 0.25, StatusWarning},
		{"value_added_ratio", 0.1, 0.25, StatusCritical},
		// Unknown direction: met or missed cannot be told.
		{"customer_takt", 2, 3, StatusUnrated},
	}
	for _, tt := range tests {
		if got := a.determineKPIStatus(tt.kpi, tt.value, tt.target, true); got != tt.want {
			t.Errorf("%s %v vs target %v: %q, want %q", tt.kpi, tt.value, tt.target, got, tt.want)
		}
	}
	// No target: no status is invented (a 500-minute lead time is not
	// "excellent").
	if got := a.determineKPIStatus("total_lead_time", 500, 0, false); got != StatusUnrated {
		t.Errorf("no target: %q, want unrated", got)
	}
}

// Through AssessValueStream: meeting a lead-time target is not critical,
// unrated KPIs are left out of the performance index and action items, and
// with no targets at all the index is 0 with a recommendation to set them.
func TestAssessValueStreamRatesOnlyTargetedKPIs(t *testing.T) {
	vs := mixedUnitStream() // lead time 28 min
	a := NewAssessor()
	got, err := a.AssessValueStream(vs, map[string]float64{"total_lead_time": 28}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if m := got.KPIs["total_lead_time"]; m.Status != StatusExcellent || !m.HasTarget {
		t.Errorf("lead time at target: %q (has target %v), want excellent", m.Status, m.HasTarget)
	}
	for name, m := range got.KPIs {
		if name != "total_lead_time" && (m.Status != StatusUnrated || m.HasTarget) {
			t.Errorf("%s without target: status %q has_target %v, want unrated", name, m.Status, m.HasTarget)
		}
	}
	if got.PerformanceIndex != 1 {
		t.Errorf("performance index = %v, want 1 (only the met KPI is rated)", got.PerformanceIndex)
	}
	if len(got.ActionItems) != 0 {
		t.Errorf("action items for unrated KPIs: %+v", got.ActionItems)
	}
	if r, ok := got.TargetAchievement["total_lead_time"]; !ok || r != 1 || len(got.TargetAchievement) != 1 {
		t.Errorf("target achievement = %v, want only total_lead_time = 1", got.TargetAchievement)
	}

	none, err := a.AssessValueStream(vs, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if none.PerformanceIndex != 0 || len(none.Recommendations) != 1 ||
		!strings.Contains(none.Recommendations[0], "no targets") {
		t.Errorf("no targets: index %v, recommendations %v", none.PerformanceIndex, none.Recommendations)
	}
}
