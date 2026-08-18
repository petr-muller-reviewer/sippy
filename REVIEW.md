---
pr: openshift/sippy#3907
title: "Trt 2709 partitioning phase2 query partitioning"
head_sha: f96c187ae48296b024af4503528ef867612ee99c
base: main
reviewed_at: 2026-08-18T15:05:10Z
verdict: request-changes
refresh_log:
  - old_sha: 63c97d78939add0e89432350b2fd1035b0cb6d32
    new_sha: f96c187ae48296b024af4503528ef867612ee99c
    summary: >-
      2 commits (a6f4b5a77 "handle missing records", f96c187ae "fix integration tests")
      responding to mstaeble's review comments about integration test failures. Parameterized
      ProwJobRunCount/HasBuildClusterData with an explicit `since time.Time`; CurrentActiveRelease
      now errors instead of returning ("", nil) when no release matches. Call sites still pass the
      same hardcoded 14-day window, so behavior is unchanged where it matters for open findings.
---

## Summary

Adds partition-pruning filters (`prow_job_release`, timestamp bounds) across
many queries so Postgres can skip partitions on large tables. Several of
these filters narrow the effective query scope compared to previous
behavior, and downstream consumers of the narrowed results were not
updated to match, or the narrowing has externally-visible correctness
implications.

Since previous review (63c97d78 -> f96c187a): mstaeble flagged integration
test failures in review comments; author pushed 2 commits to fix them
(a6f4b5a77, f96c187ae). Changes: `ProwJobRunCount` and `HasBuildClusterData`
now take an explicit `since time.Time` instead of hardcoding the 14-day
window inline (call sites still pass `time.Now().Add(-14*24*time.Hour)`,
so behavior is unchanged); `CurrentActiveRelease` now returns an error
instead of `("", nil)` when `release_definitions` has no match;
`LookupProwJobRunPartitionKeys` now returns `gorm.ErrRecordNotFound` on a
missing row instead of silently returning a zero-value struct. Integration
tests updated to match (`TestLookbackCount_NoReleasesReturnsError`,
`TestJobRunsReport_ReleaseFilter`, `TestProwJobRunCount`,
`TestHasBuildClusterData*`). PR is still open; CI (`/test e2e`) now passes.

## Findings

### [blocking] job_results last_pass window narrowed
- where: `pkg/db/functions.go:122-126`
- concern: the `lp` CTE computing `last_pass` (max last-passed timestamp) is now bounded to `prow_job_runs.timestamp BETWEEN p_start AND p_endstamp` (the reporting window), whereas previously it looked at the full history for the release. A job whose last pass predates the window now reports `last_pass = NULL` instead of the true last-pass timestamp. This is user-visible as the "Last pass" column in the Jobs UI.
- excerpt: |
    lp AS (
        SELECT prow_job_runs.prow_job_id, max(prow_job_runs.timestamp)::timestamp without time zone as last_pass
          ...
          AND prow_job_runs.timestamp BETWEEN p_start AND p_endstamp

### [blocking] ProwJobRunCount hardcoded 14-day window breaks fallback threshold
- where: `pkg/db/query/job_queries.go:69-78`, call site `pkg/api/job_runs.go:583`
- status: unresolved as of f96c187a. `ProwJobRunCount` was refactored to accept a `since time.Time` parameter (commit a6f4b5a77) instead of hardcoding the interval inline, but the only call site in `findReleaseMatchJobNames` still passes `time.Now().Add(-14*24*time.Hour)`, so the actual query window seen by the fallback threshold is unchanged. The refactor improves testability (see `TestProwJobRunCount`, now using fixed timestamps) but does not address this finding.
- concern: `ProwJobRunCount` (used in `pkg/api/job_runs.go` `findReleaseMatchJobNames`, summed into `totalJobRuns`) is still effectively scoped to the last 14 days, replacing what was previously an all-time count via `ProwJobRunIDs`. The summed total feeds `if totalJobRuns < 20` in `pkg/api/job_runs.go:583` which triggers a fallback to the prior release. A job with hundreds of historical runs but fewer than 20 in the last 14 days will still spuriously look data-starved and trigger the fallback, changing test-rate comparison behavior unrelated to partition pruning.
- excerpt: |
    func ProwJobRunCount(dbc *db.DB, prowJobID uint, release string, since time.Time) (int, error) {
    	var count int64
    	q := dbc.DB.Table("prow_job_runs").
    		Where("prow_job_id = ?", prowJobID).
    		Where("prow_job_release = ?", release).
    		Where("timestamp > ?", since)

    // pkg/api/job_runs.go:583
    runCount, err := query.ProwJobRunCount(dbc, job.ID, compareRelease, time.Now().Add(-14*24*time.Hour))
    ...
    if totalJobRuns < 20 {
    	// falls back to prior release

### [should-fix] cluster autocomplete silently filters to empty release
- where: `pkg/api/autocomplete.go:80-91`
- status: unresolved as of f96c187a, mechanism shifted. `query.CurrentActiveRelease` (`pkg/db/query/release_queries.go:36-41`) was changed (commit a6f4b5a77) to return an explicit error instead of `("", nil)` when no release matches. `autocomplete.go` was not updated to match: it still only logs the error via `log.WithError(err).Warn(...)` and falls through with `clusterRelease == ""`, so the outcome (silent empty-release filter, HTTP 200) is unchanged, just now reached via the error branch instead of the empty-no-error branch.
- concern: `query.CurrentActiveRelease(dbc)` errors (or previously, returned `("", nil)`) when no matching row exists in `release_definitions` (e.g. under-populated dev/seed DB). The code only logs a warning and continues. `clusterRelease` ends up `""` and gets applied as `Where("prow_job_release = ?", "")`, silently returning an empty list with HTTP 200 instead of surfacing that no release could be determined.
- excerpt: |
    clusterRelease, err = query.CurrentActiveRelease(dbc)
    if err != nil {
    	log.WithError(err).Warn("could not determine current development release for cluster autocomplete")
    }
    ...
    Where("prow_job_release = ?", clusterRelease).

### [should-fix] jobRunsCount and testIDsCount lose scope parity
- where: `pkg/api/tests.go:414-436`
- status: partially addressed as of f96c187a. Since `CurrentActiveRelease` now errors instead of returning `("", nil)` on no match, the specific no-release case is fixed: `GetJobRunTestsCountByLookbackAt` now propagates an error instead of silently returning a zero `jobRunsCount` alongside an all-release `testIDsCount` (see updated `TestLookbackCount_NoReleasesReturnsError`, previously `TestLookbackCount_NoReleasesReturnsZeroTests`). The core scope-parity concern below, for when a release IS found, is untouched.
- concern: `jobRunsCount` is now scoped to a single `CurrentActiveRelease`, but `testIDsCount` (computed afterward via `release_definitions` loop) still spans every release. These two values are exported together as related Prometheus gauges (`matViewUniqueNumberOfJobRuns` / `matViewUniqueNumberOfTests` in `pkg/sippyserver/server.go`). After this change they no longer represent comparable scopes, silently changing the semantics of the job-runs metric.
- excerpt: |
    release, err := query.CurrentActiveRelease(dbc)
    ...
    err = dbc.DB.Table("prow_job_runs").
    	Where("prow_job_release = ?", release).
    	Where("timestamp > ? AND deleted_at IS NULL", ...).
    	Count(&jobRunsCount).
    ...
    err = dbc.DB.Table("release_definitions").
    	Pluck("release", &releases)
    // testIDsCount later aggregated across all `releases`

## Checked
- Diffed full PR against upstream/main; traced call sites of every function whose query semantics changed.
- Verified the top findings against actual downstream consumers, not just the diff (job_runs.go fallback logic, sippyserver.go metric exports).
- General shape of partition-pruning filters (prow_job_release + timestamp bounds) is consistent with the stated goal (trt-2709) and matches the pattern used in prior commits on this branch.
- Re-checked all four findings against commits a6f4b5a77/f96c187ae (2026-08-18): none fully resolved. `ProwJobRunCount`/`HasBuildClusterData` gained a `since time.Time` param but call sites still hardcode 14 days; `CurrentActiveRelease` now errors on no-match, which fixes the no-release edge case in the tests.go finding but not the core scope-parity concern, and is silently swallowed (log+continue) at the autocomplete.go call site so that finding is also still open.
- `LookupProwJobRunPartitionKeys` (`pkg/db/query/job_queries.go:28-38`) now returns `gorm.ErrRecordNotFound` instead of a zero-value struct on no match (commit a6f4b5a77) — a correctness improvement, not covered by a prior finding, no issue found with it.

## Open questions
- For `job_results.last_pass`: was bounding it to the reporting window intentional (e.g. to match partition pruning), or should it use an unbounded/wider lookup and accept the extra partition scan?
- For `ProwJobRunCount`: now that the window is parameterized, should the `findReleaseMatchJobNames` call site pass a wider (or all-time) `since` rather than 14 days, given the result feeds a `< 20` threshold that assumed all-time counts?
- For cluster autocomplete: should the now-explicit error from `CurrentActiveRelease` be propagated as an error response instead of logged-and-ignored?
- For `tests.go`: was narrowing `jobRunsCount` to the current release intentional, and if so should `testIDsCount` be narrowed to match, or should the metric names/docs be updated to reflect the new asymmetric scope?
