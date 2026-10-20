# iso_vsm: value stream management by ISO 22468:2020, in Go

`iso_vsm` is a small Go library and command-line tool that calculates and checks value stream maps by ISO 22468:2020 *Value stream management (VSM)*.

You describe a value stream as JSON or Go structs: its customer, processes, idle segments and inventories. The library then:

- derives every value the standard's formulas allow (A.1–A.17), and records which formula produced each one;
- reports any contradictions between your numbers and those formulas, without changing what you entered;
- computes lead time, customer takt, value added ratio, OTIF, yield, inventory turns and the bottleneck;
- compares a current and a future state with the relative indicators of clause 4.5.2;
- keeps the catalogue of measures, checks each measure for SMART gaps, and ranks the measures by cost-benefit.

It does not draw maps. It does not invent scores or recommendations the standard does not define. Judgements such as choosing the product family or deciding whether a design guideline is met are recorded as your input, not calculated.

The standard itself is copyrighted and is not part of this repository. Clause and formula numbers below refer to ISO 22468:2020(E).

## Install

Requires Go 1.24 or later.

```sh
go get github.com/slopez37-eng/iso_vsm
go install github.com/slopez37-eng/iso_vsm/cmd/vsm@latest
```

## Command line

```sh
vsm template > project.json   # a starter project to edit
vsm check project.json        # evaluate a project or a single value stream
vsm check -json project.json  # the same report as JSON
vsm symbols                   # the value stream symbols of Table A.1
```

`vsm check` exits with status 1 when the file has errors (invalid input that was ignored), 2 when it cannot read the file, and 0 otherwise.

## Library

```go
import vsm "github.com/slopez37-eng/iso_vsm"

vs := &vsm.ValueStream{
    Name: "line 3", State: vsm.StateCurrent, TimeUnit: vsm.Minute,
    Customer: vsm.Customer{Period: "day", Demand: vsm.F(400), OperatingTime: vsm.F(900), PeriodLength: vsm.F(1440)},
    Processes: []vsm.Process{
        {ID: "P1", Name: "cutting", CycleTime: vsm.F(1.5), SetupTime: vsm.F(30), RepetitionTime: vsm.F(1.5), Repetitions: vsm.F(40)},
        {ID: "P2", Name: "assembly", CycleTime: vsm.F(2), ProcessTime: vsm.F(2.5)},
    },
    Idles: []vsm.Idle{{
        ID: "I1", From: "P1", To: "P2", TransportTime: vsm.F(5), WaitingTime: vsm.F(0),
        Inventories: []vsm.Inventory{{Kind: vsm.InventoryStock, Quantity: vsm.F(600)}},
    }},
}
r := vsm.Evaluate(vs)
fmt.Println(r.Totals.LeadTime, r.KPIs.CustomerTakt, r.Bottleneck)
for _, issue := range r.Issues {
    fmt.Println(issue)
}
```

For a whole VSM run, with current, ideal and future states plus the catalogue of measures, use `vsm.Project` and `vsm.EvaluateProject`. `LoadProject`, `LoadValueStream` and `Save` read and write the JSON format. Unknown JSON fields are rejected, so a misspelt field is never silently dropped.

Each formula is also a plain function. Use these when you only need one calculation:

| Formula | Function |
|---|---|
| A.1 downtime = Σ time to repair | `Downtime` |
| A.2 idle time = transport + waiting + storage | `IdleTime` |
| A.3 lead time = VAT + NVAT + NNVAT | `LeadTimeFromValueShares` |
| A.4 lead time = Σ process time + Σ idle time | `LeadTime` |
| A.5 process time from cycle time | `ProcessTimeFromCycleTime` |
| A.6 process time = setup + repetition time × repetitions | `ProcessTimeFromRepetitions` |
| A.7 range = stock quantity ÷ customer demand | `Range` |
| A.8 setup time from process time and its parts | `SetupTime`, `HandlingTime` |
| A.9 storage time = Σ ranges | `StorageTime` |
| A.10 transport time = repetitions × repetition time | `TransportTime` |
| A.11 waiting time = waiting orders × customer takt | `WaitingTime` |
| A.12 customer takt = operating time ÷ demand | `CustomerTakt` |
| A.13 inventory turns = usage ÷ average stock | `InventoryTurns` |
| A.14 OTIF = complete on-time deliveries ÷ deliveries | `OnTimeInFull` |
| A.15 rework rate = change of yield between iterations | `ReworkRate` |
| A.16 value added ratio = VAT ÷ lead time | `ValueAddedRatio` |
| A.17 scrap rate + yield factor = 100 % | `YieldFactor`, `ScrapRate` |
| 4.5.2 value adding and non-value adding shares | `ValueAddingShare`, `NonValueAddingShare` |
| 4.5.2 comparison figures ω_PT, ω_IT, ω_LT | `Compare`, `CompareStates` |

## How evaluation works

Every number in the model is optional. `nil` means "not entered" and a pointer to 0 means an entered zero (`vsm.F(0)`). The evaluator never treats a missing number as zero.

`Evaluate` repeatedly applies the formulas. Whenever all terms but one of a formula are known, it derives the missing term and records the formula as the value's `Source`. Once nothing more can be derived, it checks every formula whose terms are all known. A mismatch becomes a **warning** that names the formula; your entered value is kept. Invalid input becomes an **error**, and that value is ignored. Examples are negative times, rates above 1, duplicate ids and references to unknown processes. A **note** explains a gap, for example which segments still lack a time so that the lead time is unknown.

All times in one value stream use its `time_unit` (`s`, `min`, `h` or `d`, where a day is 24 hours). Rates are fractions: 25 % is `0.25`.

The test suite reproduces the standard's worked example (A.3.4) and the customer takt of application example B.2.

## Interpretation choices

The standard leaves some points open, and a few of its formulas read oddly. This library resolves them as follows.

**A.5** is implemented exactly as printed: process time = cycle time × (process quantity × number of resources). Common lean practice divides by resources instead. Because of this, the evaluator uses A.5 only as a last resort, when nothing else gives the process time, and adds a note each time it does.

**Process time** is made of setup, processing, waiting (within the process), handling and down time, following A.8. Ancillary time runs in parallel to process time and is not part of it (Figure A.1, Table A.2).

**Range of inventory (A.7)** is stock quantity divided by demand, which gives a number of review periods. To turn that into time, the evaluator multiplies by `customer.period_length`, the calendar length of the review period. Without it, ranges entered as quantities stay unknown, and a note says so.

**Waiting time (A.11)** uses an idle segment's `waiting_orders`, which is kept separate from its inventories. This way the same stock is not counted as both storage and waiting time.

**Idle segments** add no value from the customer's view (clause 4.5.2). They therefore carry only non-value-adding and necessary non-value-adding time. For a process, process lead time equals process time (Table A.2), so VAT + NVAT + NNVAT of a process must equal its process time.

**Bottleneck** (definition 3.2) is the process with the highest cycle time per resource. **Utilization** is cycle time ÷ (resources × customer takt). When resources are not entered, both treat them as 1.

**Rework rate** is stored as entered (Table A.2 defines it as a share). The A.15 form, the change of yield between iterations, is available as `ReworkRate`. The stream's scrap rate and yield factor come from the `quality` counts. The process yield factor and scrap rate are linked by A.17.

**Prioritisation** of measures (clause 4.5.2) sorts by benefit ÷ cost, then by net benefit. Measures without both figures come last, and done or cancelled measures are left out.

**Typos in the worked example (A.3.4).** It labels customer demand as a time and calls the number of transport repetitions a time. The library reads these as the quantities the parameter table defines.

## Scope

**Covered (normative):** the parameters of Table A.2, formulas A.1–A.17, the symbols of Table A.1 (`vsm.Symbols`), the shares and comparison figures of clause 4.5.2, the seven wastes and improvement flashes (4.3.2), the eight design guidelines as a checklist you fill in (4.3.4), the SMART catalogue of measures with status (4.4.2), and the workshop record (4.4.3).

**Not covered:** drawing value stream maps; the informative data boxes and examples of Annex B, beyond their use in tests; multi-criteria assessment by space, resources or cost (4.5.2 mentions it without defining it); and any automatic future-state design.

## License

MIT. See [LICENSE](LICENSE).
