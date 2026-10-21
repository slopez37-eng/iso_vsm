# Changelog

## Unreleased

- `vsm import` and `ReadCSV` build a value stream from a CSV file, one row per process, idle or inventory. The reader is strict: unknown columns and misplaced or malformed values are errors that name the line and column.
- Measures take a `benefit_period` (day, week, month or year). `Difference` (benefit − cost) and `Payback` (cost ÷ benefit, counted in those periods) are derived, and `vsm check` prints the payback.
- Bottleneck (3.2) is the process with the highest load, demand × cycle time ÷ its `available_time` (new, optional) or else the customer's operating time. Without a computable load it is the longest cycle time. Each process reports its load in JSON and as a column of the CLI table.
- Rework rate (A.15) is derived from `yield_factor` and the optional `yield_after_rework`, and reported per process in the JSON results with both. The three are checked against each other; a yield after rework below the yield factor is an error.
- Balanced volume (4.2.2) is preferred, not required: an unmet one gives a note. Similar process steps and representative stay warnings.
- Table A.1: the customer symbol is the end customer; a customer plant uses the external source symbol.
- The batch note (Table A.2) uses batch size only, not repetitions.
- README states which values are derived and the limits of derivation (A.7, A.11, A.5).
- Measures take `accepted`, `realistic`, `target_rule` (`at_most`, `at_least` or `equal`) and `tolerance`. Until a measure is recorded as accepted and realistic, it shows a SMART gap. `equal` accepts a monitored value within `tolerance` of the target.
- Prioritisation ranks measures by net benefit (benefit − cost) first, then by benefit ÷ cost, following Table B.5. Implemented and cancelled measures are left out.
- Project reports warn when open measures with a known net benefit use different benefit periods (4.5.2).
- Project reports note a missing procedure step once the phase reaches the next one: product family (4.2.2), ideal and future state (4.3.3, 4.3.4), catalogue (4.4.2) and monitoring (4.5.1). The workshop (4.4.3) is noted from planning, or once any measure exists.

## v1.0.0

First release of the rewritten SDK. It replaces the earlier fork entirely; nothing of its API is kept.

- Formulas A.1–A.17 and the clause 4.5.2 shares and comparison figures as plain functions.
- Value stream and project model with optional numbers, strict JSON load and save.
- `Evaluate` and `EvaluateProject`: derive what the formulas allow, report contradictions and invalid input as issues.
- Catalogue of measures with SMART gap check, cost-benefit priority and the Annex B.2.4 status stages; workshop record; Table A.1 symbols.
- Product family characteristics (4.2.2) and project phase (4.1) as recorded checklists.
- Monitoring snapshots compared with the current state (4.5.1, 4.5.2), and measure targets checked against the latest snapshot.
- `SKILL.md` for AI agents and a runnable `ExampleEvaluate`.
- `vsm` command-line tool: `check`, `symbols`, `template`, `version`. Saves replace the file atomically and keep its permissions, write through a symbolic link to its target, and fail on a read-only file; a file is checked as a project by its `current` key, then by the keys only one kind has; `Value` decodes its own JSON, including ±Inf; flash targets and information-flow ends are checked against the map's ids (information flows also accept `customer` and `planning`); `version` shows the commit, with `+dirty` for a modified tree.
