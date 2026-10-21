# Changelog

## v1.0.0

First release of the rewritten SDK. It replaces the earlier fork entirely; nothing of its API is kept.

- Formulas A.1–A.17 and the clause 4.5.2 shares and comparison figures as plain functions.
- Value stream and project model with optional numbers, strict JSON load and save.
- `Evaluate` and `EvaluateProject`: derive what the formulas allow, report contradictions and invalid input as issues.
- Catalogue of measures with SMART gap check, cost-benefit priority and the Annex B.2.4 status stages; workshop record; Table A.1 symbols.
- Product family characteristics (4.2.2) and project phase (4.1) as recorded checklists.
- Monitoring snapshots compared with the current state (4.5.1, 4.5.2), and measure targets checked against the latest snapshot.
- `SKILL.md` for AI agents and a runnable `ExampleEvaluate`.
- `vsm` command-line tool: `check`, `symbols`, `template`, `version`.
