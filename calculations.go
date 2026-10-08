package vsm

import (
	"errors"
)

// Calculator provides calculation methods for VSM parameters according to ISO 22468:2020
type Calculator struct{}

// NewCalculator creates a new VSM calculator
func NewCalculator() *Calculator {
	return &Calculator{}
}

// CalculateDowntime calculates downtime according to formula (A.1)
// DT = Σ(TBF_i + TTR_i) for i = 1 to #F
func (c *Calculator) CalculateDowntime(tbf, ttr []float64) (float64, error) {
	if len(tbf) != len(ttr) {
		return 0, errors.New("TBF and TTR arrays must have the same length")
	}

	var total float64
	for i := 0; i < len(tbf); i++ {
		total += tbf[i] + ttr[i]
	}
	return total, nil
}

// CalculateProcessTime calculates total process time according to formula (A.2)
// PT = PRT + ST + HT + WTp + AT
func (c *Calculator) CalculateProcessTime(params ProcessParameters) float64 {
	return params.ProcessingTime + params.SetupTime + params.HandlingTime +
		params.WaitingTimeProcess + params.AncillaryTime
}

// CalculateLeadTime calculates total lead time according to formula (A.3)
// LT = VAT + NVAT + NNVAT
func (c *Calculator) CalculateLeadTime(params ProcessParameters) float64 {
	return params.ValueAddingTime + params.NonValueAddingTime + params.NecessaryNonValueAddingTime
}

// CalculateTotalProcessTime calculates total process time for multiple processes according to formula (A.4)
// PT_total = Σ(PT_i) for i = 1 to n
func (c *Calculator) CalculateTotalProcessTime(processes []Process) float64 {
	var total float64
	for _, process := range processes {
		total += c.CalculateProcessTime(process.Parameters)
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

// CalculateHandlingTime calculates handling time according to formula (A.8)
// HT = PT - (ST + HT_previous + WTp + AT + DT)
func (c *Calculator) CalculateHandlingTime(params ProcessParameters) float64 {
	return params.ProcessTime - (params.SetupTime + params.HandlingTime +
		params.WaitingTimeProcess + params.AncillaryTime + params.Downtime)
}

// CalculateValueAddingTime calculates value adding time according to formula (A.9)
// VAT = Σ(VATp_i) for i = 1 to n
func (c *Calculator) CalculateValueAddingTime(processes []Process) float64 {
	var total float64
	for _, process := range processes {
		total += process.Parameters.ValueAddingTimeProcess
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
// CT = OT ÷ CD
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

// CalculateOnTimeInFull calculates OTIF according to formula (A.14)
// OTIF = (complete deliveries × in time deliveries) ÷ total deliveries
func (c *Calculator) CalculateOnTimeInFull(completeDeliveries, inTimeDeliveries, totalDeliveries float64) float64 {
	if totalDeliveries == 0 {
		return 0
	}
	return (completeDeliveries * inTimeDeliveries) / totalDeliveries
}

// CalculateReworkRate calculates rework rate according to formula (A.15)
// RR = (Σ(RR_i) for i = 1 to n) where RR_i = defective parts / total parts
func (c *Calculator) CalculateReworkRate(reworkRates []float64) float64 {
	if len(reworkRates) == 0 {
		return 0
	}

	var total float64
	for _, rate := range reworkRates {
		total += rate
	}
	return total / float64(len(reworkRates))
}

// CalculateValueAddedRatio calculates value added ratio according to formula (A.16)
// VAR = VAT ÷ LT
func (c *Calculator) CalculateValueAddedRatio(valueAddingTime, leadTime float64) float64 {
	if leadTime == 0 {
		return 0
	}
	return valueAddingTime / leadTime
}

// CalculateYieldFactor calculates yield factor according to formula (A.17)
// YF = 1 - (SR + RR)
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

// CalculateOverallLeadTime calculates overall value stream lead time
// LT = Σ(PLT_i) for i = 1 to n
func (c *Calculator) CalculateOverallLeadTime(processes []Process) float64 {
	var total float64
	for _, process := range processes {
		plt := c.CalculateProcessLeadTime(
			c.CalculateProcessTime(process.Parameters),
			process.Parameters.IdleTime,
		)
		total += plt
	}
	return total
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

// CalculateKPIs calculates all key performance indicators for a value stream
func (c *Calculator) CalculateKPIs(vs *ValueStream) map[string]float64 {
	kpis := make(map[string]float64)

	if vs == nil {
		return kpis
	}

	// Calculate customer takt
	if vs.Customer != nil {
		kpis["customer_takt"] = c.CalculateCustomerTakt(vs.Customer.Demand, 1) // Assuming 1 period
	}

	// Calculate total lead time
	totalLeadTime := c.CalculateOverallLeadTime(vs.Processes)
	kpis["total_lead_time"] = totalLeadTime

	// Calculate total value adding time
	totalVAT := c.CalculateValueAddingTime(vs.Processes)
	kpis["total_value_adding_time"] = totalVAT

	// Calculate value added ratio
	kpis["value_added_ratio"] = c.CalculateValueAddedRatio(totalVAT, totalLeadTime)

	// Calculate process efficiency
	totalProcessTime := c.CalculateTotalProcessTime(vs.Processes)
	if totalLeadTime > 0 {
		kpis["process_efficiency"] = totalProcessTime / totalLeadTime
	}

	// Calculate inventory turns for all inventory
	var totalStock float64
	for _, inv := range vs.Inventory {
		totalStock += inv.Quantity
	}
	if totalStock > 0 && vs.Customer != nil {
		kpis["inventory_turns"] = c.CalculateInventoryTurns(vs.Customer.Demand, totalStock)
	}

	// Calculate average yield factor across all processes
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
