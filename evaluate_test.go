package vsm

import (
	"encoding/json"
	"strings"
	"testing"
)

// exampleStream is the worked example of A.3.4 as a value stream: one
// process, followed by idle time to the customer.
func exampleStream() *ValueStream {
	return &ValueStream{
		Name: "wave soldering", State: StateCurrent, TimeUnit: Second,
		Customer: Customer{Name: "end customer"},
		Processes: []Process{{
			ID: "P1", Name: "wave soldering", Type: ProcessAutomated,
			ProcessTime: F(35), RepetitionTime: F(12.5), Repetitions: F(2),
			ProcessingTime: F(8), WaitingTime: F(9), TimesToRepair: []float64{2},
		}},
		Idles: []Idle{{ID: "I1", From: "P1", IdleTime: F(10)}},
	}
}

func issuesWith(r Result, sev Severity, rule string) []Issue {
	var out []Issue
	for _, i := range r.Issues {
		if i.Severity == sev && (rule == "" || i.Rule == rule) {
			out = append(out, i)
		}
	}
	return out
}

func wantValue(t *testing.T, name string, v Value, want float64, source string) {
	t.Helper()
	if !v.Known {
		t.Errorf("%s is unknown, want %v", name, want)
		return
	}
	approx(t, name, v.Value, want)
	if source != "" && v.Source != source {
		t.Errorf("%s source = %q, want %q", name, v.Source, source)
	}
}

func TestEvaluateWorkedExample(t *testing.T) {
	r := Evaluate(exampleStream())
	p := r.Processes[0]
	wantValue(t, "downtime", p.Downtime, 2, "A.1")
	wantValue(t, "setup time", p.SetupTime, 10, "A.6")
	wantValue(t, "handling time", p.HandlingTime, 6, "A.8")
	wantValue(t, "lead time", r.Totals.LeadTime, 45, "A.4")
	wantValue(t, "S_VA", r.KPIs.ValueAddingShare, 35.0/45, "4.5.2")
	if w := issuesWith(r, SeverityWarning, ""); len(w) > 0 {
		t.Errorf("unexpected warnings: %v", w)
	}
	if e := issuesWith(r, SeverityError, ""); len(e) > 0 {
		t.Errorf("unexpected errors: %v", e)
	}
}

func TestEvaluateReportsConflictWithoutOverwriting(t *testing.T) {
	vs := exampleStream()
	vs.Processes[0].SetupTime = F(12) // A.6 gives 10
	vs.Processes[0].HandlingTime = F(6)
	r := Evaluate(vs)
	wantValue(t, "setup time", r.Processes[0].SetupTime, 12, "entered")
	if len(issuesWith(r, SeverityWarning, "A.6")) != 1 {
		t.Errorf("want one A.6 warning, got %v", r.Issues)
	}
	if len(issuesWith(r, SeverityWarning, "A.8")) != 1 {
		t.Errorf("want one A.8 warning, got %v", r.Issues)
	}
}

func TestEvaluateIdleTime(t *testing.T) {
	vs := &ValueStream{
		Name: "line", TimeUnit: Day,
		Customer: Customer{Demand: F(20), OperatingTime: F(10), PeriodLength: F(1)}, // per day
		Processes: []Process{
			{ID: "P1", ProcessTime: F(0.5), CycleTime: F(0.4)},
			{ID: "P2", ProcessTime: F(0.25), CycleTime: F(0.6), Resources: F(2)},
		},
		Idles: []Idle{{
			ID: "I1", From: "P1", To: "P2",
			TransportRepetitions: F(2), TransportRepetitionTime: F(0.1),
			WaitingOrders: F(4),
			Inventories:   []Inventory{{Kind: InventoryStock, Quantity: F(40)}, {Kind: InventoryWarehouse, Range: F(1)}},
		}},
	}
	r := Evaluate(vs)
	i := r.Idles[0]
	wantValue(t, "takt", r.KPIs.CustomerTakt, 0.5, "A.12")
	wantValue(t, "transport", i.TransportTime, 0.2, "A.10")
	wantValue(t, "waiting", i.WaitingTime, 2, "A.11")
	wantValue(t, "range 1", i.Ranges[0], 2, "A.7")
	wantValue(t, "storage", i.StorageTime, 3, "A.9")
	wantValue(t, "idle", i.IdleTime, 5.2, "A.2")
	wantValue(t, "lead time", r.Totals.LeadTime, 5.95, "A.4")
	wantValue(t, "P2 takt load", r.Processes[1].TaktLoad, 1.2, "A.12")
	if r.Bottleneck != "P2" {
		t.Errorf("bottleneck = %q, want P2 (cycle time 0.6 beats 0.4)", r.Bottleneck)
	}
}

func TestEvaluateRangeNeedsPeriodLength(t *testing.T) {
	vs := &ValueStream{
		Name: "x", TimeUnit: Day, Customer: Customer{Demand: F(20)},
		Processes: []Process{{ID: "P1", ProcessTime: F(1)}},
		Idles:     []Idle{{ID: "I1", From: "P1", Inventories: []Inventory{{Kind: InventoryStock, Quantity: F(40)}}}},
	}
	r := Evaluate(vs)
	if r.Totals.LeadTime.Known {
		t.Error("lead time is known although the range cannot be converted to time")
	}
	if len(issuesWith(r, SeverityNote, "A.7")) != 1 {
		t.Errorf("want one A.7 note, got %v", r.Issues)
	}
}

func TestEvaluateNeverAppliesA5(t *testing.T) {
	vs := &ValueStream{Name: "x", TimeUnit: Second, Processes: []Process{
		{ID: "P1", CycleTime: F(2), ProcessQuantity: F(3), Resources: F(4)},
		{ID: "P2", CycleTime: F(2), ProcessQuantity: F(3), Resources: F(4), SetupTime: F(1), RepetitionTime: F(1), Repetitions: F(5)},
	}}
	r := Evaluate(vs)
	if r.Processes[0].ProcessTime.Known {
		t.Errorf("P1 process time %v was derived; A.5 must not be applied", r.Processes[0].ProcessTime)
	}
	wantValue(t, "P2 process time", r.Processes[1].ProcessTime, 6, "A.6")
	if n := len(issuesWith(r, SeverityNote, "A.5")); n != 1 {
		t.Errorf("want one A.5 note, got %d", n)
	}
}

func TestEvaluateRejectsInvalidInput(t *testing.T) {
	vs := &ValueStream{
		Name: "x", TimeUnit: "week", State: "past",
		Processes: []Process{
			{ID: "P1", ProcessTime: F(-1), ScrapRate: F(5)},
			{ID: "P1", Type: "robotic"},
		},
		Idles: []Idle{{ID: "I1", From: "nowhere"}},
	}
	r := Evaluate(vs)
	text := ""
	for _, i := range issuesWith(r, SeverityError, "") {
		text += i.String() + "\n"
	}
	for _, want := range []string{"time_unit", "state", "process_time is negative", "scrap_rate is above 1", "already used", "robotic", "nowhere"} {
		if !strings.Contains(text, want) {
			t.Errorf("errors do not mention %q:\n%s", want, text)
		}
	}
	if r.Processes[0].ProcessTime.Known {
		t.Error("a negative process time was used")
	}
}

func TestEvaluateYieldAndValueShares(t *testing.T) {
	vs := &ValueStream{Name: "x", TimeUnit: Minute, Processes: []Process{{
		ID: "P1", ProcessTime: F(10), ScrapRate: F(0.05),
		ValueShares: ValueShares{ValueAdding: F(4), NonValueAdding: F(1)},
	}},
		Quality:    &Quality{Total: 200, Defective: 10},
		Deliveries: &Deliveries{Total: 40, CompleteOnTime: 38},
		Inventory:  &Turnover{Usage: 1200, AverageStock: 100},
	}
	r := Evaluate(vs)
	p := r.Processes[0]
	wantValue(t, "yield", p.YieldFactor, 0.95, "A.17")
	wantValue(t, "NNVAT", p.NecessaryNonValueAdding, 5, "")
	wantValue(t, "VAR", r.KPIs.ValueAddedRatio, 0.4, "A.16")
	wantValue(t, "K_SR", r.KPIs.ScrapRate, 0.05, "")
	wantValue(t, "K_YF", r.KPIs.YieldFactor, 0.95, "A.17")
	wantValue(t, "OTIF", r.KPIs.OnTimeInFull, 0.95, "A.14")
	wantValue(t, "INVT", r.KPIs.InventoryTurns, 12, "A.13")
}

func TestValueJSON(t *testing.T) {
	data, err := json.Marshal([]Value{{}, {Value: 2, Known: true, Source: "A.1"}})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), `[null,{"value":2,"source":"A.1"}]`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestEvaluateRejectsZeroOperatingTimeAndPeriod(t *testing.T) {
	vs := &ValueStream{Name: "x", TimeUnit: Minute,
		Customer:  Customer{Demand: F(400), OperatingTime: F(0), PeriodLength: F(0)},
		Processes: []Process{{ID: "P1", ProcessTime: F(1)}},
		Idles:     []Idle{{ID: "I1", From: "P1", TransportTime: F(0), WaitingOrders: F(50), Inventories: []Inventory{{Kind: InventoryStock, Quantity: F(800)}}}},
	}
	r := Evaluate(vs)
	if r.Totals.LeadTime.Known {
		t.Errorf("lead time %v computed from a zero operating time or period length", r.Totals.LeadTime)
	}
	if n := len(issuesWith(r, SeverityError, "")); n != 2 {
		t.Errorf("want 2 errors, got %v", r.Issues)
	}
}

func TestEvaluateProcessTypeParts(t *testing.T) {
	vs := &ValueStream{Name: "x", TimeUnit: Minute, Processes: []Process{
		{ID: "M", Type: ProcessManual, ProcessTime: F(10), SetupTime: F(2), ProcessingTime: F(5), HandlingTime: F(1), WaitingTime: F(1)},
		{ID: "D", Type: ProcessData, ProcessTime: F(10), SetupTime: F(2), ProcessingTime: F(5)},
	}}
	r := Evaluate(vs)
	if n := len(issuesWith(r, SeverityWarning, "A.8")); n != 1 {
		t.Errorf("want one A.8 warning for the manual process, got %v", r.Issues)
	}
	wantValue(t, "data waiting time", r.Processes[1].WaitingTime, 3, "A.8")
}

func TestEvaluateExcludedPartsStayOutOfA8(t *testing.T) {
	vs := &ValueStream{Name: "x", TimeUnit: Minute, Processes: []Process{
		{ID: "M", Type: ProcessManual, ProcessTime: F(10), SetupTime: F(1), ProcessingTime: F(5), WaitingTime: F(0), HandlingTime: F(4), TimesToRepair: []float64{2}},
		{ID: "D", Type: ProcessData, ProcessTime: F(10), SetupTime: F(2), ProcessingTime: F(5), WaitingTime: F(3), HandlingTime: F(3), Downtime: F(1)},
	}}
	r := Evaluate(vs)
	if w := issuesWith(r, SeverityWarning, "A.8"); len(w) > 0 {
		t.Errorf("downtime or handling time counted in A.8: %v", w)
	}
	wantValue(t, "manual downtime", r.Processes[0].Downtime, 2, "A.1")
}

func TestEvaluateZeroDemandWithoutOperatingTime(t *testing.T) {
	vs := &ValueStream{Name: "x", TimeUnit: Minute, Customer: Customer{Demand: F(0)},
		Processes: []Process{{ID: "P1", ProcessTime: F(1)}}}
	if e := issuesWith(Evaluate(vs), SeverityError, "A.12"); len(e) != 1 {
		t.Errorf("want one A.12 error for demand 0, got %v", e)
	}
}

func TestEvaluateNotesBatchDwellTime(t *testing.T) {
	r := Evaluate(exampleStream())
	if n := len(issuesWith(r, SeverityNote, "Table A.2")); n != 1 {
		t.Errorf("want one batch note, got %v", r.Issues)
	}
}
