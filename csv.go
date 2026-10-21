package vsm

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// csvColumn is one column of a CSV import. Each path is the field it fills in
// the row's element type, or nil when that type has no such field.
type csvColumn struct {
	name string
	proc []int // a Process field, for process rows
	idle []int // an Idle field, for idle rows
	inv  []int // an Inventory field, for the inventory_ columns of idle and inventory rows
}

// ReadCSV reads processes, idles and inventories from CSV and appends them to
// vs, in row order. It is strict: an unknown column, a value in a column the
// row's element does not have, or a malformed number is an error that names
// the line (and column), and nothing is appended.
//
// The first line is the header. Its delimiter is a comma, semicolon or tab,
// whichever occurs most often. The column element is required and says what
// each row is: process, idle or inventory. Every other column is the JSON name
// of a field of Process or Idle, or inventory_ followed by the JSON name of a
// field of Inventory. An empty cell means the value was not entered. Lists of
// numbers use "|" between values, and numbers use a point as the decimal
// separator. Header names, the element cell and boolean cells are
// case-insensitive; other cells are kept as typed, so enum values must be
// written as in the JSON format, such as manual. Surrounding spaces are trimmed
// from each cell and from each value of a list.
//
// An inventory row adds an Inventory to the most recent idle row of the file,
// so it must come after one. An idle row without from or to takes the id of the
// neighbouring process row in the file: from is the previous process row, and
// to is the next one, which is empty for the segment that ends at the customer.
func ReadCSV(r io.Reader, vs *ValueStream) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	data = bytes.TrimPrefix(data, []byte("\uFEFF"))
	header, _, _ := strings.Cut(string(data), "\n")
	cr := csv.NewReader(bytes.NewReader(data))
	cr.Comma = detectDelimiter(header)

	type record struct {
		line  int
		cells []string
	}
	var records []record
	for {
		cells, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("CSV: %w", err)
		}
		line, _ := cr.FieldPos(0)
		for i := range cells {
			cells[i] = cleanCell(cells[i])
		}
		records = append(records, record{line, cells})
	}
	if len(records) == 0 {
		return errors.New("CSV: no header line")
	}

	cols, elem, err := csvHeader(records[0].cells, records[0].line)
	if err != nil {
		return err
	}

	var (
		procs       []Process
		idles       []Idle
		procIDs     []string // ids of the process rows, in file order
		procsBefore []int    // for each idle, how many process rows come before it
		lastIdle    = -1     // index in idles of the most recent idle row
	)
	for _, rec := range records[1:] {
		kind := strings.ToLower(rec.cells[elem])
		if kind == "" {
			return fmt.Errorf("CSV line %d: element is empty; use process, idle or inventory", rec.line)
		}
		switch kind {
		case "process":
			var p Process
			for j, c := range cols {
				if j == elem || rec.cells[j] == "" {
					continue
				}
				if c.proc == nil {
					return csvColumnErr(rec.line, c.name, "does not apply to a process row")
				}
				if err := setCSVCell(reflect.ValueOf(&p).Elem(), c.proc, rec.cells[j]); err != nil {
					return csvColumnErr(rec.line, c.name, err.Error())
				}
			}
			procs = append(procs, p)
			procIDs = append(procIDs, p.ID)
		case "idle":
			var idle Idle
			var inv Inventory
			hasInv := false
			for j, c := range cols {
				if j == elem || rec.cells[j] == "" {
					continue
				}
				switch {
				case c.idle != nil:
					if err := setCSVCell(reflect.ValueOf(&idle).Elem(), c.idle, rec.cells[j]); err != nil {
						return csvColumnErr(rec.line, c.name, err.Error())
					}
				case c.inv != nil:
					if err := setCSVCell(reflect.ValueOf(&inv).Elem(), c.inv, rec.cells[j]); err != nil {
						return csvColumnErr(rec.line, c.name, err.Error())
					}
					hasInv = true
				default:
					return csvColumnErr(rec.line, c.name, "does not apply to an idle row")
				}
			}
			if hasInv {
				idle.Inventories = append(idle.Inventories, inv)
			}
			idles = append(idles, idle)
			procsBefore = append(procsBefore, len(procIDs))
			lastIdle = len(idles) - 1
		case "inventory":
			if lastIdle < 0 {
				return fmt.Errorf("CSV line %d: an inventory row needs an idle row before it", rec.line)
			}
			var inv Inventory
			hasInv := false
			for j, c := range cols {
				if j == elem || rec.cells[j] == "" {
					continue
				}
				if c.inv == nil {
					return csvColumnErr(rec.line, c.name, "does not apply to an inventory row")
				}
				if err := setCSVCell(reflect.ValueOf(&inv).Elem(), c.inv, rec.cells[j]); err != nil {
					return csvColumnErr(rec.line, c.name, err.Error())
				}
				hasInv = true
			}
			if !hasInv {
				return fmt.Errorf("CSV line %d: an inventory row has no inventory_ values", rec.line)
			}
			idles[lastIdle].Inventories = append(idles[lastIdle].Inventories, inv)
		default:
			return fmt.Errorf("CSV line %d: element %q is not process, idle or inventory", rec.line, kind)
		}
	}

	for i := range idles {
		k := procsBefore[i]
		if idles[i].From == "" && k > 0 {
			idles[i].From = procIDs[k-1]
		}
		if idles[i].To == "" && k < len(procIDs) {
			idles[i].To = procIDs[k]
		}
	}
	vs.Processes = append(vs.Processes, procs...)
	vs.Idles = append(vs.Idles, idles...)
	return nil
}

// csvHeader maps each header name to its field paths and finds the element
// column. Names are case-insensitive. An unknown or repeated name is an error.
func csvHeader(names []string, line int) ([]csvColumn, int, error) {
	procPaths := jsonPaths(reflect.TypeFor[Process](), "")
	idlePaths := jsonPaths(reflect.TypeFor[Idle](), "")
	delete(idlePaths, "inventories") // the inventory_ columns fill these
	invPaths := jsonPaths(reflect.TypeFor[Inventory](), "inventory_")

	cols := make([]csvColumn, len(names))
	elem := -1
	seen := map[string]bool{}
	for j, raw := range names {
		name := strings.ToLower(raw)
		if seen[name] {
			return nil, 0, fmt.Errorf("CSV line %d: column %q appears twice", line, name)
		}
		seen[name] = true
		if name == "element" {
			elem = j
			continue
		}
		c := csvColumn{name: name, proc: procPaths[name], idle: idlePaths[name], inv: invPaths[name]}
		if c.proc == nil && c.idle == nil && c.inv == nil {
			return nil, 0, fmt.Errorf("CSV line %d: unknown column %q", line, name)
		}
		cols[j] = c
	}
	if elem < 0 {
		return nil, 0, fmt.Errorf("CSV line %d: no element column; name one column element", line)
	}
	return cols, elem, nil
}

// jsonPaths maps the JSON name of each field of a struct type, with prefix in
// front, to the index path of the field. Fields of an embedded struct that has
// no JSON name, such as ValueShares, are included. Fields without a JSON name
// are left out.
func jsonPaths(t reflect.Type, prefix string) map[string][]int {
	paths := map[string][]int{}
	var walk func(t reflect.Type, index []int)
	walk = func(t reflect.Type, index []int) {
		for i := range t.NumField() {
			f := t.Field(i)
			name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			path := append(slices.Clone(index), i)
			if f.Anonymous && name == "" && f.Type.Kind() == reflect.Struct {
				walk(f.Type, path)
			} else if name != "" && name != "-" {
				paths[prefix+name] = path
			}
		}
	}
	walk(t, nil)
	return paths
}

// detectDelimiter returns the delimiter that occurs most often in the header
// line. A tie goes to the comma.
func detectDelimiter(header string) rune {
	best, most := ',', 0
	for _, d := range []rune{',', ';', '\t'} {
		if n := strings.Count(header, string(d)); n > most {
			best, most = d, n
		}
	}
	return best
}

// cleanCell removes a byte order mark and the spaces around a cell.
func cleanCell(s string) string {
	return strings.TrimSpace(strings.TrimPrefix(s, "\uFEFF"))
}

func csvColumnErr(line int, column, msg string) error {
	return fmt.Errorf("CSV line %d, column %s: %s", line, column, msg)
}

// setCSVCell parses cell and stores it in the field at path of v, which is a
// struct. The field's type decides how the cell is read.
func setCSVCell(v reflect.Value, path []int, cell string) error {
	f := v.FieldByIndex(path)
	switch f.Kind() {
	case reflect.Pointer: // *float64
		x, err := parseNumber(cell)
		if err != nil {
			return err
		}
		f.Set(reflect.ValueOf(&x))
	case reflect.Slice: // []float64, values separated by |
		var xs []float64
		for _, part := range strings.Split(cell, "|") {
			if strings.TrimSpace(part) == "" {
				return fmt.Errorf("%q has an empty value between |", cell)
			}
			x, err := parseNumber(strings.TrimSpace(part))
			if err != nil {
				return err
			}
			xs = append(xs, x)
		}
		f.Set(reflect.ValueOf(xs))
	case reflect.Bool:
		b, err := parseBool(cell)
		if err != nil {
			return err
		}
		f.SetBool(b)
	case reflect.String: // text, and the string types such as ProcessType
		f.SetString(cell)
	default:
		return fmt.Errorf("the field has type %s, which CSV cannot fill", f.Type())
	}
	return nil
}

// parseNumber reads a finite decimal number with a point as separator.
func parseNumber(s string) (float64, error) {
	x, err := strconv.ParseFloat(s, 64)
	if err != nil {
		if strings.Contains(s, ",") {
			return 0, fmt.Errorf("%q has a comma; use a point as the decimal separator, as in 1.5", s)
		}
		return 0, fmt.Errorf("%q is not a number", s)
	}
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return 0, fmt.Errorf("%q is not a finite number", s)
	}
	return x, nil
}

// parseBool reads true, false, yes, no, 1 or 0, in any case.
func parseBool(s string) (bool, error) {
	switch strings.ToLower(s) {
	case "true", "yes", "1":
		return true, nil
	case "false", "no", "0":
		return false, nil
	}
	return false, fmt.Errorf("%q is not true, false, yes, no, 1 or 0", s)
}
