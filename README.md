# iso_vsm: value stream management by ISO 22468:2020, in Go

`iso_vsm` is a small Go library and command-line tool that calculates and checks value stream maps by ISO 22468:2020 *Value stream management (VSM)*.

You describe a value stream as JSON or Go structs: its customer, processes, idle segments and inventories. The library then:

- derives the values that the Annex A formulas give from your entered numbers, within the limits under How evaluation works, and records which formula produced each one;
- reports any contradictions between your numbers and those formulas, without changing what you entered;
- computes lead time, customer takt, value added ratio, OTIF, yield, inventory turns and the bottleneck;
- compares a current and a future state with the relative indicators of clause 4.5.2;
- keeps the catalogue of measures, checks each measure for SMART gaps, and ranks the measures by cost-benefit;
- follows the changed value stream over time with dated snapshots, and checks each measure's target against the latest one.

It does not draw maps. It does not invent scores or recommendations the standard does not define. Judgements such as whether the product family has the characteristics of clause 4.2.2, whether a design guideline is met, or whether a measure is accepted and realistic, are recorded as your input, not calculated.

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
vsm import [-base FILE] [-name NAME] [-unit UNIT] FILE.csv   # a value stream JSON from a CSV file
vsm symbols                   # the value stream symbols of Table A.1
vsm version                   # the version and commit it was built from; +dirty if the tree had local changes
```

A file is read as a project if it has a top-level `current` key. Otherwise a key that only a value stream has (such as `time_unit` or `processes`) makes it one value stream. Otherwise it is read as a project if it has a key that only a project has (`phase`, `ideal`, `future`, `monitoring`, `measures` or `workshop`), or if all its top-level keys are project fields; any other file is read as one value stream. `vsm check` exits with status 1 when the file has errors (invalid input that was ignored), 2 when it cannot read the file, and 0 otherwise.

`vsm import` reads a CSV file with one row per process, idle or inventory, and writes the value stream as JSON to standard output. It does not evaluate the result, so run `vsm check` on the output. The `element` column says what each row is. Other column names are JSON field names, with `inventory_` in front for inventory fields. Lists use `|` between values, and numbers use a point as the decimal separator. Column names, the `element` cell and boolean cells ignore case; other values are kept as typed, so write enum values as in the JSON format, such as `manual`. Spaces around a cell or a list value are ignored. An idle row takes its `from` and `to` from the neighbouring process rows when they are left empty. An error names the line and column and writes nothing; the exit status is 2. `-base` starts from an existing value stream, and `-name` and `-unit` set its name and time unit.

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

For a whole VSM run, with current, ideal and future states, monitoring snapshots, the catalogue of measures and the workshop, use `vsm.Project` and `vsm.EvaluateProject`. `LoadProject`, `LoadValueStream`, `Save` and `Write` read and write the JSON format, and `IsProjectJSON` tells a project file from a single value stream. `HasErrors` on a result or project report says whether any input was invalid. Unknown JSON fields are rejected, so a misspelt field is never silently dropped. `ReadCSV(r, vs)` appends processes, idles and inventories from CSV to a value stream, with the same rules as `vsm import`.

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
| A.15 yield after rework = yield factor + rework rate | `YieldAfterRework`, `ReworkRate` |
| A.16 value added ratio = VAT ÷ lead time | `ValueAddedRatio` |
| A.17 scrap rate + yield factor = 100 % | `YieldFactor`, `ScrapRate` |
| 4.5.2 value adding and non-value adding shares | `ValueAddingShare`, `NonValueAddingShare` |
| 4.5.2 comparison figures ω_PT, ω_IT, ω_LT | `Compare`, `CompareStates` |

## How evaluation works

Every number in the model is optional. `nil` means "not entered" and a pointer to 0 means an entered zero (`vsm.F(0)`). The evaluator never treats a missing number as zero.

`Evaluate` repeatedly applies the formulas and records the formula behind each derived value as its `Source`. For the sums (A.1, A.2, A.3, A.6, A.8, A.9, A.15 and A.17), whenever all terms but one are known it derives the missing one. For the products and quotients (A.7, A.10, A.11 and A.12), it derives only the result, never a missing factor. Once nothing more can be derived, it checks every formula whose terms are all known. A mismatch becomes a **warning** that names the formula; your entered value is kept. Invalid input becomes an **error**, and that value is ignored. Examples are negative times, rates above 1, duplicate ids and references to unknown processes. A **note** explains a gap, for example which segments still lack a time so that the lead time is unknown.

Derivation has limits. A.7 is not inverted, so a stock quantity is not derived from a range. A.11 applies to the waiting orders of idle segments, not to processes, and A.5 is never applied (see Interpretation choices). So the worked example A.3.4 cannot be rebuilt from its original inputs alone.

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

**Batch processes.** Process time is the dwell time of a piece in the process (Table A.2). A piece in a batch stays until the whole batch is done, so the time for the whole batch counts toward lead time, as in the worked example of A.3.4. A note says so for each process with a batch size above 1.

**Comparison figures (4.5.2)** divide each future time by the current one. A figure whose current time is 0, such as ω_IT when the current state has no idle time, is left unknown, and the other figures and shares are still given.

**Idle segments** add no value from the customer's view (clause 4.5.2). They therefore carry only non-value-adding and necessary non-value-adding time. For a process, process lead time equals process time (Table A.2), so VAT + NVAT + NNVAT of a process must equal its process time.

**Takt load** is cycle time ÷ customer takt, as on a leveling chart: above 100 % the cycle time is longer than the takt the customer sets. It is not equipment utilization, which would need each resource's busy time, and it uses only the customer's takt, so a process's `available_time` does not change it. **Load** (definition 3.2) is demand × cycle time ÷ the time available to the process: its `available_time` when entered, otherwise the customer's `operating_time`. Above 100 % the process cannot keep up with demand. Load equals takt load when the process has no `available_time` of its own. **Bottleneck** (definition 3.2) is the process with the highest load. Processes without a cycle time are left out. If no load can be computed, the bottleneck is the process with the longest cycle time. If only some loads can be computed, it is also the longest cycle time, and a note says so.

**Rework rate** is stored as entered (Table A.2 defines it as a share) and reported per process in the JSON results, next to scrap rate, yield factor and yield after rework. A.15 says yield after rework = yield factor + rework rate. `yield_after_rework` is an optional input. Given it and the yield factor, the rework rate is derived; given all three, a difference is a warning and your values are kept. A yield after rework below the yield factor is an error, and no rework rate is derived from it. The function `ReworkRate` gives the difference of two yields. The stream's scrap rate and yield factor come from the `quality` counts. The process yield factor and scrap rate are linked by A.17.

**Prioritisation** of measures (clause 4.5.2) ranks the open measures by net benefit (benefit − cost) first, then by benefit ÷ cost. This follows the priorities of the worked example in Table B.5, since the standard fixes no formula. Measures without both figures come last, in catalogue order, and implemented or cancelled measures are left out. Benefit is used as entered, whatever its `benefit_period`. When open measures with a known net benefit use different periods, the project report warns that their net benefits are not directly comparable.

**Net benefit and payback.** `Difference` is benefit − cost, using the figures as entered: a benefit given per period is not converted to a total. ISO 22468 defines no payback period, so payback is derived from the catalogue. `Payback` is cost ÷ benefit, counted in the periods of `benefit_period` (`day`, `week`, `month` or `year`): how many periods the benefit takes to repay the cost. Cost is a one-off amount and benefit is earned once per period. Payback is unknown when `benefit_period` is missing or not one of those values. It is 0 at zero cost and +Inf at zero benefit.

**Measure status** uses the five stages of the catalogue key in Annex B.2.4 (`problem_detected`, `responsibilities_clarified`, `measure_checked`, `implementing`, `implemented`), plus `cancelled`. Clause 4.4.2 asks for the status to be noted but names no stages, so the library takes them from the standard's own example.

**SMART gaps (4.4.2).** A measure lists the SMART criteria its fields do not yet show. Acceptance needs a responsible person, and the acceptance (`accepted`) and realism (`realistic`) of the measure are your judgement, so each stays a gap until recorded as true.

**Product family (4.2.2).** The family shall have similar process steps and be representative; it preferably has a balanced volume. Record each as a `criteria` entry with `met`. A not-met similar process steps or representative characteristic gives a warning. A not-met balanced volume gives a note, because the standard prefers it rather than requiring it. Once any is recorded, a note lists the ones not yet assessed.

**Monitoring (4.5.1, 4.5.2).** The standard asks for the changed value stream to be monitored over time. Each `monitoring` snapshot is a date and the value stream as measured then. It is evaluated like any state and compared with the current state by the figures of 4.5.2. A measure whose `kpi` names a stream total or KPI (for example `lead_time` or `scrap_rate`) is checked against its `target` in the snapshot with the latest date that has that figure. A time target is in the current state's time unit, and the measured time is converted to it. The measure's `target_rule` says how the measured value is judged: `at_most` (at or below the target), `at_least` (at or above it) or `equal` (within `tolerance` of it, in the target's unit; with no tolerance, exactly equal). Without a rule, times, the non-value adding share and the scrap rate are judged as at most the target, and the other KPIs as at least it. That default is this library's choice; the standard sets none. A tolerance given with any rule other than `equal` is ignored, with a warning. An unknown rule or a negative tolerance is an error, and that measure gets no verdict.

**Project phase (4.1)** records where the project stands: `analysis`, `design`, `planning`, `implementation`, `assessment` (check) or `adjustment` (act). The phases are in that order. A step is noted as missing once the project reaches the phase after the one that performs it: the product family (4.2.2) from `design`, the ideal state (4.3.3) and future state (4.3.4) from `planning`, the catalogue of measures (4.4.2) from `implementation`, and monitoring (4.5.1) from `adjustment`. The workshop (4.4.3) is noted from `planning`, or as soon as a measure exists, until it is recorded as agreed. A project with no phase gets none of these notes, except the workshop note once a measure exists.

**Typos in the worked example (A.3.4).** It labels customer demand as a time and calls the number of transport repetitions a time. The library reads these as the quantities the parameter table defines.

**Two bases for idle time.** A.7 measures a range against demand per period, which the library converts to time with the calendar `period_length`. A.11 measures waiting against customer takt, which is based on operating time. The same number of pieces therefore gives a different time depending on whether it is entered as stock or as waiting orders. This follows the standard's two formulas as written.

## Scope

**Covered (normative):** the parameters of Table A.2, formulas A.1–A.17, the symbols of Table A.1 (`vsm.Symbols`), the shares and comparison figures of clause 4.5.2, the seven wastes and improvement flashes (4.3.2), the eight design guidelines as a checklist you fill in (4.3.4), the product family characteristics (4.2.2), the SMART catalogue of measures with status (4.4.2), the workshop record (4.4.3), monitoring of the changed value stream (4.5.1, 4.5.2) and the project phase (4.1).

**Not covered:** drawing value stream maps; the informative data boxes and examples of Annex B, beyond the measure status stages and their use in tests; multi-criteria assessment by space, resources or cost (4.5.2 mentions it without defining it); and any automatic future-state design.

## License

MIT. See [LICENSE](LICENSE).
