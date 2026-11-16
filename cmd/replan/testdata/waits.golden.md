# Measured waits: Wait fixture

- Status date: `2026-11-12`
- Since bound: span start date on or after `2026-11-01`
- Until bound: span start date on or before `2026-11-30`
- Filter attribution: spans are attributed by their start date; durations are not clipped
- Tasks without status history: 1

## Overall waits and work time

| Kind | Count | Hours median | p75 | max | Working median | p75 | max | Calendar median | p75 | max |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| in_progress (work time) | 1 | 77.50 | 77.50 | 77.50 | 3 | 3 | 3 | 3 | 3 | 3 |
| review_wait | 1 | 2.50 | 2.50 | 2.50 | 0 | 0 | 0 | 0 | 0 | 0 |

## Per tag (at least 5 measured waits)

| Tag | Kind | Count | Hours median | p75 | max | Working median | p75 | max | Calendar median | p75 | max |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| `frequent` | in_progress (work time) | 1 | 77.50 | 77.50 | 77.50 | 3 | 3 | 3 | 3 | 3 | 3 |
| `frequent` | blocked_wait | 5 | 24.00 | 24.00 | 72.00 | 1 | 2 | 3 | 1 | 2 | 3 |

## Per assignee hint resource

| Assignee | Kind | Count | Hours median | p75 | max | Working median | p75 | max | Calendar median | p75 | max |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| `resource.owner` | in_progress (work time) | 1 | 77.50 | 77.50 | 77.50 | 3 | 3 | 3 | 3 | 3 | 3 |
| `resource.owner` | review_wait | 1 | 2.50 | 2.50 | 2.50 | 0 | 0 | 0 | 0 | 0 | 0 |

## Task spans

### Owner review

`task.review`

| Status | Kind | From date | To date | From at | To at | Hours | Precision | Working days | Calendar days | Open |
|---|---|---|---|---|---|---:|---|---:|---:|---|
| `review_ready` | review_wait | 2026-11-06 | 2026-11-06 | 2026-11-06T15:30:00+07:00 | 2026-11-06T18:00:00+07:00 | 2.50 | timestamp | 0 | 0 | false |

## Planned lead time comparison

Lead time is compared with the measured wait selected by its reason; an unclassified reason models `review_wait`.

| Task | Kind | Measured span | Planned | Measured | Difference | Reason |
|---|---|---|---:|---:|---:|---|
| `task.review` | review_wait | 2026-11-06 → 2026-11-06 | 2 working_days | 0 working_days | -2 working_days | owner review |

## Review waits without planned lead time

These are hints only; the report makes no proposal or write.

| Task | Review waits | Median working days | Median calendar days |
|---|---:|---:|---:|
| `task.unplanned-review` | 2 | 2.5 | 4 |

## Warnings
- `no_status_history`: 1 tasks have no tracking status history
