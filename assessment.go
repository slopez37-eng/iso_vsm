package vsm

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"time"
)

// AssessmentResult contains the results of value stream assessment
type AssessmentResult struct {
	ValueStreamID     string                `json:"value_stream_id"`
	AssessmentDate    time.Time             `json:"assessment_date"`
	KPIs              map[string]KPIMeasure `json:"kpis"`
	PerformanceIndex  float64               `json:"performance_index"`
	TargetAchievement map[string]float64    `json:"target_achievement"`
	Trends            []TrendAnalysis       `json:"trends"`
	ActionItems       []ActionItem          `json:"action_items"`
	PDCAStatus        PDCAStatus            `json:"pdca_status"`
	Recommendations   []string              `json:"recommendations"`
	// ConsistencyIssues lists stored values that disagree with the values
	// computed from their parts (see CheckConsistency). The KPIs use the
	// computed values.
	ConsistencyIssues []ConsistencyIssue `json:"consistency_issues,omitempty"`
	// TimeUnit is the unit of the time KPIs (the stream's reference unit),
	// DemandPeriod and WorkingDaysPerWeek the customer's, which
	// inventory_turns (turns per demand period) depends on. Optional: an
	// assessment without them (older history) is taken to be in the same
	// units as the one it is compared with. Trends and CompareAssessments
	// convert history to the current assessment's units first.
	TimeUnit           TimeUnit     `json:"time_unit,omitempty"`
	DemandPeriod       DemandPeriod `json:"demand_period,omitempty"`
	WorkingDaysPerWeek *float64     `json:"working_days_per_week,omitempty"`
}

// KPIMeasure represents a measured KPI value with metadata
type KPIMeasure struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
	// Target is the target supplied for this KPI, nil when none was (a
	// target of 0 is a real target). Without one, Status is StatusUnrated.
	Target    *float64 `json:"target,omitempty"`
	Threshold float64  `json:"threshold"`
	// Status rates the value against the target in the KPI's better
	// direction (see determineKPIStatus): StatusExcellent, StatusGood,
	// StatusWarning, StatusCritical, or StatusUnrated when there is no
	// target or the direction is unknown.
	Status string `json:"status"`
	// Trend is "improving", "declining" or "stable" for KPIs whose better
	// direction is known (see GetKPIDirection). For a KPI with unknown
	// direction it only reports the change: "increasing", "decreasing" or
	// "stable".
	Trend string `json:"trend"`
	// Direction is the KPI's better direction used to label Trend.
	Direction KPIDirection `json:"direction,omitempty"`
	// TrendChange is the relative change over the trend window as a fraction
	// ((last − first) ÷ |first|). Absent when there is no history or the
	// first value is 0 and the last is not (relative change undefined).
	TrendChange *float64  `json:"trend_change,omitempty"`
	MeasuredAt  time.Time `json:"measured_at"`
	Description string    `json:"description"`
}

// TrendAnalysis represents trend analysis for a KPI
type TrendAnalysis struct {
	KPIName string `json:"kpi_name"`
	// Direction is the KPI's better direction (see GetKPIDirection).
	Direction   KPIDirection `json:"direction,omitempty"`
	Period      string       `json:"period"` // "daily", "weekly", "monthly"
	DataPoints  []DataPoint  `json:"data_points"`
	Slope       float64      `json:"slope"`
	Correlation float64      `json:"correlation"`
	Forecast    float64      `json:"forecast"`   // Next period prediction
	Confidence  float64      `json:"confidence"` // Prediction confidence (0-1)
}

// DataPoint represents a single data point in trend analysis
type DataPoint struct {
	Date  time.Time `json:"date"`
	Value float64   `json:"value"`
}

// ActionItem represents an action item from assessment
type ActionItem struct {
	ID          string    `json:"id"`
	Description string    `json:"description"`
	Priority    int       `json:"priority"` // 1=urgent, 2=high, 3=medium, 4=low
	AssignedTo  string    `json:"assigned_to"`
	DueDate     time.Time `json:"due_date"`
	Status      string    `json:"status"` // "open", "in_progress", "completed", "overdue"
	CreatedAt   time.Time `json:"created_at"`
}

// PDCAStatus represents the status of PDCA cycle phases
type PDCAStatus struct {
	Plan  PDCAPhase `json:"plan"`
	Do    PDCAPhase `json:"do"`
	Check PDCAPhase `json:"check"`
	Act   PDCAPhase `json:"act"`
}

// PDCAPhase represents a phase in the PDCA cycle
type PDCAPhase struct {
	Status      string    `json:"status"` // "completed", "in_progress", "pending"
	CompletedAt time.Time `json:"completed_at,omitempty"`
	Notes       string    `json:"notes,omitempty"`
}

// KPI statuses (KPIMeasure.Status).
const (
	StatusExcellent = "excellent" // target met (or beaten)
	StatusGood      = "good"      // target missed by at most 10%
	StatusWarning   = "warning"   // target missed by more than 10%, at most 30%
	StatusCritical  = "critical"  // target missed by more than 30%
	// StatusUnrated: no target was given, or the KPI's better direction is
	// unknown, so whether the value is good cannot be told. Unrated KPIs
	// are left out of the performance index, target achievement, action
	// items and status-based recommendations.
	StatusUnrated = "unrated"
)

// Assessor provides methods for assessing value stream performance
type Assessor struct {
	calculator *Calculator
}

// NewAssessor creates a new VSM assessor
func NewAssessor() *Assessor {
	return &Assessor{
		calculator: NewCalculator(),
	}
}

// AssessValueStream performs comprehensive assessment of value stream performance
func (a *Assessor) AssessValueStream(vs *ValueStream, targets map[string]float64, history []AssessmentResult) (*AssessmentResult, error) {
	if vs == nil {
		return nil, errors.New("value stream cannot be nil")
	}

	result := &AssessmentResult{
		ValueStreamID:     vs.ID,
		AssessmentDate:    time.Now(),
		KPIs:              make(map[string]KPIMeasure),
		ConsistencyIssues: CheckConsistency(vs),
		TimeUnit:          a.calculator.ReferenceTimeUnit(vs.Processes),
	}
	if vs.Customer != nil {
		result.DemandPeriod = vs.Customer.DemandPeriod
		result.WorkingDaysPerWeek = vs.Customer.WorkingDaysPerWeek
	}
	// Compare like with like: history in this assessment's units.
	history = normalizeHistory(history, result)

	// Calculate current KPIs
	currentKPIs := a.calculator.CalculateKPIs(vs)

	// Create KPI measures with targets and status
	for name, value := range currentKPIs {
		target, hasTarget := targets[name]

		measure := KPIMeasure{
			Name:        name,
			Value:       value,
			MeasuredAt:  time.Now(),
			Description: a.getKPIDescription(name),
		}

		// Set target and unit
		if hasTarget {
			measure.Target = F(target)
		}
		measure.Unit = a.getKPIUnit(name)
		if measure.Unit == "time" && result.TimeUnit != "" {
			measure.Unit = string(result.TimeUnit)
		}
		if name == "inventory_turns" && result.DemandPeriod != "" {
			measure.Unit = "turns per " + string(result.DemandPeriod)
		}

		// Determine status
		measure.Status = a.determineKPIStatus(name, value, target, hasTarget)
		measure.Threshold = a.getKPIThreshold(name)

		// Analyze trend if history available
		measure.Direction = GetKPIDirection(name)
		if len(history) > 0 {
			measure.Trend, measure.TrendChange = a.analyzeTrend(name, history)
		} else {
			measure.Trend = "stable"
		}

		result.KPIs[name] = measure
	}

	// Calculate overall performance index
	result.PerformanceIndex = a.calculatePerformanceIndex(result.KPIs)

	// Calculate target achievement
	result.TargetAchievement = a.calculateTargetAchievement(result.KPIs)

	// Analyze trends
	result.Trends = a.analyzeTrends(history, 10) // Last 10 assessments

	// Generate action items
	result.ActionItems = a.generateActionItems(result.KPIs, vs)

	// Assess PDCA cycle status
	result.PDCAStatus = a.assessPDCAStatus(vs)

	// Generate recommendations
	result.Recommendations = a.generateAssessmentRecommendations(result)

	// A targeted KPI the library computes but could not compute here: the
	// values it needs are missing (CheckConsistency lists them).
	var uncomputed []string
	for name := range targets {
		if _, computed := currentKPIs[name]; !computed && kpiUnit(name) != "unit" {
			uncomputed = append(uncomputed, name)
		}
	}
	sort.Strings(uncomputed)
	for _, name := range uncomputed {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf(
			"%s has a target but cannot be computed: values it needs are missing or contradictory "+
				"(see consistency_issues); enter them to rate it.", name))
	}

	return result, nil
}

// getKPIDescription returns description for a KPI
func (a *Assessor) getKPIDescription(name string) string {
	descriptions := map[string]string{
		"customer_takt":           "Time interval corresponding to customer demand",
		"total_lead_time":         "Total time from order to delivery",
		"total_value_adding_time": "Total time adding value to product",
		"value_added_ratio":       "Ratio of value-adding time to total lead time",
		"process_efficiency":      "Efficiency of process operations",
		"inventory_turns":         "Number of times inventory turns over",
		"average_yield_factor":    "Average yield factor across processes",
	}

	if desc, exists := descriptions[name]; exists {
		return desc
	}
	return "Key performance indicator"
}

// getKPIUnit returns the unit for a KPI. Ratio KPIs are fractions (0.25 =
// 25%) and are labelled "ratio", not "%".
func (a *Assessor) getKPIUnit(name string) string {
	return kpiUnit(name)
}

// determineKPIStatus rates value against target in the KPI's better
// direction (GetKPIDirection). The target is met when value ≤ target for a
// lower-is-better KPI (lead time, scrap) and when value ≥ target for a
// higher-is-better KPI (flow efficiency, yield): StatusExcellent. A miss is
// graded by its size relative to the target: up to 10% StatusGood, up to
// 30% StatusWarning, beyond StatusCritical; any miss of a target of 0 is
// StatusCritical. Without a target, or for a KPI of unknown direction, the
// status is StatusUnrated: the library does not invent a rating.
func (a *Assessor) determineKPIStatus(name string, value, target float64, hasTarget bool) string {
	if !hasTarget {
		return StatusUnrated
	}
	var miss float64
	switch GetKPIDirection(name) {
	case HigherIsBetter:
		miss = target - value
	case LowerIsBetter:
		miss = value - target
	default:
		return StatusUnrated
	}
	if miss <= 0 || consistent(value, target) {
		return StatusExcellent
	}
	relative := math.Inf(1)
	if target != 0 {
		relative = miss / math.Abs(target)
	}
	switch {
	case relative <= 0.1:
		return StatusGood
	case relative <= 0.3:
		return StatusWarning
	}
	return StatusCritical
}

// getKPIThreshold returns the threshold value for a KPI
func (a *Assessor) getKPIThreshold(name string) float64 {
	thresholds := map[string]float64{
		"value_added_ratio":    0.25,
		"process_efficiency":   0.5,
		"inventory_turns":      4.0,
		"average_yield_factor": 0.85,
		"total_lead_time":      100.0, // Time units
	}

	if threshold, exists := thresholds[name]; exists {
		return threshold
	}
	return 0.0
}

// trendThreshold is the relative change (5%) below which a KPI is "stable".
const trendThreshold = 0.05

// analyzeTrend compares the first and last of the KPI's last 5 historical
// values and labels the change using the KPI's direction (GetKPIDirection):
// for lower-is-better KPIs such as lead time, a fall is "improving". KPIs of
// unknown direction get "increasing"/"decreasing" instead of a guess. It
// also returns the relative change (nil when undefined because the first
// value is 0 and the last is not; the label then follows the sign).
func (a *Assessor) analyzeTrend(kpiName string, history []AssessmentResult) (string, *float64) {
	// The last 5 assessments, by date.
	history = chronological(history)
	values := []float64{}
	for i := max(len(history)-5, 0); i < len(history); i++ {
		if kpi, exists := history[i].KPIs[kpiName]; exists {
			values = append(values, kpi.Value)
		}
	}

	if len(values) < 2 {
		return "stable", nil
	}

	// Simple trend analysis: compare first and last values
	first := values[0]
	last := values[len(values)-1]

	var change *float64
	var sign int // +1 rising, −1 falling, 0 stable
	if first != 0 {
		c := (last - first) / math.Abs(first)
		change = &c
		switch {
		case c > trendThreshold:
			sign = 1
		case c < -trendThreshold:
			sign = -1
		}
	} else {
		switch {
		case last > 0:
			sign = 1
		case last < 0:
			sign = -1
		default:
			zero := 0.0
			change = &zero
		}
	}

	return trendLabel(GetKPIDirection(kpiName), sign), change
}

// normalizeHistory returns copies of history with KPI values converted to
// the units of to: time KPIs to to.TimeUnit, inventory_turns to to's demand
// period (see periodFactor). An assessment or side that records no unit or
// period is taken to be in to's. A value that cannot be converted (unknown
// unit, a month or year period against a day or week) is left out.
func normalizeHistory(history []AssessmentResult, to *AssessmentResult) []AssessmentResult {
	c := NewCalculator()
	out := make([]AssessmentResult, len(history))
	for i, h := range history {
		out[i] = h
		kpis := make(map[string]KPIMeasure, len(h.KPIs))
		for name, m := range h.KPIs {
			v, ok := m.Value, true
			switch {
			case kpiUnit(name) == "time" && h.TimeUnit != "" && to.TimeUnit != "" && h.TimeUnit != to.TimeUnit:
				ok = isKnownTimeUnit(h.TimeUnit) && isKnownTimeUnit(to.TimeUnit)
				v = c.ConvertTimeUnit(v, h.TimeUnit, to.TimeUnit)
			case name == "inventory_turns" && h.DemandPeriod != "" && to.DemandPeriod != "":
				var factor float64
				factor, ok = periodFactor(
					&Customer{DemandPeriod: h.DemandPeriod, WorkingDaysPerWeek: h.WorkingDaysPerWeek},
					&Customer{DemandPeriod: to.DemandPeriod, WorkingDaysPerWeek: to.WorkingDaysPerWeek})
				v *= factor
			}
			if ok {
				m.Value = v
				kpis[name] = m
			}
		}
		out[i].KPIs = kpis
		out[i].TimeUnit, out[i].DemandPeriod, out[i].WorkingDaysPerWeek = to.TimeUnit, to.DemandPeriod, to.WorkingDaysPerWeek
	}
	return out
}

// chronological returns a copy of history sorted by AssessmentDate, oldest
// first (assessments with the same date keep their order).
func chronological(history []AssessmentResult) []AssessmentResult {
	sorted := append([]AssessmentResult(nil), history...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].AssessmentDate.Before(sorted[j].AssessmentDate)
	})
	return sorted
}

// trendLabel names a rising (+1), falling (−1) or stable (0) movement.
func trendLabel(direction KPIDirection, sign int) string {
	if sign == 0 {
		return "stable"
	}
	switch direction {
	case HigherIsBetter:
		if sign > 0 {
			return "improving"
		}
		return "declining"
	case LowerIsBetter:
		if sign < 0 {
			return "improving"
		}
		return "declining"
	default:
		if sign > 0 {
			return "increasing"
		}
		return "decreasing"
	}
}

// calculatePerformanceIndex is the mean status weight of the rated KPIs
// (excellent 1, good 0.8, warning 0.5, critical 0.2). Unrated KPIs are left
// out; with no rated KPI the index is 0 (a rated index is at least 0.2).
func (a *Assessor) calculatePerformanceIndex(kpis map[string]KPIMeasure) float64 {
	statusWeights := map[string]float64{
		StatusExcellent: 1.0,
		StatusGood:      0.8,
		StatusWarning:   0.5,
		StatusCritical:  0.2,
	}

	var totalScore float64
	rated := 0
	for _, kpi := range kpis {
		if weight, ok := statusWeights[kpi.Status]; ok {
			totalScore += weight
			rated++
		}
	}
	if rated == 0 {
		return 0
	}
	return totalScore / float64(rated)
}

// calculateTargetAchievement returns, for each rated KPI, how far it meets
// its target in its better direction, where 1 or more means met:
// value ÷ target for higher-is-better KPIs, target ÷ value for
// lower-is-better ones. A value of 0 for a lower-is-better KPI, or a target
// of 0 for a higher-is-better one, is met and gives 1. Unrated KPIs (no
// target, or unknown direction) are left out.
func (a *Assessor) calculateTargetAchievement(kpis map[string]KPIMeasure) map[string]float64 {
	achievement := make(map[string]float64)

	for name, kpi := range kpis {
		if kpi.Status == StatusUnrated || kpi.Target == nil {
			continue
		}
		top, bottom := kpi.Value, *kpi.Target
		if GetKPIDirection(name) == LowerIsBetter {
			top, bottom = *kpi.Target, kpi.Value
		}
		if bottom == 0 {
			achievement[name] = 1
		} else {
			achievement[name] = top / bottom
		}
	}

	return achievement
}

// analyzeTrends performs trend analysis on historical data: for each key
// KPI, the last `periods` assessments that have it, oldest first (history
// is sorted by AssessmentDate, so its order does not matter). Slope is per
// day (least squares). Forecast is the value one average assessment
// interval after the latest assessment: latest value + slope × interval;
// Period names that interval ("daily" up to 1.5 days, "weekly" up to 10
// days, else "monthly").
func (a *Assessor) analyzeTrends(history []AssessmentResult, periods int) []TrendAnalysis {
	trends := []TrendAnalysis{}

	if len(history) < 2 {
		return trends
	}
	history = chronological(history)

	// Analyze trends for key KPIs
	keyKPIs := []string{
		"total_lead_time",
		"value_added_ratio",
		"process_efficiency",
		"inventory_turns",
	}

	for _, kpiName := range keyKPIs {
		dataPoints := []DataPoint{}
		for _, h := range history {
			if kpi, exists := h.KPIs[kpiName]; exists {
				dataPoints = append(dataPoints, DataPoint{Date: h.AssessmentDate, Value: kpi.Value})
			}
		}
		if len(dataPoints) > periods {
			dataPoints = dataPoints[len(dataPoints)-periods:]
		}

		if len(dataPoints) >= 2 {
			first, latest := dataPoints[0], dataPoints[len(dataPoints)-1]
			intervalDays := latest.Date.Sub(first.Date).Hours() / 24 / float64(len(dataPoints)-1)
			trend := TrendAnalysis{
				KPIName:    kpiName,
				Direction:  GetKPIDirection(kpiName),
				Period:     periodName(intervalDays),
				DataPoints: dataPoints,
			}

			// Calculate slope (simple linear regression)
			trend.Slope = a.calculateSlope(dataPoints)

			// Calculate correlation coefficient
			trend.Correlation = a.calculateCorrelation(dataPoints)

			// Linear extrapolation one interval past the latest point.
			trend.Forecast = latest.Value + trend.Slope*intervalDays
			// Strength of the linear fit, whichever way it slopes.
			trend.Confidence = math.Abs(trend.Correlation)

			trends = append(trends, trend)
		}
	}

	return trends
}

// relativeChange returns the change of the fitted trend line over the
// trend window (Slope × days from the first to the last data point) as a
// fraction of the first value's magnitude (of the mean magnitude when the
// first value is 0): dimensionless, so independent of the KPI's unit. ok
// is false with fewer than two data points or when every value is 0.
func (t TrendAnalysis) relativeChange() (float64, bool) {
	n := len(t.DataPoints)
	if n < 2 {
		return 0, false
	}
	first, last := t.DataPoints[0], t.DataPoints[n-1]
	days := last.Date.Sub(first.Date).Hours() / 24
	base := math.Abs(first.Value)
	if base == 0 {
		for _, p := range t.DataPoints {
			base += math.Abs(p.Value) / float64(n)
		}
	}
	if base == 0 {
		return 0, false
	}
	return t.Slope * days / base, true
}

// periodName names an assessment interval given in days.
func periodName(days float64) string {
	switch {
	case days <= 1.5:
		return "daily"
	case days <= 10:
		return "weekly"
	}
	return "monthly"
}

// calculateSlope calculates the slope of a linear trend
func (a *Assessor) calculateSlope(dataPoints []DataPoint) float64 {
	if len(dataPoints) < 2 {
		return 0
	}

	n := float64(len(dataPoints))
	sumX, sumY, sumXY, sumXX := 0.0, 0.0, 0.0, 0.0

	baseTime := dataPoints[0].Date.Unix()

	for _, point := range dataPoints {
		x := float64(point.Date.Unix()-baseTime) / (24 * 3600) // Days since first point
		y := point.Value

		sumX += x
		sumY += y
		sumXY += x * y
		sumXX += x * x
	}

	numerator := n*sumXY - sumX*sumY
	denominator := n*sumXX - sumX*sumX

	if denominator == 0 {
		return 0
	}

	return numerator / denominator
}

// calculateCorrelation calculates Pearson correlation coefficient
func (a *Assessor) calculateCorrelation(dataPoints []DataPoint) float64 {
	if len(dataPoints) < 2 {
		return 0
	}

	n := float64(len(dataPoints))
	sumX, sumY, sumXY, sumXX, sumYY := 0.0, 0.0, 0.0, 0.0, 0.0

	baseTime := dataPoints[0].Date.Unix()

	for _, point := range dataPoints {
		x := float64(point.Date.Unix()-baseTime) / (24 * 3600) // Days since first point
		y := point.Value

		sumX += x
		sumY += y
		sumXY += x * y
		sumXX += x * x
		sumYY += y * y
	}

	numerator := n*sumXY - sumX*sumY
	denominator := math.Sqrt((n*sumXX - sumX*sumX) * (n*sumYY - sumY*sumY))

	if denominator == 0 {
		return 0
	}

	return numerator / denominator
}

// generateActionItems generates action items based on KPI assessment
func (a *Assessor) generateActionItems(kpis map[string]KPIMeasure, vs *ValueStream) []ActionItem {
	items := []ActionItem{}
	now := time.Now()

	for name, kpi := range kpis {
		if kpi.Status == StatusCritical {
			item := ActionItem{
				ID: fmt.Sprintf("action_%s_%d", name, now.Unix()),
				Description: fmt.Sprintf("Address critical %s KPI (current: %.2f, target: %.2f)",
					name, kpi.Value, num(kpi.Target)),
				Priority:   1,
				AssignedTo: "VSM Coordinator",
				DueDate:    now.AddDate(0, 0, 7), // 1 week
				Status:     "open",
				CreatedAt:  now,
			}
			items = append(items, item)
		} else if kpi.Status == StatusWarning {
			item := ActionItem{
				ID: fmt.Sprintf("action_%s_%d", name, now.Unix()),
				Description: fmt.Sprintf("Improve %s KPI performance (current: %.2f, target: %.2f)",
					name, kpi.Value, num(kpi.Target)),
				Priority:   2,
				AssignedTo: "Process Owner",
				DueDate:    now.AddDate(0, 0, 14), // 2 weeks
				Status:     "open",
				CreatedAt:  now,
			}
			items = append(items, item)
		}
	}

	return items
}

// assessPDCAStatus assesses the status of PDCA cycle phases
func (a *Assessor) assessPDCAStatus(vs *ValueStream) PDCAStatus {
	status := PDCAStatus{}

	// Plan phase - completed if future state exists
	if vs.State == StateFuture {
		status.Plan.Status = "completed"
		status.Plan.CompletedAt = vs.UpdatedAt
	} else {
		status.Plan.Status = "in_progress"
	}

	// Do phase - completed if implementation has started
	status.Do.Status = "in_progress"
	status.Do.Notes = "Implementation in progress"

	// Check phase - current assessment
	status.Check.Status = "completed"
	status.Check.CompletedAt = time.Now()
	status.Check.Notes = "Regular assessment completed"

	// Act phase - continuous improvement
	status.Act.Status = "in_progress"
	status.Act.Notes = "Continuous improvement activities ongoing"

	return status
}

// generateAssessmentRecommendations generates recommendations based on assessment
func (a *Assessor) generateAssessmentRecommendations(result *AssessmentResult) []string {
	recommendations := []string{}

	// Performance-based recommendations, over the rated KPIs only.
	rated := 0
	for _, kpi := range result.KPIs {
		if kpi.Status != StatusUnrated {
			rated++
		}
	}
	if len(result.KPIs) > 0 && rated == 0 {
		recommendations = append(recommendations,
			"KPIs have no targets (or no known better direction), so performance is not rated; "+
				"set targets for the KPIs that matter.")
	} else if rated > 0 && result.PerformanceIndex < 0.6 {
		recommendations = append(recommendations,
			"Overall performance needs significant improvement. Focus on high-impact measures.")
	}

	// KPI-specific recommendations
	for name, kpi := range result.KPIs {
		switch kpi.Status {
		case StatusCritical:
			switch name {
			case "total_lead_time":
				recommendations = append(recommendations,
					"Implement flow optimization to reduce lead time by at least 30%.")
			case "value_added_ratio":
				recommendations = append(recommendations,
					"Eliminate non-value-adding activities to improve value-added ratio.")
			}
		case StatusWarning:
			recommendations = append(recommendations,
				fmt.Sprintf("Monitor and improve %s KPI performance.", name))
		}
	}

	// Trend-based recommendations: a KPI moving the wrong way for its
	// direction (falling flow efficiency, rising lead time) by more than
	// trendThreshold of its value over the trend window (see
	// TrendAnalysis.relativeChange). A relative change has no unit, so
	// the decision is the same whether lead time is in minutes or hours.
	for _, trend := range result.Trends {
		change, ok := trend.relativeChange()
		worsening := ok && ((trend.Direction == HigherIsBetter && change < -trendThreshold) ||
			(trend.Direction == LowerIsBetter && change > trendThreshold))
		if worsening {
			recommendations = append(recommendations,
				fmt.Sprintf("Address declining trend in %s KPI.", trend.KPIName))
		}
	}

	return recommendations
}

// CompareAssessments compares two assessment results. previous is
// converted to current's units first (see normalizeHistory).
func (a *Assessor) CompareAssessments(current, previous *AssessmentResult) map[string]float64 {
	comparison := make(map[string]float64)

	if current == nil || previous == nil {
		return comparison
	}
	previous = &normalizeHistory([]AssessmentResult{*previous}, current)[0]

	for name, currentKPI := range current.KPIs {
		if previousKPI, exists := previous.KPIs[name]; exists {
			change := currentKPI.Value - previousKPI.Value
			comparison[name+"_change"] = change

			if previousKPI.Value != 0 {
				changePercent := (change / previousKPI.Value) * 100
				comparison[name+"_change_percent"] = changePercent
			}
		}
	}

	return comparison
}
