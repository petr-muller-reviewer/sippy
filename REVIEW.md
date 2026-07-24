---
pr: openshift/sippy#3811
title: "TRT-2741: Fix chat agent references to dropped matviews and add pre-aggregated table docs"
head_sha: 29e595dbbcb2f14fb7ec04fcbcb683500046be6a
base: main
reviewed_at: 2026-07-24T10:35:31Z
verdict: approve
---

## Summary

Docs/prompt-only change to `chat/prompts/test-analysis.yaml` and `chat/sippy_agent/tools/database_query.py`. Removes references to three dropped matviews (`prow_test_report_7d_matview`, `prow_test_report_2d_matview`, `prow_job_failed_tests_by_hour_matview`), documents new pre-aggregated tables `test_daily_totals`/`test_cumulative_summaries`, adds partition-pruning guidance, and fixes several latent SQL bugs in example queries (bigint timestamp comparison, `text[]` `->>` misuse, off-by-one date ranges, missing `LIMIT`).

## Verification performed

- Cross-checked `test_daily_totals`/`test_cumulative_summaries` column names against `pkg/db/dailysummary/dailysummary.go` and `pkg/db/cumulativesummary/cumulative_summary.go` — match exactly.
- Cross-checked partition key columns (`prow_job_run_release`/`prow_job_run_timestamp`, `prow_job_run_test_release`/`prow_job_run_test_timestamp`) against `pkg/db/models/prow.go` — match.
- Confirmed `prow_job_runs_report_matview.timestamp` is bigint epoch-ms via `pkg/db/views.go:217` — the new example 5 fix (`(EXTRACT(EPOCH FROM NOW() - INTERVAL '7 days') * 1000)::bigint`) is correct; the old direct comparison to `NOW() - INTERVAL` was broken.
- Confirmed `variants` is `text[]` not JSONB (views.go, prow job model) — the `->> ` to `unnest()` fix in example 5 is correct.
- Confirmed `prow_job_runs.prow_job_release` is the real column name (gorm default snake_case of `ProwJobRelease` field) used in the two-step partition-key lookup pattern (examples 3, 7).
- Verified prefix-sum subtraction pattern in example 9 matches the recurrence used by `UpdateDateForRelease` in `cumulative_summary.go`.
- Repo-wide grep confirms no remaining references to the three dropped matviews anywhere.
- `python3 -c "import ast; ast.parse(...)"` on `database_query.py` — parses fine (change is inside a string literal).
- `python3 -c "import yaml; yaml.safe_load(...)"` on `test-analysis.yaml` — parses fine.

## Findings

### [question] Assumed timestamp equality between parent run and partitioned test row
- where: `chat/sippy_agent/tools/database_query.py:181-199` (example 3), `:262-289` (example 7)
- concern: The two-step pattern looks up `prow_job_runs.timestamp` and reuses it as a literal filter for `prow_job_run_tests.prow_job_run_timestamp`/`prow_job_run_test_outputs.prow_job_run_test_timestamp`. This assumes the denormalized timestamp on the child partitioned tables is always bit-for-bit identical to the parent's `timestamp`. Model comments suggest this ("denormalized ... for partitioning") but it's not verified end-to-end; if it ever drifts, the two-step query would silently return zero rows rather than erroring.
- excerpt: |
    AND pjrt.prow_job_run_timestamp = '2026-07-15T10:30:00Z'::timestamptz -- Partition key: from step 1

### [nit] No mention of staleness/lag for pre-aggregated tables
- where: `chat/sippy_agent/tools/database_query.py:76-92`
- concern: Docs don't caveat that `test_daily_totals`/`test_cumulative_summaries` may lag behind `prow_job_run_tests` (aggregation is presumably batch/async). Minor, since it doesn't affect correctness of the SQL itself, just could mislead the agent about "as of now" freshness.
- excerpt: |
    * **`test_daily_totals`**: Pre-aggregated daily test pass/fail/flake counts. Use this instead of scanning `prow_job_run_tests` with COUNT/GROUP BY.

## Checked
- Schema/column-name accuracy of all new table docs against Go models and query code.
- Correctness of all fixed SQL bugs (bigint timestamp, text[] unnest, off-by-one ranges, missing LIMIT).
- No stale matview references left anywhere in the repo.
- Python/YAML files still parse after the string-literal edits.
- Pure docs/prompt change — no test coverage needed, consistent with PR's own manual test plan.

## Open questions
- Is the assumption that `prow_job_run_tests.prow_job_run_timestamp` always exactly equals the parent `prow_job_runs.timestamp` guaranteed by the ingestion code, or just true in practice today?
- Should the docs mention aggregation lag/freshness for `test_daily_totals`/`test_cumulative_summaries`?
