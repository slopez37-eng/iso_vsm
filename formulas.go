package vsm

import (
	"errors"
	"fmt"
)

// ErrUndefined is returned when a formula would divide by zero.
var ErrUndefined = errors.New("vsm: undefined result (division by zero)")

// This file holds the calculation procedures of ISO 22468:2020, Annex A.3.2
// (formulas A.1 to A.11) and A.3.3 (key performance indicators, formulas
// A.12 to A.17), plus the relative indicators of clause 4.5.2.
//
// Every function takes and returns plain numbers. Time values must share one
// time unit; the result is in that unit. Percentages are fractions (0.25 means
// 25 %).

// Downtime returns the downtime as the sum of the times to repair (A.1).
func Downtime(timesToRepair ...float64) float64 {
	return sum(timesToRepair)
}

// IdleTime returns the idle time as transport time plus waiting time plus
// storage time (A.2).
func IdleTime(transport, waiting, storage float64) float64 {
	return transport + waiting + storage
}

// LeadTimeFromValueShares returns the lead time as the sum of value adding,
// non-value adding and necessary non-value adding time (A.3).
func LeadTimeFromValueShares(valueAdding, nonValueAdding, necessaryNonValueAdding float64) float64 {
	return valueAdding + nonValueAdding + necessaryNonValueAdding
}

// LeadTime returns the lead time as the sum of all process times plus the
// sum of all idle times (A.4).
func LeadTime(processTimes, idleTimes []float64) float64 {
	return sum(processTimes) + sum(idleTimes)
}

// ProcessTimeFromCycleTime returns the process time as cycle time multiplied
// by the product of process quantity and number of resources, as printed in
// A.5. See the README for why this form is kept as printed.
func ProcessTimeFromCycleTime(cycleTime, processQuantity, resources float64) float64 {
	return cycleTime * (processQuantity * resources)
}

// ProcessTimeFromRepetitions returns the process time as setup time plus
// repetition time multiplied by the number of repetitions (A.6).
func ProcessTimeFromRepetitions(setupTime, repetitionTime, repetitions float64) float64 {
	return setupTime + repetitionTime*repetitions
}

// Range returns the range of inventory as stock quantity divided by customer
// demand (A.7). The result is in review periods: if demand is per day, the
// range is in days.
func Range(stockQuantity, customerDemand float64) (float64, error) {
	return divide(stockQuantity, customerDemand)
}

// SetupTime returns the setup time as process time minus processing,
// waiting, handling and down time (A.8).
func SetupTime(processTime, processingTime, waitingTime, handlingTime, downtime float64) float64 {
	return processTime - (processingTime + waitingTime + handlingTime + downtime)
}

// HandlingTime rearranges A.8 for the handling time, as the worked example
// in A.3.4 does.
func HandlingTime(processTime, setupTime, processingTime, waitingTime, downtime float64) float64 {
	return processTime - (setupTime + processingTime + waitingTime + downtime)
}

// StorageTime returns the storage time as the sum of the ranges of stocks
// and warehouses (A.9).
func StorageTime(ranges ...float64) float64 {
	return sum(ranges)
}

// TransportTime returns the transport time as the number of transport
// repetitions multiplied by the transport repetition time (A.10).
func TransportTime(transportRepetitions, transportRepetitionTime float64) float64 {
	return transportRepetitions * transportRepetitionTime
}

// WaitingTime returns the waiting time as the number of waiting orders
// multiplied by the customer takt (A.11).
func WaitingTime(waitingOrders, customerTakt float64) float64 {
	return waitingOrders * customerTakt
}

// CustomerTakt returns the operating time divided by the customer demand of
// the same review period (A.12), in time units per piece.
func CustomerTakt(operatingTime, customerDemand float64) (float64, error) {
	return divide(operatingTime, customerDemand)
}

// InventoryTurns returns the usage divided by the average stock (A.13).
func InventoryTurns(usage, averageStock float64) (float64, error) {
	return divide(usage, averageStock)
}

// OnTimeInFull returns complete and on-time deliveries divided by total
// deliveries (A.14).
func OnTimeInFull(completeOnTimeDeliveries, totalDeliveries float64) (float64, error) {
	if completeOnTimeDeliveries > totalDeliveries {
		return 0, fmt.Errorf("vsm: %v complete on-time deliveries exceed %v total deliveries", completeOnTimeDeliveries, totalDeliveries)
	}
	return divide(completeOnTimeDeliveries, totalDeliveries)
}

// ReworkRate returns the rework rate as the yield factor after a rework
// iteration minus the yield factor before it (A.15). By A.17 this equals the
// scrap rate before minus the scrap rate after.
func ReworkRate(yieldBefore, yieldAfter float64) float64 {
	return yieldAfter - yieldBefore
}

// ValueAddedRatio returns the value adding time divided by the lead time
// (A.16).
func ValueAddedRatio(valueAddingTime, leadTime float64) (float64, error) {
	return divide(valueAddingTime, leadTime)
}

// YieldFactor returns 1 minus the scrap rate (A.17).
func YieldFactor(scrapRate float64) float64 {
	return 1 - scrapRate
}

// ScrapRate returns 1 minus the yield factor (A.17).
func ScrapRate(yieldFactor float64) float64 {
	return 1 - yieldFactor
}

// ValueAddingShare returns the total process time divided by the lead time
// (S_VA, clause 4.5.2).
func ValueAddingShare(processTime, leadTime float64) (float64, error) {
	return divide(processTime, leadTime)
}

// NonValueAddingShare returns the total idle time divided by the lead time
// (S_NVA, clause 4.5.2).
func NonValueAddingShare(idleTime, leadTime float64) (float64, error) {
	return divide(idleTime, leadTime)
}

// Comparison holds the relative comparison figures of clause 4.5.2: each is
// the target (future) value divided by the actual (current) value, so 0.6
// means the target needs 60 % of today's time.
type Comparison struct {
	ProcessTime float64 `json:"omega_pt"` // ω_PT
	IdleTime    float64 `json:"omega_it"` // ω_IT
	LeadTime    float64 `json:"omega_lt"` // ω_LT
}

// Compare returns the comparison figures ω_PT, ω_IT and ω_LT (clause 4.5.2)
// from the total process and idle times of the actual and target states.
func Compare(actualPT, actualIT, targetPT, targetIT float64) (Comparison, error) {
	pt, err := divide(targetPT, actualPT)
	if err != nil {
		return Comparison{}, fmt.Errorf("ω_PT: %w", err)
	}
	it, err := divide(targetIT, actualIT)
	if err != nil {
		return Comparison{}, fmt.Errorf("ω_IT: %w", err)
	}
	lt, err := divide(targetPT+targetIT, actualPT+actualIT)
	if err != nil {
		return Comparison{}, fmt.Errorf("ω_LT: %w", err)
	}
	return Comparison{ProcessTime: pt, IdleTime: it, LeadTime: lt}, nil
}

func divide(numerator, denominator float64) (float64, error) {
	if denominator == 0 {
		return 0, ErrUndefined
	}
	return numerator / denominator, nil
}

func sum(values []float64) float64 {
	total := 0.0
	for _, v := range values {
		total += v
	}
	return total
}
