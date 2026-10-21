package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	vsm "github.com/slopez37-eng/iso_vsm"
)

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
