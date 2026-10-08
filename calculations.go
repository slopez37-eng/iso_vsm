package vsm

import (
	"errors"
)

// Calculator provides calculation methods for VSM parameters according to ISO 22468:2020.
//
// Zero denominators: the single-formula methods below keep a float64-only
// signature and return 0 when their denominator is zero. CalculateKPIs does
// not do that: it omits a KPI whose denominator is zero instead of reporting
// a misleading 0.
//
// Time units: methods that take plain numbers assume all their inputs share
// one unit. Methods that aggregate over []Process (CalculateTotalProcessTime,
// CalculateOverallLeadTime, CalculateValueAddingTime, CalculateKPIs) first
// convert every process's times into the reference unit returned by
// ReferenceTimeUnit.
type Calculator struct{}

// NewCalculator creates a new VSM calculator
func NewCalculator() *Calculator {
	return &Calculator{}
}

// CalculateDowntime calculates total downtime from a failure log (A.1).
// DT = Σ TTR_i for i = 1 to #F
//
// Downtime is the time spent down, i.e. the sum of the repair times. The
// times between failures are up-time and are not part of downtime; tbf is
// accepted (and must have the same length as ttr) so the log stays paired,
// and so the same inputs can be passed to CalculateAvailability.
// Not verified against ISO 22468 text; standard lean definition used.
func (c *Calculator) CalculateDowntime(tbf, ttr []float64) (float64, error) {
	if len(tbf) != len(ttr) {
		return 0, errors.New("TBF and TTR arrays must have the same length")
	}

	var total float64
	for _, repair := range ttr {
		total += repair
	}
	return total, nil
}

// CalculateAvailability calculates equipment availability from a failure log.
// A = ΣTBF ÷ (ΣTBF + ΣTTR), equivalent to MTBF ÷ (MTBF + MTTR).
// Returns 0 when the log is empty or all times are zero.
func (c *Calculator) CalculateAvailability(tbf, ttr []float64) (float64, error) {
	if len(tbf) != len(ttr) {
		return 0, errors.New("TBF and TTR arrays must have the same length")
	}

	var up, down float64
	for i := range tbf {
		up += tbf[i]
		down += ttr[i]
	}
	if up+down == 0 {
		return 0, nil
	}
	return up / (up + down), nil
}

// CalculateProcessTime calculates process time from its components (A.2).
// PT = PRT + ST + HT + WTp + AT
//
// This is the component sum only; the stored ProcessParameters.ProcessTime
// field is not read. Aggregating methods use effectiveProcessTime, which
// prefers the stored ProcessTime and falls back to this sum.
func (c *Calculator) CalculateProcessTime(params ProcessParameters) float64 {
	return params.ProcessingTime + params.SetupTime + params.HandlingTime +
		params.WaitingTimeProcess + params.AncillaryTime
}

// CalculateLeadTime calculates one process's lead time from its value
// classification (A.3).
// LT = VAT + NVAT + NNVAT
//
// This is an alternative breakdown of the same time, by value category.
// CalculateKPIs does not add it to CalculateOverallLeadTime (that would
// count the same time twice).
func (c *Calculator) CalculateLeadTime(params ProcessParameters) float64 {
	return params.ValueAddingTime + params.NonValueAddingTime + params.NecessaryNonValueAddingTime
}

// CalculateTotalProcessTime calculates total process time over all processes (A.4).
// PT_total = Σ(PT_i) for i = 1 to n
//
// PT_i is the stored ProcessTime when it is set (> 0), otherwise the A.2
// component sum. Each process is converted to ReferenceTimeUnit(processes).
func (c *Calculator) CalculateTotalProcessTime(processes []Process) float64 {
	unit := c.ReferenceTimeUnit(processes)
	var total float64
	for _, process := range processes {
		total += c.effectiveProcessTime(c.normalizeTimes(process.Parameters, unit))
	}
	return total
}

// CalculateProcessingTime calculates processing time according to formula (A.5)
// PRT = CT × (PQ ÷ #RES)
func (c *Calculator) CalculateProcessingTime(cycleTime float64, processQuantity int, resourceCount int) float64 {
	if resourceCount == 0 {
		return 0
	}
	return cycleTime * (float64(processQuantity) / float64(resourceCount))
}

// CalculateRepetitiveProcessTime calculates process time for repetitive processes according to formula (A.6)
// PT = ST + (RT × #Rep)
func (c *Calculator) CalculateRepetitiveProcessTime(setupTime, repetitionTime float64, repetitionCount int) float64 {
	return setupTime + (repetitionTime * float64(repetitionCount))
}

// CalculateStockQuantity calculates stock quantity according to formula (A.7)
// SQ = PT × PQ
func (c *Calculator) CalculateStockQuantity(processTime float64, processQuantity float64) float64 {
	return processTime * processQuantity
}

// CalculateHandlingTime derives handling time as the part of the stored
// process time not explained by the other A.2 components (A.8).
// HT = PT − (PRT + ST + WTp + AT)
//
// This is A.2 solved for HT. Downtime is not an A.2 component, so it is not
// subtracted. A negative result means the components add up to more than
// the stored ProcessTime, i.e. the inputs are inconsistent.
// Not verified against ISO 22468 text; standard lean definition used.
func (c *Calculator) CalculateHandlingTime(params ProcessParameters) float64 {
	return params.ProcessTime - (params.ProcessingTime + params.SetupTime +
		params.WaitingTimeProcess + params.AncillaryTime)
}

// CalculateValueAddingTime calculates total value adding time over all processes (A.9).
// VAT = Σ(VATp_i) for i = 1 to n
//
// VATp_i is ValueAddingTimeProcess when set (> 0), otherwise the process's
// ValueAddingTime. Each process is converted to ReferenceTimeUnit(processes).
func (c *Calculator) CalculateValueAddingTime(processes []Process) float64 {
	unit := c.ReferenceTimeUnit(processes)
	var total float64
	for _, process := range processes {
		total += c.effectiveValueAddingTime(c.normalizeTimes(process.Parameters, unit))
	}
	return total
}

// CalculateTransportRepetitionTime calculates transport repetition time according to formula (A.10)
// TT = RT × #TRep
func (c *Calculator) CalculateTransportRepetitionTime(transportRepetitionTime float64, transportRepetitionCount int) float64 {
	return transportRepetitionTime * float64(transportRepetitionCount)
}

// CalculateWaitingTime calculates waiting time according to formula (A.11)
// WT = SQ × CT
func (c *Calculator) CalculateWaitingTime(stockQuantity float64, cycleTime float64) float64 {
	return stockQuantity * cycleTime
}

// CalculateCustomerTakt calculates customer takt according to formula (A.12)
// Takt = OT ÷ CD (available operating time per period ÷ demand per period)
func (c *Calculator) CalculateCustomerTakt(operatingTime, customerDemand float64) float64 {
	if customerDemand == 0 {
		return 0
	}
	return operatingTime / customerDemand
}

// CalculateInventoryTurns calculates inventory turns according to formula (A.13)
// IT = CD ÷ SQ
func (c *Calculator) CalculateInventoryTurns(customerDemand, stockQuantity float64) float64 {
	if stockQuantity == 0 {
		return 0
	}
	return customerDemand / stockQuantity
}

// CalculateOTIF calculates on-time-in-full from a joint count.
// OTIF = deliveries that were both on time AND complete ÷ total deliveries
//
// This is the standard definition; prefer it whenever the joint count is known.
func (c *Calculator) CalculateOTIF(onTimeAndInFullDeliveries, totalDeliveries float64) float64 {
	if totalDeliveries == 0 {
		return 0
	}
	return onTimeAndInFullDeliveries / totalDeliveries
}

// CalculateOnTimeInFull estimates OTIF from separate counts (A.14).
// OTIF ≈ (complete ÷ total) × (in time ÷ total)
//
// With only separate "complete" and "in time" counts, the true OTIF (orders
// that were both) cannot be known. This product of the two rates is exact
// only if lateness and incompleteness are independent; the true value lies
// between max(0, complete + in time − total) ÷ total and
// min(complete, in time) ÷ total. Use CalculateOTIF when the joint count is
// available. (The previous version divided by total only once, which gave a
// count, not a rate.)
// Not verified against ISO 22468 text; standard lean definition used.
func (c *Calculator) CalculateOnTimeInFull(completeDeliveries, inTimeDeliveries, totalDeliveries float64) float64 {
	if totalDeliveries == 0 {
		return 0
	}
	return (completeDeliveries / totalDeliveries) * (inTimeDeliveries / totalDeliveries)
}

// CalculateReworkRate calculates the value stream rework rate (A.15).
// RR = Σ(RR_i) for i = 1 to n, where RR_i = reworked parts ÷ total parts at process i
//
// The sum is the expected number of rework occurrences per part travelling
// the whole stream (for small rates, close to the share of parts reworked at
// least once). An average of the RR_i would understate stream rework by a
// factor of n, so the sum is used, as the formula states.
// Not verified against ISO 22468 text; standard lean definition used.
func (c *Calculator) CalculateReworkRate(reworkRates []float64) float64 {
	var total float64
	for _, rate := range reworkRates {
		total += rate
	}
	return total
}

// CalculateValueAddedRatio calculates the value added ratio (A.16), also
// called flow efficiency.
// VAR = VAT ÷ LT
func (c *Calculator) CalculateValueAddedRatio(valueAddingTime, leadTime float64) float64 {
	if leadTime == 0 {
		return 0
	}
	return valueAddingTime / leadTime
}

// CalculateYieldFactor calculates the first-pass yield (A.17).
// YF = 1 − (SR + RR)
//
// This is first-pass yield (FPY): the share of parts that come out good the
// first time, so both scrapped and reworked parts count against it. Final
// yield, where reworked parts are recovered, would be 1 − SR.
func (c *Calculator) CalculateYieldFactor(scrapRate, reworkRate float64) float64 {
	return 1 - (scrapRate + reworkRate)
}

// CalculateOperatingTime calculates operating time
// OT = available time - breaks - downtime
func (c *Calculator) CalculateOperatingTime(availableTime, breaks, downtime float64) float64 {
	return availableTime - breaks - downtime
}

// CalculateRangeOfInventory calculates range of inventory in time units
// RI = SQ × CT (converted to appropriate time unit)
func (c *Calculator) CalculateRangeOfInventory(stockQuantity, cycleTime float64, fromUnit, toUnit TimeUnit) float64 {
	baseValue := stockQuantity * cycleTime
	return c.ConvertTimeUnit(baseValue, fromUnit, toUnit)
}

// CalculateTotalIdleTime calculates total idle time
// IT_total = TT + WT + ST_T
func (c *Calculator) CalculateTotalIdleTime(transportTime, waitingTime, storageTime float64) float64 {
	return transportTime + waitingTime + storageTime
}

// CalculateProcessLeadTime calculates process lead time
// PLT = PT + IT
func (c *Calculator) CalculateProcessLeadTime(processTime, idleTime float64) float64 {
	return processTime + idleTime
}

// CalculateOverallLeadTime calculates overall value stream lead time.
// LT = Σ(PT_i + IT_i) for i = 1 to n
//
// PT_i is the stored ProcessTime when set (> 0), otherwise the A.2 component
// sum. IT_i is the stored IdleTime when set (> 0), otherwise
// TransportTime + WaitingTime + StorageTime. Each process is converted to
// ReferenceTimeUnit(processes).
func (c *Calculator) CalculateOverallLeadTime(processes []Process) float64 {
	unit := c.ReferenceTimeUnit(processes)
	var total float64
	for _, process := range processes {
		params := c.normalizeTimes(process.Parameters, unit)
		total += c.CalculateProcessLeadTime(c.effectiveProcessTime(params), c.effectiveIdleTime(params))
	}
	return total
}

// ReferenceTimeUnit returns the unit that aggregated times and CalculateKPIs
// time KPIs are expressed in: the TimeUnit of the first process that declares
// one. Processes with an empty TimeUnit are assumed to already be in this
// unit. Returns "" when no process declares a unit (no conversion happens).
func (c *Calculator) ReferenceTimeUnit(processes []Process) TimeUnit {
	for _, process := range processes {
		if process.Parameters.TimeUnit != "" {
			return process.Parameters.TimeUnit
		}
	}
	return ""
}

// ConvertTimeUnit converts time values between different units
func (c *Calculator) ConvertTimeUnit(value float64, fromUnit, toUnit TimeUnit) float64 {
	// Convert to base unit (seconds)
	var seconds float64
	switch fromUnit {
	case TimeUnitSeconds:
		seconds = value
	case TimeUnitMinutes:
		seconds = value * 60
	case TimeUnitHours:
		seconds = value * 3600
	case TimeUnitDays:
		seconds = value * 86400
	case TimeUnitWeeks:
		seconds = value * 604800
	default:
		return value
	}

	// Convert from seconds to target unit
	switch toUnit {
	case TimeUnitSeconds:
		return seconds
	case TimeUnitMinutes:
		return seconds / 60
	case TimeUnitHours:
		return seconds / 3600
	case TimeUnitDays:
		return seconds / 86400
	case TimeUnitWeeks:
		return seconds / 604800
	default:
		return seconds
	}
}

// isKnownTimeUnit reports whether ConvertTimeUnit understands u.
func isKnownTimeUnit(u TimeUnit) bool {
	switch u {
	case TimeUnitSeconds, TimeUnitMinutes, TimeUnitHours, TimeUnitDays, TimeUnitWeeks:
		return true
	}
	return false
}

// normalizeTimes returns a copy of params with every time field converted to
// unit to. If either unit is empty or unknown, or they are equal, params is
// returned unchanged.
func (c *Calculator) normalizeTimes(params ProcessParameters, to TimeUnit) ProcessParameters {
	from := params.TimeUnit
	if from == to || !isKnownTimeUnit(from) || !isKnownTimeUnit(to) {
		return params
	}
	conv := func(v float64) float64 { return c.ConvertTimeUnit(v, from, to) }

	p := params
	p.ProcessTime = conv(p.ProcessTime)
	p.CycleTime = conv(p.CycleTime)
	p.SetupTime = conv(p.SetupTime)
	p.Downtime = conv(p.Downtime)
	p.IdleTime = conv(p.IdleTime)
	p.WaitingTime = conv(p.WaitingTime)
	p.WaitingTimeProcess = conv(p.WaitingTimeProcess)
	p.TransportTime = conv(p.TransportTime)
	p.TransportRepetitionTime = conv(p.TransportRepetitionTime)
	p.StorageTime = conv(p.StorageTime)
	p.HandlingTime = conv(p.HandlingTime)
	p.AncillaryTime = conv(p.AncillaryTime)
	p.TimeBetweenFailures = conv(p.TimeBetweenFailures)
	p.TimeToRepair = conv(p.TimeToRepair)
	p.RepetitionTime = conv(p.RepetitionTime)
	p.ValueAddingTime = conv(p.ValueAddingTime)
	p.NonValueAddingTime = conv(p.NonValueAddingTime)
	p.NecessaryNonValueAddingTime = conv(p.NecessaryNonValueAddingTime)
	p.ValueAddingTimeProcess = conv(p.ValueAddingTimeProcess)
	p.LeadTime = conv(p.LeadTime)
	p.OperatingTime = conv(p.OperatingTime)
	p.ProcessingTime = conv(p.ProcessingTime)
	p.RangeOfInventory = conv(p.RangeOfInventory)
	p.TimeUnit = to
	return p
}

// effectiveProcessTime is the stored ProcessTime when set (> 0), otherwise
// the A.2 component sum. Using one or the other, never both, avoids double
// counting.
func (c *Calculator) effectiveProcessTime(params ProcessParameters) float64 {
	if params.ProcessTime > 0 {
		return params.ProcessTime
	}
	return c.CalculateProcessTime(params)
}

// effectiveIdleTime is the stored IdleTime when set (> 0), otherwise
// TransportTime + WaitingTime + StorageTime.
func (c *Calculator) effectiveIdleTime(params ProcessParameters) float64 {
	if params.IdleTime > 0 {
		return params.IdleTime
	}
	return c.CalculateTotalIdleTime(params.TransportTime, params.WaitingTime, params.StorageTime)
}

// effectiveValueAddingTime is ValueAddingTimeProcess when set (> 0),
// otherwise ValueAddingTime.
func (c *Calculator) effectiveValueAddingTime(params ProcessParameters) float64 {
	if params.ValueAddingTimeProcess > 0 {
		return params.ValueAddingTimeProcess
	}
	return params.ValueAddingTime
}

// ValidateProcessParameters validates that process parameters are consistent
func (c *Calculator) ValidateProcessParameters(params ProcessParameters) []error {
	var errs []error

	// Basic validation rules
	if params.ProcessTime < 0 {
		errs = append(errs, errors.New("process time cannot be negative"))
	}
	if params.CycleTime < 0 {
		errs = append(errs, errors.New("cycle time cannot be negative"))
	}
	if params.CustomerDemand < 0 {
		errs = append(errs, errors.New("customer demand cannot be negative"))
	}

	// Logical validations
	if params.ValueAddingTime > params.ProcessTime {
		errs = append(errs, errors.New("value adding time cannot exceed process time"))
	}

	if params.YieldFactor < 0 || params.YieldFactor > 1 {
		errs = append(errs, errors.New("yield factor must be between 0 and 1"))
	}

	if params.ScrapRate < 0 || params.ScrapRate > 1 {
		errs = append(errs, errors.New("scrap rate must be between 0 and 1"))
	}

	if params.ReworkRate < 0 || params.ReworkRate > 1 {
		errs = append(errs, errors.New("rework rate must be between 0 and 1"))
	}

	return errs
}

// CalculateKPIs calculates the key performance indicators for a value stream.
//
// Time KPIs are in ReferenceTimeUnit(vs.Processes). A KPI is left out of the
// map, rather than reported as 0, when it cannot be computed honestly:
//   - customer_takt: Customer.AvailableTime ÷ Customer.Demand when both are
//     set; otherwise Customer.TaktTime when set; otherwise omitted.
//   - total_lead_time, total_value_adding_time: omitted when there are no
//     processes. Lead time = Σ(process time + idle time); see
//     CalculateOverallLeadTime.
//   - value_added_ratio (flow efficiency) = value adding time ÷ lead time,
//     and process_efficiency = process time ÷ lead time (share of lead time
//     spent in processes, including non-value-adding process time):
//     omitted when lead time is 0.
//   - inventory_turns = Customer.Demand ÷ total inventory quantity: omitted
//     when there is no customer, demand is 0, or inventory is 0.
//   - average_yield_factor: mean of the processes' YieldFactor values that
//     are set (> 0); omitted when none are set.
func (c *Calculator) CalculateKPIs(vs *ValueStream) map[string]float64 {
	kpis := make(map[string]float64)

	if vs == nil {
		return kpis
	}

	// Customer takt = available operating time ÷ customer demand.
	if vs.Customer != nil {
		switch {
		case vs.Customer.AvailableTime > 0 && vs.Customer.Demand > 0:
			kpis["customer_takt"] = c.CalculateCustomerTakt(vs.Customer.AvailableTime, vs.Customer.Demand)
		case vs.Customer.TaktTime > 0:
			kpis["customer_takt"] = vs.Customer.TaktTime
		}
	}

	if len(vs.Processes) > 0 {
		totalLeadTime := c.CalculateOverallLeadTime(vs.Processes)
		totalVAT := c.CalculateValueAddingTime(vs.Processes)
		totalProcessTime := c.CalculateTotalProcessTime(vs.Processes)

		kpis["total_lead_time"] = totalLeadTime
		kpis["total_value_adding_time"] = totalVAT

		if totalLeadTime > 0 {
			kpis["value_added_ratio"] = c.CalculateValueAddedRatio(totalVAT, totalLeadTime)
			kpis["process_efficiency"] = totalProcessTime / totalLeadTime
		}
	}

	// Inventory turns for all inventory
	var totalStock float64
	for _, inv := range vs.Inventory {
		totalStock += inv.Quantity
	}
	if totalStock > 0 && vs.Customer != nil && vs.Customer.Demand > 0 {
		kpis["inventory_turns"] = c.CalculateInventoryTurns(vs.Customer.Demand, totalStock)
	}

	// Average yield factor across processes that report one
	var totalYield float64
	processCount := 0
	for _, process := range vs.Processes {
		if process.Parameters.YieldFactor > 0 {
			totalYield += process.Parameters.YieldFactor
			processCount++
		}
	}
	if processCount > 0 {
		kpis["average_yield_factor"] = totalYield / float64(processCount)
	}

	return kpis
}

// CalculateImprovementMetrics calculates metrics for comparing current vs future state
func (c *Calculator) CalculateImprovementMetrics(current, future *ValueStream) map[string]float64 {
	metrics := make(map[string]float64)

	if current == nil || future == nil {
		return metrics
	}

	currentKPIs := c.CalculateKPIs(current)
	futureKPIs := c.CalculateKPIs(future)

	// Calculate improvement ratios
	for key, currentValue := range currentKPIs {
		if futureValue, exists := futureKPIs[key]; exists && currentValue != 0 {
			ratio := futureValue / currentValue
			metrics[key+"_improvement_ratio"] = ratio

			// Calculate percentage improvement
			improvement := (futureValue - currentValue) / currentValue * 100
			metrics[key+"_improvement_percent"] = improvement
		}
	}

	return metrics
}
