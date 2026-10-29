# Lint baseline 2026-09-24

Status: closed baseline; the full local lint gate is clean.

Task: `task.self.add-go-lint-baseline-and-ci-gate`.

Configuration: [`../../.golangci.yml`](../../.golangci.yml), golangci-lint
v2.13.2; shell policy in [`../../.shellcheckrc`](../../.shellcheckrc).

## How the gate works

CI is paused. The local gate runs `golangci-lint run ./...` without a
new-issues selector. Since the baseline was cleared on 2026-10-27, every
finding fails the gate. `shellcheck` continues to run with no baseline.

Fixed while taking the baseline: an unused function, a loop that
`staticcheck` simplifies, two if-else chains that are tagged switches, the four
test-only `*DryRun` service wrappers, two unused shell variables. The
`$APP_USER` quoting that `shellcheck` reports in the plugin image build is
intentional: Docker expands it, and the live image's plugin files are owned by
`app:app`.

## Baseline

Complexity and duplication hotspots are reported, not refactored, by this
task (GAP-020). Refactor them when a task has to change them anyway, and
remove the line here when the finding disappears.

Removed since the baseline: `WriteProjectStateMarkdown` (gocognit 143),
split into section writers on 2026-09-25; `validateWorkspace` (gocognit 167),
split into one validator per record kind on 2026-10-15, its diagnostics held
byte-identical by golden files; `(ImportAcceptService).Accept` (gocognit 124)
and its copied apply branches (dupl), one flow with an explicit description
per accept mode on 2026-10-15, held byte-identical by a golden run of every
mode.

Taken again on 2026-10-15 with the same golangci-lint, the report shows that
this list drifted with changes made after it: one more gocyclo finding in
`cmd/replan/main.go`, and `internal/app/workload.go` reported as gocognit
where this list has gocyclo, 21 findings against 20 here. They are not
refreshed here; a run with `--new-from-rev` is what the gate checks.

| Linter | Location | Finding |
| --- | --- | --- |
| dupl | `cmd/replan/main.go:2027` | 2027-2071 lines are duplicate of `cmd/replan/main.go:2073-2117` |
| dupl | `cmd/replan/main.go:2073` | 2073-2117 lines are duplicate of `cmd/replan/main.go:2027-2071` |
| dupl | `cmd/replan/main.go:2143` | 2143-2171 lines are duplicate of `cmd/replan/main.go:2330-2358` |
| dupl | `cmd/replan/main.go:2330` | 2330-2358 lines are duplicate of `cmd/replan/main.go:2143-2171` |
| dupl | `internal/app/import_openproject.go:648` | 648-685 lines are duplicate of `internal/app/import_openproject.go:687-724` |
| dupl | `internal/app/import_openproject.go:687` | 687-724 lines are duplicate of `internal/app/import_openproject.go:648-685` |
| dupl | `internal/app/import_openproject.go:922` | 922-943 lines are duplicate of `internal/app/import_openproject.go:983-1004` |
| dupl | `internal/app/import_openproject.go:983` | 983-1004 lines are duplicate of `internal/app/import_openproject.go:1044-1065` |
| dupl | `internal/app/import_openproject.go:1044` | 1044-1065 lines are duplicate of `internal/app/import_openproject.go:922-943` |
| gocognit | `cmd/replan/main.go:3653` | cognitive complexity 74 of func `writeOpenProjectImportMarkdown` is high (> 60) |
| gocognit | `cmd/replan/main.go:3762` | cognitive complexity 63 of func `writeImportAcceptMarkdown` is high (> 60) |
| gocognit | `internal/app/report.go:200` | cognitive complexity 90 of func `dashboardTrackingRows` is high (> 60) |
| gocyclo | `cmd/replan/main.go:844` | cyclomatic complexity 31 of func `importTaskJugglerResult` is high (> 30) |
| gocyclo | `cmd/replan/main.go:4197` | cyclomatic complexity 35 of func `writeDependencyListMarkdown` is high (> 30) |
| gocyclo | `cmd/replan/main.go:4456` | cyclomatic complexity 32 of func `writeActualSetRemainingDryRunMarkdown` is high (> 30) |
| gocyclo | `internal/app/import_openproject.go:371` | cyclomatic complexity 35 of func `(OpenProjectImportService).Import` is high (> 30) |
| gocyclo | `internal/app/tracking.go:66` | cyclomatic complexity 35 of func `(TrackingService).Suggest` is high (> 30) |
| gocyclo | `internal/app/workload.go:51` | cyclomatic complexity 40 of func `(WorkloadService).Calculate` is high (> 30) |
| gocyclo | `internal/reports/html.go:681` | cyclomatic complexity 31 of func `writeDashboardGanttChartHTML` is high (> 30) |
| gocyclo | `internal/reports/markdown.go:10` | cyclomatic complexity 31 of func `WriteWeeklyMarkdown` is high (> 30) |

Total: 20 findings.

## 2026-10-27 clean baseline

The full pinned run started with 22 findings (`dupl` 9, `gocognit` 5,
`gocyclo` 8) and finishes with `0 issues.` with all configured thresholds
unchanged.

The refactor:

- replaced the two OpenProject sidecar readers with one typed reader while
  preserving their paths and diagnostic text;
- replaced the three OpenProject collection page fetchers with one typed HTTP
  fetcher while preserving pagination, headers, status handling and
  collection-specific diagnostics;
- moved scenario task, demand and dependency mutations into named store
  methods, leaving the write and audit order explicit;
- split dashboard tracking index construction, task-row construction and
  demand-row calculation into named steps;
- shared CLI result rendering for proposal/audit list commands and shared the
  actual lifecycle command flow for close-demand and add-status;
- split OpenProject import enrichment and snapshot finalization into named
  steps;
- made TaskJuggler import an explicit ordered step list, tracking suggestion a
  stateful validate/analyze/prepare/write flow, and workload calculation a
  stateful sequence of scenario, indexing, capacity, actual, demand and finish
  passes;
- kept only flat document renderers annotated, where extracting individual
  output lines would make byte-stable order harder to audit.

Remaining `nolint` annotations:

- `cmd/replan/main.go`: `writeOpenProjectImportMarkdown` (`gocognit`,
  `gocyclo`) and `writeImportAcceptMarkdown` (`gocognit`, `gocyclo`) preserve
  byte-stable section order in flat Markdown renderers;
- `cmd/replan/main.go`: `writeDependencyListMarkdown` (`gocyclo`),
  `writeActualSetRemainingDryRunMarkdown` (`gocyclo`) and
  `writeActualCloseDemandDryRunMarkdown` (`gocyclo`) keep conditional output
  lines in their exact document order;
- `internal/reports/html.go`: `writeDashboardGanttChartHTML` (`gocyclo`) keeps
  the ordered HTML chart fragments together;
- `internal/reports/markdown.go`: `WriteWeeklyMarkdown` (`gocyclo`) keeps the
  weekly document's one-line output steps in byte-stable order.

No golden file was regenerated. Existing import-accept golden coverage,
OpenProject API pagination/error/retry tests, report HTML golden coverage,
tracking suggestion tests, workload tests, scenario-write tests and CLI tests
cover the refactored paths.

Round 2 added precise precedence assertions before splitting the four ordered
pipelines: OpenProject preload errors remain ordered and stop before snapshot
loading; tracking load and actor errors remain ordered and stop before
analysis; workload load errors still precede scenario resolution; and a
TaskJuggler conflicting mode still wins before actor and run-directory
validation. Only the seven flat renderers above retain annotations.
