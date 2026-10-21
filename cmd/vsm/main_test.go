package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"

	vsm "github.com/slopez37-eng/iso_vsm"
)

func TestVersionNamesCommit(t *testing.T) {
	dirty := &debug.BuildInfo{
		Main: debug.Module{Version: "v1.2.3"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "594ef6b0123456789abcdef"},
			{Key: "vcs.modified", Value: "true"},
		},
	}
	devel := &debug.BuildInfo{
		Main:     debug.Module{Version: "(devel)"},
		Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "594ef6b"}},
	}
	for _, c := range []struct {
		info *debug.BuildInfo
		ok   bool
		want string
	}{
		{dirty, true, "iso_vsm v1.2.3 594ef6b01234+dirty"},
		{devel, true, "iso_vsm (devel) 594ef6b"},
		{&debug.BuildInfo{Main: debug.Module{Version: "v1.2.3"}}, true, "iso_vsm v1.2.3"},
		{nil, false, "iso_vsm (unknown version)"},
	} {
		if got := versionFrom(c.info, c.ok); got != c.want {
			t.Errorf("versionFrom = %q, want %q", got, c.want)
		}
	}
}

func TestCheckTemplate(t *testing.T) {
	dir := t.TempDir()
	var tpl bytes.Buffer
	if err := template(&tpl); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "project.json")
	if err := os.WriteFile(path, tpl.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code, err := check([]string{path}, &out)
	if err != nil || code != 0 {
		t.Fatalf("code %d, err %v\n%s", code, err, out.String())
	}
	if !strings.Contains(out.String(), "lead time") {
		t.Errorf("report has no lead time:\n%s", out.String())
	}
}

func TestCheckExitsOneOnErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stream.json")
	stream := `{"name":"x","time_unit":"s","processes":[{"id":"P1","process_time":-1}]}`
	if err := os.WriteFile(path, []byte(stream), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code, err := check([]string{"-json", path}, &out)
	if err != nil || code != 1 {
		t.Fatalf("code %d, err %v, want 1", code, err)
	}
	if !strings.Contains(out.String(), `"severity": "error"`) {
		t.Errorf("JSON report has no error:\n%s", out.String())
	}
}

func TestCheckProjectWithoutCurrent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project.json")
	if err := os.WriteFile(path, []byte(`{"schema_version":1,"name":"p"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code, err := check([]string{path}, &out)
	if err != nil || code != 1 || !strings.Contains(out.String(), "needs a current state") {
		t.Fatalf("code %d, err %v\n%s", code, err, out.String())
	}
}

func TestCheckPrintsMonitoring(t *testing.T) {
	stream := func(idle float64) *vsm.ValueStream {
		return &vsm.ValueStream{Name: "stream", TimeUnit: vsm.Minute,
			Processes: []vsm.Process{{ID: "P1", ProcessTime: vsm.F(5)}},
			Idles:     []vsm.Idle{{ID: "I1", From: "P1", IdleTime: vsm.F(idle)}}}
	}
	p := vsm.Project{Name: "p", Current: stream(1), Monitoring: []vsm.Snapshot{{Date: "2027-01-01", State: stream(0.5)}}}
	path := filepath.Join(t.TempDir(), "project.json")
	if err := vsm.Save(path, &p); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code, err := check([]string{path}, &out)
	if err != nil || code != 0 {
		t.Fatalf("code %d, err %v\n%s", code, err, out.String())
	}
	for _, want := range []string{"== Monitoring 2027-01-01 ==", "Current vs 2027-01-01 (4.5.2)"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("report has no %q:\n%s", want, out.String())
		}
	}
}

func TestCheckFlagAfterFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project.json")
	if err := os.WriteFile(path, []byte(`{"schema_version":1,"name":"p"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if _, err := check([]string{path, "-json"}, &out); err != nil || !strings.Contains(out.String(), `"severity"`) {
		t.Fatalf("err %v, want a JSON report:\n%s", err, out.String())
	}
}

func TestImportCSVWritesAStreamThatEvaluates(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base.json")
	if err := os.WriteFile(base, []byte(`{"name":"base","state":"current","time_unit":"min","customer":{"name":"Customer","demand":100,"operating_time":480}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	rows := "element,id,name,process_time,from,to,idle_time\n" +
		"process,P1,cutting,2,,,\n" +
		"idle,I1,,,,,3\n" +
		"process,P2,assembly,4,,,\n"
	csvPath := filepath.Join(dir, "stream.csv")
	if err := os.WriteFile(csvPath, []byte(rows), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	// The flags come after the file name, which is allowed.
	if err := importCSV([]string{csvPath, "-base", base, "-name", "Line 2"}, &out); err != nil {
		t.Fatal(err)
	}
	vs, err := vsm.ReadValueStream(&out)
	if err != nil {
		t.Fatal(err)
	}
	if vs.Name != "Line 2" || len(vs.Processes) != 2 || len(vs.Idles) != 1 {
		t.Fatalf("name %q, %d processes, %d idles", vs.Name, len(vs.Processes), len(vs.Idles))
	}
	if res := vsm.Evaluate(vs); res.HasErrors() {
		t.Errorf("imported stream has errors: %+v", res.Issues)
	}
}

func TestImportCSVRejectsBadUnit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stream.csv")
	if err := os.WriteFile(path, []byte("element,id\nprocess,P1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := importCSV([]string{"-unit", "fortnight", path}, &out); err == nil || !strings.Contains(err.Error(), `"fortnight"`) {
		t.Errorf("err = %v, want the unit named", err)
	}
}

func TestPrintsPaybackWithPeriodUnit(t *testing.T) {
	stream := &vsm.ValueStream{Name: "stream", TimeUnit: vsm.Minute,
		Processes: []vsm.Process{{ID: "P1", ProcessTime: vsm.F(5)}}}
	p := vsm.Project{Name: "p", Current: stream, Measures: []vsm.Measure{
		{Number: 1, Status: vsm.StatusMeasureChecked, Cost: vsm.F(120), Benefit: vsm.F(30), BenefitPeriod: vsm.PeriodMonth},
		{Number: 2, Status: vsm.StatusMeasureChecked, Cost: vsm.F(60), Benefit: vsm.F(60), BenefitPeriod: vsm.PeriodYear},
	}}
	var out bytes.Buffer
	printProject(&out, vsm.EvaluateProject(&p))
	for _, want := range []string{"measure 1: payback 4 months", "measure 2: payback 1 year"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("report has no %q:\n%s", want, out.String())
		}
	}
}

func TestCheckStreamWithoutProcesses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stream.json")
	if err := os.WriteFile(path, []byte(`{"name":"x","time_unit":"min"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code, err := check([]string{path}, &out)
	if err != nil || code != 1 || !strings.Contains(out.String(), "has no processes") {
		t.Fatalf("code %d, err %v\n%s", code, err, out.String())
	}
}
