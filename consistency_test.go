package vsm

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// optF is F for a test input where 0 stands for "not entered" (nil).
func optF(v float64) *float64 {
	if v == 0 {
		return nil
	}
	return F(v)
}

// fieldOf returns a getter for one resolved value.
func fieldOf(name string) func(ProcessParameters) float64 {
	return map[string]func(ProcessParameters) float64{
		"process_time":       func(p ProcessParameters) float64 { return num(p.ProcessTime) },
		"idle_time":          func(p ProcessParameters) float64 { return num(p.IdleTime) },
		"transport_time":     func(p ProcessParameters) float64 { return num(p.TransportTime) },
		"waiting_time":       func(p ProcessParameters) float64 { return num(p.WaitingTime) },
		"range_of_inventory": func(p ProcessParameters) float64 { return num(p.RangeOfInventory) },
		"processing_time":    func(p ProcessParameters) float64 { return num(p.ProcessingTime) },
		"value_adding_time":  func(p ProcessParameters) float64 { return num(p.ValueAddingTime) },
		"lead_time":          func(p ProcessParameters) float64 { return num(p.LeadTime) },
		"yield_factor":       func(p ProcessParameters) float64 { return num(p.YieldFactor) },
	}[name]
}

// TestResolveRelationships covers every enforced relationship: parts only,
// total only, both agreeing, both disagreeing (issue reported, parts used).
func TestResolveRelationships(t *testing.T) {
	min := TimeUnitMinutes
	tests := []struct {
		name      string
		params    ProcessParameters
		field     string // resolved value checked
		want      float64
		issue     string // field of the expected issue, "" for none
		issueFrom float64
	}{
		// Process time (A.2): strict, the parts sum is used when any part is given.
		{"PT parts only", ProcessParameters{ProcessingTime: F(6), SetupTime: F(2), AncillaryTime: F(1)}, "process_time", 9, "", 0},
		{"PT total only", ProcessParameters{ProcessTime: F(12)}, "process_time", 12, "", 0},
		{"PT complete parts agree", ProcessParameters{ProcessTime: F(12), ProcessingTime: F(6), SetupTime: F(2), HandlingTime: F(4)}, "process_time", 12, "", 0},
		{"PT complete parts disagree", ProcessParameters{ProcessTime: F(20), ProcessingTime: F(6), SetupTime: F(2), HandlingTime: F(4)}, "process_time", 12, "process_time", 20},
		{"PT parts fall short of total", ProcessParameters{ProcessTime: F(20), SetupTime: F(4)}, "process_time", 4, "process_time", 20},
		{"PT parts exceed total", ProcessParameters{ProcessTime: F(5), ProcessingTime: F(6), SetupTime: F(2)}, "process_time", 8, "process_time", 5},
		{"PT within 0.5% tolerance", ProcessParameters{ProcessTime: F(12.05), ProcessingTime: F(6), SetupTime: F(2), HandlingTime: F(4)}, "process_time", 12, "", 0},
		// Repetitive process time (A.6).
		{"A.6 parts only", ProcessParameters{SetupTime: F(5), RepetitionTime: F(2), RepetitionCount: I(3)}, "process_time", 11, "", 0},
		{"A.6 agrees with stored PT", ProcessParameters{ProcessTime: F(11), SetupTime: F(5), RepetitionTime: F(2), RepetitionCount: I(3)}, "process_time", 11, "", 0},
		{"A.6 disagrees with stored PT", ProcessParameters{ProcessTime: F(30), SetupTime: F(5), RepetitionTime: F(2), RepetitionCount: I(3)}, "process_time", 11, "process_time", 30},
		{"A.6 with A.2 parts above it", ProcessParameters{SetupTime: F(5), HandlingTime: F(1), ProcessingTime: F(10), RepetitionTime: F(2), RepetitionCount: I(3)}, "process_time", 11, fieldProcessParts, 16},
		{"A.6 with A.2 parts below it", ProcessParameters{SetupTime: F(5), HandlingTime: F(1), RepetitionTime: F(2), RepetitionCount: I(3)}, "process_time", 11, fieldProcessParts, 6},
		{"A.6 agrees with A.2 parts", ProcessParameters{SetupTime: F(5), ProcessingTime: F(6), RepetitionTime: F(2), RepetitionCount: I(3)}, "process_time", 11, "", 0},
		// Idle time.
		{"IT parts only", ProcessParameters{TransportTime: F(2), WaitingTime: F(3), StorageTime: F(1)}, "idle_time", 6, "", 0},
		{"IT total only", ProcessParameters{IdleTime: F(40)}, "idle_time", 40, "", 0},
		{"IT agree", ProcessParameters{IdleTime: F(6), TransportTime: F(2), WaitingTime: F(3), StorageTime: F(1)}, "idle_time", 6, "", 0},
		{"IT disagree", ProcessParameters{IdleTime: F(5), WaitingTime: F(100)}, "idle_time", 100, "idle_time", 5},
		// Transport time (A.10).
		{"TT parts only", ProcessParameters{TransportRepetitionTime: F(4), TransportRepetitionCount: I(3)}, "transport_time", 12, "", 0},
		{"TT total only", ProcessParameters{TransportTime: F(7)}, "transport_time", 7, "", 0},
		{"TT agree", ProcessParameters{TransportTime: F(12), TransportRepetitionTime: F(4), TransportRepetitionCount: I(3)}, "transport_time", 12, "", 0},
		{"TT disagree", ProcessParameters{TransportTime: F(10), TransportRepetitionTime: F(4), TransportRepetitionCount: I(3)}, "transport_time", 12, "transport_time", 10},
		{"TT feeds idle time", ProcessParameters{TransportRepetitionTime: F(4), TransportRepetitionCount: I(3), WaitingTime: F(8)}, "idle_time", 20, "", 0},
		// Waiting time and range of inventory: stock ÷ demand rate (0.5 per
		// minute below), whatever the cycle time.
		{"WT parts only", ProcessParameters{StockQuantity: F(50), CycleTime: F(7)}, "waiting_time", 100, "", 0},
		{"WT total only", ProcessParameters{WaitingTime: F(30)}, "waiting_time", 30, "", 0},
		{"WT agree", ProcessParameters{WaitingTime: F(100), StockQuantity: F(50), CycleTime: F(7)}, "waiting_time", 100, "", 0},
		{"WT disagree", ProcessParameters{WaitingTime: F(30), StockQuantity: F(50), CycleTime: F(7)}, "waiting_time", 100, "waiting_time", 30},
		{"WT not stock × cycle time", ProcessParameters{WaitingTime: F(350), StockQuantity: F(50), CycleTime: F(7)}, "waiting_time", 100, "waiting_time", 350},
		{"range disagree", ProcessParameters{RangeOfInventory: F(60), StockQuantity: F(50), CycleTime: F(7)}, "range_of_inventory", 100, "range_of_inventory", 60},
		// Processing time (A.5).
		{"PRT parts only", ProcessParameters{CycleTime: F(2), OrderQuantity: F(10), ResourceCount: I(2)}, "processing_time", 10, "", 0},
		{"PRT total only", ProcessParameters{ProcessingTime: F(7)}, "processing_time", 7, "", 0},
		{"PRT agree", ProcessParameters{ProcessingTime: F(10), CycleTime: F(2), OrderQuantity: F(10), ResourceCount: I(2)}, "processing_time", 10, "", 0},
		{"PRT disagree", ProcessParameters{ProcessingTime: F(15), CycleTime: F(2), OrderQuantity: F(10), ResourceCount: I(2)}, "processing_time", 10, "processing_time", 15},
		{"PRT feeds process time", ProcessParameters{CycleTime: F(2), OrderQuantity: F(10), ResourceCount: I(2), SetupTime: F(5)}, "process_time", 15, "", 0},
		// Value adding time (VATp, A.9).
		{"VATp only", ProcessParameters{ValueAddingTimeProcess: F(4)}, "value_adding_time", 4, "", 0},
		{"VAT only", ProcessParameters{ValueAddingTime: F(4)}, "value_adding_time", 4, "", 0},
		{"VAT agree", ProcessParameters{ValueAddingTime: F(4), ValueAddingTimeProcess: F(4)}, "value_adding_time", 4, "", 0},
		{"VAT disagree", ProcessParameters{ValueAddingTime: F(9), ValueAddingTimeProcess: F(4)}, "value_adding_time", 4, "value_adding_time", 9},
		{"VAT above process time", ProcessParameters{ProcessTime: F(3), ValueAddingTime: F(5)}, "value_adding_time", 5, "value_adding_time", 5},
		// Lead time: PT + IT, classified as VAT + NVAT + NNVAT (A.3).
		{"LT flow only", ProcessParameters{ProcessTime: F(10), IdleTime: F(30)}, "lead_time", 40, "", 0},
		{"LT classification only", ProcessParameters{ValueAddingTime: F(5), NonValueAddingTime: F(30), NecessaryNonValueAddingTime: F(5)}, "lead_time", 40, "", 0},
		{"LT stored only", ProcessParameters{LeadTime: F(40)}, "lead_time", 40, "", 0},
		{"LT flow and classification agree", ProcessParameters{ProcessTime: F(10), IdleTime: F(30), ValueAddingTime: F(5), NonValueAddingTime: F(30), NecessaryNonValueAddingTime: F(5)}, "lead_time", 40, "", 0},
		{"LT classification disagrees", ProcessParameters{ProcessTime: F(10), IdleTime: F(30), ValueAddingTime: F(5), NonValueAddingTime: F(5)}, "lead_time", 40, fieldClassSum, 10},
		{"LT stored disagrees", ProcessParameters{ProcessTime: F(10), IdleTime: F(30), LeadTime: F(25)}, "lead_time", 40, "lead_time", 25},
		{"LT VAT only (rest unclassified)", ProcessParameters{ProcessTime: F(10), IdleTime: F(30), ValueAddingTime: F(5)}, "lead_time", 40, "", 0},
		// Yield (A.17).
		{"YF parts only", ProcessParameters{ScrapRate: F(0.02), ReworkRate: F(0.03)}, "yield_factor", 0.95, "", 0},
		{"YF total only", ProcessParameters{YieldFactor: F(0.9)}, "yield_factor", 0.9, "", 0},
		{"YF agree", ProcessParameters{YieldFactor: F(0.95), ScrapRate: F(0.02), ReworkRate: F(0.03)}, "yield_factor", 0.95, "", 0},
		{"YF disagree", ProcessParameters{YieldFactor: F(0.99), ScrapRate: F(0.02), ReworkRate: F(0.03)}, "yield_factor", 0.95, "yield_factor", 0.99},
	}
	c := NewCalculator()
	// 720 per day = 0.5 per minute.
	ctx := streamContext{customer: &Customer{Demand: F(720), DemandPeriod: DemandPeriodDay}, unit: min}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.params.TimeUnit = min
			got, issues := c.resolveParameters(tt.params, ctx)
			if v := fieldOf(tt.field)(got); !approx(v, tt.want) {
				t.Errorf("%s = %v, want %v", tt.field, v, tt.want)
			}
			if tt.issue == "" {
				if len(issues) != 0 {
					t.Errorf("unexpected issues: %+v", issues)
				}
				return
			}
			if len(issues) != 1 {
				t.Fatalf("got %d issues, want 1 on %s: %+v", len(issues), tt.issue, issues)
			}
			if is := issues[0]; is.Field != tt.issue || !approx(is.StoredValue, tt.issueFrom) || is.Message == "" || is.Rule == "" {
				t.Errorf("issue = %+v, want field %s stored %v", is, tt.issue, tt.issueFrom)
			}
		})
	}
}

// TestProcessTimeShortfallMessage: a stored process time larger than its
// parts is reported in plain terms, naming the parts that were given.
func TestProcessTimeShortfallMessage(t *testing.T) {
	_, issues := NewCalculator().resolveParameters(ProcessParameters{ProcessTime: F(20), SetupTime: F(4)}, streamContext{})
	if len(issues) != 1 {
		t.Fatalf("got %+v, want one issue", issues)
	}
	want := "process_time: stored 20, but its parts sum to 4 (only setup_time given) — enter the missing parts, " +
		"or clear the given parts to use the stored total; calculations use 4"
	if issues[0].Message != want {
		t.Errorf("message = %q\nwant      %q", issues[0].Message, want)
	}
	_, issues = NewCalculator().resolveParameters(ProcessParameters{ProcessTime: F(5), ProcessingTime: F(6), SetupTime: F(2)}, streamContext{})
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "correct the parts or the stored value") ||
		!strings.Contains(issues[0].Message, "processing_time + setup_time") {
		t.Errorf("excess issue = %+v", issues)
	}
}

func TestCheckConsistencyNamesProcessAndUnit(t *testing.T) {
	vs := &ValueStream{ID: "vs", Processes: []Process{
		{ID: "ok", Parameters: ProcessParameters{TimeUnit: TimeUnitMinutes, ProcessTime: F(2), IdleTime: F(3)}},
		{ID: "bad", Parameters: ProcessParameters{TimeUnit: TimeUnitSeconds, ProcessTime: F(600), ProcessingTime: F(30), SetupTime: F(30), HandlingTime: F(60)}},
	}}
	issues := contradictions(CheckConsistency(vs))
	if len(issues) != 1 {
		t.Fatalf("got %+v, want one issue", issues)
	}
	is := issues[0]
	// Values are reported in the process's own unit (seconds), not converted.
	if is.Element != "process" || is.ID != "bad" || is.Field != "process_time" ||
		is.StoredValue != 600 || is.ComputedValue != 120 || is.Unit != "seconds" {
		t.Errorf("issue = %+v", is)
	}
	if !strings.Contains(is.Message, "process bad") || !strings.Contains(is.Message, "process_time") {
		t.Errorf("message %q does not name the process and field", is.Message)
	}
}

// TestCheckConsistencyMixedUnits: per-process checks are unit-free, and
// the stream totals use the computed (component) values after conversion.
func TestCheckConsistencyMixedUnits(t *testing.T) {
	vs := &ValueStream{ID: "vs", Processes: []Process{
		// 1 h stored, parts 30 + 15 min: contradiction, 45 min used.
		{ID: "p1", Parameters: ProcessParameters{TimeUnit: TimeUnitMinutes, IdleTime: F(60), WaitingTime: F(30), TransportTime: F(15), ProcessTime: F(15)}},
		// 0.25 h process + 0.5 h idle, consistent.
		{ID: "p2", Parameters: ProcessParameters{TimeUnit: TimeUnitHours, ProcessTime: F(0.25), IdleTime: F(0.5), WaitingTime: F(0.5)}},
	}}
	issues := contradictions(CheckConsistency(vs))
	if len(issues) != 1 || issues[0].ID != "p1" || issues[0].Field != "idle_time" || issues[0].ComputedValue != 45 {
		t.Fatalf("issues = %+v", issues)
	}
	// Lead time in minutes: (15 + 45) + (15 + 30) = 105.
	if lt := NewCalculator().CalculateOverallLeadTime(vs.Processes); !approx(lt, 105) {
		t.Errorf("lead time = %v, want 105", lt)
	}
}

// TestWholeStreamLeadTimeConsistency: lead time = Σ(process + idle time)
// across the stream, and the value classification of the same time sums to
// the same lead time, so flow efficiency is the same either way.
func TestWholeStreamLeadTimeConsistency(t *testing.T) {
	// 720 per day = 0.5 per minute, so 10 in stock wait 20 minutes.
	vs := &ValueStream{ID: "vs", Name: "LT", Customer: &Customer{ID: "c", Demand: F(720), DemandPeriod: DemandPeriodDay}, Processes: []Process{
		{ID: "p1", Parameters: ProcessParameters{TimeUnit: TimeUnitMinutes,
			ProcessingTime: F(6), SetupTime: F(4), StockQuantity: F(10), CycleTime: F(2), TransportTime: F(5),
			ValueAddingTime: F(6), NonValueAddingTime: F(25), NecessaryNonValueAddingTime: F(4)}},
		{ID: "p2", Parameters: ProcessParameters{TimeUnit: TimeUnitHours,
			ProcessTime: F(0.5), IdleTime: F(1), ValueAddingTimeProcess: F(0.25), NonValueAddingTime: F(1.25)}},
	}}
	if issues := contradictions(CheckConsistency(vs)); len(issues) != 0 {
		t.Fatalf("unexpected issues: %+v", issues)
	}
	c := NewCalculator()
	// p1: PT 10 + IT (20 + 5) = 35 min; p2: 1.5 h = 90 min.
	lt := c.CalculateKPIs(vs)["total_lead_time"]
	if !approx(lt, 125) {
		t.Errorf("lead time = %v, want 125", lt)
	}
	var classified float64
	for _, p := range vs.Processes {
		r, _ := c.resolvedIn(p.Parameters, c.contextOf(vs))
		classified += c.CalculateLeadTime(r)
	}
	if !approx(classified, lt) {
		t.Errorf("VAT + NVAT + NNVAT over the stream = %v, want lead time %v", classified, lt)
	}
	kpis := c.CalculateKPIs(vs)
	if !approx(kpis["value_added_ratio"], 21.0/125) || !approx(kpis["process_efficiency"], 40.0/125) {
		t.Errorf("kpis = %v", kpis)
	}

	// Break the classification of p2: reported, lead time unchanged.
	vs.Processes[1].Parameters.NonValueAddingTime = F(2)
	issues := contradictions(CheckConsistency(vs))
	if len(issues) != 1 || issues[0].ID != "p2" || issues[0].Field != fieldClassSum {
		t.Fatalf("issues = %+v", issues)
	}
	if got := c.CalculateKPIs(vs)["total_lead_time"]; !approx(got, 125) {
		t.Errorf("lead time = %v, want 125 (flow wins)", got)
	}
}

func TestCheckConsistencyCustomerAndStoredKPIs(t *testing.T) {
	vs := &ValueStream{ID: "vs",
		Customer:  &Customer{ID: "c", Demand: F(200), AvailableTime: F(480), TaktTime: F(3)},
		Processes: []Process{{ID: "p", Parameters: ProcessParameters{TimeUnit: TimeUnitMinutes, ProcessTime: F(2), IdleTime: F(8)}}},
		KPIs:      map[string]float64{"total_lead_time": 12, "made_up_kpi": 1},
	}
	issues := contradictions(CheckConsistency(vs))
	if len(issues) != 2 {
		t.Fatalf("issues = %+v", issues)
	}
	if is := issues[0]; is.Element != "customer" || is.Field != "takt_time" || is.StoredValue != 3 || is.ComputedValue != 2.4 {
		t.Errorf("takt issue = %+v", is)
	}
	if is := issues[1]; is.Element != "value_stream" || is.Field != "kpis.total_lead_time" || is.ComputedValue != 10 || is.Unit != "minutes" {
		t.Errorf("kpi issue = %+v", is)
	}
	vs.Customer.TaktTime, vs.KPIs = F(2.4), NewCalculator().CalculateKPIs(vs)
	if issues := contradictions(CheckConsistency(vs)); len(issues) != 0 {
		t.Errorf("consistent stream gave %+v", issues)
	}
}

func TestContradictionsSurfaceInResults(t *testing.T) {
	vs := targetStream()
	vs.Processes[0].Parameters.IdleTime = F(99) // parts say 30
	analysis, err := NewAnalyzer().AnalyzeCurrentState(vs)
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.ConsistencyIssues) != 1 || analysis.ConsistencyIssues[0].Field != "idle_time" {
		t.Errorf("analysis issues = %+v", analysis.ConsistencyIssues)
	}
	// The analysis uses the parts (30), not the stored 99.
	if got := analysis.IdleTimeAnalysis.IdleTimeByProcess["p1"]; got != 30 {
		t.Errorf("p1 idle = %v, want 30", got)
	}
	data, _ := json.Marshal(analysis)
	if !strings.Contains(string(data), `"consistency_issues"`) {
		t.Error("analysis JSON lacks consistency_issues")
	}

	design, err := NewDesigner().DesignFutureState(vs, analysis)
	if err != nil {
		t.Fatal(err)
	}
	if len(design.ConsistencyIssues) != 1 {
		t.Errorf("design issues = %+v", design.ConsistencyIssues)
	}
	// The future state stores the computed value, so it is consistent.
	if issues := contradictions(CheckConsistency(design.FutureState)); len(issues) != 0 {
		t.Errorf("future state issues = %+v", issues)
	}
	assessment, err := NewAssessor().AssessValueStream(vs, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(assessment.ConsistencyIssues) != 1 {
		t.Errorf("assessment issues = %+v", assessment.ConsistencyIssues)
	}

	// A consistent stream: the field is left out of the JSON.
	clean, _ := NewAnalyzer().AnalyzeCurrentState(targetStream())
	data, _ = json.Marshal(clean)
	if strings.Contains(string(data), "consistency_issues") {
		t.Error("consistency_issues present for a consistent stream")
	}
}

func TestDeserializeAndLoadReportIssues(t *testing.T) {
	data := []byte(`{"id":"vs","name":"Old","state":"current","product_family":"x",
		"processes":[{"id":"p","name":"P","type":"material","parameters":{"process_time":1,"idle_time":5,"waiting_time":7}}],
		"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","version":"1"}`)
	s := NewSerializer()
	vs, issues, err := s.DeserializeValueStreamWithIssues(data)
	if err != nil {
		t.Fatal(err)
	}
	issues = contradictions(issues)
	if len(issues) != 1 || issues[0].Field != "idle_time" || len(contradictions(vs.ConsistencyIssues)) != 1 {
		t.Fatalf("issues = %+v / %+v", issues, vs.ConsistencyIssues)
	}
	plain, err := s.DeserializeValueStream(data)
	if err != nil || len(contradictions(plain.ConsistencyIssues)) != 1 {
		t.Fatalf("DeserializeValueStream: %v, %+v", err, plain)
	}

	path := filepath.Join(t.TempDir(), "vs.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	var loaded ValueStream
	if err := s.LoadFromFile(path, &loaded); err != nil {
		t.Fatal(err)
	}
	if len(contradictions(loaded.ConsistencyIssues)) != 1 {
		t.Errorf("LoadFromFile issues = %+v", loaded.ConsistencyIssues)
	}
	_, fileIssues, err := s.LoadValueStreamFromFile(path)
	if err != nil || len(contradictions(fileIssues)) != 1 {
		t.Errorf("LoadValueStreamFromFile: %v, %+v", err, fileIssues)
	}

	// A stale issues list in the file is replaced on load.
	vs.ConsistencyIssues = append(vs.ConsistencyIssues, ConsistencyIssue{Field: "stale"})
	vs.Processes[0].Parameters.IdleTime = F(7)
	if err := s.SaveToFile(vs, path); err != nil {
		t.Fatal(err)
	}
	var reloaded ValueStream
	if err := s.LoadFromFile(path, &reloaded); err != nil {
		t.Fatal(err)
	}
	if len(contradictions(reloaded.ConsistencyIssues)) != 0 {
		t.Errorf("reloaded issues = %+v", reloaded.ConsistencyIssues)
	}
}

func TestIdealStateIsConsistent(t *testing.T) {
	vs := targetStream()
	vs.Processes = append(vs.Processes, Process{ID: "p3", Name: "Batch", Parameters: ProcessParameters{
		TimeUnit: TimeUnitMinutes, CycleTime: F(2), OrderQuantity: F(10), ResourceCount: I(1), SetupTime: F(5),
		// 5 in stock at 100 per day wait 72 min: lead time 25 + 6 + 72 = 103.
		StockQuantity: F(5), TransportRepetitionTime: F(3), TransportRepetitionCount: I(2),
		ValueAddingTime: F(12), NonValueAddingTime: F(91), ScrapRate: F(0.1),
	}})
	if issues := contradictions(CheckConsistency(vs)); len(issues) != 0 {
		t.Fatalf("test stream inconsistent: %+v", issues)
	}
	ideal, err := NewDesigner().CreateIdealState(vs)
	if err != nil {
		t.Fatal(err)
	}
	if issues := contradictions(CheckConsistency(ideal)); len(issues) != 0 {
		t.Errorf("ideal state issues: %+v", issues)
	}
	// Lead time = value adding time only: 5 + 4 + 12.
	if lt := ideal.KPIs["total_lead_time"]; !approx(lt, 21) {
		t.Errorf("ideal lead time = %v, want 21", lt)
	}
}

// TestInventoryTimeIsStockOverDemand: the waiting time of stock is how long
// it lasts at the customer's demand (standard VSM inventory time), not stock
// × cycle time. Here cycle time (60 s) differs from takt (240 s), so the
// two disagree: 240 in stock at 120 per day last 2 days, not 4 hours.
func TestInventoryTimeIsStockOverDemand(t *testing.T) {
	const twoDays = 2 * 86400.0
	stream := func(stored float64) *ValueStream {
		return &ValueStream{ID: "vs", Name: "Coverage",
			Customer: &Customer{ID: "c", Demand: F(120), DemandPeriod: DemandPeriodDay, AvailableTime: F(28800)},
			Processes: []Process{{ID: "p", Parameters: ProcessParameters{
				TimeUnit: TimeUnitSeconds, ProcessTime: F(60), CycleTime: F(60), StockQuantity: F(240), WaitingTime: optF(stored),
			}}},
			// The same stock as the process's stock_quantity: counted once.
			Inventory: []Inventory{{ID: "i", Quantity: F(240), Range: F(twoDays), ProcessID: "p"}},
		}
	}
	c := NewCalculator()

	// A correct stored 2-day waiting time is consistent.
	vs := stream(twoDays)
	if issues := contradictions(CheckConsistency(vs)); len(issues) != 0 {
		t.Errorf("correct 2-day waiting time flagged: %+v", issues)
	}
	// Not stored: derived as 2 days, and lead time includes it.
	vs = stream(0)
	if wt := num(c.ResolveParametersInStream(vs.Processes[0].Parameters, vs).WaitingTime); !approx(wt, twoDays) {
		t.Errorf("derived waiting time = %v s, want %v", wt, twoDays)
	}
	if lt := c.CalculateKPIs(vs)["total_lead_time"]; !approx(lt, 60+twoDays) {
		t.Errorf("lead time = %v, want %v", lt, 60+twoDays)
	}
	// Stock × cycle time (14,400 s) stored as waiting time is a contradiction.
	vs = stream(240 * 60)
	issues := contradictions(CheckConsistency(vs))
	if len(issues) != 1 || issues[0].Field != "waiting_time" || !approx(issues[0].ComputedValue, twoDays) {
		t.Errorf("issues = %+v, want one waiting_time issue computing %v", issues, twoDays)
	}
	// A wrong inventory range is reported too.
	vs = stream(twoDays)
	vs.Inventory[0].Range = F(240 * 60)
	issues = contradictions(CheckConsistency(vs))
	if len(issues) != 1 || issues[0].Element != "inventory" || !approx(issues[0].ComputedValue, twoDays) {
		t.Errorf("inventory issues = %+v", issues)
	}

	// Demand rate unknown (no customer, or a month period with no fixed
	// length): nothing is derived from demand. The linked inventory's
	// 2-day range is still the same stock's time as the process's stored
	// 4-hour waiting time, so that contradiction is the only one reported.
	for name, cust := range map[string]*Customer{
		"no customer":  nil,
		"month period": {ID: "c", Demand: F(2400), DemandPeriod: DemandPeriodMonth},
		"no period":    {ID: "c", Demand: F(120)},
	} {
		vs = stream(240 * 60)
		vs.Customer = cust
		if issues := contradictions(CheckConsistency(vs)); len(issues) != 1 || issues[0].Element != "inventory" ||
			issues[0].Field != "range" || !approx(issues[0].ComputedValue, 240*60) {
			t.Errorf("%s: issues = %+v, want one inventory range issue", name, issues)
		}
		vs = stream(twoDays)
		vs.Customer = cust
		if issues := contradictions(CheckConsistency(vs)); len(issues) != 0 {
			t.Errorf("%s: agreeing range flagged: %+v", name, issues)
		}
		vs = stream(0)
		vs.Customer = cust
		if wt := num(c.ResolveParametersInStream(vs.Processes[0].Parameters, vs).WaitingTime); wt != 0 {
			t.Errorf("%s: waiting time derived as %v", name, wt)
		}
	}
}

func TestCalculateInventoryTimeAndQuantity(t *testing.T) {
	c := NewCalculator()
	if got, ok := c.CalculateInventoryTime(240, 120, DemandPeriodDay, TimeUnitHours); !ok || !approx(got, 48) {
		t.Errorf("inventory time = %v %v, want 48 h", got, ok)
	}
	if got, ok := c.CalculateInventoryTime(70, 35, DemandPeriodWeek, TimeUnitDays); !ok || !approx(got, 14) {
		t.Errorf("weekly inventory time = %v %v, want 14 days", got, ok)
	}
	if got, ok := c.CalculateInventoryQuantity(48, TimeUnitHours, 120, DemandPeriodDay); !ok || !approx(got, 240) {
		t.Errorf("inventory quantity = %v %v, want 240", got, ok)
	}
	if _, ok := c.CalculateInventoryTime(240, 120, DemandPeriodYear, TimeUnitHours); ok {
		t.Error("year period has no fixed length; want ok = false")
	}
}

// Scrap 0.8 + rework 0.4 is impossible (more than every part is defective):
// it is rejected by validation, reported by CheckConsistency, and the
// process's yield is treated as unknown instead of −0.2.
func TestDefectFractionAboveOne(t *testing.T) {
	c := NewCalculator()
	bad := ProcessParameters{ProcessTime: F(10), ScrapRate: F(0.8), ReworkRate: F(0.4), TimeUnit: TimeUnitMinutes}

	found := false
	for _, err := range c.ValidateProcessParameters(bad) {
		if strings.Contains(err.Error(), "scrap rate + rework rate") {
			found = true
		}
	}
	if !found {
		t.Errorf("ValidateProcessParameters did not reject scrap + rework = 1.2")
	}

	vs := &ValueStream{Processes: []Process{
		{ID: "bad", Parameters: bad},
		{ID: "good", Parameters: ProcessParameters{ProcessTime: F(10), YieldFactor: F(0.9), TimeUnit: TimeUnitMinutes}},
	}}
	issues := contradictions(CheckConsistency(vs))
	if len(issues) != 1 || issues[0].ID != "bad" || issues[0].Field != fieldDefects {
		t.Fatalf("issues = %+v, want one %q issue for process bad", issues, fieldDefects)
	}
	if issues[0].Unit != "" {
		t.Errorf("defect issue unit = %q, want none (rates are fractions)", issues[0].Unit)
	}

	kpis := c.CalculateKPIs(vs)
	if got := kpis["average_yield_factor"]; !approx(got, 0.9) {
		t.Errorf("average_yield_factor = %v, want 0.9 (impossible yield left out)", got)
	}
	if y := num(c.ResolveParameters(bad).YieldFactor); y < 0 {
		t.Errorf("resolved yield = %v, must not be negative", y)
	}
}

// With 5 working days a week and demand per working day, 200 in stock at
// 100 per day is 2 working days of cover = 2 × 7 ÷ 5 = 2.8 calendar days.
// The same rule applies to process stock, inventory points, targets and the
// exported map; weekly demand is unaffected (a week is 7 calendar days).
func TestWorkingDaysPerWeek(t *testing.T) {
	c := NewCalculator()
	cust := &Customer{ID: "c", Demand: F(100), DemandPeriod: DemandPeriodDay, WorkingDaysPerWeek: F(5)}
	if got, ok := c.CalculateCustomerInventoryTime(200, cust, TimeUnitDays); !ok || !approx(got, 2.8) {
		t.Errorf("inventory time = %v %v, want 2.8 days", got, ok)
	}
	if got, ok := c.CalculateCustomerInventoryQuantity(2.8, TimeUnitDays, cust); !ok || !approx(got, 200) {
		t.Errorf("inventory quantity = %v %v, want 200", got, ok)
	}
	weekly := &Customer{ID: "c", Demand: F(500), DemandPeriod: DemandPeriodWeek, WorkingDaysPerWeek: F(5)}
	if got, ok := c.CalculateCustomerInventoryTime(500, weekly, TimeUnitDays); !ok || !approx(got, 7) {
		t.Errorf("weekly inventory time = %v %v, want 7 days", got, ok)
	}
	// Absent = 7 working days: the calendar day rule.
	if got, ok := c.CalculateCustomerInventoryTime(200, &Customer{Demand: F(100), DemandPeriod: DemandPeriodDay}, TimeUnitDays); !ok || !approx(got, 2) {
		t.Errorf("default inventory time = %v %v, want 2 days", got, ok)
	}

	vs := &ValueStream{ID: "vs", Name: "wd", Customer: cust,
		Processes: []Process{{ID: "p", Name: "p", Parameters: ProcessParameters{
			ProcessTime: F(1), StockQuantity: F(200), TimeUnit: TimeUnitDays}}},
		Inventory: []Inventory{{ID: "i", Name: "i", Quantity: F(100)}},
	}
	if wt := num(c.ResolveParametersInStream(vs.Processes[0].Parameters, vs).WaitingTime); !approx(wt, 2.8) {
		t.Errorf("process waiting time = %v days, want 2.8", wt)
	}
	if r, ok := c.contextOf(vs).inventoryRange(vs.Inventory[0]); !ok || !approx(r, 1.4) {
		t.Errorf("inventory range = %v %v, want 1.4 days", r, ok)
	}
	future, err := NewDesigner().DesignFutureStateFromTargets(vs, &FutureStateTargets{
		Default:          &ProcessTargets{WaitingTime: AbsoluteTarget(1.4)},
		InventoryDefault: &InventoryTargets{Range: AbsoluteTarget(2.8)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if q := num(future.Processes[0].Parameters.StockQuantity); !approx(q, 100) {
		t.Errorf("future stock = %v, want 100 (1.4 calendar days = 1 working day)", q)
	}
	if q := num(future.Inventory[0].Quantity); !approx(q, 200) {
		t.Errorf("future inventory = %v, want 200", q)
	}
	text, err := NewSerializer().ExportValueStreamMap(vs)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "5 working days/week") || !strings.Contains(text, "1.40 days range") {
		t.Errorf("export missing working days or calendar range:\n%s", text)
	}

	// Outside 1–7 is invalid: reported, and nothing is derived from it.
	bad := *vs
	bad.Customer = &Customer{ID: "c", Demand: F(100), DemandPeriod: DemandPeriodDay, WorkingDaysPerWeek: F(8)}
	issues := contradictions(CheckConsistency(&bad))
	if len(issues) != 1 || issues[0].Field != "working_days_per_week" {
		t.Errorf("issues = %+v, want one working_days_per_week issue", issues)
	}
	if err := NewValidator().ValidateValueStream(&bad); err == nil {
		t.Errorf("validator accepted working_days_per_week = 8")
	}
}

// invStream is one 10-minute process (all value adding) and 200 units of
// stock at a customer demand of 100 per day (2 days = 2880 min of cover).
func invStream() *ValueStream {
	return &ValueStream{ID: "vs", Name: "inv",
		Customer: &Customer{ID: "c", Demand: F(100), DemandPeriod: DemandPeriodDay},
		Processes: []Process{{ID: "p", Name: "p", Parameters: ProcessParameters{
			ProcessTime: F(10), ValueAddingTime: F(10), TimeUnit: TimeUnitMinutes}}},
		Inventory: []Inventory{{ID: "i", Name: "i", Quantity: F(200)}},
	}
}

// An inventory point only in ValueStream.Inventory is part of the lead time
// (standard VSM: lead time = process times + inventory times).
func TestLeadTimeIncludesInventoryPoints(t *testing.T) {
	c := NewCalculator()
	vs := invStream()
	kpis := c.CalculateKPIs(vs)
	if lt := kpis["total_lead_time"]; !approx(lt, 2890) {
		t.Errorf("total_lead_time = %v min, want 10 + 2880", lt)
	}
	if v := kpis["value_added_ratio"]; !approx(v, 10.0/2890) {
		t.Errorf("value_added_ratio = %v, want %v", v, 10.0/2890)
	}
	idle, _ := NewAnalyzer().AnalyzeIdleTimes(vs)
	if !approx(val(idle.TotalIdleTime), 2880) || !approx(idle.IdleTimeByInventory["i"], 2880) {
		t.Errorf("idle = %v (by inventory %v), want 2880", val(idle.TotalIdleTime), idle.IdleTimeByInventory)
	}
	// Rate unknown: the stored range (reference unit) is used.
	vs.Customer = nil
	vs.Inventory[0].Range = F(600)
	if lt := c.CalculateKPIs(vs)["total_lead_time"]; !approx(lt, 610) {
		t.Errorf("rate unknown: total_lead_time = %v, want 10 + stored range 600", lt)
	}
	// Cutting the inventory in the future state cuts the lead time.
	future, err := NewDesigner().DesignFutureStateFromTargets(invStream(), &FutureStateTargets{
		InventoryDefault: &InventoryTargets{Quantity: ReductionTarget(50)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if lt := future.KPIs["total_lead_time"]; !approx(lt, 10+1440) {
		t.Errorf("future total_lead_time = %v, want 1450", lt)
	}
}

// When an inventory is the same stock as a process's stock_quantity
// (process_id links them), it is counted once, through the process.
func TestLinkedInventoryNotDoubleCounted(t *testing.T) {
	c := NewCalculator()
	vs := invStream()
	vs.Processes[0].Parameters.StockQuantity = F(200)
	vs.Inventory[0].ProcessID = "p"
	if lt := c.CalculateKPIs(vs)["total_lead_time"]; !approx(lt, 2890) {
		t.Errorf("linked: total_lead_time = %v, want 2890 (stock counted once)", lt)
	}
	if issues := contradictions(CheckConsistency(vs)); len(issues) != 0 {
		t.Errorf("linked and equal: issues %+v", issues)
	}
	// Without the link both are counted: they are two different stocks.
	vs.Inventory[0].ProcessID = ""
	if lt := c.CalculateKPIs(vs)["total_lead_time"]; !approx(lt, 10+2*2880) {
		t.Errorf("unlinked: total_lead_time = %v, want two stocks", lt)
	}
	// A link that disagrees with the process's stock is reported.
	vs.Inventory[0].ProcessID = "p"
	vs.Inventory[0].Quantity = F(150)
	issues := contradictions(CheckConsistency(vs))
	if len(issues) != 1 || issues[0].Element != "inventory" || issues[0].Field != "quantity" || issues[0].ComputedValue != 200 {
		t.Errorf("issues = %+v, want one inventory quantity issue (process has 200)", issues)
	}
	// A future stock target on the process moves the linked inventory too.
	vs.Inventory[0].Quantity = F(200)
	future, err := NewDesigner().DesignFutureStateFromTargets(vs, &FutureStateTargets{
		Default: &ProcessTargets{StockQuantity: ReductionTarget(50)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if inv := future.Inventory[0]; inv.ProcessID != "p_future" || !approx(num(inv.Quantity), 100) {
		t.Errorf("future inventory = %+v, want 100 linked to p_future", inv)
	}
	if issues := contradictions(CheckConsistency(future)); len(issues) != 0 {
		t.Errorf("future issues: %+v", issues)
	}
	if lt := future.KPIs["total_lead_time"]; !approx(lt, 10+1440) {
		t.Errorf("future total_lead_time = %v, want 1450", lt)
	}
	// Targeting the linked inventory directly is an error.
	_, err = NewDesigner().DesignFutureStateFromTargets(vs, &FutureStateTargets{
		Inventory: map[string]InventoryTargets{"i": {Quantity: ReductionTarget(50)}},
	})
	if err == nil {
		t.Errorf("target on linked inventory accepted")
	}
}

// linkStream: demand 100/day; process p (minutes) with process time 10 and
// stock; optionally one inventory.
func linkStream(stock float64, inv ...Inventory) *ValueStream {
	return &ValueStream{ID: "vs", Name: "Link",
		Customer: &Customer{ID: "c", Demand: F(100), DemandPeriod: DemandPeriodDay},
		Processes: []Process{{ID: "p", Parameters: ProcessParameters{
			TimeUnit: TimeUnitMinutes, ProcessTime: F(10), StockQuantity: F(stock)}}},
		Inventory: inv,
	}
}

// Inventory turns count the same stock the lead time counts: each process's
// stock once, a linked inventory through its process, and unlinked
// inventories on their own.
func TestInventoryTurnsCountStockOnce(t *testing.T) {
	c := NewCalculator()
	for name, tc := range map[string]struct {
		vs    *ValueStream
		turns float64
	}{
		// Lead time and CheckConsistency use the process's 200; so do turns.
		"linked, quantities differ": {linkStream(200, Inventory{ID: "i", Quantity: F(100), ProcessID: "p"}), 0.5},
		"process only":              {linkStream(200), 0.5},
		"unlinked quantity":         {linkStream(200, Inventory{ID: "i", Quantity: F(100)}), 100.0 / 300},
		// Range alone: 1440 min at 100/day is 100 units.
		"unlinked range only": {linkStream(200, Inventory{ID: "i", Range: F(1440)}), 100.0 / 300},
	} {
		if got, ok := c.CalculateKPIs(tc.vs)["inventory_turns"]; !ok || !approx(got, tc.turns) {
			t.Errorf("%s: inventory_turns = %v (present %v), want %v", name, got, ok, tc.turns)
		}
	}
	vs := linkStream(200, Inventory{ID: "i", Quantity: F(100), ProcessID: "p"})
	if lt := c.CalculateKPIs(vs)["total_lead_time"]; !approx(lt, 10+2880) {
		t.Errorf("linked: total_lead_time = %v, want 2890 (the process's 200)", lt)
	}
	// Range alone without a demand rate: its stock is unknown, so turns are.
	vs = linkStream(200, Inventory{ID: "i", Range: F(1440)})
	vs.Customer.DemandPeriod = DemandPeriodMonth
	if got, ok := c.CalculateKPIs(vs)["inventory_turns"]; ok {
		t.Errorf("unknown stock: inventory_turns = %v, want omitted", got)
	}
}

// A linked inventory's time is skipped only when its process accounts for
// it. Without a demand rate the process cannot derive its waiting time from
// stock, so the inventory's stored range is the only measure and counts.
func TestLinkedInventoryKeepsOnlyKnownTime(t *testing.T) {
	c := NewCalculator()
	vs := linkStream(100, Inventory{ID: "i", Quantity: F(100), Range: F(1440), ProcessID: "p"})
	vs.Customer = nil
	if lt := c.CalculateKPIs(vs)["total_lead_time"]; !approx(lt, 10+1440) {
		t.Errorf("total_lead_time = %v, want 1450", lt)
	}
	if issues := contradictions(CheckConsistency(vs)); len(issues) != 0 {
		t.Errorf("issues = %+v, want none", issues)
	}
	// With the process's waiting time stored, it is counted once, through
	// the process.
	vs.Processes[0].Parameters.WaitingTime = F(1440)
	if lt := c.CalculateKPIs(vs)["total_lead_time"]; !approx(lt, 10+1440) {
		t.Errorf("waiting stored: total_lead_time = %v, want 1450", lt)
	}
	if issues := contradictions(CheckConsistency(vs)); len(issues) != 0 {
		t.Errorf("waiting stored: issues = %+v, want none", issues)
	}
}

// An entered zero yield that scrap and rework contradict is reported, not
// silently replaced.
func TestExplicitZeroYieldCompared(t *testing.T) {
	vs := &ValueStream{ID: "vs", Processes: []Process{{ID: "p", Parameters: ProcessParameters{
		ProcessTime: F(10), YieldFactor: F(0), ScrapRate: F(0.1)}}}}
	issues := contradictions(CheckConsistency(vs))
	if len(issues) != 1 || issues[0].Field != "yield_factor" || issues[0].StoredValue != 0 ||
		!approx(issues[0].ComputedValue, 0.9) {
		t.Errorf("issues = %+v, want one yield_factor issue (stored 0, computed 0.9)", issues)
	}
	// Not entered: no issue.
	vs.Processes[0].Parameters.YieldFactor = nil
	if issues := contradictions(CheckConsistency(vs)); len(issues) != 0 {
		t.Errorf("yield not entered flagged: %+v", issues)
	}
}

// Scrap + rework of 1.004 is within the stored-vs-computed tolerance but is
// still impossible: the bound is mathematical, not a measurement tolerance.
func TestDefectsJustAboveOneImpossible(t *testing.T) {
	vs := &ValueStream{ID: "vs", Processes: []Process{{ID: "p", Parameters: ProcessParameters{
		ProcessTime: F(10), ScrapRate: F(0.6), ReworkRate: F(0.404)}}}}
	issues := contradictions(CheckConsistency(vs))
	if len(issues) != 1 || issues[0].Field != fieldDefects {
		t.Errorf("issues = %+v, want one %s issue", issues, fieldDefects)
	}
	if y, ok := NewCalculator().CalculateKPIs(vs)["average_yield_factor"]; ok {
		t.Errorf("average_yield_factor = %v, want omitted (yield unknown)", y)
	}
	// Float rounding at exactly 1 is not a contradiction.
	vs.Processes[0].Parameters.ScrapRate, vs.Processes[0].Parameters.ReworkRate = F(0.7), F(0.3)
	if issues := contradictions(CheckConsistency(vs)); len(issues) != 0 {
		t.Errorf("scrap 0.7 + rework 0.3 flagged: %+v", issues)
	}
	// Targets cannot create it either.
	vs.Processes[0].Parameters.ScrapRate, vs.Processes[0].Parameters.ReworkRate = F(0.1), F(0.1)
	_, err := NewDesigner().DesignFutureStateFromTargets(vs, &FutureStateTargets{
		Default: &ProcessTargets{ScrapRate: AbsoluteTarget(0.6), ReworkRate: AbsoluteTarget(0.404)}})
	if err == nil {
		t.Errorf("targets scrap 0.6 + rework 0.404 accepted")
	}
}

// Value adding time above the lead time (process time not given) is
// reported, and flow efficiency is unknown rather than above 100%.
func TestValueAddingTimeAboveLeadTime(t *testing.T) {
	vs := &ValueStream{ID: "vs", Name: "VAT", Processes: []Process{{ID: "p", Name: "P", Parameters: ProcessParameters{
		TimeUnit: TimeUnitMinutes, IdleTime: F(10), ValueAddingTime: F(20)}}}}
	issues := contradictions(CheckConsistency(vs))
	if len(issues) != 1 || issues[0].Field != "value_adding_time" || !approx(issues[0].ComputedValue, 10) {
		t.Errorf("issues = %+v, want one value_adding_time issue (lead time 10)", issues)
	}
	if r, ok := NewCalculator().CalculateKPIs(vs)["value_added_ratio"]; ok {
		t.Errorf("value_added_ratio = %v, want omitted", r)
	}
	// It surfaces on load too.
	data, err := json.Marshal(vs)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := NewSerializer().DeserializeValueStream(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(contradictions(loaded.ConsistencyIssues)) != 1 {
		t.Errorf("loaded issues = %+v, want one", loaded.ConsistencyIssues)
	}
	// Within the lead time it is fine.
	vs.Processes[0].Parameters.IdleTime = F(40)
	if issues := contradictions(CheckConsistency(vs)); len(issues) != 0 {
		t.Errorf("VAT 20 within lead time 40 flagged: %+v", issues)
	}
	if r := NewCalculator().CalculateKPIs(vs)["value_added_ratio"]; !approx(r, 0.5) {
		t.Errorf("value_added_ratio = %v, want 0.5", r)
	}
}

// TestEnteredZeroVersusNotEntered pins the difference between a value
// entered as 0 and one not entered, for every relationship where it matters.
func TestEnteredZeroVersusNotEntered(t *testing.T) {
	c := NewCalculator()
	// 720 per day = 0.5 per minute.
	ctx := streamContext{customer: &Customer{Demand: F(720), DemandPeriod: DemandPeriodDay}, unit: TimeUnitMinutes}
	resolve := func(p ProcessParameters) (ProcessParameters, []ConsistencyIssue) {
		p.TimeUnit = TimeUnitMinutes
		return c.resolveParameters(p, ctx)
	}
	fields := func(issues []ConsistencyIssue) []string {
		var out []string
		for _, is := range issues {
			out = append(out, is.Field)
		}
		return out
	}

	// Process time parts: a setup time entered as 0 is a part, so the
	// parts (summing to 0) replace the stored 30 and the gap is reported.
	got, issues := resolve(ProcessParameters{ProcessTime: F(30), SetupTime: F(0)})
	if num(got.ProcessTime) != 0 || len(issues) != 1 || issues[0].Field != "process_time" ||
		!strings.Contains(issues[0].Message, "only setup_time given") {
		t.Errorf("setup 0: PT %v, issues %+v; want 0 and one process_time issue", num(got.ProcessTime), issues)
	}
	got, issues = resolve(ProcessParameters{ProcessTime: F(30)})
	if num(got.ProcessTime) != 30 || len(issues) != 0 {
		t.Errorf("setup not entered: PT %v, issues %+v; want 30, none", num(got.ProcessTime), issues)
	}
	got, issues = resolve(ProcessParameters{ProcessTime: F(30), SetupTime: F(0), ProcessingTime: F(30)})
	if num(got.ProcessTime) != 30 || len(issues) != 0 {
		t.Errorf("setup 0 + processing 30: PT %v, issues %+v; want 30, none", num(got.ProcessTime), issues)
	}
	// A.6: an A.2 part beyond setup entered as 0 is a second breakdown.
	_, issues = resolve(ProcessParameters{SetupTime: F(5), RepetitionTime: F(2), RepetitionCount: I(3), HandlingTime: F(0)})
	if f := fields(issues); len(f) != 1 || f[0] != fieldProcessParts {
		t.Errorf("A.6 with handling 0: issues %v, want the parts sum", f)
	}
	// A repetition count of 0 is entered: A.6 gives setup + 0.
	if got, _ := resolve(ProcessParameters{SetupTime: F(5), RepetitionTime: F(2), RepetitionCount: I(0)}); num(got.ProcessTime) != 5 {
		t.Errorf("A.6 with 0 repetitions: PT %v, want 5", num(got.ProcessTime))
	}

	// Idle time parts.
	got, issues = resolve(ProcessParameters{IdleTime: F(10), TransportTime: F(0)})
	if got.IdleTime == nil || *got.IdleTime != 0 || len(issues) != 1 || issues[0].Field != "idle_time" {
		t.Errorf("transport 0: IT %v, issues %+v; want 0 and one idle_time issue", num(got.IdleTime), issues)
	}
	got, issues = resolve(ProcessParameters{IdleTime: F(10)})
	if num(got.IdleTime) != 10 || len(issues) != 0 {
		t.Errorf("idle total only: IT %v, issues %+v", num(got.IdleTime), issues)
	}
	if got, _ = resolve(ProcessParameters{WaitingTime: F(0), StorageTime: F(5)}); num(got.IdleTime) != 5 {
		t.Errorf("waiting 0 + storage 5: IT %v, want 5", num(got.IdleTime))
	}

	// Stock: 0 entered means no inventory time; not entered means unknown.
	got, issues = resolve(ProcessParameters{ProcessTime: F(10), StockQuantity: F(0)})
	if got.WaitingTime == nil || *got.WaitingTime != 0 || got.RangeOfInventory == nil || len(issues) != 0 ||
		num(got.LeadTime) != 10 {
		t.Errorf("stock 0: WT %v range %v LT %v issues %+v; want 0, 0, 10, none",
			got.WaitingTime, got.RangeOfInventory, num(got.LeadTime), issues)
	}
	got, _ = resolve(ProcessParameters{ProcessTime: F(10)})
	if got.WaitingTime != nil || got.RangeOfInventory != nil {
		t.Errorf("stock not entered: WT %v range %v, want both not entered", got.WaitingTime, got.RangeOfInventory)
	}
	_, issues = resolve(ProcessParameters{ProcessTime: F(10), StockQuantity: F(0), WaitingTime: F(30)})
	if f := fields(issues); len(f) != 1 || f[0] != "waiting_time" {
		t.Errorf("stock 0 with stored waiting 30: issues %v, want waiting_time", f)
	}
	if got, issues = resolve(ProcessParameters{ProcessTime: F(10), WaitingTime: F(30)}); num(got.WaitingTime) != 30 || len(issues) != 0 {
		t.Errorf("waiting 30, stock not entered: WT %v issues %+v", num(got.WaitingTime), issues)
	}

	// Yield.
	if got, _ = resolve(ProcessParameters{ScrapRate: F(0)}); got.YieldFactor == nil || *got.YieldFactor != 1 {
		t.Errorf("scrap 0: yield %v, want 1", got.YieldFactor)
	}
	if got, _ = resolve(ProcessParameters{ProcessTime: F(1)}); got.YieldFactor != nil {
		t.Errorf("no quality data: yield %v, want not entered", *got.YieldFactor)
	}
	_, issues = resolve(ProcessParameters{YieldFactor: F(0), ReworkRate: F(0)})
	if f := fields(issues); len(f) != 1 || f[0] != "yield_factor" {
		t.Errorf("yield 0 with rework 0 (yield 1): issues %v, want yield_factor", f)
	}

	// Lead time from a classification entered as 0.
	if got, _ = resolve(ProcessParameters{NonValueAddingTime: F(0)}); got.LeadTime == nil || *got.LeadTime != 0 {
		t.Errorf("NVAT 0 alone: LT %v, want 0", got.LeadTime)
	}
	if got, _ = resolve(ProcessParameters{}); got.LeadTime != nil {
		t.Errorf("nothing entered: LT %v, want not entered", *got.LeadTime)
	}
}

// Average yield counts a yield entered as 0 and skips one not entered.
func TestAverageYieldZeroVersusNotEntered(t *testing.T) {
	stream := func(first *float64) *ValueStream {
		return &ValueStream{ID: "vs", Processes: []Process{
			{ID: "a", Parameters: ProcessParameters{ProcessTime: F(1), YieldFactor: first}},
			{ID: "b", Parameters: ProcessParameters{ProcessTime: F(1), YieldFactor: F(1)}},
		}}
	}
	c := NewCalculator()
	if got := c.CalculateKPIs(stream(F(0)))["average_yield_factor"]; !approx(got, 0.5) {
		t.Errorf("yield 0 and 1: average %v, want 0.5", got)
	}
	if got := c.CalculateKPIs(stream(nil))["average_yield_factor"]; !approx(got, 1) {
		t.Errorf("yield not entered and 1: average %v, want 1", got)
	}
	none := &ValueStream{ID: "vs", Processes: []Process{{ID: "a", Parameters: ProcessParameters{ProcessTime: F(1)}}}}
	if got, ok := c.CalculateKPIs(none)["average_yield_factor"]; ok {
		t.Errorf("no quality data: average %v, want omitted", got)
	}
}

// Inventories: a quantity of 0 is an empty inventory (no time, no stock);
// a quantity not entered leaves the range as the measure.
func TestInventoryQuantityZeroVersusNotEntered(t *testing.T) {
	stream := func(inv Inventory, stock *float64) *ValueStream {
		return &ValueStream{ID: "vs", Name: "Inv",
			Customer: &Customer{ID: "c", Demand: F(100), DemandPeriod: DemandPeriodDay},
			Processes: []Process{{ID: "p", Parameters: ProcessParameters{
				TimeUnit: TimeUnitMinutes, ProcessTime: F(10), StockQuantity: stock}}},
			Inventory: []Inventory{inv},
		}
	}
	c := NewCalculator()
	field := func(issues []ConsistencyIssue) string {
		if len(issues) != 1 {
			return fmt.Sprint(issues)
		}
		return issues[0].Field
	}

	// Quantity 0 with a stored range: range is 0 ÷ rate = 0; the stored
	// 1440 contradicts it.
	vs := stream(Inventory{ID: "i", Quantity: F(0), Range: F(1440)}, nil)
	if lt := c.CalculateKPIs(vs)["total_lead_time"]; !approx(lt, 10) {
		t.Errorf("quantity 0: lead time %v, want 10", lt)
	}
	if f := field(contradictions(CheckConsistency(vs))); f != "range" {
		t.Errorf("quantity 0, range 1440: issue %v, want range", f)
	}
	// Quantity not entered: the range is the measure (1 day of stock).
	vs = stream(Inventory{ID: "i", Range: F(1440)}, nil)
	if lt := c.CalculateKPIs(vs)["total_lead_time"]; !approx(lt, 1450) {
		t.Errorf("range only: lead time %v, want 1450", lt)
	}
	if issues := contradictions(CheckConsistency(vs)); len(issues) != 0 {
		t.Errorf("range only: issues %+v", issues)
	}
	if turns := c.CalculateKPIs(vs)["inventory_turns"]; !approx(turns, 1) {
		t.Errorf("range only: turns %v, want 1 (100 in stock at 100/day)", turns)
	}
	// Linked to a process whose stock is entered as 0: the same empty
	// stock, consistent.
	vs = stream(Inventory{ID: "i", Quantity: F(0), ProcessID: "p"}, F(0))
	if issues := contradictions(CheckConsistency(vs)); len(issues) != 0 {
		t.Errorf("empty linked inventory, stock 0: issues %+v", issues)
	}
	if _, ok := c.CalculateKPIs(vs)["inventory_turns"]; ok {
		t.Errorf("no stock at all: inventory_turns must be omitted")
	}
	// Linked inventory of 50 to a process whose stock is entered as 0.
	vs = stream(Inventory{ID: "i", Quantity: F(50), ProcessID: "p"}, F(0))
	if f := field(contradictions(CheckConsistency(vs))); f != "quantity" {
		t.Errorf("quantity 50 vs stock 0: issue %v, want quantity", f)
	}
	// Linked inventory to a process whose stock is not entered: not linked.
	vs = stream(Inventory{ID: "i", Quantity: F(50), ProcessID: "p"}, nil)
	if f := field(contradictions(CheckConsistency(vs))); f != "process_id" {
		t.Errorf("stock not entered: issue %v, want process_id", f)
	}
}

// Working days per week: not entered means 7; entered 0 is invalid.
func TestWorkingDaysZeroIsInvalid(t *testing.T) {
	vs := &ValueStream{ID: "vs", Name: "W", Processes: []Process{{ID: "p", Name: "p",
		Parameters: ProcessParameters{ProcessTime: F(1)}}},
		Customer: &Customer{ID: "c", Demand: F(100), DemandPeriod: DemandPeriodDay, WorkingDaysPerWeek: F(0)}}
	if issues := contradictions(CheckConsistency(vs)); len(issues) != 1 || issues[0].Field != "working_days_per_week" {
		t.Errorf("working days 0: issues %+v", issues)
	}
	if err := NewValidator().ValidateValueStream(vs); err == nil {
		t.Errorf("working days 0 must fail validation")
	}
	vs.Customer.WorkingDaysPerWeek = nil
	if issues := contradictions(CheckConsistency(vs)); len(issues) != 0 {
		t.Errorf("working days not entered: issues %+v", issues)
	}
}
