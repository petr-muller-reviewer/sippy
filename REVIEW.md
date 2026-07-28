---
pr: openshift/sippy#3798
title: "TRT-2811: Make prow loader batch writes resilient to single-batch failures"
head_sha: d31a9aca5b9efb612aa44f804f8ca53c345a8cc4
base: main
reviewed_at: 2026-07-27T11:34:05Z
verdict: approve
refresh_log:
  - from: a101186b17dcbcbbb5b6001dc97018064b8b5136
    to: d31a9aca5b9efb612aa44f804f8ca53c345a8cc4
    summary: >-
      Author extracted a batchWriterFunc seam and added accumulate_test.go
      (221 lines) covering all-succeed, first/trailing/all-batch failure,
      per-batch pl.errors, and cancellation-with-pending-batch cases.
      CodeRabbit independently raised the same two findings from the prior
      review (test coverage, and flushing after ctx cancellation); the
      coverage finding is now resolved, and the cancellation-flush finding
      was withdrawn after the author explained the behavior is intentional
      (avoids silently dropping accounted-for results). Dropped [WIP] from
      title; PR self-approved and CI green.
---

## Summary

Changes `accumulateAndWriteJobRuns` (pkg/dataloader/prowloader/prow.go) from abort-on-first-failed-batch to log-and-continue. Each failed batch write is appended to `pl.errors` individually (was one wrapped error for the whole call), giving `LoaderWithMetrics`'s per-loader error histogram finer-grained counts. Drops the explicit `cancelFetch()` on write failure since fetch and `loadDailyTestAnalysisByJob` (BigQuery) don't depend on PG writes succeeding. Adds a `ctx.Err()` check inside the accumulation loop.

Since previous review: added `batchWriterFunc` type + `ProwLoader.batchWriter` field (wired to `pl.writeJobRunBatch` in `New()`), and `pkg/dataloader/prowloader/accumulate_test.go` with 5+ table-driven cases plus two dedicated tests for per-batch error recording and cancellation accounting. No production logic changed beyond the indirection through `pl.batchWriter`.

## Findings

### [nit] `flush`'s `msg` param name doesn't convey it's failure-only
- where: `pkg/dataloader/prowloader/prow.go:1214-1223`
- concern: `msg` is only used inside the `if err != nil` branch. Naming it e.g. `failureMsg` would make that clear without reading the closure body. Still unaddressed as of `d31a9ac`.
- excerpt: |
    flush := func(msg string) {
        if err := pl.batchWriter(ctx, batch); err != nil {
            log.WithError(err).WithField("batchSize", len(batch)).Warning(msg)

## Resolved

### [should-fix, resolved] final flush still runs after breaking on ctx cancellation
- where: `pkg/dataloader/prowloader/prow.go:1225-1236`
- resolution: Confirmed intentional by the author (PR review comment, 2026-07-26T14:03:10Z): skipping the flush on cancellation would silently drop the already-accumulated batch from both `total` and `failed` accounting. By attempting the write anyway, a canceled-context failure is recorded as a failed batch with an error in `pl.errors`, matching the "no silent data loss" design goal. CodeRabbit raised the identical concern independently and withdrew it after the same explanation. `TestAccumulateAndWriteJobRuns_CancelledContextDoesNotSilentlyDrop` in `accumulate_test.go` now codifies this behavior.

### [nit, resolved] no test coverage for new partial-failure/counting logic
- where: `pkg/dataloader/prowloader/prow.go:1206-1247`
- resolution: Addressed in commit `d31a9aca5`. Added `type batchWriterFunc func(ctx context.Context, batch []jobRunResult) error` and a `batchWriter` field on `ProwLoader` (defaulting to `pl.writeJobRunBatch` in `New()`), exactly the function-type-field seam pattern suggested. `accumulate_test.go` covers: all-succeed, first-batch-fails, all-fail, trailing-batch-fails, and cancellation-with-pending-batch, plus dedicated tests for per-batch `pl.errors` entries and cancellation accounting.

## Checked
- `fmt` and `github.com/pkg/errors` imports remain used elsewhere in the file after switching per-batch errors to `fmt.Errorf` — no import churn needed.
- `results` channel is buffered to `len(entries)` (line 274), so fetch-worker sends can never block even if the consumer loop exits early on cancellation — no goroutine leak from the new early-break path.
- `fetchCtx` is derived from `pl.ctx` via `context.WithCancel`, so removing the explicit `cancelFetch()` call on write failure is safe: cancellation of `pl.ctx` still propagates to `fetchCtx` and stops fetch workers through their own `ctx.Err()` checks.
- `errorMetric.Observe(len(loader.Errors()))` in `loaderwithmetrics.go:86` confirms per-batch error entries do produce more granular Prometheus counts, matching the PR description's claim.
- `prowLoaderProcessedMetricGauge` semantics unchanged: still counts only successfully-written runs (`total`), consistent with pre-existing behavior.
- No cross-provider (BigQuery/Postgres) parity concern — this only touches the Postgres write path.
- New `batchWriter` indirection (`pl.batchWriter(ctx, batch)` replacing the direct `pl.writeJobRunBatch(ctx, batch)` call) is wired correctly: `New()` sets `pl.batchWriter = pl.writeJobRunBatch`, so production behavior is unchanged; only test construction bypasses `New()` to inject a stub.
- `accumulate_test.go` test cases are table-driven with descriptive names, matching project convention; `countFailedRuns` helper (`len(errs) * 100`) in the cancellation test is a rough approximation, not an exact accounting, but the test only asserts `> 0` so this is fine for its purpose.

## Open questions
- None outstanding — the WIP label has been dropped, the PR is self-approved with CI green, and both open items from the prior review are resolved.
