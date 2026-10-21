# iso_vsm: value stream management by ISO 22468:2020, in Go

`iso_vsm` is a small Go library and command-line tool that calculates and checks value stream maps by ISO 22468:2020 *Value stream management (VSM)*.

You describe a value stream as JSON or Go structs: its customer, processes, idle segments and inventories. The library then:

- derives every value the standard's formulas allow (A.1–A.17), and records which formula produced each one;
- reports any contradictions between your numbers and those formulas, without changing what you entered;
- computes lead time, customer takt, value added ratio, OTIF, yield, inventory turns and the bottleneck;
- compares a current and a future state with the relative indicators of clause 4.5.2;
- keeps the catalogue of measures, checks each measure for SMART gaps, and ranks the measures by cost-benefit;
- follows the changed value stream over time with dated snapshots, and checks each measure's target against the latest one.

It does not draw maps. It does not invent scores or recommendations the standard does not define. Judgements such as whether the product family has the characteristics of clause 4.2.2, or whether a design guideline is met, are recorded as your input, not calculated.

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
vsm version                   # the version and commit it was built from; +dirty if the tree had local changes
```

A file is read as a project if it has a key that only a project has (`phase`, `current`, `ideal`, `future`, `monitoring`, `measures` or `workshop`), or if all its top-level keys are project fields; any other file is read as one value stream. `vsm check` exits with status 1 when the file has errors (invalid input that was ignored), 2 when it cannot read the file, and 0 otherwise.

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

For a whole VSM run, with current, ideal and future states, monitoring snapshots, the catalogue of measures and the workshop, use `vsm.Project` and `vsm.EvaluateProject`. `LoadProject`, `LoadValueStream`, `Save` and `Write` read and write the JSON format, and `IsProjectJSON` tells a project file from a single value stream. `HasErrors` on a result or project report says whether any input was invalid. Unknown JSON fields are rejected, so a misspelt field is never silently dropped.

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

`Evaluate` repeatedly applies the formulas and records the formula behind each derived value as its `Source`. For the sums (A.1, A.2, A.3, A.6, A.8, A.9 and A.17), whenever all terms but one are known it derives the missing one. For the products and quotients (A.7, A.10, A.11 and A.12), it derives only the result, never a missing factor. Once nothing more can be derived, it checks every formula whose terms are all known. A mismatch becomes a **warning** that names the formula; your entered value is kept. Invalid input becomes an **error**, and that value is ignored. Examples are negative times, rates above 1, duplicate ids and references to unknown processes. A **note** explains a gap, for example which segments still lack a time so that the lead time is unknown.

All times in one value stream use its `time_unit` (`s`, `min`, `h` or `d`, where a day is 24 hours). Rates are fractions: 25 % is `0.25`.

The test suite reproduces the standard's worked example (A.3.4) step by step with the formula functions, evaluates the same process as a value stream, and checks the customer takt of application example B.2.

## Interpretation choices

The standard leaves some points open, and a few of its formulas read oddly. This library resolves them as follows.

**Cycle time** is the observed interval at which the whole process completes products, losses included. This is the classic lean definition, the one you plot against takt. It is not a single machine's own cycle, and it is not divided by the number of resources: the observed interval already reflects any parallel machines.

**A.5** is available exactly as printed, as `ProcessTimeFromCycleTime`: process time = cycle time × (process quantity × number of resources). The evaluator never applies it. With the cycle time above, Little's law gives process time = cycle time × work in process, with no resources term. A.5 matches that only if process quantity means the work in process *per resource*. Table A.2 instead defines it as the total number of orders being processed, and read that way A.5 overstates process time by the number of resources. The law also relates steady-state averages, and process quantity counts orders while cycle time is per product, so A.5 additionally assumes one product per order. When a process lacks a process time but has the A.5 inputs, a note shows what A.5 would give, so you can judge it and enter `process_time` yourself.

**Process time** is made of setup, processing, waiting (within the process), handling and down time, following A.8. Ancillary time runs in parallel to process time and is not part of it (Figure A.1, Table A.2).

**Range of inventory (A.7)** is stock quantity divided by demand, which gives a number of review periods. To turn that into time, the evaluator multiplies by `customer.period_length`, the calendar length of the review period. Without it, ranges entered as quantities stay unknown, and a note says so.

**Waiting time (A.11)** uses an idle segment's `waiting_orders`, which is kept separate from its inventories. This way the same stock is not counted as both storage and waiting time.

**Process types** follow the cases of Figure A.1. Downtime is part of process time only for an automated process, and handling time is not part of it for a data-related process. A.8 leaves these parts out for those types even when you enter them, so a gap in A.8 shows up as a warning instead of being booked as downtime. An automated process, or one with no type set, uses all five parts.

**Batch processes.** Process time is the dwell time of a piece in the process (Table A.2). A piece in a batch stays until the whole batch is done, so the time for the whole batch counts toward lead time, as in the worked example of A.3.4. A note says so for each process with a batch size or more than one repetition.

**Comparison figures (4.5.2)** divide each future time by the current one. A figure whose current time is 0, such as ω_IT when the current state has no idle time, is left unknown, and the other figures and shares are still given.

**Idle segments** add no value from the customer's view (clause 4.5.2). They therefore carry only non-value-adding and necessary non-value-adding time. For a process, process lead time equals process time (Table A.2), so VAT + NVAT + NNVAT of a process must equal its process time.

**Takt load** is cycle time ÷ customer takt: how much of a process's capacity customer demand takes up, as on a leveling chart. Above 100 % the process cannot keep up. It is not equipment utilization, which would need each resource's busy time. **Bottleneck** (definition 3.2) is the process with the longest cycle time. That identifies the most heavily loaded process because every process in one value stream shares the stream's operating time and product family.

**Rework rate** is stored as entered (Table A.2 defines it as a share). The A.15 form, the change of yield between iterations, is available as `ReworkRate`. The stream's scrap rate and yield factor come from the `quality` counts. The process yield factor and scrap rate are linked by A.17.

**Prioritisation** of measures (clause 4.5.2) sorts by benefit ÷ cost, then by net benefit. Measures without both figures come last, and implemented or cancelled measures are left out.

**Measure status** uses the five stages of the catalogue key in Annex B.2.4 (`problem_detected`, `responsibilities_clarified`, `measure_checked`, `implementing`, `implemented`), plus `cancelled`. Clause 4.4.2 asks for the status to be noted but names no stages, so the library takes them from the standard's own example.

**Product family (4.2.2).** The family shall have similar process steps, be representative and have a balanced volume. Record each as a `criteria` entry with `met`. A characteristic marked not met gives a warning, and once any is recorded, a note lists the ones not yet assessed.

**Monitoring (4.5.1, 4.5.2).** The standard asks for the changed value stream to be monitored over time. Each `monitoring` snapshot is a date and the value stream as measured then. It is evaluated like any state and compared with the current state by the figures of 4.5.2. A measure whose `kpi` names a stream total or KPI (for example `lead_time` or `scrap_rate`) is checked against its `target` in the snapshot with the latest date that has that figure. A time target is in the current state's time unit, and the measured time is converted to it. Times, the non-value adding share and the scrap rate meet the target at or below it; the other KPIs meet it at or above it.

**Project phase (4.1)** records where the project stands: `analysis`, `design`, `planning`, `implementation`, `assessment` (check) or `adjustment` (act).

**Typos in the worked example (A.3.4).** It labels customer demand as a time and calls the number of transport repetitions a time. The library reads these as the quantities the parameter table defines.

**Two bases for idle time.** A.7 measures a range against demand per period, which the library converts to time with the calendar `period_length`. A.11 measures waiting against customer takt, which is based on operating time. The same number of pieces therefore gives a different time depending on whether it is entered as stock or as waiting orders. This follows the standard's two formulas as written.

## Scope

**Covered (normative):** the parameters of Table A.2, formulas A.1–A.17, the symbols of Table A.1 (`vsm.Symbols`), the shares and comparison figures of clause 4.5.2, the seven wastes and improvement flashes (4.3.2), the eight design guidelines as a checklist you fill in (4.3.4), the product family characteristics (4.2.2), the SMART catalogue of measures with status (4.4.2), the workshop record (4.4.3), monitoring of the changed value stream (4.5.1, 4.5.2) and the project phase (4.1).

**Not covered:** drawing value stream maps; the informative data boxes and examples of Annex B, beyond the measure status stages and their use in tests; multi-criteria assessment by space, resources or cost (4.5.2 mentions it without defining it); and any automatic future-state design.

## License

MIT. See [LICENSE](LICENSE).
