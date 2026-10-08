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
// This is the component sum only (components not entered add 0); the stored
// ProcessParameters.ProcessTime field is not read. ResolveParameters decides
// between this sum and a stored ProcessTime (see there).
func (c *Calculator) CalculateProcessTime(params ProcessParameters) float64 {
	return num(params.ProcessingTime) + num(params.SetupTime) + num(params.HandlingTime) +
		num(params.WaitingTimeProcess) + num(params.AncillaryTime)
}

// CalculateLeadTime calculates one process's lead time from its value
// classification (A.3).
// LT = VAT + NVAT + NNVAT
//
// This is a second breakdown of the same lead time, by value category: it
// must equal process time + idle time (CheckConsistency reports it when it
// does not). It is used as the lead time only when process and idle time are
// not given (see ResolveParameters).
//
// Components not entered add 0.
func (c *Calculator) CalculateLeadTime(params ProcessParameters) float64 {
	return num(params.ValueAddingTime) + num(params.NonValueAddingTime) + num(params.NecessaryNonValueAddingTime)
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
// ResolveParametersInStream). The lead time also includes the time of every
// inventory point counted on its own (see inventoryTimes).
func (c *Calculator) streamTotals(vs *ValueStream) (processTime, leadTime, valueAdding float64) {
	ctx := c.contextOf(vs)
	for _, process := range vs.Processes {
		p := c.resolvedIn(process.Parameters, ctx)
		processTime += num(p.ProcessTime)
		leadTime += num(p.LeadTime)
		valueAdding += num(p.ValueAddingTime)
	}
	_, inventory := c.inventoryTimes(vs)
	return processTime, leadTime + inventory, valueAdding
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
// Values not entered count as 0.
func (c *Calculator) CalculateHandlingTime(params ProcessParameters) float64 {
	return num(params.ProcessTime) - (num(params.ProcessingTime) + num(params.SetupTime) +
		num(params.WaitingTimeProcess) + num(params.AncillaryTime))
}

// CalculateValueAddingTime calculates total value adding time over all processes (A.9).
// VAT = Σ(VATp_i) for i = 1 to n
//
// VATp_i is ValueAddingTimeProcess when entered, otherwise the process's
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
// are stock ÷ customer demand rate (see CalculateCustomerInventoryTime). The two
// agree only when the cycle time equals the takt time.
func (c *Calculator) CalculateWaitingTime(stockQuantity float64, cycleTime float64) float64 {
	return stockQuantity * cycleTime
}

// demandPeriodSeconds is the calendar length of a demand period in
// seconds, for a plant working every day. Months and years have no fixed
// length, so they are not listed. See periodSeconds for working days.
var demandPeriodSeconds = map[DemandPeriod]float64{
	DemandPeriodDay:  86400,
	DemandPeriodWeek: 604800,
}

// workingDaysValid reports whether n is a usable WorkingDaysPerWeek: nil
// (not entered, meaning 7) or between 1 and 7.
func workingDaysValid(n *float64) bool {
	return n == nil || (*n >= 1 && *n <= 7)
}

// periodSeconds returns the average calendar (elapsed) time, in seconds,
// that one demand period of cust covers, and false when it has no fixed
// length or WorkingDaysPerWeek is invalid. A "day" period is a working day:
// with w working days a week, one working day covers 7 ÷ w calendar days
// on average. A week is 7 calendar days whatever w is.
func periodSeconds(cust *Customer) (float64, bool) {
	if cust == nil || !workingDaysValid(cust.WorkingDaysPerWeek) {
		return 0, false
	}
	seconds, ok := demandPeriodSeconds[cust.DemandPeriod]
	if !ok {
		return 0, false
	}
	if cust.DemandPeriod == DemandPeriodDay && cust.WorkingDaysPerWeek != nil {
		seconds *= 7 / *cust.WorkingDaysPerWeek
	}
	return seconds, true
}

// customerDemandRate returns cust's demand as units per one unit of
// calendar time u, and false when it cannot be known (see
// CalculateCustomerInventoryTime).
func customerDemandRate(cust *Customer, u TimeUnit) (float64, bool) {
	seconds, ok := periodSeconds(cust)
	if !ok || !positive(cust.Demand) || !isKnownTimeUnit(u) {
		return 0, false
	}
	return *cust.Demand / NewCalculator().ConvertTimeUnit(seconds, TimeUnitSeconds, u), true
}

// CalculateCustomerInventoryTime returns how long stockQuantity units last
// at cust's demand rate, as calendar (elapsed) time in unit: the standard
// VSM inventory time (Rother & Shook: inventory ÷ daily customer demand).
// IT_inv = SQ ÷ (CD ÷ period length)
//
// cust.Demand is units per cust.DemandPeriod. A "day" period is a working
// day: stock ÷ daily demand is working days of cover, converted to calendar
// time as working days × 7 ÷ WorkingDaysPerWeek days (7 when absent, so a
// day is then 24 h). A "week" is 7 calendar days. ok is false, and the time
// is not known, when cust is nil, demand is not entered or 0, unit is unknown,
// WorkingDaysPerWeek is outside 1–7, or the period is not "day" or "week"
// (months and years have no fixed length; state demand per day or week
// instead). The model uses this for waiting_time and range_of_inventory of
// a process with stock_quantity, and for an inventory's range.
func (c *Calculator) CalculateCustomerInventoryTime(stockQuantity float64, cust *Customer, unit TimeUnit) (float64, bool) {
	rate, ok := customerDemandRate(cust, unit)
	if !ok {
		return 0, false
	}
	return stockQuantity / rate, true
}

// CalculateCustomerInventoryQuantity returns the stock that covers
// coverageTime (calendar time, in unit) at cust's demand rate: the inverse
// of CalculateCustomerInventoryTime.
// SQ = (CD ÷ period length) × coverage time
//
// ok is false under the same conditions as CalculateCustomerInventoryTime.
func (c *Calculator) CalculateCustomerInventoryQuantity(coverageTime float64, unit TimeUnit, cust *Customer) (float64, bool) {
	rate, ok := customerDemandRate(cust, unit)
	if !ok {
		return 0, false
	}
	return rate * coverageTime, true
}

// CalculateInventoryTime is CalculateCustomerInventoryTime for a customer
// working 7 days a week (a day period is 24 h): stock ÷ (demand per period
// ÷ period length).
func (c *Calculator) CalculateInventoryTime(stockQuantity, demand float64, period DemandPeriod, unit TimeUnit) (float64, bool) {
	return c.CalculateCustomerInventoryTime(stockQuantity, &Customer{Demand: F(demand), DemandPeriod: period}, unit)
}

// CalculateInventoryQuantity is CalculateCustomerInventoryQuantity for a
// customer working 7 days a week: the inverse of CalculateInventoryTime.
func (c *Calculator) CalculateInventoryQuantity(coverageTime float64, unit TimeUnit, demand float64, period DemandPeriod) (float64, bool) {
	return c.CalculateCustomerInventoryQuantity(coverageTime, unit, &Customer{Demand: F(demand), DemandPeriod: period})
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
// CalculateCustomerInventoryTime.
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
// resolves in the stream and does derive it, and also adds the inventory
// points of ValueStream.Inventory, which this process-only sum does not
// see.
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
// unit to (fields not entered stay nil). If either unit is empty or unknown,
// or they are equal, params is returned unchanged.
func (c *Calculator) normalizeTimes(params ProcessParameters, to TimeUnit) ProcessParameters {
	from := params.TimeUnit
	if from == to || !isKnownTimeUnit(from) || !isKnownTimeUnit(to) {
		return params
	}
	p := params
	for _, field := range timeFields {
		if v := *field(&p); v != nil {
			*field(&p) = F(c.ConvertTimeUnit(*v, from, to))
		}
	}
	p.TimeUnit = to
	return p
}

// timeFields are the ProcessParameters fields that are times (in the
// process's TimeUnit).
var timeFields = []func(*ProcessParameters) **float64{
	func(p *ProcessParameters) **float64 { return &p.ProcessTime },
	func(p *ProcessParameters) **float64 { return &p.CycleTime },
	func(p *ProcessParameters) **float64 { return &p.SetupTime },
	func(p *ProcessParameters) **float64 { return &p.Downtime },
	func(p *ProcessParameters) **float64 { return &p.IdleTime },
	func(p *ProcessParameters) **float64 { return &p.WaitingTime },
	func(p *ProcessParameters) **float64 { return &p.WaitingTimeProcess },
	func(p *ProcessParameters) **float64 { return &p.TransportTime },
	func(p *ProcessParameters) **float64 { return &p.TransportRepetitionTime },
	func(p *ProcessParameters) **float64 { return &p.StorageTime },
	func(p *ProcessParameters) **float64 { return &p.HandlingTime },
	func(p *ProcessParameters) **float64 { return &p.AncillaryTime },
	func(p *ProcessParameters) **float64 { return &p.TimeBetweenFailures },
	func(p *ProcessParameters) **float64 { return &p.TimeToRepair },
	func(p *ProcessParameters) **float64 { return &p.RepetitionTime },
	func(p *ProcessParameters) **float64 { return &p.ValueAddingTime },
	func(p *ProcessParameters) **float64 { return &p.NonValueAddingTime },
	func(p *ProcessParameters) **float64 { return &p.NecessaryNonValueAddingTime },
	func(p *ProcessParameters) **float64 { return &p.ValueAddingTimeProcess },
	func(p *ProcessParameters) **float64 { return &p.LeadTime },
	func(p *ProcessParameters) **float64 { return &p.OperatingTime },
	func(p *ProcessParameters) **float64 { return &p.ProcessingTime },
	func(p *ProcessParameters) **float64 { return &p.RangeOfInventory },
}

// ValidateProcessParameters validates that process parameters are consistent.
// Values that are not entered are not checked.
func (c *Calculator) ValidateProcessParameters(params ProcessParameters) []error {
	var errs []error
	negative := func(v *float64) bool { return v != nil && *v < 0 }
	outside01 := func(v *float64) bool { return v != nil && (*v < 0 || *v > 1) }

	// Basic validation rules
	if negative(params.ProcessTime) {
		errs = append(errs, errors.New("process time cannot be negative"))
	}
	if negative(params.CycleTime) {
		errs = append(errs, errors.New("cycle time cannot be negative"))
	}
	if negative(params.CustomerDemand) {
		errs = append(errs, errors.New("customer demand cannot be negative"))
	}

	// Logical validations (on the resolved process time, see ResolveParameters)
	if vatImpossible(c.ResolveParameters(params)) {
		errs = append(errs, errors.New("value adding time cannot exceed process time or lead time"))
	}

	if outside01(params.YieldFactor) {
		errs = append(errs, errors.New("yield factor must be between 0 and 1"))
	}

	if outside01(params.ScrapRate) {
		errs = append(errs, errors.New("scrap rate must be between 0 and 1"))
	}

	if outside01(params.ReworkRate) {
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
//     entered and demand is above 0 (both are per Customer.DemandPeriod, so
//     the period cancels and DemandPeriod is not required here); otherwise
//     Customer.TaktTime when entered; otherwise omitted.
//   - total_lead_time, total_value_adding_time: omitted when there are no
//     processes. Lead time = Σ(process time + idle time) over the processes
//     (see CalculateOverallLeadTime) + the time of each inventory point in
//     vs.Inventory: quantity ÷ customer demand rate (see
//     CalculateCustomerInventoryTime), or its stored range when the rate is
//     not known. No stock is counted twice: an inventory whose process_id
//     names a process with stock_quantity is that process's stock, already
//     in its waiting time, so it adds nothing more (see Inventory.ProcessID);
//     only when that process has no waiting time to account for it (no
//     demand rate and no waiting_time stored) does the inventory's range
//     count instead.
//     Inventory time is waiting, never value adding.
//   - value_added_ratio (flow efficiency) = value adding time ÷ lead time,
//     and process_efficiency = process time ÷ lead time (share of lead time
//     spent in processes, including non-value-adding process time):
//     omitted when lead time is 0. value_added_ratio is also omitted when a
//     process's value adding time exceeds its process time or lead time
//     (impossible; CheckConsistency reports it), so it never exceeds 1.
//   - inventory_turns = Customer.Demand ÷ total stock (turns per
//     Customer.DemandPeriod). Total stock is the stock the lead time
//     counts, each once: every process's stock_quantity, plus each
//     inventory not linked to a process with stock (its quantity, or range
//     × demand rate when given by range alone). Omitted when there is no
//     customer, demand is not entered or 0, stock is 0, or an inventory's
//     stock cannot be known (range alone, no demand rate). A stock entered
//     as 0 counts as 0; one not entered adds nothing.
//   - average_yield_factor: mean of the processes' yield factors (1 − scrap −
//     rework when those are given, else the stored YieldFactor) over the
//     processes whose yield is known: yield_factor, scrap_rate or
//     rework_rate entered (0 included: a yield of 0, or a scrap rate of 0,
//     is quality data). A process that scraps everything counts with yield
//     0. A process whose scrap_rate + rework_rate is above 1
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
			// A process whose value adding time exceeds its process or lead
			// time (impossible; CheckConsistency reports it) makes the
			// stream's flow efficiency unknown.
			vatKnown := !exceeds(totalVAT, totalLeadTime)
			ctx := c.contextOf(vs)
			for _, process := range vs.Processes {
				if vatImpossible(c.resolvedIn(process.Parameters, ctx)) {
					vatKnown = false
				}
			}
			if vatKnown {
				kpis["value_added_ratio"] = c.CalculateValueAddedRatio(totalVAT, totalLeadTime)
			}
			kpis["process_efficiency"] = totalProcessTime / totalLeadTime
		}
	}

	// Inventory turns over the same stock the lead time counts.
	if totalStock, ok := c.streamStock(vs); ok && totalStock > 0 && vs.Customer != nil && positive(vs.Customer.Demand) {
		kpis["inventory_turns"] = c.CalculateInventoryTurns(*vs.Customer.Demand, totalStock)
	}

	// Average yield factor across processes whose yield is known (a yield
	// of 0 included).
	var totalYield float64
	processCount := 0
	for _, process := range vs.Processes {
		if !yieldKnown(process.Parameters) {
			continue
		}
		totalYield += *c.ResolveParametersInStream(process.Parameters, vs).YieldFactor
		processCount++
	}
	if processCount > 0 {
		kpis["average_yield_factor"] = totalYield / float64(processCount)
	}

	return kpis
}

// yieldKnown reports whether a process has usable quality data: a yield
// factor, or a scrap or rework rate it is computed from, entered (0
// included). Scrap + rework above 1 is impossible, so such a yield is not
// known (see defectsImpossible).
func yieldKnown(p ProcessParameters) bool {
	if defectsImpossible(p) {
		return false
	}
	return p.YieldFactor != nil || p.ScrapRate != nil || p.ReworkRate != nil
}

// defectsImpossible reports whether scrap_rate + rework_rate is above 1
// (beyond float rounding, see exceeds): more than every part defective.
func defectsImpossible(p ProcessParameters) bool {
	return exceeds(num(p.ScrapRate)+num(p.ReworkRate), 1)
}

// vatImpossible reports whether a resolved process's value adding time
// exceeds its process time or its lead time (when known): a part larger
// than its whole. Its flow efficiency is then unknown.
func vatImpossible(p ProcessParameters) bool {
	if p.ValueAddingTime == nil {
		return false
	}
	return (p.ProcessTime != nil && exceeds(*p.ValueAddingTime, *p.ProcessTime)) ||
		(p.LeadTime != nil && exceeds(*p.ValueAddingTime, *p.LeadTime))
}

// CalculateImprovementMetrics compares future with current KPIs: for each
// KPI both states have, <kpi>_improvement_ratio = future ÷ current and
// <kpi>_improvement_percent = (future − current) ÷ current × 100.
//
// The two states are compared in one unit: time KPIs of both are converted
// to the current state's reference time unit (the future's when the current
// declares none), and inventory_turns (turns per Customer.DemandPeriod) of
// the future are converted to the current state's demand period (in
// calendar time, counting Customer.WorkingDaysPerWeek). When the periods
// differ and cannot be converted (a month or year period), or only one
// state has a customer, inventory_turns is not compared.
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
		if factor, ok := periodFactor(future.Customer, current.Customer); ok {
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

// periodFactor returns how many of from's demand periods fit in one of
// to's, in calendar time (see periodSeconds), so a rate per "from" period
// times the factor is the rate per "to" period. ok is false when the
// periods differ and are not both of fixed length, or only one stream has
// a customer.
func periodFactor(from, to *Customer) (float64, bool) {
	if from == nil || to == nil {
		return 1, from == nil && to == nil
	}
	if from.DemandPeriod == to.DemandPeriod && workingDays(from) == workingDays(to) {
		return 1, true
	}
	f, okFrom := periodSeconds(from)
	t, okTo := periodSeconds(to)
	if !okFrom || !okTo {
		return 0, false
	}
	return t / f, true
}

// workingDays returns cust's working days per week (7 when not entered).
func workingDays(cust *Customer) float64 {
	if cust == nil || cust.WorkingDaysPerWeek == nil {
		return 7
	}
	return *cust.WorkingDaysPerWeek
}
