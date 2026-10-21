// Command vsm evaluates value stream maps by ISO 22468:2020.
//
//	vsm check [-json] FILE   evaluate a value stream or project JSON file
//	vsm symbols              list the value stream symbols of Table A.1
//	vsm template             print a starter project JSON
//
// check exits with status 1 when the file has errors.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	vsm "github.com/slopez37-eng/iso_vsm"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	code := 0
	switch os.Args[1] {
	case "check":
		code, err = check(os.Args[2:], os.Stdout)
	case "symbols":
		err = symbols(os.Stdout)
	case "template":
		err = template(os.Stdout)
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "vsm:", err)
		os.Exit(2)
	}
	os.Exit(code)
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: vsm check [-json] FILE | vsm symbols | vsm template")
	os.Exit(2)
}

func check(args []string, w io.Writer) (int, error) {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "print the report as JSON")
	fs.Parse(args)
	// Flags may also follow the file name.
	if fs.NArg() == 0 {
		usage()
	}
	file := fs.Arg(0)
	fs.Parse(fs.Args()[1:])
	if fs.NArg() != 0 {
		usage()
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return 0, err
	}
	isProject, err := vsm.IsProjectJSON(data)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", file, err)
	}
	var report any
	var hasErrors bool
	var show func()
	if isProject {
		p, err := vsm.ReadProject(bytes.NewReader(data))
		if err != nil {
			return 0, err
		}
		r := vsm.EvaluateProject(p)
		report, hasErrors, show = r, r.HasErrors(), func() { printProject(w, r) }
	} else {
		vs, err := vsm.ReadValueStream(bytes.NewReader(data))
		if err != nil {
			return 0, err
		}
		r := vsm.Evaluate(vs)
		report, hasErrors, show = r, r.HasErrors(), func() { printResult(w, r) }
	}
	if *asJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return 0, err
		}
	} else {
		show()
	}
	if hasErrors {
		return 1, nil
	}
	return 0, nil
}

func printProject(w io.Writer, r vsm.ProjectReport) {
	fmt.Fprintf(w, "Project %s\n", r.Name)
	for _, s := range []struct {
		label string
		r     *vsm.Result
	}{{"Current state", r.Current}, {"Ideal state", r.Ideal}, {"Future state", r.Future}} {
		if s.r != nil {
			fmt.Fprintf(w, "\n== %s ==\n", s.label)
			printResult(w, *s.r)
		}
	}
	if c := r.Comparison; c != nil {
		fmt.Fprintf(w, "\n== Current vs future (4.5.2), in %s ==\n", c.TimeUnit)
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "\tcurrent\tfuture\tω (future ÷ current)")
		fmt.Fprintf(tw, "process time\t%s\t%s\t%s\n", num(c.ActualProcessTime), num(c.TargetProcessTime), val(c.Figures.ProcessTime))
		fmt.Fprintf(tw, "idle time\t%s\t%s\t%s\n", num(c.ActualIdleTime), num(c.TargetIdleTime), val(c.Figures.IdleTime))
		fmt.Fprintf(tw, "lead time\t%s\t%s\t%s\n", num(c.ActualLeadTime), num(c.TargetLeadTime), val(c.Figures.LeadTime))
		fmt.Fprintf(tw, "value adding share\t%s\t%s\t\n", pct(c.ActualValueAddingShare), pct(c.TargetValueAddingShare))
		fmt.Fprintf(tw, "non-value adding share\t%s\t%s\t\n", pct(c.ActualNonValueAddingShare), pct(c.TargetNonValueAddingShare))
		tw.Flush()
	}
	if len(r.Measures) > 0 {
		fmt.Fprintf(w, "\n== Measures ==\npriority (benefit ÷ cost): %v\n", r.Priority)
		for _, m := range r.Measures {
			if len(m.SMARTGaps) > 0 {
				fmt.Fprintf(w, "measure %d: %s\n", m.Number, strings.Join(m.SMARTGaps, "; "))
			}
		}
	}
	printIssues(w, r.Issues)
}

func printResult(w io.Writer, r vsm.Result) {
	fmt.Fprintf(w, "%s (times in %s)\n", r.Name, r.TimeUnit)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "process\tPT\tCT\ttakt load")
	for _, p := range r.Processes {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", p.ID, val(p.ProcessTime), val(p.CycleTime), pctVal(p.TaktLoad))
	}
	tw.Flush()
	if len(r.Idles) > 0 {
		tw = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "idle\tIT\tTT\tWT\tSTT")
		for _, i := range r.Idles {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", i.ID, val(i.IdleTime), val(i.TransportTime), val(i.WaitingTime), val(i.StorageTime))
		}
		tw.Flush()
	}
	t, k := r.Totals, r.KPIs
	fmt.Fprintf(w, "lead time %s = process %s + idle %s\n", val(t.LeadTime), val(t.ProcessTime), val(t.IdleTime))
	fmt.Fprintf(w, "customer takt %s, value added ratio %s, value adding share %s\n", val(k.CustomerTakt), pctVal(k.ValueAddedRatio), pctVal(k.ValueAddingShare))
	if k.OnTimeInFull.Known || k.YieldFactor.Known || k.InventoryTurns.Known {
		fmt.Fprintf(w, "OTIF %s, yield %s, inventory turns %s\n", pctVal(k.OnTimeInFull), pctVal(k.YieldFactor), val(k.InventoryTurns))
	}
	if r.Bottleneck != "" {
		fmt.Fprintf(w, "bottleneck: %s\n", r.Bottleneck)
	}
	printIssues(w, r.Issues)
}

func printIssues(w io.Writer, issues []vsm.Issue) {
	for _, i := range issues {
		fmt.Fprintln(w, "  "+i.String())
	}
}

func num(v float64) string { return fmt.Sprintf("%.4g", v) }
func pct(v float64) string { return fmt.Sprintf("%.1f%%", v*100) }

func val(v vsm.Value) string {
	if !v.Known {
		return "?"
	}
	return num(v.Value)
}

func pctVal(v vsm.Value) string {
	if !v.Known {
		return "?"
	}
	return pct(v.Value)
}

func symbols(w io.Writer) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "id\tcategory\tterm\tinformation")
	for _, s := range vsm.Symbols {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", s.ID, s.Category, s.Term, s.Information)
	}
	return tw.Flush()
}

func template(w io.Writer) error {
	p := vsm.Project{
		Name:  "my product family",
		Phase: vsm.PhaseAnalysis,
		Current: &vsm.ValueStream{
			Name: "current state", State: vsm.StateCurrent, TimeUnit: vsm.Minute,
			ProductFamily: &vsm.ProductFamily{Name: "family A", Rationale: "similar process steps, stable demand",
				Criteria: []vsm.CriterionStatus{{Criterion: vsm.FamilySimilarSteps, Met: true}, {Criterion: vsm.FamilyRepresentative, Met: true},
					{Criterion: vsm.FamilyBalancedVolume, Met: true}}},
			Customer:  vsm.Customer{Name: "end customer", Period: "day", Demand: vsm.F(400), OperatingTime: vsm.F(900), PeriodLength: vsm.F(1440)},
			Suppliers: []vsm.Supplier{{ID: "S1", Name: "supplier", DeliveryFrequency: "daily"}},
			Processes: []vsm.Process{
				{ID: "P1", Name: "cutting", Type: vsm.ProcessAutomated, Resources: vsm.F(1), Operators: vsm.F(1),
					CycleTime: vsm.F(1.5), SetupTime: vsm.F(30), RepetitionTime: vsm.F(1.5), Repetitions: vsm.F(40)},
				{ID: "P2", Name: "assembly", Type: vsm.ProcessManual, Operators: vsm.F(2), CycleTime: vsm.F(2), ProcessTime: vsm.F(2.5),
					ScrapRate: vsm.F(0.02)},
			},
			Idles: []vsm.Idle{
				{ID: "I0", From: "S1", To: "P1", TransportTime: vsm.F(0), WaitingTime: vsm.F(0), Inventories: []vsm.Inventory{{Kind: vsm.InventoryWarehouse, Quantity: vsm.F(1200)}}},
				{ID: "I1", From: "P1", To: "P2", TransportTime: vsm.F(5), WaitingTime: vsm.F(0), Inventories: []vsm.Inventory{{Kind: vsm.InventoryStock, Quantity: vsm.F(600)}}},
				{ID: "I2", From: "P2", TransportTime: vsm.F(0), WaitingTime: vsm.F(0), Inventories: []vsm.Inventory{{Kind: vsm.InventoryStock, Quantity: vsm.F(800)}}},
			},
			Flashes: []vsm.Flash{{Number: 1, Target: "I1", Description: "replace stock with a FIFO lane", Wastes: []vsm.Waste{vsm.WasteInventory}}},
		},
		Measures: []vsm.Measure{{Number: 1, Potential: "stock quantity", Description: "FIFO lane between cutting and assembly",
			Metric: "range between P1 and P2 (min)", Target: vsm.F(240), Responsible: "logistics lead", Deadline: "2027-01-31",
			Status: vsm.StatusMeasureChecked, Flashes: []int{1}, Cost: vsm.F(2000), Benefit: vsm.F(9000)}},
	}
	return vsm.Write(w, &p)
}
