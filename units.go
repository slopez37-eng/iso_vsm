package vsm

import "fmt"

// TimeUnit is the unit every time value of a value stream is entered in.
type TimeUnit string

const (
	Second TimeUnit = "s"
	Minute TimeUnit = "min"
	Hour   TimeUnit = "h"
	Day    TimeUnit = "d" // a calendar day of 24 hours
)

var secondsPer = map[TimeUnit]float64{Second: 1, Minute: 60, Hour: 3600, Day: 86400}

// Valid reports whether u is a known time unit.
func (u TimeUnit) Valid() bool {
	_, ok := secondsPer[u]
	return ok
}

// Convert converts value from one time unit to another.
func Convert(value float64, from, to TimeUnit) (float64, error) {
	f, ok := secondsPer[from]
	if !ok {
		return 0, fmt.Errorf("vsm: unknown time unit %q", from)
	}
	t, ok := secondsPer[to]
	if !ok {
		return 0, fmt.Errorf("vsm: unknown time unit %q", to)
	}
	return value * f / t, nil
}
