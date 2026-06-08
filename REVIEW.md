---
pr: openshift/sippy#3542
title: "Trt 1989 migration queries"
head_sha: 912c2faec06c5a056ab3520c1cbe7ba9ae8e44a9
base: main
reviewed_at: 2026-06-03T11:29:02Z
verdict: needs-discussion
refresh_log:
  - previous_sha: 0cf023a1d146521758e3cee0444233da4c0a90cd
    new_sha: 35565d2a34587c629ea0c366c8e71d5da75fff8a
    summary: "8 new commits: join table partitioning filters added to job_results() CTEs and PR/repo queries, annotation query gets release filter, GatherLabelsFromBQ refactored to batch, benchmark suite expanded with 8 new cases and file output support, created_at filtering removed in further places."
  - previous_sha: 35565d2a34587c629ea0c366c8e71d5da75fff8a
    new_sha: 6e7dde01a7da7ad9156698526f0819405e871d31
    summary: "6 new commits, net +6/-8 in pkg/flags/postgres_benchmarking_test.go only: benchmark file path now uses filepath.Join+filepath.Clean (resolves nit). openshift-merge-bot triggered e2e tests. No human reviews."
  - previous_sha: 6e7dde01a7da7ad9156698526f0819405e871d31
    new_sha: 0e1bda0e6ed5a7a6b2b18cf49f1e7d99d8b029ab
    summary: "1 new commit (+104/-10): TestOutputs refactored to start from prow_job_run_tests, join to outputs uses timestamp equality for partition pruning, status filter added (failure/flake only), JobDetailsReport preload gains partitioning filters. Author posted comparison test results: 16=16 exact match, old query took 4h41m, new takes 247ms."
  - previous_sha: 0e1bda0e6ed5a7a6b2b18cf49f1e7d99d8b029ab
    new_sha: 912c2faec06c5a056ab3520c1cbe7ba9ae8e44a9
    summary: "PR rebased onto main (all prior SHAs changed). 1 genuinely new commit (912c2faec, Jun 2): force_custom_plan connection parameter via pgx stdlib adapter prevents 17+ min planning on 10k+ partitions; ProwJobHistoricalTestCounts gains release param; test_analysis_by_job_by_dates gets denormalized release filter. CI: yaml-lint and images build failing at PR head."
  - previous_sha: 912c2faec06c5a056ab3520c1cbe7ba9ae8e44a9
    new_sha: 912c2faec06c5a056ab3520c1cbe7ba9ae8e44a9
    summary: "No code changes. Activity: neisw retested images (/test images), requested CodeRabbit re-review. CodeRabbit CHANGES_REQUESTED: (1) historicalTestCount cache keyed only by job ID but now returns release-scoped counts — stale cache on multi-release jobs; (2) pgx/v4 at v4.18.2, v4.18.3 is latest, v4 EOL Jul 2025. openshift-merge-bot triggered e2e."
---

Since previous review (2026-05-26): 8 new commits. Key changes:
- Join table (`prow_job_run_prow_pull_requests`) now gets partitioning filters in `job_results()` CTEs, `PullRequestReport`, and `RepositoryReport`.
- `GatherLabelsFromBQ` refactored from single-build-ID to batch query (returns `map[string]pq.StringArray`).
- Annotation query in `JobsRunsReportFromDB` gains conditional release filter.
- Benchmark suite expanded with 8 new cases and optional file output.

Since previous review (2026-05-30): 1 new commit, +104/-10 lines.
- `TestOutputs` refactored: query now starts from `prow_job_run_tests` (not `prow_job_run_test_outputs`); join to outputs uses `prow_job_run_test_timestamp` equality for partition pruning; `status IN (failure, flake)` filter added (was missing entirely before); output column and ordering qualified explicitly.
- `JobDetailsReport` preload gains `prow_job_run_release` and `prow_job_run_timestamp` conditions.
- `Test_CompareTestOutputsQueries` benchmark added; author ran it and posted results: 16=16 exact match, old query 4h41m, new query 247ms.

Since previous review (2026-06-01): PR rebased onto main. 1 genuinely new commit (912c2faec, Jun 2).
- `pkg/db/db.go`: Switches from `postgres.Open(dsn)` to `pgx.ParseConfig` + `stdlib.OpenDB` with `plan_cache_mode = "force_custom_plan"` set as a runtime parameter. Prevents 17+ minute plan generation on tables with 10k+ partitions (e.g. `test_analysis_by_job_by_dates`). This is a global connection-level change affecting all queries.
- `pkg/db/query/job_queries.go`: `ProwJobHistoricalTestCounts` gains `release string` param; adds `AND prow_job_run_tests.prow_job_run_release = ?` filter.
- `pkg/api/test_analysis.go`: Adds `Where("test_analysis_by_job_by_dates.release = ?", release)` to `GetTestAnalysisByJobFromDB`.
- CI: `ci/prow/yaml-lint` and `ci/prow/images` both failing at current PR head — openshift-ci[bot] reported this on 2026-06-02.

Since previous review (2026-06-02): No code changes.
- neisw retested CI: `/test images` (2026-06-03T00:38).
- neisw requested CodeRabbit re-review; CodeRabbit posted CHANGES_REQUESTED (2026-06-03T00:49) with two actionable findings (see new findings below). openshift-merge-bot triggered e2e tests.

## Findings

### [should-fix] Dropped-join queries break before backfill
- where: `pkg/db/functions.go:53`, `pkg/db/views.go:233`, `pkg/db/views.go:265`, `pkg/db/views.go:291`, `pkg/db/query/job_queries.go:76-80`, `pkg/db/query/test_queries.go:320-333`
- concern: Queries that dropped joins to `prow_job_runs`/`prow_jobs` now filter solely on denormalized columns (`prow_job_run_timestamp`, `prow_job_run_release`). If deployed before backfill, historical rows with NULL/empty/zero denormalized columns will be silently excluded. Other queries in this PR (e.g. `GetRecentTestFailures`, CR queries) use the safer redundant-filter strategy. The mixed approach means some pages break pre-backfill and others don't.
- excerpt: |
    -- test_results() function
    WHERE prow_job_run_tests.prow_job_run_timestamp BETWEEN $1 AND $3
    GROUP BY tests.id, prow_job_run_tests.prow_job_run_release

### [nit] TestOutputs: `prow_jobs.release` redundant filter still absent after refactor
- where: `pkg/db/query/test_queries.go:280-306`
- concern: The query was substantially refactored (now starts from `prow_job_run_tests`, joins outputs with timestamp equality, adds status filter). Release scoping comes solely from `prow_job_run_tests.prow_job_run_release`. The join to `prow_jobs` is still present for variants, so a redundant `prow_jobs.release = ?` filter would have been free. Author posted comparison test showing 16=16 exact result match against the original query, which validates correctness for populated data. The structural redundancy gap remains for pre-backfill scenarios but the test evidence lowers the practical risk.
- note: Downgraded from should-fix to nit given the author's comparison test evidence.

### [nit] TestOutputs: new status filter is a behavior change
- where: `pkg/db/query/test_queries.go:286`
- concern: `Where("prow_job_run_tests.status IN ?", []int{failure, flake})` was not present in any previous version of this query. The old query returned outputs for all test statuses (including success). The comparison test matched 16=16, but that may be because the test name chosen only has failure outputs — it doesn't prove zero regressions for tests that had success-status rows with outputs. Callers expecting outputs for passing tests would now get none. Worth confirming this is intentional.
- excerpt: |
    Where("prow_job_run_tests.status IN ?", []int{int(v1.TestStatusFailure), int(v1.TestStatusFlake)})

### [should-fix] `historicalTestCount` cache key bug — release-scoped counts keyed only by job ID
- where: `pkg/sippyserver/pr_new_tests_worker.go:235-244`
- concern: `pgJobRunFilter.historicalTestCount` is `map[uint]int` — keyed by `run.ProwJob.ID`. But `ProwJobHistoricalTestCounts` now returns release-scoped counts (added `prow_job_run_release` filter in `912c2faec`). If the same job ID runs against multiple releases, the first cached result is returned for all subsequent releases. This would undercount or overcount historical test totals for multi-release jobs, corrupting the PR new-tests detection logic. Fix: composite key `fmt.Sprintf("%d:%s", run.ProwJob.ID, run.ProwJob.Release)` (spotted by CodeRabbit, 2026-06-03).
- excerpt: |
    // Cache keyed only by job ID — stale if same job appears in multiple releases:
    historicalCount, ok := jrf.historicalTestCount[run.ProwJob.ID]
    if !ok {
        historicalCount, err = query.ProwJobHistoricalTestCounts(jrf.dbc, run.ProwJobID, run.ProwJob.Release)
        jrf.historicalTestCount[run.ProwJob.ID] = historicalCount  // should be keyed by "id:release"
    }

### [should-fix] No backfill for existing `prow_job_run_prow_pull_requests` rows — now actively queried
- where: `pkg/db/models/prow.go:74-79`, `pkg/db/functions.go:87-90,100-103`, `pkg/db/query/pull_request_queries.go:31`, `pkg/db/query/repository_queries.go:29`
- concern: The new commits add `prow_job_run_prow_pull_requests.prow_job_run_release` filters to `job_results()` CTEs, `PullRequestReport`, and `RepositoryReport`. This makes the missing backfill more urgent: existing join table rows have empty release and zero timestamp, so these queries will now exclude all historical PR associations. Previously this was a latent concern; now it actively affects query results.

### [question] `force_custom_plan` is global — performance trade-off for non-partitioned queries
- where: `pkg/db/db.go:56-74`
- concern: The new pgx adapter sets `plan_cache_mode = "force_custom_plan"` as a runtime parameter on all connections. This prevents PostgreSQL from caching generic plans for any prepared statement — not just the partitioned tables that motivated this change. Queries on small, non-partitioned tables lose plan reuse. For a system that runs many short, repeated queries (cache lookups, variant registry queries, etc.) this could add measurable per-query planning overhead. Was the global scope intentional, or was a table-level or query-level approach considered? If global is deliberate, a comment on the trade-off would help future maintainers.
- excerpt: |
    pgxConfig.RuntimeParams["plan_cache_mode"] = "force_custom_plan"

### [nit] CI failing at current PR head
- where: PR CI at `912c2faec`
- concern: `openshift-ci[bot]` reported (2026-06-02) `ci/prow/yaml-lint` and `ci/prow/images` failing. neisw ran `/test images` on 2026-06-03; result not yet confirmed. openshift-merge-bot triggered e2e tests. PR should not merge until `images` passes.

### [question] BuildClusterHealth named param mixing
- where: `pkg/db/query/build_clusters.go:30-35`
- concern: `Select()` passes `@start`, `@boundary`, `@end` via `sql.Named`. The new `Where()` passes `@start`, `@end` again separately. GORM should merge named params across the chain, but this pattern is unusual. Has this been tested against the actual DB?
- excerpt: |
    Select(`...`, sql.Named("start", start), sql.Named("boundary", boundary), sql.Named("end", end)).
    ...
    Where("prow_job_runs.timestamp BETWEEN @start AND @end", sql.Named("start", start), sql.Named("end", end)).

### [question] GatherLabelsFromBQ date filter relaxed
- where: `pkg/dataloader/prowloader/prow.go:1133-1134`
- concern: The batch refactor changed `DATE(prowjob_start) = DATE(@StartTime)` (exact date match) to `DATE(prowjob_start) >= DATE(@ReleaseTime)` (open-ended range). For the single-build-ID caller this is fine (one job, one date), but the batch form could scan a much wider partition range in BigQuery. If `buildIDs` span multiple days, this scans from the earliest `startTime` forward with no upper bound. The parameter was also renamed from `StartTime` to `ReleaseTime` which is confusing — the caller passes `pj.Status.StartTime`, not a release time.

### [nit] `created_at` to `prow_job_run_timestamp` is a semantic change
- where: `pkg/api/tests.go:399`
- concern: `created_at` (DB insertion time) replaced with `prow_job_run_timestamp` (job execution time). Probably correct, but it changes what `GetJobRunTestsCountByLookback` measures. For recently loaded data the values are close; for backfilled data they could differ significantly.

### [nit] `varchar(10)` silently dropped from ProwJob.Release
- where: `pkg/db/models/prow.go:20`
- concern: GORM tag changed from `gorm:"varchar(10);index"` to `gorm:"index"`. Won't affect existing tables (AutoMigrate doesn't alter column types), but a fresh database will get `text` instead of `varchar(10)`. Probably fine since GORM doesn't honor `varchar(N)` in tags anyway.

### [nit] pgx/v4 at v4.18.2; v4.18.3 is latest, v4 is EOL
- where: `go.mod:23`
- concern: PR promotes `github.com/jackc/pgx/v4` to a direct dependency at `v4.18.2`. The latest patch is `v4.18.3`. CodeRabbit confirmed no CVEs specific to `v4.18.2` (CVE-2024-27289 and CVE-2024-27304 were fixed in earlier releases). However, pgx v4 reached end-of-life on July 1, 2025. Pinning a new direct dependency to an EOL minor series is worth noting — a follow-up migration to pgx v5 should be tracked.

### [nit] Plan docs committed to `docs/plans/`
- where: `docs/plans/trt-1989-partitioning-prep.md`, `docs/plans/trt-1989-phase2-indexes.md`, `docs/plans/trt-1989-phase3-query-optimization.md`
- concern: 220+128+172 lines of planning docs will become stale. These are useful context but better suited to the PR description or a wiki. If kept in-repo, they should be marked as historical/superseded after the work lands.

### [resolved] Benchmark file output path construction
- where: `pkg/flags/postgres_benchmarking_test.go:77-89`
- resolved: Manual slash-append replaced with `filepath.Join` + `filepath.Clean`. Log message now shows full path. `#nosec G703` annotation added.

## Checked
- Model struct changes: new fields have correct GORM tags, composite index names are consistent, no missing indexes.
- Seed data: all new fields populated with correct values from parent objects.
- Test fixtures: updated in `job_runs_test.go`, `pr_new_tests_worker_test.go` to include new fields.
- `SetupJoinTable` registration in `db.go` placed before `AutoMigrate` call.
- Explicit join table model added to `modelsToMigrate` list.
- Transaction wrapping for ProwJobRun + PullRequest inserts in prow loader.
- `prowJobRunTestsFromGCS` and `extractTestCases` signature changes propagated correctly to all call sites.
- Materialized view template tokens (|||TIMENOW|||, |||START|||, etc.) used correctly in new filters.
- `payloadTestFailuresMatView` adds timestamp filter on both `pjrt` and `pjr` — correct since both tables are scanned.
- Benchmark test queries updated to use denormalized columns.
- New join table filters in `job_results()`, `PullRequestReport`, `RepositoryReport` are syntactically correct and use the right column names.
- `GatherLabelsFromBQ` batch refactor: iterator loop with `iterator.Done` check is correct, error handling returns partial results.
- Annotation release filter in `JobsRunsReportFromDB` correctly guarded by `len(release) > 0`.
- New benchmark cases cover the right API surfaces and pass correct parameters.
- `TestOutputs` refactor: join condition `prow_job_run_test_outputs.prow_job_run_test_timestamp = prow_job_run_tests.prow_job_run_timestamp` is correct and serves as partition pruning hint. `output` column qualified to avoid ambiguity. Ordering by `prow_job_run_timestamp DESC, id DESC` is correct.
- `JobDetailsReport` preload conditions syntactically correct; timestamp bound matches outer `since` variable.
- Author's `Test_CompareTestOutputsQueries` comparison test: 16=16, 0 missing, 0 mismatched. Old query: 4h41m; new: 247ms — dramatic improvement confirmed.
- `ProwJobHistoricalTestCounts` release param: all three call sites updated (`job_queries.go` benchmark, `job_runs.go` risk analysis, `pr_new_tests_worker.go`). Raw SQL parameter order matches: `prowJobID, release`.
- `test_analysis_by_job_by_dates.release` filter: adds local release scoping without dropping the `prow_jobs.release` join filter — redundant/safe approach.
- `pgx/v4` promoted from indirect to direct in `go.mod`: consistent with its explicit use in `db.go`.

## Open questions
- What is the deployment ordering plan? Will backfill run before this code goes live, or are the dropped-join queries expected to degrade gracefully?
- The `prow_job_run_prow_pull_requests` backfill is now urgent since new commits actively filter on its denormalized columns. Is it planned?
- For `BuildClusterHealth`: was the named-param mixing tested with GORM against Postgres?
- `GatherLabelsFromBQ`: was the date filter relaxation (`= DATE(...)` to `>= DATE(...)`) intentional, and is the lack of an upper bound acceptable for BigQuery cost?
- `TestOutputs`: is the new `status IN (failure, flake)` filter intentional? The old query had no status filter, so callers expecting outputs for passing tests will now get empty results.
- `force_custom_plan`: was the global scope (all connections, all queries) intentional? Were per-query or per-table alternatives considered? What's the expected overhead on non-partitioned queries?
- CI: `ci/prow/images` was failing at `912c2faec`; neisw retested 2026-06-03. Did it pass?
- `historicalTestCount` cache: will the composite key fix (`"jobID:release"`) be added before merge?
- pgx/v4 EOL (Jul 2025): is there a plan to migrate to v5?
