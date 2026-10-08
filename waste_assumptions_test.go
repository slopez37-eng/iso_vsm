package vsm

import (
	"strings"
	"testing"
)

// wasteStream: one process (minutes) with process time 10 = setup 6 +
// handling 4, idle 25 = transport 5 + waiting 20, scrap 0.1, and one
// inventory counted on its own with a range of 10. No customer, so no
// utilization and no over-production.
func wasteStream(wa *WasteAssumptions) *ValueStream {
	return &ValueStream{ID: "vs", Name: "vs", WasteAssumptions: wa,
		Processes: []Process{{ID: "p", Name: "P", Parameters: ProcessParameters{
			SetupTime: F(6), HandlingTime: F(4), TransportTime: F(5), WaitingTime: F(20),
			ValueAddingTime: F(3), ScrapRate: F(0.1), TimeUnit: TimeUnitMinutes}}},
		Inventory: []Inventory{{ID: "i", Name: "I", Range: F(10)}}}
}

func hasNote(notes []string, field string) bool {
	for _, n := range notes {
		if strings.Contains(n, field) {
			return true
		}
	}
	return false
}

func TestWasteAssumptionsAbsentNotEstimated(t *testing.T) {
	w, err := NewAnalyzer().AnalyzeWaste(wasteStream(nil))
	if err != nil {
		t.Fatal(err)
	}
	if w.TotalWaste != nil || w.WasteReductionPotential != nil {
		t.Errorf("total %v, potential %v: want not estimated (motion share missing)", w.TotalWaste, w.WasteReductionPotential)
	}
	for _, kind := range []string{"inventory", "waiting", "motion"} {
		if _, ok := w.WasteByType[kind]; ok {
			t.Errorf("%s waste = %v, want not estimated", kind, w.WasteByType[kind])
		}
	}
	// Measured waste is still reported.
	want := map[string]float64{"transportation": 5, "defects": 1, "over_processing": 0, "over_production": 0}
	for kind, v := range want {
		if got, ok := w.WasteByType[kind]; !ok || !approx(got, v) {
			t.Errorf("%s waste = %v (present %v), want %v", kind, got, ok, v)
		}
	}
	if !approx(val(w.WaitingAndInventoryWaste), 30) {
		t.Errorf("waiting and inventory waste = %v, want 30", val(w.WaitingAndInventoryWaste))
	}
	if !approx(w.WasteByInventory["i"], 10) {
		t.Errorf("inventory waste = %v, want 10", w.WasteByInventory)
	}
	if _, ok := w.WasteByProcess["p"]; ok {
		t.Errorf("process waste = %v, want not estimated", w.WasteByProcess["p"])
	}
	for _, field := range []string{fieldInventoryShare, fieldHandlingShare} {
		if !hasNote(w.Notes, field) {
			t.Errorf("notes %q do not name %s", w.Notes, field)
		}
	}
}

func TestWasteAssumptionsEntered(t *testing.T) {
	w, err := NewAnalyzer().AnalyzeWaste(wasteStream(&WasteAssumptions{
		InventoryShareOfWaiting: F(0.25), WasteShareOfHandling: F(0.5), ReductionPotential: F(0.2)}))
	if err != nil {
		t.Fatal(err)
	}
	// waiting 20 + inventory 10 = 30: inventory 7.5, waiting 22.5; motion
	// 4 × 0.5 = 2; transport 5; defects 0.1 × 10 = 1.
	want := map[string]float64{"inventory": 7.5, "waiting": 22.5, "motion": 2, "transportation": 5,
		"defects": 1, "over_processing": 0, "over_production": 0}
	for kind, v := range want {
		if got, ok := w.WasteByType[kind]; !ok || !approx(got, v) {
			t.Errorf("%s waste = %v (present %v), want %v", kind, got, ok, v)
		}
	}
	if !approx(val(w.TotalWaste), 38) {
		t.Errorf("total waste = %v, want 38", val(w.TotalWaste))
	}
	if !approx(val(w.WasteReductionPotential), 7.6) {
		t.Errorf("reduction potential = %v, want 7.6", val(w.WasteReductionPotential))
	}
	if !approx(w.WasteByProcess["p"], 28) {
		t.Errorf("process waste = %v, want 28", w.WasteByProcess["p"])
	}
	if len(w.Notes) != 0 {
		t.Errorf("notes = %q, want none", w.Notes)
	}
}

func TestWasteReductionPotentialNeedsAssumption(t *testing.T) {
	w, _ := NewAnalyzer().AnalyzeWaste(wasteStream(&WasteAssumptions{WasteShareOfHandling: F(0)}))
	if !approx(val(w.TotalWaste), 36) || w.WasteReductionPotential != nil {
		t.Errorf("total %v, potential %v: want 36 and not estimated", val(w.TotalWaste), w.WasteReductionPotential)
	}
	if !hasNote(w.Notes, fieldReductionPotential) {
		t.Errorf("notes %q do not name %s", w.Notes, fieldReductionPotential)
	}
}

func TestWasteWithoutHandlingOrWaitingNeedsNoAssumption(t *testing.T) {
	vs := &ValueStream{Processes: []Process{{ID: "p", Parameters: ProcessParameters{
		ProcessTime: F(10), ValueAddingTime: F(8), TransportTime: F(2), TimeUnit: TimeUnitMinutes}}}}
	w, _ := NewAnalyzer().AnalyzeWaste(vs)
	if !approx(val(w.TotalWaste), 2) {
		t.Errorf("total waste = %v, want 2", val(w.TotalWaste))
	}
	for _, kind := range wasteKinds {
		if _, ok := w.WasteByType[kind]; !ok {
			t.Errorf("%s waste missing; nothing needs a split", kind)
		}
	}
}

func TestOverProductionNeedsOperatingTime(t *testing.T) {
	// Demand 100 per day, 480 min a day, cycle time 2: utilization 100 × 2
	// ÷ 480 = 0.4167, below 0.7.
	stream := func(ot *float64) *ValueStream {
		return &ValueStream{
			Customer: &Customer{ID: "c", Demand: F(100), DemandPeriod: DemandPeriodDay, AvailableTime: F(480)},
			Processes: []Process{{ID: "p", Parameters: ProcessParameters{ProcessTime: F(10), CycleTime: F(2),
				ValueAddingTime: F(8), OperatingTime: ot, TimeUnit: TimeUnitMinutes}}}}
	}
	a := NewAnalyzer()
	w, _ := a.AnalyzeWaste(stream(nil))
	if _, ok := w.WasteByType["over_production"]; ok || w.TotalWaste != nil {
		t.Errorf("over-production %v, total %v: want not estimated", w.WasteByType, w.TotalWaste)
	}
	if !hasNote(w.Notes, "operating_time") {
		t.Errorf("notes %q do not name operating_time", w.Notes)
	}
	w, _ = a.AnalyzeWaste(stream(F(480)))
	if want := (1 - 200.0/480) * 480; !approx(w.WasteByType["over_production"], want) || !approx(val(w.TotalWaste), want) {
		t.Errorf("over-production %v, total %v: want %v", w.WasteByType["over_production"], val(w.TotalWaste), want)
	}
}

func TestAssumptionsOutOfRangeReported(t *testing.T) {
	vs := wasteStream(&WasteAssumptions{
		InventoryShareOfWaiting: F(1.5), WasteShareOfHandling: F(-0.1), ReductionPotential: F(2)})
	vs.InformationFlowQuality = F(1.2)
	fields := map[string]bool{}
	for _, issue := range CheckConsistency(vs) {
		if issue.Kind == IssueContradiction && issue.Element == "value_stream" {
			fields[issue.Field] = true
		}
	}
	for _, f := range []string{fieldInventoryShare, fieldHandlingShare, fieldReductionPotential, fieldInfoFlowQuality} {
		if !fields[f] {
			t.Errorf("no contradiction for %s (got %v)", f, fields)
		}
	}
	// Out-of-range values are treated as not entered.
	result, err := NewAnalyzer().AnalyzeCurrentState(vs)
	if err != nil {
		t.Fatal(err)
	}
	w := result.WasteAnalysis
	if _, ok := w.WasteByType["inventory"]; ok || w.TotalWaste != nil || result.FlowAnalysis.InformationFlowQuality != nil {
		t.Errorf("waste %+v, info quality %v: want not estimated", w, result.FlowAnalysis.InformationFlowQuality)
	}
	// In range (0 and 1 included): no issue.
	vs = wasteStream(&WasteAssumptions{InventoryShareOfWaiting: F(0), WasteShareOfHandling: F(1), ReductionPotential: F(0)})
	vs.InformationFlowQuality = F(1)
	if issues := assumptionIssues(vs); len(issues) != 0 {
		t.Errorf("issues for in-range values: %v", issues)
	}
}

func TestInformationFlowQualityUserEntered(t *testing.T) {
	a := NewAnalyzer()
	vs := &ValueStream{Processes: []Process{{ID: "p", Parameters: ProcessParameters{ProcessTime: F(1)}}}}
	got, _ := a.AnalyzeFlow(vs)
	if got.InformationFlowQuality != nil || !hasNote(got.Notes, fieldInfoFlowQuality) {
		t.Errorf("quality %v, notes %q: want not estimated with a note", got.InformationFlowQuality, got.Notes)
	}
	for _, q := range []float64{0, 0.7} {
		vs.InformationFlowQuality = F(q)
		got, _ = a.AnalyzeFlow(vs)
		if got.InformationFlowQuality == nil || *got.InformationFlowQuality != q {
			t.Errorf("quality = %v, want %v", got.InformationFlowQuality, q)
		}
	}
}

func TestFlowTypeFromKnownValuesOnly(t *testing.T) {
	a := NewAnalyzer()
	proc := func(p ProcessParameters) Process {
		p.TimeUnit = TimeUnitMinutes
		return Process{ID: "x", Parameters: p}
	}
	tests := []struct {
		name string
		ps   []Process
		want FlowType
	}{
		// The second process's process time is unknown (an impossible
		// setup time): before, its waiting 50 against an unknown process
		// time read as 0 counted as push.
		{"unknown process time not counted", []Process{
			proc(ProcessParameters{ProcessTime: F(10), WaitingTime: F(0.5)}),
			proc(ProcessParameters{SetupTime: F(-1), WaitingTime: F(50)})}, FlowTypePull},
		// Only a cycle time: neither process time nor waiting is known.
		// Before, unknown waiting 0 < unknown process time 0 × 0.1 failed
		// and counted as push.
		{"cycle time only", []Process{proc(ProcessParameters{CycleTime: F(2)})}, ""},
		// Idle time given only as a total: its waiting share is unknown.
		{"idle total does not give waiting", []Process{
			proc(ProcessParameters{ProcessTime: F(10), WaitingTime: F(0.5)}),
			proc(ProcessParameters{ProcessTime: F(10), IdleTime: F(30)}),
			proc(ProcessParameters{ProcessTime: F(10), IdleTime: F(40)})}, FlowTypePull},
		// Process time with no idle parts: idle (and waiting) is 0.
		{"known idle 0 is waiting 0", []Process{proc(ProcessParameters{ProcessTime: F(10)})}, FlowTypePull},
		{"known push", []Process{proc(ProcessParameters{ProcessTime: F(10), WaitingTime: F(5)})}, FlowTypePush},
		// Waiting time alone: by the blank-parts rule its process time is
		// 0, so the waiting time is long against it.
		{"waiting only is push", []Process{proc(ProcessParameters{WaitingTime: F(5)})}, FlowTypePush},
		{"nothing known", []Process{proc(ProcessParameters{}), proc(ProcessParameters{TimeUnit: "fortnights"})}, ""},
		{"no processes", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := a.AnalyzeFlow(&ValueStream{Processes: tt.ps})
			if err != nil {
				t.Fatal(err)
			}
			if got.FlowType != tt.want {
				t.Errorf("flow type = %q, want %q", got.FlowType, tt.want)
			}
			if (tt.want == "") != hasNote(got.Notes, "flow type not determined") {
				t.Errorf("notes = %q", got.Notes)
			}
		})
	}
}

func TestAssumptionsJSONRoundTrip(t *testing.T) {
	s := NewSerializer()
	vs := wasteStream(&WasteAssumptions{InventoryShareOfWaiting: F(0), ReductionPotential: F(0.3)})
	vs.InformationFlowQuality = F(0.5)
	data, err := s.SerializeValueStream(vs)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"waste_assumptions"`, `"inventory_share_of_waiting": 0`, `"reduction_potential": 0.3`,
		`"information_flow_quality": 0.5`} {
		if !strings.Contains(string(data), key) {
			t.Errorf("JSON lacks %s:\n%s", key, data)
		}
	}
	if strings.Contains(string(data), "waste_share_of_handling") {
		t.Errorf("not-entered field written:\n%s", data)
	}
	back, err := s.DeserializeValueStream(data)
	if err != nil {
		t.Fatal(err)
	}
	wa := back.WasteAssumptions
	if wa == nil || val(wa.InventoryShareOfWaiting) != 0 || wa.InventoryShareOfWaiting == nil ||
		wa.WasteShareOfHandling != nil || val(wa.ReductionPotential) != 0.3 || val(back.InformationFlowQuality) != 0.5 {
		t.Errorf("round trip = %+v, quality %v", wa, back.InformationFlowQuality)
	}
	// A file without the fields loads with nothing entered.
	old, err := s.DeserializeValueStream([]byte(`{"id":"o","name":"o","processes":[{"id":"p","name":"p","parameters":{"process_time":1}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if old.WasteAssumptions != nil || old.InformationFlowQuality != nil {
		t.Errorf("legacy file: assumptions %+v, quality %v", old.WasteAssumptions, old.InformationFlowQuality)
	}
}
