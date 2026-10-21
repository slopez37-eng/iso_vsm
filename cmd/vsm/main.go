// Command vsm evaluates value stream maps by ISO 22468:2020.
//
//	vsm check [-json] FILE                              evaluate a value stream or project JSON file
//	vsm import [-base FILE] [-name NAME] [-unit UNIT] FILE.csv   write a value stream JSON from a CSV file
//	vsm symbols                                         list the value stream symbols of Table A.1
//	vsm template                                        print a starter project JSON
//	vsm version                                         print the module version and the commit it was built from
//
// check exits with status 1 when the file has errors. import writes the value
// stream to standard output without evaluating it, and exits with status 2 on
// errors.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
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
	case "import":
		err = importCSV(os.Args[2:], os.Stdout)
	case "symbols":
		err = symbols(os.Stdout)
	case "template":
		err = template(os.Stdout)
	case "version":
		fmt.Println(version())
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
	fmt.Fprintln(os.Stderr, "usage: vsm check [-json] FILE | vsm import [-base FILE] [-name NAME] [-unit UNIT] FILE.csv | vsm symbols | vsm template | vsm version")
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

// importCSV writes a value stream built from a CSV file as JSON. It starts from
// the value stream in -base, or from an empty current state, and -name and
// -unit replace its name and time unit. The result is not evaluated.
func importCSV(args []string, w io.Writer) error {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	base := fs.String("base", "", "start from this value stream JSON file")
	name := fs.String("name", "", "name of the value stream")
	unit := fs.String("unit", "", "time unit of the CSV times: s, min, h or d")
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

	vs := &vsm.ValueStream{State: vsm.StateCurrent}
	if *base != "" {
		b, err := vsm.LoadValueStream(*base)
		if err != nil {
			return err
		}
		vs = b
	}
	if *name != "" {
		vs.Name = *name
	}
	if *unit != "" {
		if !vsm.TimeUnit(*unit).Valid() {
			return fmt.Errorf("-unit %q is not one of s, min, h, d", *unit)
		}
		vs.TimeUnit = vsm.TimeUnit(*unit)
	}
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := vsm.ReadCSV(f, vs); err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	return vsm.Write(w, vs)
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
	if r.Comparison != nil {
		printComparison(w, "future", r.Comparison)
	}
	for _, m := range r.Monitoring {
		fmt.Fprintf(w, "\n== Monitoring %s ==\n", m.Date)
		printResult(w, *m.Result)
		if m.Comparison != nil {
			printComparison(w, m.Date, m.Comparison)
		}
	}
	if len(r.Measures) > 0 {
		fmt.Fprintf(w, "\n== Measures ==\npriority (benefit − cost, then benefit ÷ cost): %v\n", r.Priority)
		for _, m := range r.Measures {
			if len(m.SMARTGaps) > 0 {
				fmt.Fprintf(w, "measure %d: %s\n", m.Number, strings.Join(m.SMARTGaps, "; "))
			}
			if m.Measured.Known {
				line := fmt.Sprintf("measure %d: measured %s", m.Number, val(m.Measured))
				if m.Target != nil {
					line += ", target " + num(*m.Target)
				}
				if m.Met != nil {
					verdict := "not met"
					if *m.Met {
						verdict = "met"
					}
					line += ", " + verdict
				}
				fmt.Fprintln(w, line)
			}
			if m.Payback.Known {
				fmt.Fprintf(w, "measure %d: payback %s %s\n", m.Number, val(m.Payback), periodUnit(m.Payback.Value, m.BenefitPeriod))
			}
		}
	}
	printIssues(w, r.Issues)
}

// printComparison prints the 4.5.2 comparison of the current state with label,
// which names a later state or a monitoring date.
func printComparison(w io.Writer, label string, c *vsm.StateComparison) {
	fmt.Fprintf(w, "\n== Current vs %s (4.5.2), in %s ==\n", label, c.TimeUnit)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "\tcurrent\t%s\tω (%s ÷ current)\n", label, label)
	fmt.Fprintf(tw, "process time\t%s\t%s\t%s\n", num(c.ActualProcessTime), num(c.TargetProcessTime), val(c.Figures.ProcessTime))
	fmt.Fprintf(tw, "idle time\t%s\t%s\t%s\n", num(c.ActualIdleTime), num(c.TargetIdleTime), val(c.Figures.IdleTime))
	fmt.Fprintf(tw, "lead time\t%s\t%s\t%s\n", num(c.ActualLeadTime), num(c.TargetLeadTime), val(c.Figures.LeadTime))
	fmt.Fprintf(tw, "value adding share\t%s\t%s\t\n", pct(c.ActualValueAddingShare), pct(c.TargetValueAddingShare))
	fmt.Fprintf(tw, "non-value adding share\t%s\t%s\t\n", pct(c.ActualNonValueAddingShare), pct(c.TargetNonValueAddingShare))
	tw.Flush()
}

func printResult(w io.Writer, r vsm.Result) {
	fmt.Fprintf(w, "%s (times in %s)\n", r.Name, r.TimeUnit)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "process\tPT\tCT\ttakt load\tload")
	for _, p := range r.Processes {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", p.ID, val(p.ProcessTime), val(p.CycleTime), pctVal(p.TaktLoad), pctVal(p.Load))
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

// periodUnit names a benefit period for a count, such as "month" or "months".
func periodUnit(n float64, p vsm.Period) string {
	if n == 1 {
		return string(p)
	}
	return string(p) + "s"
}

func pctVal(v vsm.Value) string {
	if !v.Known {
		return "?"
	}
	return pct(v.Value)
}

// version names the module version and the commit the tool was built from.
func version() string {
	info, ok := debug.ReadBuildInfo()
	return versionFrom(info, ok)
}

// versionFrom formats build information as "iso_vsm VERSION COMMIT", with
// +dirty after the commit when the tree had local changes. Without commit
// information, such as a go test binary, it gives the version alone.
func versionFrom(info *debug.BuildInfo, ok bool) string {
	if !ok {
		return "iso_vsm (unknown version)"
	}
	s := "iso_vsm " + info.Main.Version
	var rev string
	var dirty bool
	for _, set := range info.Settings {
		switch set.Key {
		case "vcs.revision":
			rev = set.Value
		case "vcs.modified":
			dirty = set.Value == "true"
		}
	}
	if rev == "" {
		return s
	}
	if len(rev) > 12 {
		rev = rev[:12]
	}
	s += " " + rev
	if dirty {
		s += "+dirty"
	}
	return s
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
					CycleTime: vsm.F(1.5), SetupTime: vsm.F(30), RepetitionTime: vsm.F(1.5), Repetitions: vsm.F(40), BatchSize: vsm.F(40)},
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
			Metric: "range between P1 and P2 (min)", Target: vsm.F(240), Responsible: "logistics lead", Accepted: true, Realistic: true, Deadline: "2027-01-31",
			Status: vsm.StatusMeasureChecked, Flashes: []int{1}, Cost: vsm.F(2000), Benefit: vsm.F(9000)}},
	}
	return vsm.Write(w, &p)
}
