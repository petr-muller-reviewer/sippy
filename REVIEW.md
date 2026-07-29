---
pr: openshift/sippy#3835
title: "TRT-2835: Add integration tests for job queries and build cluster queries"
head_sha: 35eff6e74af1482e009d4cf13c71193dc131a006
base: main
reviewed_at: 2026-07-28T23:55:19Z
verdict: approve
---

## Summary

PR 2 of 7 in TRT-2835 series, building on scaffolding from #3829. Adds 21 integration
tests for `pkg/db/query/job_queries.go` and `pkg/db/query/build_clusters.go`, plus a
shared `test/integration/util/fixtures.go` helper file. Also ships two real bug fixes
found while writing the tests, and one dead-code removal.

Changes:
- `LoadBugsForJobs`: fixed AND/OR operator-precedence bug in the bug recency filter.
- `ProwJobHistoricalTestCounts`: replaced Go-side `gorm.ErrRecordNotFound` handling with
  `COALESCE(avg(count), 0)` in SQL.
- `BuildClusterAnalysis`: removed dead `percentages` CTE (unreferenced since 2022).
- New tests: `test/integration/build_clusters_test.go`, additions to
  `test/integration/jobs_test.go`, new `test/integration/util/fixtures.go`.

## Findings

### [nit] duplicated struct-building logic between fixture constructors
- where: `test/integration/util/fixtures.go:15-46`
- concern: `CreateProwJob` and `CreateProwJobWithOptions` build the same `models.ProwJob{Name, Release, Variants}` struct. `CreateProwJob` could just call `CreateProwJobWithOptions(t, dbc, name, release, variants)` with no options to remove the duplication.
- excerpt: |
    func CreateProwJob(t *testing.T, dbc *db.DB, name, release string, variants []string) models.ProwJob {
        t.Helper()
        job := models.ProwJob{
            Name:     name,
            Release:  release,
            Variants: pq.StringArray(variants),
        }
        require.NoError(t, dbc.DB.Create(&job).Error, "creating ProwJob %q", name)
        return job
    }

### [nit] unused fixture helper
- where: `test/integration/util/fixtures.go:62-67`
- concern: `CreateSuite` is added but not called anywhere in this PR's tests. Likely scaffolding for a later PR in the 7-part series; flagging in case it was accidental.
- excerpt: |
    func CreateSuite(t *testing.T, dbc *db.DB, name string) models.Suite {

## Checked

- Verified worktree is at PR head (`git rev-parse HEAD` == `headRefOid` from `gh pr view`).
- `LoadBugsForJobs` precedence bug: confirmed old code produced `A OR B and C and D`, parsed as `A OR (B and C and D)` due to AND binding tighter than OR — closed/verified bugs modified recently could leak past the status filter. Fix wraps `timeLimit` in parens; `TestLoadBugsForJobsExcludesStaleClosedBugs` and `TestLoadBugsForJobsFiltersVerified` cover it.
- `ProwJobHistoricalTestCounts` COALESCE change: aggregate query without GROUP BY always returns exactly one row (NULL on empty input), so `q.First()` now always succeeds — correctly replaces the old `ErrRecordNotFound` branch. `TestProwJobHistoricalTestCountsNoData` covers it.
- `BuildClusterAnalysis` dead CTE removal: confirmed final `SELECT` never referenced `percentages`; `count(*)` under `GROUP BY` can't be zero, so no new division-by-zero risk.
- No BigQuery equivalents exist for either modified query function — provider-parity rule from project CLAUDE.md doesn't apply.
- Ran `make integration` locally (58/58 pass; required starting `podman.socket` and `TESTCONTAINERS_RYUK_DISABLED=true` in this environment, unrelated to the PR).
- `go vet ./pkg/db/query/... ./test/integration/...` clean.
- `gofmt -l` on all changed files: clean.
- `TestLoadBugsForJobsMultipleJobIDsReturnsFirstOnly` and `TestLoadBugsForJobsEmptyJobIDs` intentionally document pre-existing quirky behavior (First() on multi-ID query, empty-slice error) rather than hiding it — matches the TODO already in source.
- `TestBuildClusterHealthZeroRunsInPreviousPeriod` comment about `NULLIF(0,0)` scanning to `0.0` for a non-pointer `float64` field verified against `models.BuildClusterHealthReport` struct definition (plain `float64`, not pointer) — test behavior is accurate.

## Open questions

- None blocking. Could ask if `CreateSuite` is intentionally unused scaffolding for a later PR in the series.
