---
pr: openshift/sippy#3659
title: "add daily summary table to accelerate matview refreshes"
head_sha: 9babd2dc1829c21a5dfc53912e0ca35440711db3
base: main
reviewed_at: 2026-06-23T19:59:50Z
verdict: approve
refresh_log:
  - from: 50ff28f54727a131cdc9c9a03e7b1a5dc3231223
    to: 9babd2dc1829c21a5dfc53912e0ca35440711db3
    summary: "Rebased on main (#3658 merged). Fix commit 9babd2dc1 addresses error handling bugs (parallel worker continue-on-error, fatal RefreshData error, truncate ordering). CodeRabbit suggestions incorporated (COALESCE in matview, CASCADE in down migration, start>end validation). Tests improved (thread-safe fakeStore, parallel test, time.Date instead of tp helper). not-stbenjam CHANGES_REQUESTED 3 bugs — all addressed by fix commit."
---

## Summary

Adds `test_daily_summaries` table pre-aggregating `prow_job_run_tests` into daily buckets per (test_id, prow_job_id, suite_id, release, date). Matview definitions rewritten to read from summary table. Daily summary refresh runs before matview refreshes in `RefreshData()`. New CLI flags on `sippy refresh` for rebuild and date overrides. Benchmarks show 2-3x speedup (30min to 12min for 7d, 25min to 8min for 2d).

Since previous review: rebased on main (includes merged #3658 CASCADE change). Fix commit `9babd2dc1` addresses:
- Parallel worker error handling: `return` changed to `continue` so workers keep consuming after per-release failure.
- `RefreshData` now returns early on daily summary failure, preventing matview rebuild on empty/stale data.
- Truncate moved after `dateRange`/`Releases` queries, reducing window for empty-table failure mode.
- Release aggregation parallelized across 4 workers via `sync.WaitGroup.Go()`.
- INSERT split from ON CONFLICT clause; `skipConflictDetection` skips upsert overhead on rebuild/empty table.
- COALESCE(SUM(...), 0) wraps all matview SUM() FILTER expressions.
- CASCADE added to down migration.
- Start > end date validation in CLI.
- Tests: thread-safe fakeStore, `time.Date` replaces `tp()` helper, new parallel and matview-creation-order tests.

## Findings

### [nit] No test for EndOverride-only path
- where: `pkg/db/dailysummary/dailysummary.go:171-175`
- concern: `dateRange` handles EndOverride-only (start auto-resolved, end overridden) but no test covers this path. Code is straightforward so low risk.
- excerpt: |
    endDate := now
    if opts.EndOverride != nil {
        endDate = *opts.EndOverride
    }

### [question] Stale summaries after source data deletion
- where: `pkg/db/dailysummary/dailysummary.go:27-50`
- concern: If rows are deleted from `prow_job_run_tests`, orphaned summary rows persist with old counts. The upsert INSERT...SELECT only produces rows for data that currently exists, so deleted-source rows are never corrected. Append-only ingestion makes this unlikely, and `--rebuild-daily-summaries` covers it, but operators should know.

### [question] Residual truncate-then-fail window (not-stbenjam Bug 2)
- where: `pkg/db/dailysummary/dailysummary.go:100-107`
- concern: When `Rebuild=true`, truncate still commits immediately outside any transaction. If `aggregateReleases` subsequently fails, the table is empty. The sting is largely removed because `RefreshData` now returns early (matviews aren't rebuilt from empty data), and the next incremental refresh auto-recovers (empty table triggers default lookback). But a transactional truncate+repopulate would eliminate the window entirely. Low practical risk given the current mitigations.
- excerpt: |
    if skipConflictDetection {
        log.Info("rebuild requested, truncating test_daily_summaries")
        if err := store.Truncate(); err != nil {
            return fmt.Errorf("truncating table: %w", err)
        }
    }

## Resolved

### [blocking] Compilation break: missed RefreshData call site
- where: `pkg/flags/postgres_benchmarking_test.go:1233`
- resolved_by: fix commit 9babd2dc1 — added `dailysummary.Options{}` as 4th argument.

### [nit] Time-dependent hardcoded date in test
- where: `pkg/db/dailysummary/dailysummary_test.go:55`
- resolved_by: fix commit 9babd2dc1 — `tp()` helper removed, all test dates use `time.Date()`.

### [question] CASCADE commit overlap with #3658
- where: `pkg/db/views.go:156`
- resolved_by: #3658 merged (commit c32851453). CASCADE change is now part of main; this PR's rebase carries it naturally.

### [not-stbenjam Bug 1] Worker goroutine exits on first error
- where: `pkg/db/dailysummary/dailysummary.go:139`
- resolved_by: fix commit 9babd2dc1 — `return` changed to `continue`, workers keep consuming from work channel.

### [not-stbenjam Bug 3] RefreshData swallows daily summary error
- where: `pkg/sippyserver/server.go:393-395`
- resolved_by: fix commit 9babd2dc1 — error now causes early `return`, matview refresh is skipped.

## Checked

- `suite_id=0` for NULL suites: `COALESCE(suite_id, 0)` in upsert + LEFT JOINs to `suites` and `test_ownerships` behave identically for id=0 as for NULL. Semantics preserved.
- Date boundary precision: DATE-to-TIMESTAMP comparisons can exclude up to ~1 day at window start. Documented in PR description, acceptable for trend analysis.
- `runs` column counts all rows via `COUNT(*)` including non-success/failure/flake statuses. Consistent with original matview behavior.
- `valueColumns` slice generates both INSERT and ON CONFLICT clauses, preventing column drift.
- Per-release aggregation loop avoids 51s planner overhead on 3553 sub-partitions. Now parallelized across 4 workers.
- Non-fatal daily summary failure: `server.go` now returns early, preventing matview rebuild on empty/stale data.
- Migration idempotency: `IF NOT EXISTS` on both table and indexes. Down migration drops cleanly with CASCADE.
- All `RefreshData` call sites updated with 4-arg signature.
- No data pruning: summaries accumulate indefinitely. `summary_date` index keeps 14-day filter selective.
- `TestDailySummary` model in `prow.go` matches migration schema.
- `sync.WaitGroup.Go()` is valid — go.mod specifies `go 1.25.0`.
- `errs` channel capacity `len(releases)` — can't block since each worker sends at most one error per release.
- `skipConflictDetection` correctly skips ON CONFLICT for rebuild (post-truncate) and empty table (no conflicts possible).
- COALESCE(SUM(...), 0) prevents NULL propagation through matview for window-boundary tests.
- Start > end validation in `dailySummaryOptions()` rejects reversed override ranges.
- fakeStore in tests is thread-safe (mutex on calls slice) for parallel worker testing.
- `TestRefresh_ParallelProcessesAllReleases` verifies all releases processed regardless of worker count.
- `TestMatviewSourcesCreatedBeforeDependents` validates matview creation order in `PostgresMatViews` slice.

## Open questions

- Should stale-summary-after-deletion be documented somewhere for operators, or is `--rebuild-daily-summaries` sufficient as a catch-all?
- Is the residual truncate-then-fail window acceptable, or should a transactional approach be considered for a follow-up?
