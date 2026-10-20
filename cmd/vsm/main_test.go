package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
