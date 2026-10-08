package vsm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fieldOf returns a getter for one resolved value.
func fieldOf(name string) func(ProcessParameters) float64 {
	return map[string]func(ProcessParameters) float64{
		"process_time":       func(p ProcessParameters) float64 { return p.ProcessTime },
		"idle_time":          func(p ProcessParameters) float64 { return p.IdleTime },
		"transport_time":     func(p ProcessParameters) float64 { return p.TransportTime },
		"waiting_time":       func(p ProcessParameters) float64 { return p.WaitingTime },
		"range_of_inventory": func(p ProcessParameters) float64 { return p.RangeOfInventory },
		"processing_time":    func(p ProcessParameters) float64 { return p.ProcessingTime },
		"value_adding_time":  func(p ProcessParameters) float64 { return p.ValueAddingTime },
		"lead_time":          func(p ProcessParameters) float64 { return p.LeadTime },
		"yield_factor":       func(p ProcessParameters) float64 { return p.YieldFactor },
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
		{"PT parts only", ProcessParameters{ProcessingTime: 6, SetupTime: 2, AncillaryTime: 1}, "process_time", 9, "", 0},
		{"PT total only", ProcessParameters{ProcessTime: 12}, "process_time", 12, "", 0},
		{"PT complete parts agree", ProcessParameters{ProcessTime: 12, ProcessingTime: 6, SetupTime: 2, HandlingTime: 4}, "process_time", 12, "", 0},
		{"PT complete parts disagree", ProcessParameters{ProcessTime: 20, ProcessingTime: 6, SetupTime: 2, HandlingTime: 4}, "process_time", 12, "process_time", 20},
		{"PT parts fall short of total", ProcessParameters{ProcessTime: 20, SetupTime: 4}, "process_time", 4, "process_time", 20},
		{"PT parts exceed total", ProcessParameters{ProcessTime: 5, ProcessingTime: 6, SetupTime: 2}, "process_time", 8, "process_time", 5},
		{"PT within 0.5% tolerance", ProcessParameters{ProcessTime: 12.05, ProcessingTime: 6, SetupTime: 2, HandlingTime: 4}, "process_time", 12, "", 0},
		// Repetitive process time (A.6).
		{"A.6 parts only", ProcessParameters{SetupTime: 5, RepetitionTime: 2, RepetitionCount: 3}, "process_time", 11, "", 0},
		{"A.6 agrees with stored PT", ProcessParameters{ProcessTime: 11, SetupTime: 5, RepetitionTime: 2, RepetitionCount: 3}, "process_time", 11, "", 0},
		{"A.6 disagrees with stored PT", ProcessParameters{ProcessTime: 30, SetupTime: 5, RepetitionTime: 2, RepetitionCount: 3}, "process_time", 11, "process_time", 30},
		{"A.6 with A.2 parts above it", ProcessParameters{SetupTime: 5, HandlingTime: 1, ProcessingTime: 10, RepetitionTime: 2, RepetitionCount: 3}, "process_time", 11, fieldProcessParts, 16},
		{"A.6 with A.2 parts below it", ProcessParameters{SetupTime: 5, HandlingTime: 1, RepetitionTime: 2, RepetitionCount: 3}, "process_time", 11, fieldProcessParts, 6},
		{"A.6 agrees with A.2 parts", ProcessParameters{SetupTime: 5, ProcessingTime: 6, RepetitionTime: 2, RepetitionCount: 3}, "process_time", 11, "", 0},
		// Idle time.
		{"IT parts only", ProcessParameters{TransportTime: 2, WaitingTime: 3, StorageTime: 1}, "idle_time", 6, "", 0},
		{"IT total only", ProcessParameters{IdleTime: 40}, "idle_time", 40, "", 0},
		{"IT agree", ProcessParameters{IdleTime: 6, TransportTime: 2, WaitingTime: 3, StorageTime: 1}, "idle_time", 6, "", 0},
		{"IT disagree", ProcessParameters{IdleTime: 5, WaitingTime: 100}, "idle_time", 100, "idle_time", 5},
		// Transport time (A.10).
		{"TT parts only", ProcessParameters{TransportRepetitionTime: 4, TransportRepetitionCount: 3}, "transport_time", 12, "", 0},
		{"TT total only", ProcessParameters{TransportTime: 7}, "transport_time", 7, "", 0},
		{"TT agree", ProcessParameters{TransportTime: 12, TransportRepetitionTime: 4, TransportRepetitionCount: 3}, "transport_time", 12, "", 0},
		{"TT disagree", ProcessParameters{TransportTime: 10, TransportRepetitionTime: 4, TransportRepetitionCount: 3}, "transport_time", 12, "transport_time", 10},
		{"TT feeds idle time", ProcessParameters{TransportRepetitionTime: 4, TransportRepetitionCount: 3, WaitingTime: 8}, "idle_time", 20, "", 0},
		// Waiting time and range of inventory: stock ÷ demand rate (0.5 per
		// minute below), whatever the cycle time.
		{"WT parts only", ProcessParameters{StockQuantity: 50, CycleTime: 7}, "waiting_time", 100, "", 0},
		{"WT total only", ProcessParameters{WaitingTime: 30}, "waiting_time", 30, "", 0},
		{"WT agree", ProcessParameters{WaitingTime: 100, StockQuantity: 50, CycleTime: 7}, "waiting_time", 100, "", 0},
		{"WT disagree", ProcessParameters{WaitingTime: 30, StockQuantity: 50, CycleTime: 7}, "waiting_time", 100, "waiting_time", 30},
		{"WT not stock × cycle time", ProcessParameters{WaitingTime: 350, StockQuantity: 50, CycleTime: 7}, "waiting_time", 100, "waiting_time", 350},
		{"range disagree", ProcessParameters{RangeOfInventory: 60, StockQuantity: 50, CycleTime: 7}, "range_of_inventory", 100, "range_of_inventory", 60},
		// Processing time (A.5).
		{"PRT parts only", ProcessParameters{CycleTime: 2, OrderQuantity: 10, ResourceCount: 2}, "processing_time", 10, "", 0},
		{"PRT total only", ProcessParameters{ProcessingTime: 7}, "processing_time", 7, "", 0},
		{"PRT agree", ProcessParameters{ProcessingTime: 10, CycleTime: 2, OrderQuantity: 10, ResourceCount: 2}, "processing_time", 10, "", 0},
		{"PRT disagree", ProcessParameters{ProcessingTime: 15, CycleTime: 2, OrderQuantity: 10, ResourceCount: 2}, "processing_time", 10, "processing_time", 15},
		{"PRT feeds process time", ProcessParameters{CycleTime: 2, OrderQuantity: 10, ResourceCount: 2, SetupTime: 5}, "process_time", 15, "", 0},
		// Value adding time (VATp, A.9).
		{"VATp only", ProcessParameters{ValueAddingTimeProcess: 4}, "value_adding_time", 4, "", 0},
		{"VAT only", ProcessParameters{ValueAddingTime: 4}, "value_adding_time", 4, "", 0},
		{"VAT agree", ProcessParameters{ValueAddingTime: 4, ValueAddingTimeProcess: 4}, "value_adding_time", 4, "", 0},
		{"VAT disagree", ProcessParameters{ValueAddingTime: 9, ValueAddingTimeProcess: 4}, "value_adding_time", 4, "value_adding_time", 9},
		{"VAT above process time", ProcessParameters{ProcessTime: 3, ValueAddingTime: 5}, "value_adding_time", 5, "value_adding_time", 5},
		// Lead time: PT + IT, classified as VAT + NVAT + NNVAT (A.3).
		{"LT flow only", ProcessParameters{ProcessTime: 10, IdleTime: 30}, "lead_time", 40, "", 0},
		{"LT classification only", ProcessParameters{ValueAddingTime: 5, NonValueAddingTime: 30, NecessaryNonValueAddingTime: 5}, "lead_time", 40, "", 0},
		{"LT stored only", ProcessParameters{LeadTime: 40}, "lead_time", 40, "", 0},
		{"LT flow and classification agree", ProcessParameters{ProcessTime: 10, IdleTime: 30, ValueAddingTime: 5, NonValueAddingTime: 30, NecessaryNonValueAddingTime: 5}, "lead_time", 40, "", 0},
		{"LT classification disagrees", ProcessParameters{ProcessTime: 10, IdleTime: 30, ValueAddingTime: 5, NonValueAddingTime: 5}, "lead_time", 40, fieldClassSum, 10},
		{"LT stored disagrees", ProcessParameters{ProcessTime: 10, IdleTime: 30, LeadTime: 25}, "lead_time", 40, "lead_time", 25},
		{"LT VAT only (rest unclassified)", ProcessParameters{ProcessTime: 10, IdleTime: 30, ValueAddingTime: 5}, "lead_time", 40, "", 0},
		// Yield (A.17).
		{"YF parts only", ProcessParameters{ScrapRate: 0.02, ReworkRate: 0.03}, "yield_factor", 0.95, "", 0},
		{"YF total only", ProcessParameters{YieldFactor: 0.9}, "yield_factor", 0.9, "", 0},
		{"YF agree", ProcessParameters{YieldFactor: 0.95, ScrapRate: 0.02, ReworkRate: 0.03}, "yield_factor", 0.95, "", 0},
		{"YF disagree", ProcessParameters{YieldFactor: 0.99, ScrapRate: 0.02, ReworkRate: 0.03}, "yield_factor", 0.95, "yield_factor", 0.99},
	}
	c := NewCalculator()
	// 720 per day = 0.5 per minute.
	ctx := streamContext{customer: &Customer{Demand: 720, DemandPeriod: DemandPeriodDay}, unit: min}
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
	_, issues := NewCalculator().resolveParameters(ProcessParameters{ProcessTime: 20, SetupTime: 4}, streamContext{})
	if len(issues) != 1 {
		t.Fatalf("got %+v, want one issue", issues)
	}
	want := "process_time: stored 20, but its parts sum to 4 (only setup_time given) — enter the missing parts, " +
		"or clear the given parts to use the stored total; calculations use 4"
	if issues[0].Message != want {
		t.Errorf("message = %q\nwant      %q", issues[0].Message, want)
	}
	_, issues = NewCalculator().resolveParameters(ProcessParameters{ProcessTime: 5, ProcessingTime: 6, SetupTime: 2}, streamContext{})
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "correct the parts or the stored value") ||
		!strings.Contains(issues[0].Message, "processing_time + setup_time") {
		t.Errorf("excess issue = %+v", issues)
	}
}

func TestCheckConsistencyNamesProcessAndUnit(t *testing.T) {
	vs := &ValueStream{ID: "vs", Processes: []Process{
		{ID: "ok", Parameters: ProcessParameters{TimeUnit: TimeUnitMinutes, ProcessTime: 2, IdleTime: 3}},
		{ID: "bad", Parameters: ProcessParameters{TimeUnit: TimeUnitSeconds, ProcessTime: 600, ProcessingTime: 30, SetupTime: 30, HandlingTime: 60}},
	}}
	issues := CheckConsistency(vs)
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
		{ID: "p1", Parameters: ProcessParameters{TimeUnit: TimeUnitMinutes, IdleTime: 60, WaitingTime: 30, TransportTime: 15, ProcessTime: 15}},
		// 0.25 h process + 0.5 h idle, consistent.
		{ID: "p2", Parameters: ProcessParameters{TimeUnit: TimeUnitHours, ProcessTime: 0.25, IdleTime: 0.5, WaitingTime: 0.5}},
	}}
	issues := CheckConsistency(vs)
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
	vs := &ValueStream{ID: "vs", Name: "LT", Customer: &Customer{ID: "c", Demand: 720, DemandPeriod: DemandPeriodDay}, Processes: []Process{
		{ID: "p1", Parameters: ProcessParameters{TimeUnit: TimeUnitMinutes,
			ProcessingTime: 6, SetupTime: 4, StockQuantity: 10, CycleTime: 2, TransportTime: 5,
			ValueAddingTime: 6, NonValueAddingTime: 25, NecessaryNonValueAddingTime: 4}},
		{ID: "p2", Parameters: ProcessParameters{TimeUnit: TimeUnitHours,
			ProcessTime: 0.5, IdleTime: 1, ValueAddingTimeProcess: 0.25, NonValueAddingTime: 1.25}},
	}}
	if issues := CheckConsistency(vs); len(issues) != 0 {
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
		r := c.resolvedIn(p.Parameters, c.contextOf(vs))
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
	vs.Processes[1].Parameters.NonValueAddingTime = 2
	issues := CheckConsistency(vs)
	if len(issues) != 1 || issues[0].ID != "p2" || issues[0].Field != fieldClassSum {
		t.Fatalf("issues = %+v", issues)
	}
	if got := c.CalculateKPIs(vs)["total_lead_time"]; !approx(got, 125) {
		t.Errorf("lead time = %v, want 125 (flow wins)", got)
	}
}

func TestCheckConsistencyCustomerAndStoredKPIs(t *testing.T) {
	vs := &ValueStream{ID: "vs",
		Customer:  &Customer{ID: "c", Demand: 200, AvailableTime: 480, TaktTime: 3},
		Processes: []Process{{ID: "p", Parameters: ProcessParameters{TimeUnit: TimeUnitMinutes, ProcessTime: 2, IdleTime: 8}}},
		KPIs:      map[string]float64{"total_lead_time": 12, "made_up_kpi": 1},
	}
	issues := CheckConsistency(vs)
	if len(issues) != 2 {
		t.Fatalf("issues = %+v", issues)
	}
	if is := issues[0]; is.Element != "customer" || is.Field != "takt_time" || is.StoredValue != 3 || is.ComputedValue != 2.4 {
		t.Errorf("takt issue = %+v", is)
	}
	if is := issues[1]; is.Element != "value_stream" || is.Field != "kpis.total_lead_time" || is.ComputedValue != 10 || is.Unit != "minutes" {
		t.Errorf("kpi issue = %+v", is)
	}
	vs.Customer.TaktTime, vs.KPIs = 2.4, NewCalculator().CalculateKPIs(vs)
	if issues := CheckConsistency(vs); len(issues) != 0 {
		t.Errorf("consistent stream gave %+v", issues)
	}
}

func TestContradictionsSurfaceInResults(t *testing.T) {
	vs := targetStream()
	vs.Processes[0].Parameters.IdleTime = 99 // parts say 30
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
	if issues := CheckConsistency(design.FutureState); len(issues) != 0 {
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
	if len(issues) != 1 || issues[0].Field != "idle_time" || len(vs.ConsistencyIssues) != 1 {
		t.Fatalf("issues = %+v / %+v", issues, vs.ConsistencyIssues)
	}
	plain, err := s.DeserializeValueStream(data)
	if err != nil || len(plain.ConsistencyIssues) != 1 {
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
	if len(loaded.ConsistencyIssues) != 1 {
		t.Errorf("LoadFromFile issues = %+v", loaded.ConsistencyIssues)
	}
	_, fileIssues, err := s.LoadValueStreamFromFile(path)
	if err != nil || len(fileIssues) != 1 {
		t.Errorf("LoadValueStreamFromFile: %v, %+v", err, fileIssues)
	}

	// A stale issues list in the file is replaced on load.
	vs.ConsistencyIssues = append(vs.ConsistencyIssues, ConsistencyIssue{Field: "stale"})
	vs.Processes[0].Parameters.IdleTime = 7
	if err := s.SaveToFile(vs, path); err != nil {
		t.Fatal(err)
	}
	var reloaded ValueStream
	if err := s.LoadFromFile(path, &reloaded); err != nil {
		t.Fatal(err)
	}
	if len(reloaded.ConsistencyIssues) != 0 {
		t.Errorf("reloaded issues = %+v", reloaded.ConsistencyIssues)
	}
}

func TestIdealStateIsConsistent(t *testing.T) {
	vs := targetStream()
	vs.Processes = append(vs.Processes, Process{ID: "p3", Name: "Batch", Parameters: ProcessParameters{
		TimeUnit: TimeUnitMinutes, CycleTime: 2, OrderQuantity: 10, ResourceCount: 1, SetupTime: 5,
		// 5 in stock at 100 per day wait 72 min: lead time 25 + 6 + 72 = 103.
		StockQuantity: 5, TransportRepetitionTime: 3, TransportRepetitionCount: 2,
		ValueAddingTime: 12, NonValueAddingTime: 91, ScrapRate: 0.1,
	}})
	if issues := CheckConsistency(vs); len(issues) != 0 {
		t.Fatalf("test stream inconsistent: %+v", issues)
	}
	ideal, err := NewDesigner().CreateIdealState(vs)
	if err != nil {
		t.Fatal(err)
	}
	if issues := CheckConsistency(ideal); len(issues) != 0 {
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
			Customer: &Customer{ID: "c", Demand: 120, DemandPeriod: DemandPeriodDay, AvailableTime: 28800},
			Processes: []Process{{ID: "p", Parameters: ProcessParameters{
				TimeUnit: TimeUnitSeconds, ProcessTime: 60, CycleTime: 60, StockQuantity: 240, WaitingTime: stored,
			}}},
			Inventory: []Inventory{{ID: "i", Quantity: 240, Range: twoDays}},
		}
	}
	c := NewCalculator()

	// A correct stored 2-day waiting time is consistent.
	vs := stream(twoDays)
	if issues := CheckConsistency(vs); len(issues) != 0 {
		t.Errorf("correct 2-day waiting time flagged: %+v", issues)
	}
	// Not stored: derived as 2 days, and lead time includes it.
	vs = stream(0)
	if wt := c.ResolveParametersInStream(vs.Processes[0].Parameters, vs).WaitingTime; !approx(wt, twoDays) {
		t.Errorf("derived waiting time = %v s, want %v", wt, twoDays)
	}
	if lt := c.CalculateKPIs(vs)["total_lead_time"]; !approx(lt, 60+twoDays) {
		t.Errorf("lead time = %v, want %v", lt, 60+twoDays)
	}
	// Stock × cycle time (14,400 s) stored as waiting time is a contradiction.
	vs = stream(240 * 60)
	issues := CheckConsistency(vs)
	if len(issues) != 1 || issues[0].Field != "waiting_time" || !approx(issues[0].ComputedValue, twoDays) {
		t.Errorf("issues = %+v, want one waiting_time issue computing %v", issues, twoDays)
	}
	// A wrong inventory range is reported too.
	vs = stream(twoDays)
	vs.Inventory[0].Range = 240 * 60
	issues = CheckConsistency(vs)
	if len(issues) != 1 || issues[0].Element != "inventory" || !approx(issues[0].ComputedValue, twoDays) {
		t.Errorf("inventory issues = %+v", issues)
	}

	// Demand rate unknown (no customer, or a month period with no fixed
	// length): nothing is derived or flagged.
	for name, cust := range map[string]*Customer{
		"no customer":  nil,
		"month period": {ID: "c", Demand: 2400, DemandPeriod: DemandPeriodMonth},
		"no period":    {ID: "c", Demand: 120},
	} {
		vs = stream(240 * 60)
		vs.Customer = cust
		if issues := CheckConsistency(vs); len(issues) != 0 {
			t.Errorf("%s: issues = %+v", name, issues)
		}
		vs = stream(0)
		vs.Customer = cust
		if wt := c.ResolveParametersInStream(vs.Processes[0].Parameters, vs).WaitingTime; wt != 0 {
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
	bad := ProcessParameters{ProcessTime: 10, ScrapRate: 0.8, ReworkRate: 0.4, TimeUnit: TimeUnitMinutes}

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
		{ID: "good", Parameters: ProcessParameters{ProcessTime: 10, YieldFactor: 0.9, TimeUnit: TimeUnitMinutes}},
	}}
	issues := CheckConsistency(vs)
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
	if y := c.ResolveParameters(bad).YieldFactor; y < 0 {
		t.Errorf("resolved yield = %v, must not be negative", y)
	}
}
