---
pr: openshift/sippy#3876
title: "TRT-2821: Optimize GetJobRunTestsCountByLookback using cumulative summaries"
head_sha: ae890fbbaf5f72c98471a91400dfafcbd8c567f5
base: main
reviewed_at: 2026-08-08T12:04:34Z
verdict: request-changes
---

## Summary

Replaces a single `COUNT(DISTINCT)` scan over `prow_job_run_tests` (~70M rows) with: (1) a direct `COUNT` against `prow_job_runs` for job-run counts, and (2) per-release, concurrent (errgroup limit 4) self-joins against `test_cumulative_summaries` for distinct test-ID counts, merged via `sets.New[int64]()`. Refactored into a testable `GetJobRunTestsCountByLookbackAt(dbc, lookbackDays, today civil.Date)` with `GetJobRunTestsCountByLookback` as a thin wrapper. New integration test suite added at `test/integration/lookback_count_test.go`.

Verified by hand: the job-run window (`> today-lookbackDays` midnight) and the test-ID window (`(today-lookbackDays-1, today]`) both resolve to the same `lookbackDays+1`-day span, matching the codebase's established `P(End-1) - P(Start-1)` prefix-sum convention (`pkg/db/query/cumulative_query.go`). No off-by-one between the two returned counts.

## Findings

### [should-fix] goroutine leak when an errgroup release query fails
- where: `pkg/api/tests.go:419-465`
- concern: `ch` is only closed in the success path (line 464). If `g.Wait()` returns an error (line 461-463), the function returns immediately without closing `ch`. The background collector goroutine (lines 421-427, `for ids := range ch`) then blocks forever since the channel is never closed — a goroutine leak on every transient failure (partition hiccup, connection blip). This function runs on every `RefreshData` cycle, so leaks can accumulate in a long-running server. Buffered channel size equals `len(releases)` so non-failing goroutines' sends never block, meaning the leak is real (not masked by a deadlock).
- excerpt: |
    if err := g.Wait(); err != nil {
        return -1, -1, err
    }
    close(ch)
    <-done
- fix: close (and drain) before checking the error:
    ```go
    err := g.Wait()
    close(ch)
    <-done
    if err != nil {
        return -1, -1, err
    }
    ```

### [should-fix] channel/collector-goroutine fan-in is unnecessary indirection
- where: `pkg/api/tests.go:419-427,457,464-465`
- concern: the channel plus separate goroutine exists only to merge per-release `[]int64` results into a `sets.Set[int64]`. A `sync.Mutex` guarding `testIDs.Insert(ids...)` called directly inside each `g.Go` closure gives the same result with less code, no channel lifecycle to manage, and removes the leak above entirely (nothing to forget to close). Also more consistent with the simpler errgroup fan-out pattern already used in `pkg/api/job_runs.go`.
- excerpt: |
    ch := make(chan []int64, len(releases))
    testIDs := sets.New[int64]()
    done := make(chan struct{})
    go func() {
        for ids := range ch {
            testIDs.Insert(ids...)
        }
        close(done)
    }()
    ...
    g.Go(func() error {
        var releaseTestIDs []int64
        queryErr := dbc.DB.Raw(...).Scan(&releaseTestIDs).Error
        if queryErr != nil {
            return fmt.Errorf(...)
        }
        ch <- releaseTestIDs
        return nil
    })

### [should-fix] no defensive clamp to MAX(date) before querying date = today
- where: `pkg/api/tests.go:434-451`
- concern: every other caller that self-joins `test_cumulative_summaries` (`pkg/db/query/cumulative_query.go`'s `resolvePrefixSumDates`, `pkg/db/query/feature_gates.go:18-24`) first clamps via `ResolveDateRanges` to the latest available date for the release, because "today"'s cumulative row only exists once `cumulativesummary.Refresh` completes for the day (`pkg/db/cumulativesummary/cumulative_summary.go`). This PR queries `date = ?` with the literal `today` civil.Date and no such clamp. It's currently safe only because the sole call site (`refreshMaterializedViews`, `pkg/sippyserver/server.go:411`) runs strictly after `cumulativesummary.Refresh` inside `RefreshData` (`server.go:399-411`). That ordering is an implicit invariant, not an enforced one — any future caller (manual trigger, new endpoint) that queries before the daily refresh completes will silently undercount (empty `end_sums` for the day) rather than error.
- excerpt: |
    WITH end_sums AS (
      SELECT test_id, SUM(prefix_sum_runs) AS total_runs
      FROM test_cumulative_summaries
      WHERE release = ? AND date = ?
      GROUP BY test_id
    ),

### [nit] confusing variable name `startMinusOne`
- where: `pkg/api/tests.go:391`
- concern: it's not "start minus one" but "the day before the window's exclusive start boundary" for the prefix-sum delta. A name like `windowStartExclusive` would read more clearly against the `P(End) - P(Start)` convention used elsewhere in the codebase.
- excerpt: |
    startMinusOne := today.AddDays(-lookbackDays - 1)

### [question] parity test between the two count windows
- where: `test/integration/lookback_count_test.go`
- concern: the new tests cover cross-release dedup, zero/negative delta exclusion, missing prior-day rows, soft-deleted runs, and multi-job aggregation individually, but nothing directly asserts that `jobRunsCount`'s window and `testIDsCount`'s window cover the same span. This is easy to regress silently since the two counts come from separate queries with separately-computed boundaries.
- excerpt: |
    jobRunsCount, testIDsCount, err := api.GetJobRunTestsCountByLookbackAt(dbc, 14, lookbackDate)

## Checked
- Date-boundary math for both counts hand-verified against `P(End-1) - P(Start-1)` convention in `cumulative_query.go` — consistent, no off-by-one between job-run and test-ID windows.
- `prow_job_runs` soft-delete: `.Table()` bypasses GORM's automatic scope, so the explicit `deleted_at IS NULL` clause is required and correct; covered by `TestLookbackCount_JobRunsCountExcludesDeleted`.
- SQL injection: `release` values are parameterized bind params sourced from `release_definitions`, not user input.
- Loop variable capture in `g.Go(func() error { ... release ... })` — safe under Go 1.25 (per-iteration loop var semantics), confirmed via `go.mod`.
- Structured logging (`log.WithField(s)`) used instead of `Infof` string formatting, per project convention.
- `sets.New[int64]()` used for dedup per project convention (no hand-rolled `map[string]bool`).
- No BigQuery counterpart exists for `GetJobRunTestsCountByLookback`, so the CLAUDE.md provider-parity rule does not apply here.
- gofmt/style of new code visually consistent with the rest of the file (could not run `gofmt`/`go vet` directly — toolchain unavailable in this sandbox).

## Open questions
- Is there a reason `GetJobRunTestsCountByLookbackAt` doesn't clamp to `MAX(date)` like `ResolveDateRanges` does elsewhere, or is `refreshMaterializedViews`'s post-cumulative-refresh call order considered a sufficient guarantee going forward?
- Was the goroutine-leak-on-error path noticed during benchmarking, or did all iterations succeed so it never triggered?
- Any reason to prefer the channel-based fan-in over a mutex-guarded set insert, given the latter is both simpler and avoids the leak?
