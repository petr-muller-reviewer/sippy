---
pr: openshift/sippy#3852
title: "TRT-2848: Refresh summary tables incrementally during prow load"
head_sha: 3ba7641f7470e8e28746ebebad76cd7cdd7b8d79
base: main
reviewed_at: 2026-07-31T11:03:11Z
verdict: needs-discussion
refresh_log:
  - from: 3ba7641f7470e8e28746ebebad76cd7cdd7b8d79
    to: 3ba7641f7470e8e28746ebebad76cd7cdd7b8d79
    at: 2026-07-31T11:03:11Z
    summary: No code changes. petr-muller submitted an APPROVED review with /hold at 11:00:51Z ("found no problems, but I do not feel too confident about knowing this part of Sippy that well... hold for the case you want someone better with DBs than me to look as well"); PRB approval-notifier bot comment followed at 11:01:24Z.
---

## Summary

Replaces the periodic full-table refresh of `test_daily_totals`/`test_cumulative_summaries`
with per-batch incremental upserts during prow loading. Extracts DB-write logic from
`prow.go` into new `pkg/dataloader/prowloader/pgwriter` package. Parallelizes cross-release
carry-forward (errgroup, limit 4). Pins `currentDate` once at `ProwLoader` construction.
Adds 32 integration tests in `test/integration/pgwriter_test.go`. Removes
`dailysummary.Refresh`/`cumulativesummary.Refresh` calls from `RefreshData` (backfill code
path retained and still used by `sippy backfill`).

Since previous review: no code changes. petr-muller (reviewer) submitted an `APPROVED` review
with `/hold` at 2026-07-31T11:00:51Z, noting reduced confidence in this area of the DB layer and
requesting a second pair of eyes with stronger DB expertise before unholding. The blocking
finding below (batch-wide rollback on future-dated data) remains unaddressed.

Verified: `gofmt -l` clean on touched files; `go vet ./pkg/dataloader/prowloader/...` clean;
`golang.org/x/sync/errgroup` already a repo dependency (used elsewhere); moved SQL/Go bodies
(`insertJobRuns`, `insertAnnotations`, `upsertPullRequests`, `insertPRAssociations`,
`insertTestResults`) are byte-identical to what was deleted from `prow.go` — pure relocation,
no incidental behavior change.

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

## Open questions
- Is the batch-wide rollback on future-dated test data an accepted risk, or should this PR scope the rejection to just the offending job run(s) before merge?
- Is there existing/planned tooling to detect and clear a "stuck" poison-pill batch in production?
- Was the removed `cumulativeSummaryRefreshMetric` histogram intentionally dropped, or should an equivalent be added for the new incremental path?
