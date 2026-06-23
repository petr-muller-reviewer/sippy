---
pr: openshift/sippy#3659
title: "[WIP] Add daily summary table to accelerate matview refreshes"
head_sha: 50ff28f54727a131cdc9c9a03e7b1a5dc3231223
base: main
reviewed_at: 2026-06-22T12:45:47Z
verdict: request-changes
---

## Summary

Adds `test_daily_summaries` table pre-aggregating `prow_job_run_tests` into daily buckets per (test_id, prow_job_id, suite_id, release, date). Matview definitions rewritten to read from summary table. Daily summary refresh runs before matview refreshes in `RefreshData()`. New CLI flags on `sippy refresh` for rebuild and date overrides. Benchmarks show 2-3x speedup (30min to 12min for 7d, 25min to 8min for 2d).

## Findings

### [blocking] Compilation break: missed RefreshData call site
- where: `pkg/flags/postgres_benchmarking_test.go:1233`
- concern: Still uses old 3-argument signature `sippyserver.RefreshData(dbc, nil, false)`. Function now requires a 4th `dailysummary.Options` argument. Fails `go build ./...`.
- excerpt: |
    sippyserver.RefreshData(dbc, nil, false)

### [nit] Time-dependent hardcoded date in test
- where: `pkg/db/dailysummary/dailysummary_test.go:55`
- concern: `TestRefresh_Incremental` uses `tp("2026-06-17")` which works only because the date is already past. Compare with `TestRefresh_IncrementalCapsAtYesterday` which correctly uses `time.Now().AddDate(0, 0, 1)`. Inconsistent style; not a practical problem today.
- excerpt: |
    store := &fakeStore{maxSummary: tp("2026-06-17")}

### [nit] No test for EndOverride-only path
- where: `pkg/db/dailysummary/dailysummary.go:120-124`
- concern: `dateRange` handles EndOverride-only (start auto-resolved, end overridden) but no test covers this path. Code is straightforward so low risk.
- excerpt: |
    endDate := now
    if opts.EndOverride != nil {
        endDate = *opts.EndOverride
    }

### [question] Stale summaries after source data deletion
- where: `pkg/db/dailysummary/dailysummary.go:27-50`
- concern: If rows are deleted from `prow_job_run_tests`, orphaned summary rows persist with old counts. The upsert INSERT...SELECT only produces rows for data that currently exists, so deleted-source rows are never corrected. Append-only ingestion makes this unlikely, and `--rebuild-daily-summaries` covers it, but operators should know. Worth a sentence in a comment or docs?

### [question] CASCADE commit overlap with #3658
- where: `pkg/db/views.go:156`
- concern: This PR carries the CASCADE change that is also PR #3658. If #3658 merges first and this is rebased, the change is a no-op. Is the plan to squash, or will #3658 be dropped in favor of this PR carrying it?

## Checked

- `suite_id=0` for NULL suites: `COALESCE(suite_id, 0)` in upsert + LEFT JOINs to `suites` and `test_ownerships` behave identically for id=0 as for NULL (no match, NULL result). Semantics preserved.
- Date boundary precision: DATE-to-TIMESTAMP comparisons can exclude up to ~1 day at window start. Documented in PR description, acceptable for trend analysis. Midnight-aligned pinned times avoid it entirely.
- `runs` column counts all rows via `COUNT(*)` including non-success/failure/flake statuses. Consistent with original matview behavior.
- `valueColumns` slice generates both SET and WHERE clauses in upsert, preventing column drift.
- Per-release aggregation loop avoids 51s planner overhead on 3553 sub-partitions.
- Non-fatal daily summary failure in `server.go:393-394` logs error and continues matview refresh.
- Migration is idempotent (`IF NOT EXISTS`). Down migration drops cleanly.
- All 4 `RefreshData` call sites updated (load.go, refresh.go, seed_data.go, updatesuites.go) except the benchmarking test.
- No data pruning: summaries accumulate indefinitely. At a few hundred rows per release per day, stays small for years. `summary_date` index keeps 14-day filter selective.
- `TestDailySummary` model in `prow.go` matches migration schema.

## Open questions

- Should stale-summary-after-deletion be documented somewhere for operators, or is `--rebuild-daily-summaries` sufficient as a catch-all?
- What is the merge plan for the CASCADE change relative to #3658?
