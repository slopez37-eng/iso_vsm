package vsm

import (
	"errors"
	"math"
	"testing"
)

func approx(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9*math.Max(1, math.Abs(want)) {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}

// TestWorkedExampleA34 follows the calculation of the handling time in the
// standard's worked example (A.3.4), step by step with the formula functions.
func TestWorkedExampleA34(t *testing.T) {
	const (
		reps, demand, idle, lead, operating = 2.0, 20.0, 10.0, 45.0, 3.0
		processing, rng, repTime, ttr       = 8.0, 3.0, 12.5, 2.0
	)
	pt := lead - idle // A.4 rearranged
	approx(t, "PT", pt, 35)
	st := pt - repTime*reps // A.6 rearranged
	approx(t, "ST", st, 10)
	approx(t, "PT from A.6", ProcessTimeFromRepetitions(st, repTime, reps), pt)
	sq := rng * demand // A.7 rearranged
	approx(t, "SQ", sq, 60)
	r, err := Range(sq, demand)
	if err != nil {
		t.Fatal(err)
	}
	approx(t, "R", r, rng)
	cut, err := CustomerTakt(operating, demand)
	if err != nil {
		t.Fatal(err)
	}
	approx(t, "CUT", cut, 0.15)
	wt := WaitingTime(sq, cut)
	approx(t, "WT", wt, 9)
	dt := Downtime(ttr)
	approx(t, "DT", dt, 2)
	approx(t, "HT", HandlingTime(pt, st, processing, wt, dt), 6)
	approx(t, "ST from A.8", SetupTime(pt, processing, wt, 6, dt), st)
	approx(t, "LT", LeadTime([]float64{pt}, []float64{idle}), lead)
}

// TestCustomerTaktB2 checks the takt of application example B.2: 5 760 hours
// of operating time per year for 3 000 pieces is 115,2 minutes per piece.
func TestCustomerTaktB2(t *testing.T) {
	ot, _ := Convert(5760, Hour, Minute)
	cut, err := CustomerTakt(ot, 3000)
	if err != nil {
		t.Fatal(err)
	}
	approx(t, "CUT", cut, 115.2)
}

func TestFormulas(t *testing.T) {
	approx(t, "A.2", IdleTime(1, 2, 3), 6)
	approx(t, "A.3", LeadTimeFromValueShares(1, 2, 3), 6)
	approx(t, "A.5", ProcessTimeFromCycleTime(2, 3, 4), 24)
	approx(t, "A.9", StorageTime(1.5, 2.5), 4)
	approx(t, "A.10", TransportTime(4, 2.5), 10)
	approx(t, "A.15", ReworkRate(0.90, 0.95), 0.05)
	approx(t, "A.17 YF", YieldFactor(0.1), 0.9)
	approx(t, "A.17 SR", ScrapRate(0.9), 0.1)
	turns, _ := InventoryTurns(120, 40)
	approx(t, "A.13", turns, 3)
	otif, _ := OnTimeInFull(45, 50)
	approx(t, "A.14", otif, 0.9)
	if _, err := OnTimeInFull(51, 50); err == nil {
		t.Error("A.14 accepted more complete deliveries than total deliveries")
	}
	var_, _ := ValueAddedRatio(5, 50)
	approx(t, "A.16", var_, 0.1)
	for name, f := range map[string]func() (float64, error){
		"Range":          func() (float64, error) { return Range(1, 0) },
		"CustomerTakt":   func() (float64, error) { return CustomerTakt(1, 0) },
		"InventoryTurns": func() (float64, error) { return InventoryTurns(1, 0) },
	} {
		if _, err := f(); !errors.Is(err, ErrUndefined) {
			t.Errorf("%s by zero: err = %v, want ErrUndefined", name, err)
		}
	}
}

// TestComparisonIdentity checks the identity of clause 4.5.2:
// ω_LT = S_VA,ACT × ω_PT + S_NVA,ACT × ω_IT.
func TestComparisonIdentity(t *testing.T) {
	apt, ait, tpt, tit := 30.0, 970.0, 25.0, 300.0
	c := Compare(apt, ait, tpt, tit)
	approx(t, "ω_PT", c.ProcessTime.Value, 25.0/30)
	approx(t, "ω_IT", c.IdleTime.Value, 300.0/970)
	sva, _ := ValueAddingShare(apt, apt+ait)
	snva, _ := NonValueAddingShare(ait, apt+ait)
	approx(t, "ω_LT", c.LeadTime.Value, sva*c.ProcessTime.Value+snva*c.IdleTime.Value)
}

func TestConvert(t *testing.T) {
	v, err := Convert(1.5, Hour, Minute)
	if err != nil {
		t.Fatal(err)
	}
	approx(t, "1.5 h in min", v, 90)
	if _, err := Convert(1, "week", Day); err == nil {
		t.Error("Convert accepted an unknown unit")
	}
}
