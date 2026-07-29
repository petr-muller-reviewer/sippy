---
pr: openshift/sippy#3838
title: "TRT-2814: Eliminate payload_test_failures_14d_matview"
head_sha: 76fe2aef7e2491491e83d0276c3a2661a4d67ab5
base: main
reviewed_at: 2026-07-28T22:41:36Z
verdict: approve
---

## Summary

Replaces `payload_test_failures_14d_matview` with a direct partition-pruned SQL
query (`query.GetTestFailuresForPayloadStream`) that pre-aggregates one row per
test via `array_agg`, instead of one row per (test, job, payload). Fixes two
non-strict-weak-order `sort.Slice` comparators, corrects a blocker-score reason
string, rejects sort params on `/api/releases/test_failures` (400), and updates
`scripts/rejected-payloads.py` to match. Adds 36 integration tests.

## Findings

### [should-fix] confirm matview drop on deploy
- where: `pkg/db/views.go:20-30` (removed entry from `PostgresMatViews`)
- concern: Removing the `payload_test_failures_14d_matview` entry from the `PostgresMatViews` slice stops future refreshes/creation, but it's not evident from the diff whether the migration/db-init layer issues a `DROP MATERIALIZED VIEW` for matviews removed from this list, or whether the object is simply left behind (harmless but orphaned) in production Postgres.
- excerpt: |
    -	{
    -		Name:           "payload_test_failures_14d_matview",
    -		Definition:     payloadTestFailuresMatView,
    -		IndexColumns:   []string{"release", "architecture", "stream", "prow_job_run_id", "test_id", "suite_id"},
    -		ReplaceStrings: map[string]string{},
    -	},

### [should-fix] release_time boundary/clock semantics changed
- where: `pkg/db/query/payload_queries.go:52-84`
- concern: Old matview used `rt.release_time > (NOW() - 14 days)` (strict `>`, wall-clock). New query uses `reportEnd.Add(-14 * 24h)` with `>=`. This is a minor boundary widening and a switch from `NOW()` to the caller-supplied `reportEnd` — likely intentional/more testable but not called out explicitly in the PR description.
- excerpt: |
    fourteenDaysAgo := reportEnd.Add(-14 * 24 * time.Hour)
    ...
    WHERE rt.release = ?
      AND rt.architecture = ?
      AND rt.stream = ?
      AND rt.release_time >= ?

### [nit] no test for single-payload sort tiebreaker
- where: `pkg/api/releases.go:128-133` (`GetPayloadTestFailures`)
- concern: The stream path's new tiebreaker sort (`BlockerScore` -> `FailureCount` -> `Name`) is covered by `TestGetPayloadStreamTestFailures_SortTiebreakers`, but the analogous fix in the single-payload path (`FailureCount` -> `Name`) has no direct test. Same pattern, low risk, but worth a quick case for completeness.
- excerpt: |
    sort.Slice(testFailures, func(i, j int) bool {
    	if testFailures[i].FailureCount != testFailures[j].FailureCount {
    		return testFailures[i].FailureCount > testFailures[j].FailureCount
    	}
    	return testFailures[i].Name < testFailures[j].Name
    })

### [nit] no test for new 400 branch on sort rejection
- where: `pkg/sippyserver/server.go:626-635`
- concern: The new `filterOpts.SortField != ""` -> 400 check in `jsonGetPayloadAnalysis` has no test coverage. Small, easily-eyeballed handler change, so this is a minor gap rather than a blocker.
- excerpt: |
    if filterOpts.SortField != "" {
    	failureResponse(w, http.StatusBadRequest, "sorting is not supported for this endpoint")
    	return
    }

### [question] rejected-payloads.py raw query untested
- where: `scripts/rejected-payloads.py:27-53`
- concern: The ORM-mapped matview access was replaced with a raw parameterized `text()` query. Parameters are bound correctly (no injection risk) and logic mirrors the old view's WHERE clause, but this script has no automated coverage (consistent with its status as an ops utility outside `make test`/`make integration`). Was this manually run against a real DB to confirm `Row` attribute access (`test_failure.prow_job_name`, `.name`) works as expected with the installed SQLAlchemy version?

## Checked

- Partition pruning: `rt.release = ?` + `pjrt.prow_job_run_release = ?` using the same literal is logically equivalent to the old column-to-column join (`pjrt.prow_job_run_release = rt.release`), since both are pinned to the same value — correct and now prunable.
- Dropping `SELECT DISTINCT` from the old matview is safe: `release_job_runs` has a unique index on `prow_job_run_id` (`pkg/db/models/releases.go`) enforced via `ON CONFLICT (prow_job_run_id) DO UPDATE` in the loader (`pkg/dataloader/releaseloader/releasesync.go`), so the duplicate-row risk the old `TODO` comment warned about cannot occur today.
- `sort.Slice` comparators in both `GetPayloadStreamTestFailures` and `GetPayloadTestFailures` were previously non-strict-weak-order (`>=`-based) — undefined behavior per Go's `sort` docs for equal elements. New tiebreaker chains fix this.
- `array_agg(... ORDER BY pj.name, pjr.id)` produces deterministic parallel arrays; per-release-tag sub-ordering by job name is preserved since the global sort key is a superset ordering. Confirmed by `TestGetPayloadStreamTestFailures_MultipleJobsPerPayload`.
- `filter.FilterableDBResult` only calls `.Order()` when `SortField` is non-empty, so passing `""` as the default sort field to `FilterOptionsFromRequest` in `jsonGetPayloadAnalysis` does not leak an unintended default ordering.
- `gofmt -l` and `go vet` are clean on all touched Go files.
- Integration test coverage (36 tests) is thorough: 14-day window boundaries, stream/arch filtering, multi-payload aggregation, multi-job-per-payload arrays, blocker score + streak-override math, sort tiebreakers, `openshift-tests` exclusion, and force-accepted-payload-with-failed-blocking-job edge case.

## Open questions

- Does removing an entry from `PostgresMatViews` drop the existing materialized view object on deploy, or is cleanup needed as a follow-up migration?
- Was the `release_time` boundary change (`>` → `>=`, `NOW()` → `reportEnd`) intentional, or just a side effect of reusing `reportEnd`?
- Was `scripts/rejected-payloads.py` manually exercised against a real database after the raw-query rewrite?
