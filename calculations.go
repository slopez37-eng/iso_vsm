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
// field is not read. ResolveParameters decides between this sum and a stored
// ProcessTime (see there).
func (c *Calculator) CalculateProcessTime(params ProcessParameters) float64 {
	return params.ProcessingTime + params.SetupTime + params.HandlingTime +
		params.WaitingTimeProcess + params.AncillaryTime
}

// CalculateLeadTime calculates one process's lead time from its value
// classification (A.3).
// LT = VAT + NVAT + NNVAT
//
// This is a second breakdown of the same lead time, by value category: it
// must equal process time + idle time (CheckConsistency reports it when it
// does not). It is used as the lead time only when process and idle time are
// not given (see ResolveParameters).
func (c *Calculator) CalculateLeadTime(params ProcessParameters) float64 {
	return params.ValueAddingTime + params.NonValueAddingTime + params.NecessaryNonValueAddingTime
}

// CalculateTotalProcessTime calculates total process time over all processes (A.4).
// PT_total = Σ(PT_i) for i = 1 to n
//
// PT_i is the resolved process time (see ResolveParameters). Each process is
// converted to ReferenceTimeUnit(processes).
func (c *Calculator) CalculateTotalProcessTime(processes []Process) float64 {
	pt, _, _ := c.streamTotals(&ValueStream{Processes: processes})
	return pt
}

// streamTotals returns vs's total process time, lead time and value adding
// time in its reference unit, each process resolved in the stream (see
// ResolveParametersInStream).
func (c *Calculator) streamTotals(vs *ValueStream) (processTime, leadTime, valueAdding float64) {
	ctx := c.contextOf(vs)
	for _, process := range vs.Processes {
		p := c.resolvedIn(process.Parameters, ctx)
		processTime += p.ProcessTime
		leadTime += p.LeadTime
		valueAdding += p.ValueAddingTime
	}
	return processTime, leadTime, valueAdding
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

// CalculateStockQuantity returns processTime × processQuantity (formerly
// presented as A.7, SQ = PT × PQ).
//
// Deprecated: the result is not a quantity. A time multiplied by a quantity
// is time × units (2 min × 3 units = 6 "units" in minutes, but 360 in
// seconds), so the answer changes with the time unit. The library does not
// use it. To size stock, use CalculateInventoryQuantity (customer demand
// rate × the time the stock must cover, with explicit units).
func (c *Calculator) CalculateStockQuantity(processTime float64, processQuantity float64) float64 {
	return processTime * processQuantity
}

// CalculateHandlingTime returns the stored process time minus the other A.2
// components, as a diagnostic.
// HT = PT − (PRT + ST + WTp + AT)
//
// This is A.2 rearranged, not a separate ISO 22468 formula (not verified
// against the ISO text). The model does not use it: process time is the sum
// of its parts whenever parts are given (see ResolveParameters). Downtime is
// not an A.2 component, so it is not subtracted. A negative result means the
// components add up to more than the stored ProcessTime.
func (c *Calculator) CalculateHandlingTime(params ProcessParameters) float64 {
	return params.ProcessTime - (params.ProcessingTime + params.SetupTime +
		params.WaitingTimeProcess + params.AncillaryTime)
}

// CalculateValueAddingTime calculates total value adding time over all processes (A.9).
// VAT = Σ(VATp_i) for i = 1 to n
//
// VATp_i is ValueAddingTimeProcess when set (> 0), otherwise the process's
// ValueAddingTime (see ResolveParameters). Each process is converted to
// ReferenceTimeUnit(processes).
func (c *Calculator) CalculateValueAddingTime(processes []Process) float64 {
	_, _, vat := c.streamTotals(&ValueStream{Processes: processes})
	return vat
}

// CalculateTransportRepetitionTime calculates transport repetition time according to formula (A.10)
// TT = RT × #TRep
func (c *Calculator) CalculateTransportRepetitionTime(transportRepetitionTime float64, transportRepetitionCount int) float64 {
	return transportRepetitionTime * float64(transportRepetitionCount)
}

// CalculateWaitingTime returns stock quantity × cycle time.
// SQ × CT
//
// This is the clearing time: how long the process, at its own cycle time,
// needs to work through the stock. It is a diagnostic only. The time a part
// waits in an inventory is set by how fast the customer uses the stock up,
// not by the cycle time, so the model's waiting_time and range_of_inventory
// are stock ÷ customer demand rate (see CalculateInventoryTime). The two
// agree only when the cycle time equals the takt time.
func (c *Calculator) CalculateWaitingTime(stockQuantity float64, cycleTime float64) float64 {
	return stockQuantity * cycleTime
}

// demandPeriodSeconds is the length of a demand period in seconds. Months
// and years have no fixed length, so they are not listed.
var demandPeriodSeconds = map[DemandPeriod]float64{
	DemandPeriodDay:  86400,
	DemandPeriodWeek: 604800,
}

// demandRate returns demand (units per period) as units per one unit of
// time u, and false when it cannot be known (see CalculateInventoryTime).
func demandRate(demand float64, period DemandPeriod, u TimeUnit) (float64, bool) {
	seconds, ok := demandPeriodSeconds[period]
	if !ok || demand <= 0 || !isKnownTimeUnit(u) {
		return 0, false
	}
	return demand / NewCalculator().ConvertTimeUnit(seconds, TimeUnitSeconds, u), true
}

// CalculateInventoryTime returns how long stockQuantity units last at the
// customer's demand rate, in unit: the standard VSM inventory time (Rother &
// Shook: inventory ÷ daily customer demand).
// IT_inv = SQ ÷ (CD ÷ period length)
//
// demand is units per period. The period is converted to unit as calendar
// time (a day is 24 h, a week 7 days). ok is false, and the time is not
// known, when demand is 0, unit is unknown, or period is not "day" or
// "week" (months and years have no fixed length; state demand per day or
// week instead). The model uses this for waiting_time and
// range_of_inventory of a process with stock_quantity, and for an
// inventory's range.
func (c *Calculator) CalculateInventoryTime(stockQuantity, demand float64, period DemandPeriod, unit TimeUnit) (float64, bool) {
	rate, ok := demandRate(demand, period, unit)
	if !ok {
		return 0, false
	}
	return stockQuantity / rate, true
}

// CalculateInventoryQuantity returns the stock that covers coverageTime (in
// unit) at the customer's demand rate: the inverse of
// CalculateInventoryTime.
// SQ = (CD ÷ period length) × coverage time
//
// ok is false under the same conditions as CalculateInventoryTime.
func (c *Calculator) CalculateInventoryQuantity(coverageTime float64, unit TimeUnit, demand float64, period DemandPeriod) (float64, bool) {
	rate, ok := demandRate(demand, period, unit)
	if !ok {
		return 0, false
	}
	return rate * coverageTime, true
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

// CalculateRangeOfInventory returns stock quantity × cycle time, converted
// from fromUnit to toUnit.
//
// Deprecated: despite the name this is the clearing time (see
// CalculateWaitingTime), not the range of inventory. The range of inventory
// is how long the stock lasts at the customer's demand rate; use
// CalculateInventoryTime.
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
// LT = Σ LT_i for i = 1 to n, with LT_i = PT_i + IT_i
//
// LT_i is the resolved lead time (see ResolveParameters): process time + idle
// time, each computed from its parts when they are given. Only when a process
// has neither does LT_i fall back to its value classification
// VAT + NVAT + NNVAT (A.3), and then to a stored lead_time. Each process is
// converted to ReferenceTimeUnit(processes).
//
// Without the customer, waiting time cannot be derived from stock_quantity
// (see ResolveParameters), so stored waiting times are used. CalculateKPIs
// resolves in the stream and does derive it.
func (c *Calculator) CalculateOverallLeadTime(processes []Process) float64 {
	_, lt, _ := c.streamTotals(&ValueStream{Processes: processes})
	return lt
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

	// Logical validations (on the resolved process time, see ResolveParameters)
	if resolved := c.ResolveParameters(params); params.ValueAddingTime > resolved.ProcessTime {
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

	if defectsImpossible(params) {
		errs = append(errs, errors.New("scrap rate + rework rate cannot exceed 1"))
	}

	return errs
}

// CalculateKPIs calculates the key performance indicators for a value stream.
//
// Time KPIs are in ReferenceTimeUnit(vs.Processes). A KPI is left out of the
// map, rather than reported as 0, when it cannot be computed honestly:
//   - customer_takt: Customer.AvailableTime ÷ Customer.Demand when both are
//     set (both are per Customer.DemandPeriod, so the period cancels and
//     DemandPeriod is not required here); otherwise Customer.TaktTime when
//     set; otherwise omitted.
//   - total_lead_time, total_value_adding_time: omitted when there are no
//     processes. Lead time = Σ(process time + idle time); see
//     CalculateOverallLeadTime.
//   - value_added_ratio (flow efficiency) = value adding time ÷ lead time,
//     and process_efficiency = process time ÷ lead time (share of lead time
//     spent in processes, including non-value-adding process time):
//     omitted when lead time is 0.
//   - inventory_turns = Customer.Demand ÷ total inventory quantity (turns
//     per Customer.DemandPeriod): omitted when there is no customer, demand
//     is 0, or inventory is 0.
//   - average_yield_factor: mean of the processes' yield factors (1 − scrap −
//     rework when those are given, else the stored YieldFactor) over the
//     processes whose yield is known: yield_factor, scrap_rate or
//     rework_rate given (> 0). A process that scraps everything counts with
//     yield 0. A process whose scrap_rate + rework_rate is above 1
//     (impossible; CheckConsistency reports it) is left out rather than
//     counted with a negative yield. Omitted when no process has quality
//     data.
//
// All process values are resolved in the stream first (see
// ResolveParametersInStream), so stored
// totals that disagree with their parts do not change the KPIs; see
// CheckConsistency for those disagreements.
func (c *Calculator) CalculateKPIs(vs *ValueStream) map[string]float64 {
	kpis := make(map[string]float64)

	if vs == nil {
		return kpis
	}

	// Customer takt = available operating time ÷ customer demand.
	if takt, ok := c.customerTakt(vs.Customer); ok {
		kpis["customer_takt"] = takt
	}

	if len(vs.Processes) > 0 {
		totalProcessTime, totalLeadTime, totalVAT := c.streamTotals(vs)

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

	// Average yield factor across processes whose yield is known. A yield
	// of 0 (everything scrapped) is known when scrap or rework is given.
	var totalYield float64
	processCount := 0
	for _, process := range vs.Processes {
		if !yieldKnown(process.Parameters) {
			continue
		}
		totalYield += c.ResolveParametersInStream(process.Parameters, vs).YieldFactor
		processCount++
	}
	if processCount > 0 {
		kpis["average_yield_factor"] = totalYield / float64(processCount)
	}

	return kpis
}

// yieldKnown reports whether a process has usable quality data: a yield
// factor, or a scrap or rework rate it is computed from. Scrap + rework
// above 1 is impossible, so such a yield is not known (see
// defectsImpossible).
func yieldKnown(p ProcessParameters) bool {
	if defectsImpossible(p) {
		return false
	}
	return p.YieldFactor > 0 || p.ScrapRate > 0 || p.ReworkRate > 0
}

// defectsImpossible reports whether scrap_rate + rework_rate is above 1
// (beyond ConsistencyTolerance): more than every part defective.
func defectsImpossible(p ProcessParameters) bool {
	sum := p.ScrapRate + p.ReworkRate
	return sum > 1 && !consistent(sum, 1)
}

// CalculateImprovementMetrics compares future with current KPIs: for each
// KPI both states have, <kpi>_improvement_ratio = future ÷ current and
// <kpi>_improvement_percent = (future − current) ÷ current × 100.
//
// The two states are compared in one unit: time KPIs of both are converted
// to the current state's reference time unit (the future's when the current
// declares none), and inventory_turns (turns per Customer.DemandPeriod) of
// the future are converted to the current state's demand period. When the
// periods differ and cannot be converted (a month or year period), or only
// one state has a period, inventory_turns is not compared.
func (c *Calculator) CalculateImprovementMetrics(current, future *ValueStream) map[string]float64 {
	metrics := make(map[string]float64)

	if current == nil || future == nil {
		return metrics
	}

	unit := c.ReferenceTimeUnit(current.Processes)
	if unit == "" {
		unit = c.ReferenceTimeUnit(future.Processes)
	}
	currentKPIs := c.kpisInUnit(current, unit)
	futureKPIs := c.kpisInUnit(future, unit)
	if turns, ok := futureKPIs["inventory_turns"]; ok {
		if factor, ok := periodFactor(demandPeriodOf(future), demandPeriodOf(current)); ok {
			futureKPIs["inventory_turns"] = turns * factor
		} else {
			delete(futureKPIs, "inventory_turns")
		}
	}

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

// kpisInUnit returns CalculateKPIs(vs) with its time KPIs converted from
// vs's reference unit to unit (unchanged when either unit is unknown).
func (c *Calculator) kpisInUnit(vs *ValueStream, unit TimeUnit) map[string]float64 {
	kpis := c.CalculateKPIs(vs)
	from := c.ReferenceTimeUnit(vs.Processes)
	if !isKnownTimeUnit(from) || !isKnownTimeUnit(unit) {
		return kpis
	}
	for name, v := range kpis {
		if kpiUnit(name) == "time" {
			kpis[name] = c.ConvertTimeUnit(v, from, unit)
		}
	}
	return kpis
}

// demandPeriodOf returns vs's customer demand period ("" when none).
func demandPeriodOf(vs *ValueStream) DemandPeriod {
	if vs.Customer == nil {
		return ""
	}
	return vs.Customer.DemandPeriod
}

// periodFactor returns how many "from" periods fit in one "to" period, so a
// rate per "from" period times the factor is the rate per "to" period. ok
// is false when the periods differ and are not both of fixed length.
func periodFactor(from, to DemandPeriod) (float64, bool) {
	if from == to {
		return 1, true
	}
	f, okFrom := demandPeriodSeconds[from]
	t, okTo := demandPeriodSeconds[to]
	if !okFrom || !okTo {
		return 0, false
	}
	return t / f, true
}
