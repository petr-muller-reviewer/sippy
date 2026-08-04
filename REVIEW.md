---
pr: openshift/sippy#3852
title: "TRT-2848: Refresh summary tables incrementally during prow load"
head_sha: ac76eb7b6b2a8de093cdce23608d2bdf0a1ae92c
base: main
reviewed_at: 2026-08-04T21:01:15Z
verdict: needs-discussion
pr_state: MERGED
refresh_log:
  - from: 3ba7641f7470e8e28746ebebad76cd7cdd7b8d79
    to: 3ba7641f7470e8e28746ebebad76cd7cdd7b8d79
    at: 2026-07-31T11:03:11Z
    summary: No code changes. petr-muller submitted an APPROVED review with /hold at 11:00:51Z ("found no problems, but I do not feel too confident about knowing this part of Sippy that well... hold for the case you want someone better with DBs than me to look as well"); PRB approval-notifier bot comment followed at 11:01:24Z.
  - from: 3ba7641f7470e8e28746ebebad76cd7cdd7b8d79
    to: ac76eb7b6b2a8de093cdce23608d2bdf0a1ae92c
    at: 2026-08-04T12:58:57Z
    summary: PR rebased onto current main to absorb merged PR #3862 ("Add lifecycle as a dimension in summary tables"). Only content change is `lifecycle` threaded through pgwriter.go's batch_deltas, ensureDailyTotalRows/updateDailyTotals, ensureCumulativeSummaryRows/updateCumulativeSummaries, and carryForwardRelease's INSERT, matching the existing dailysummary.go schema/ON CONFLICT clause. Verified every touched SQL statement includes lifecycle consistently (no query missing it in SELECT/GROUP BY/WHERE). New integration test TestDailyTotalsScopedByLifecycle covers the partitioning. The previously-flagged blocking finding (batch-wide rollback on future-dated data) is unchanged and still unaddressed. do-not-merge/hold and approved labels both still present; PR's own CI (test e2e) passed 2026-08-04T04:40Z.
  - from: ac76eb7b6b2a8de093cdce23608d2bdf0a1ae92c
    to: ac76eb7b6b2a8de093cdce23608d2bdf0a1ae92c
    at: 2026-08-04T21:01:15Z
    summary: No code changes. neisw commented "/lgtm" at 17:42:34Z (lgtm label added). mstaeble ran "/hold cancel" at 18:53:59Z, removing the do-not-merge/hold label without any code change addressing the blocking finding below. PR merged. CI (test e2e) passed again at 19:18:43Z. The batch-wide-rollback finding was never resolved in code; it merged on the strength of the author's own approval plus neisw's lgtm.
---

## Summary

Replaces the periodic full-table refresh of `test_daily_totals`/`test_cumulative_summaries`
with per-batch incremental upserts during prow loading. Extracts DB-write logic from
`prow.go` into new `pkg/dataloader/prowloader/pgwriter` package. Parallelizes cross-release
carry-forward (errgroup, limit 4). Pins `currentDate` once at `ProwLoader` construction.
Adds 32 integration tests in `test/integration/pgwriter_test.go`. Removes
`dailysummary.Refresh`/`cumulativesummary.Refresh` calls from `RefreshData` (backfill code
path retained and still used by `sippy backfill`).

Since previous review: PR was rebased onto current `main`, absorbing merged PR #3862
("Add lifecycle as a dimension in summary tables"). This required threading a new
`lifecycle` column through every summary-table SQL statement in `pgwriter.go`. Verified this
addition is complete and consistent across `batch_deltas`, daily-totals ensure/update,
cumulative-summaries ensure/update, and carry-forward's INSERT — matching the pre-existing
`ON CONFLICT (release, date, test_id, suite_id, lifecycle, prow_job_id)` constraint in
`dailysummary.go`. A new integration test (`TestDailyTotalsScopedByLifecycle`) exercises the
lifecycle partitioning directly. No new bugs introduced by this rebase. The blocking finding
below (batch-wide rollback on future-dated data) remains unaddressed since 2026-07-31.

Since second review: no further code changes. neisw lgtm'd, mstaeble cancelled the hold, and
the PR merged at head `ac76eb7b6b2a8de093cdce23608d2bdf0a1ae92c` without the blocking finding
being addressed in code. Recorded here for history; no further action possible on a merged PR.

## Findings

### [blocking] Future-dated test result rolls back the entire batch, not just the offending run
- where: `pkg/dataloader/prowloader/pgwriter/pgwriter.go:596-609`
- concern: `upsertSummaryTables` checks for any row in `batch_deltas` dated more than one day
  past `currentDate` and returns an error if found. That error propagates out of `Write()`,
  rolling back the whole transaction — including `prow_job_runs`, annotations, PR
  associations, and `prow_job_run_tests` inserts for every run in the batch (up to
  `flushThreshold = 100` runs per `accumulateAndWrite`). One job with a clock-skewed or
  malformed JUnit timestamp can drop up to 99 unrelated, valid job runs from a batch. Because
  the failing run's ID never gets persisted to `prow_job_runs`, it is never marked "loaded,"
  so it will be re-fetched and re-fail on every subsequent load cycle — a poison pill that
  can wedge ingestion indefinitely. The only test covering this path
  (`TestFutureDatedTestResultsRejectBatch`) uses a single-run batch, so the collateral damage
  to co-batched runs is untested and unasserted anywhere.
- excerpt: |
    tomorrow := currentDate.AddDays(1)
    var hasFuture bool
    if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM batch_deltas WHERE batch_date > $1)`, tomorrow).Scan(&hasFuture); err != nil {
        return fmt.Errorf("checking for future-dated batch rows: %w", err)
    }
    if hasFuture {
        return fmt.Errorf("batch contains test results dated after %s; refusing to write summaries", tomorrow)
    }

### [should-fix] Prometheus histogram removed with no replacement
- where: `pkg/sippyserver/server.go:148-153` (deleted `cumulativeSummaryRefreshMetric`)
- concern: The old `sippy_cumulative_summary_refresh_millis` histogram is deleted along with
  the old refresh call; the new incremental path only emits `log.WithField("elapsed",
  ...).Debug(...)` lines in `pgwriter.go`. Any dashboard/alert on that metric goes silently
  stale. Not called out in the PR description as an intentional observability change.
- excerpt: |
    -var cumulativeSummaryRefreshMetric = promauto.NewHistogram(prometheus.HistogramOpts{
    -	Name:    "sippy_cumulative_summary_refresh_millis",
    -	Help:    "Milliseconds to refresh the cumulative summary tables",
    -	Buckets: []float64{100, 500, 1000, 5000, 10000, 30000, 60000, 300000, 600000},
    -})

### [nit] Postgres-specific NULL-ignoring semantics of LEAST/GREATEST relied on without comment
- where: `pkg/dataloader/prowloader/pgwriter/pgwriter.go:720-742`, `766-794`
- concern: `updateDailyTotals`/`updateCumulativeSummaries` use `LEAST`/`GREATEST` against
  columns that start NULL (via `ensureDailyTotalRows`/`ensureCumulativeSummaryRows`, which
  don't set the timestamp columns). This is correct only because Postgres's `LEAST`/`GREATEST`
  ignore NULL arguments (non-ANSI-standard behavior) — worth a one-line comment so a future
  reader porting this logic doesn't assume standard SQL NULL-propagation semantics.
- excerpt: |
    first_failure_timestamp = LEAST(dt.first_failure_timestamp, bd.min_failure_ts),
    last_failure_timestamp = GREATEST(dt.last_failure_timestamp, bd.max_failure_ts),

### [question] Is the batch-wide rejection on future-dated data an intentional tradeoff?
- concern: Is "fail the whole batch loudly" preferred over "skip/clamp the offending run's
  summary contribution and keep the rest"? If intentional, worth documenting the operational
  runbook for un-wedging a stuck batch (e.g., is there tooling to identify/skip the offending
  prow job run?).

## Checked
- SQL correctness of `ensureCumulativeSummaryRows`'s `GROUP BY` over query-literal columns — valid, not a bug.
- `carryForwardRelease`/`findLatestDateWithData` day-by-day backward walk (capped 30 days) — idempotent, confirmed against `TestCarryForwardIsIdempotent` and the "already up to date" short-circuit.
- No BigQuery counterpart exists for `test_daily_totals`/`test_cumulative_summaries` (Postgres-only ETL, not part of the dual-provider `componentreadiness/dataprovider` abstraction) — project's BQ/Postgres parity rule doesn't apply here.
- `RefreshData` doc comment accurately reflects new behavior; `dailysummary`/`cumulativesummary` imports in `server.go` still used by `BackfillData`, no dead code.
- `accumulate_test.go` mechanically updated to new `pgwriter.JobRunResult`/`pgwriter.RunRow` types and refactored `accumulateAndWrite(ctx, results, writerFunc)` signature — correct, no behavior change.
- Test coverage of happy paths, carry-forward parallelism, release scoping, soft-delete restoration, PR metadata preservation, dedup — thorough (32 integration tests).
- No SQL string concatenation of untrusted input; all queries parameterized.
- New `lifecycle` column additions from the rebase onto merged PR #3862: every touched SQL statement in `pgwriter.go` (batch_deltas, daily-totals ensure/update, cumulative-summaries ensure/update, carry-forward INSERT) consistently includes `lifecycle` in SELECT/GROUP BY/WHERE, matching `dailysummary.go`'s existing `ON CONFLICT (release, date, test_id, suite_id, lifecycle, prow_job_id)`. No column left un-threaded.
- `TestDailyTotalsScopedByLifecycle` integration test exercises lifecycle partitioning directly.

## Open questions
- Is the batch-wide rollback on future-dated test data an accepted risk, or should this PR scope the rejection to just the offending job run(s) before merge?
- Is there existing/planned tooling to detect and clear a "stuck" poison-pill batch in production?
- Was the removed `cumulativeSummaryRefreshMetric` histogram intentionally dropped, or should an equivalent be added for the new incremental path?
