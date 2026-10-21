---
name: iso-vsm
description: Use when evaluating or checking a value stream map or VSM project in JSON with the iso_vsm tool (ISO 22468:2020), for lead time, takt, value adding share, bottleneck or data errors.
---

# iso-vsm

## What the tool does

The `vsm` command and the `iso_vsm` Go library (module `github.com/slopez37-eng/iso_vsm`) evaluate value stream maps and VSM projects by ISO 22468:2020. They:

- derive missing times from the Annex A formulas and record which formula gave each value;
- check the numbers you entered against those formulas and report contradictions;
- compute totals (process, idle and lead time), KPIs (customer takt, value added ratio, value adding share, OTIF, yield, inventory turns) and the bottleneck;
- compare a current and a future state (clause 4.5.2) and check the catalogue of measures for SMART gaps.

They do not draw maps and do not design a future state.

## Workflow

1. `vsm template > project.json` writes a starter project. Edit that file.
2. Leave out numbers you do not know (see the data rules below).
3. `vsm check project.json` prints the results and the issues. Add `-json` for machine-readable output.
4. Fix every error, then read the warnings and notes.

`vsm symbols` lists the value stream symbols of Table A.1.

A file whose top-level keys are all project fields (`name`, `current`, `ideal`, `future`, `measures`, `workshop`) is checked as a project. Any other file is checked as one value stream.

Exit codes: 0 means no errors. 1 means the data has errors (invalid input was ignored). 2 means a usage error, or the file could not be read or decoded. Unknown JSON field names count as unreadable, so a misspelt field gives exit 2. Warnings and notes never change the exit code.

## Data-entry rules

- Leave a number out when it is unknown. Do not enter 0 for an unknown value: an omitted number means "not entered", while 0 means an entered zero.
- Enter 0 only for a part that truly does not exist, such as `transport_time: 0` for a segment with no transport.
- Rates are fractions: 2 % is `0.02`. A rate above 1 is an error.
- Every time in one value stream uses that stream's single `time_unit` (`s`, `min`, `h` or `d`). This includes the customer's `operating_time` and `period_length`.
- `customer.period_length` converts an inventory range given as a quantity into time (A.7). Without it, such storage time stays unknown and lead time may stay unknown too.
- `quality` and `deliveries` are counts (defective of total, complete on time of total), not rates.
- Process `type` follows the cases of Figure A.1: `manual` (case A), `automated` (case B) or `data` (case C). Downtime counts in process time only for `automated`. Handling time does not count for `data`. With no type, all parts count.
- `cycle_time` is the observed interval at which the whole process completes products, losses included.
- For a batch process, enter `process_time` as the dwell time of one piece, which stays for the whole batch (Table A.2).
- A.5 is shown in notes but never applied. When a note says what A.5 would give, enter `process_time` yourself if you accept it.

## Reading the output

- **error**: the input was invalid, so that value was ignored. Fix it.
- **warning**: an entered or derived number contradicts a formula. Nothing was overwritten; decide which number is right.
- **note**: a gap or a choice the evaluation made, for example which segments still lack a time, or why A.5 was not applied.
- Each issue is printed as `severity: subject: message [rule]`. The subject is such as `process P2`, and the rule is the clause or formula it rests on, such as `A.6`.
- In `-json`, each derived or entered value is an object with `value` and `source` (`entered` or the formula). An unknown value is `null`.
- In text output, an unknown value shows as `?`.
- A project report has one section per state. The comparison appears only when the current and future lead times are both known and not zero; otherwise a note says why.
- Errors in any state, or in the project itself, make the exit status 1.

## Library use

- `vsm.Evaluate(vs)` returns a `Result` for one value stream. `vsm.EvaluateProject(p)` returns a `ProjectReport` with one `Result` per state, the comparison and the measures.
- `Result` has `Issues` and `HasErrors()`. `ProjectReport` has `AllIssues()` and `HasErrors()`.
- Optional numbers are pointers: `vsm.F(x)` is an entered value, and `nil` is not entered.
- `LoadValueStream`, `LoadProject`, `Save` and `Write` read and write the JSON format. Reads reject unknown fields.
