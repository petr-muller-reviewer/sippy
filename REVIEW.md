---
pr: openshift/sippy#3815
title: "TRT-2805: Optimize /api/tests/recent_failures query performance"
head_sha: fdbffcfddaff5550eb752be4eaaadbe660c1f252
base: main
reviewed_at: 2026-07-24T17:46:44Z
verdict: needs-discussion
---

## Summary

Rewrites `GetRecentTestFailures` to fix production timeouts (>90s -> ~600ms):
- Reverses join order: filters `prow_job_run_tests` by status/partition columns first, defers `tests`/`suites`/`test_ownerships` joins to after aggregation.
- Drops `prow_job_runs`/`prow_jobs` joins from the main aggregation query, relying on denormalized `prow_job_run_release`/`prow_job_run_timestamp` (these are the actual partition keys per `pkg/db/migrations/000001_create_partitioned_tables.up.sql`).
- Runs COUNT and Scan concurrently via `errgroup` + `gorm.Session`.
- Scopes `findLastPass`/`fetchOutputs` to the current page only; `findLastPass` walks backward one day at a time using `civil.Date` for partition alignment.
- Correlates on `(test_id, suite_id)` everywhere instead of `test_id` alone (fixes a pre-existing suite-conflation bug).

## Findings

### [should-fix] Inconsistent deleted_at handling across query paths
- where: `pkg/api/recent_test_failures.go:259-301` (buildRecentFailuresQuery), `pkg/api/recent_test_failures.go:313-364` (findLastPass), vs `pkg/api/recent_test_failures.go:376-436` (fetchOutputs)
- concern: The original query filtered `prow_job_runs.deleted_at IS NULL` and `prow_jobs.deleted_at IS NULL` in every query. The new main aggregation, its `NOT EXISTS` subquery, and `findLastPass` no longer join those tables and only check `prow_job_run_tests.deleted_at`. `fetchOutputs` still joins `prow_job_runs`/`prow_jobs` and checks both. If those tables are ever soft-deleted independently of `prow_job_run_tests`, failure counts and last-pass timestamps could include soft-deleted runs while outputs would exclude them — an inconsistency between code paths in the same feature.
- excerpt: |
    FROM prow_job_run_tests pjrt
    WHERE pjrt.status = ?
        AND pjrt.prow_job_run_release = ?
        AND pjrt.prow_job_run_timestamp >= ? AND pjrt.prow_job_run_timestamp < ?
        AND pjrt.deleted_at IS NULL
    -- no prow_job_runs/prow_jobs join or deleted_at check, unlike fetchOutputs

### [should-fix] findLastPass lookback window changed from period-derived to fixed 90 days
- where: `pkg/api/recent_test_failures.go:326-328`
- concern: Previously `lastPassLookback` was `periodStart` (or `periodStart - previousPeriod`), tied to the report's own window. Now it's hardcoded to 90 days back from `reportEnd` regardless of `period`/`previousPeriod`. For any report configuration where `period + previousPeriod` exceeds 90 days, this silently narrows the search and can return `nil` (no last pass found) where the old code would have found one. Should be confirmed intentional or made to respect the configured window.
- excerpt: |
    endDate := civil.DateOf(reportEnd.UTC())
    limitDate := endDate.AddDays(-90)

### [nit] findLastPass worst case is 90 sequential per-day queries
- where: `pkg/api/recent_test_failures.go:329-361`
- concern: For a test that hasn't passed within 90 days (new or permanently-failing test), the loop runs all 90 day-partition queries before giving up. Fine for the common case (~3 iterations per the PR's own benchmarks) but a page dominated by never-passing tests could reintroduce a latency cliff. Worth a spot-check against a known always-failing test.
- excerpt: |
    for date := endDate; remaining.Len() > 0 && !date.Before(limitDate); date = date.AddDays(-1) {

### [should-fix] No test coverage for new logic
- where: `pkg/api/recent_test_failures.go:248-436` (buildRecentFailuresQuery, testSuiteKey, findLastPass, testIDsFromKeys, fetchOutputs)
- concern: Substantial rewrite with zero unit tests. `testSuiteKey` construction and `testIDsFromKeys` are pure logic (no DB needed) and should have table-driven tests per repo convention. DB-dependent pieces fit the existing functional-test-behind-env-var pattern (see `releasesync_functional_test.go`).
- excerpt: |
    type testSuiteKey struct {
        testID  uint
        suiteID uint // 0 represents NULL suite_id
    }

### [question] Concurrent Count/Scan via gorm Session cloning is correct but relies on non-obvious internals
- where: `pkg/api/recent_test_failures.go:86-101`
- concern: `scanQuery`/`countQuery` both derive from `filteredQuery.Session(&gorm.Session{NewDB: false})` and run concurrently via errgroup. This avoids a data race only because `Limit`/`Offset` on `scanQuery` force gorm (v1.22.2) to clone its `Statement` before the goroutines are spawned; `countQuery`'s clone happens lazily inside its own goroutine, reading (not writing) the shared original `Statement`. Correct today, but fragile if the `pagination != nil` guard around Limit/Offset changes, or if gorm's clone-on-write semantics change. Suggest verifying with `go test -race` and/or a comment explaining why this is safe.
- excerpt: |
    scanQuery := filteredQuery.Session(&gorm.Session{NewDB: false})
    if pagination != nil {
        scanQuery = scanQuery.Limit(pagination.PerPage).Offset(pagination.Page * pagination.PerPage)
    }
    ...
    countQuery := filteredQuery.Session(&gorm.Session{NewDB: false})

### [nit] suiteID 0 as NULL sentinel is implicit
- where: `pkg/api/recent_test_failures.go:304-307`
- concern: `testSuiteKey{suiteID: 0}` represents `suite_id IS NULL`. Verified `Suite` uses `gorm.Model` (auto-increment starting at 1), so this is safe in practice, but the invariant isn't documented beyond a one-word comment.
- excerpt: |
    type testSuiteKey struct {
        testID  uint
        suiteID uint // 0 represents NULL suite_id
    }

## Checked
- SQL injection via `fmt.Sprintf` embedding `innerSQL` into `outerSQL` in `buildRecentFailuresQuery`: only static SQL text is interpolated, all dynamic values go through `?` placeholders — no injection risk.
- `civil.Date` passed directly as a query param (`findLastPass`): implements `driver.Valuer` (`Value() (driver.Value, error)` returning `d.String()`), confirmed in vendored `civil.go` — works correctly with the postgres driver.
- Denormalized `prow_job_run_release`/`prow_job_run_timestamp` columns used in place of joined `prow_jobs`/`prow_job_runs` filters: these are the actual partition keys per the partitioned-tables migration, so the simplification is sound, not a shortcut that risks incorrect matches.
- `RecentTestFailure.SuiteID` correctly threaded through `GetNumericalValue` for filter/sort parity with other fields.
- Suite ID auto-increment starts at 1 (`gorm.Model`), making the `suiteID: 0` NULL sentinel safe.

## Open questions
- Is dropping the `prow_job_runs`/`prow_jobs` `deleted_at` checks in the main aggregation/`findLastPass` intentional, given `fetchOutputs` still applies them?
- Should `findLastPass`'s 90-day lookback instead be derived from `period`/`previousPeriod` as before, to avoid narrowing results for longer report windows?
- Was `go test -race` run against the concurrent Count/Scan path?
