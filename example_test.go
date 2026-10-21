package vsm_test

import (
	"fmt"

	vsm "github.com/slopez37-eng/iso_vsm"
)

func ExampleEvaluate() {
	vs := &vsm.ValueStream{
		Name: "line 3", State: vsm.StateCurrent, TimeUnit: vsm.Minute,
		Customer: vsm.Customer{Name: "end customer", Period: "day",
			Demand: vsm.F(400), OperatingTime: vsm.F(800)},
		Processes: []vsm.Process{
			{ID: "P1", Name: "cutting", ProcessTime: vsm.F(4), CycleTime: vsm.F(1.5)},
			{ID: "P2", Name: "assembly", ProcessTime: vsm.F(6), CycleTime: vsm.F(1.8)},
		},
		Idles: []vsm.Idle{
			{ID: "I1", From: "P1", To: "P2", IdleTime: vsm.F(10)},
			{ID: "I2", From: "P2", IdleTime: vsm.F(20)}, // To empty: ends at the customer
		},
	}
	r := vsm.Evaluate(vs)
	fmt.Printf("Lead time: %g %s\n", r.Totals.LeadTime.Value, r.TimeUnit)
	fmt.Printf("Value adding share: %.1f %%\n", r.KPIs.ValueAddingShare.Value*100)
	fmt.Printf("Customer takt: %g %s\n", r.KPIs.CustomerTakt.Value, r.TimeUnit)
	fmt.Printf("Bottleneck: %s\n", r.Bottleneck)

	// Output:
	// Lead time: 40 min
	// Value adding share: 25.0 %
	// Customer takt: 2 min
	// Bottleneck: P2
}
