# Actuals staging fix review 2026-09-24

Status: reviewed; accepted with notes; should-fix items resolved.

Reviewed commit: `ec3a8d6`, the implementation of
`task.self.fix-actual-audit-ids-and-demand-reopen` (GAP-015, GAP-016,
GAP-019, GAP-024).

Reviewer: a separate AI agent session with fresh context, read-only, which
reproduced each finding on scratch copies of the fixtures. The implementer
and the reviewer are the same model, so this is a second pass, not an
independent human review.

## Verdict

Accept with notes. `go vet`, `go test` and the MVP smoke passed. No
blocker. The staging design held: every store method goes through the
file-system layer, and a validation failure leaves the disk untouched.

## Should-fix findings and resolution

| # | Finding | Resolution |
| --- | --- | --- |
| 1 | A commit that failed part way, reproduced with a read-only `audit/` directory, returned `ok: false` with an empty error list; `written_files` was accurate but the cause was lost. | `commit_failed` diagnostic with the written files; test with a read-only audit directory. |
| 2 | `taskjuggler import-result --write-candidate` still wrote scenario and proposal files directly, without staging or validation. Older than the commit. | Routed through a staged `CreateProjectFile` and `app.CreateProjectFiles`. |
| 3 | `reopen-demand --remaining NaN` and `set-remaining --amount NaN` were accepted; the saved `.nan` then broke every JSON workload report while `validate` still passed. | Commands refuse non-finite amounts; `validate` rejects non-finite or negative `planned_amount` and `remaining_amount`. |
| 4 | Sidecar imports still turned `remaining_amount: 0` into the planned amount. Older than the commit. | The sidecar field is a pointer; explicit zero kept, missing value defaults to planned; test added. |

## Notes and resolution

- Removals were committed after audit lines, so an interrupted proposal
  decision could leave the proposal in both `open/` and `accepted/` with an
  audit line. Removals now commit before audit files.
- TaskJuggler export treated an explicit zero as "not estimated" and
  scheduled the full planned effort. It now skips a demand with nothing left.
- `acknowledge-overrun` accepted cancelled demands and ignored the unit.
  Cancelled demands are refused, suppression requires the same unit, and
  `validate` checks the review's unit, actor and evidence.
- `AppendAudit` now rewrites the whole shard instead of appending. It is
  correct under staging, costs O(n) per write and, like the YAML files, is not
  safe for two concurrent writers. Recorded, not changed.
- `proposal accept` records `PreviousValue: 0` when the remaining amount was
  unset. Low impact; not changed.
- Behaviour changes to expect: explicit `remaining_amount: 0` and
  `overrun_review` appear in files and JSON; `written_files` follows commit
  order; an already invalid project blocks every writing command, including
  one that would repair it.

## Residual

Each file is replaced atomically; a set of files is not. A crash in the
middle of a commit can leave a partial multi-file change, now reported by
`commit_failed` when the process survives. See GAP-019.
