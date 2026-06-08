---
pr: openshift/sippy#3571
title: "Trt 1989 migration schemas job run tests only nested"
head_sha: 6ecf3771f466025ffd2dfee20663425f06ebb27c
base: main
reviewed_at: 2026-06-08T16:39:57Z
verdict: needs-discussion
refresh_log:
  - old_sha: b80e1abd1f11cc21caa552afb8db6f77bec44b12
    new_sha: 6ecf3771f466025ffd2dfee20663425f06ebb27c
    summary: "2 commits: migration ordering comments resolved, query release filters added, gopar bumped, work_mem added globally, +491 lines of benchmark tests, plan doc rewritten to scope phase 4a (3 tables) vs 4b (remaining 3)"
---

## Findings

### [blocking] Down migration drops production tables unconditionally
- where: `pkg/db/migrations/000001_create_partitioned_tables.down.sql:6-8`
- concern: The down migration drops `prow_job_run_tests`, `prow_job_run_test_outputs`, and `test_analysis_by_job_by_dates` with CASCADE. These are the production table names, not `_new` suffixed staging tables. A rollback permanently destroys all test result data.
- excerpt: |
    DROP TABLE IF EXISTS prow_job_run_tests CASCADE;
    DROP TABLE IF EXISTS prow_job_run_test_outputs CASCADE;
    DROP TABLE IF EXISTS test_analysis_by_job_by_dates CASCADE;

### [blocking] test_analysis_by_job_by_dates partitioning changed from RANGE(date) to LIST(release) in same migration version
- where: `pkg/db/migrations/000001_create_partitioned_tables.up.sql:87-98`
- concern: The old migration 000001 created this table as `PARTITION BY RANGE (date)`. The replacement uses `PARTITION BY LIST (release)`. On existing databases, golang-migrate sees version 1 as applied and skips it. The table keeps its old RANGE schema but the code now creates LIST->RANGE partitions against it. Additionally, the inline partition-creation code in `loadDailyTestAnalysisByJob` that created RANGE date partitions was removed, so no new partitions will be created for the old schema either.
- excerpt: |
    ) PARTITION BY LIST (release);

### [resolved] Unresolved uncertainty about migration ordering
- where: `pkg/db/db.go:100-103`
- resolution: The uncertain comments ("unsure if we need to break out...", "FIRST??") were replaced with clear forward-looking notes in commit 6d2aa7777. The ordering is now RunMigrations -> SetupJoinTable -> AutoMigrate with a note that AutoMigrate may need to run first when `prow_job_runs` moves to managed migrations in phase 4b.

### [should-fix] gopar is a pre-release personal dependency by the PR author
- where: `go.mod:27`
- concern: `github.com/neisw/gopar v0.0.0-20260602170650-3755a6d55bbe` is a pre-release library authored by the same person submitting this PR. It becomes a critical infrastructure dependency (partition lifecycle). Should have explicit team review/ownership and ideally live under the org or be vendored with review.
- excerpt: |
    github.com/neisw/gopar v0.0.0-20260602170650-3755a6d55bbe

### [should-fix] force_custom_plan and work_mem applied globally to all connections
- where: `pkg/db/db.go:67-72`
- concern: `plan_cache_mode = force_custom_plan` prevents PostgreSQL from caching plans for any prepared statement. Additionally, `work_mem = "128MB"` was added (new in 6ecf3771f), setting memory per sort/hash operation globally. Both are applied to all connections regardless of query complexity. Consider documenting the measured trade-off or scoping if possible.
- excerpt: |
    pgxConfig.RuntimeParams["plan_cache_mode"] = "force_custom_plan"
    pgxConfig.RuntimeParams["work_mem"] = "128MB"

### [should-fix] TestOutputs query adds status filter — behavioral change beyond partitioning
- where: `pkg/db/query/test_queries.go:284`
- concern: The query now filters `prow_job_run_tests.status IN [Failure, Flake]`. Previously all statuses were returned. If intentional (outputs only exist for failures/flakes), it should be called out as a deliberate semantic change. If not, it silently filters data.
- excerpt: |
    Where("prow_job_run_tests.status IN ?", []int{int(v1.TestStatusFailure), int(v1.TestStatusFlake)})

### [should-fix] TestOutputs join on prow_job_run_test_outputs missing release column
- where: `pkg/db/query/test_queries.go:279`
- concern: The join matches on `prow_job_run_test_id = prow_job_run_tests.id AND prow_job_run_test_timestamp = prow_job_run_tests.prow_job_run_timestamp` but omits `prow_job_run_test_release`. With partitioned tables, ID values are only unique within a (release, timestamp) partition. Cross-partition ID collisions are theoretically possible and would produce incorrect joins.
- excerpt: |
    Joins("JOIN prow_job_run_test_outputs ON prow_job_run_test_outputs.prow_job_run_test_id = prow_job_run_tests.id AND prow_job_run_test_outputs.prow_job_run_test_timestamp = prow_job_run_tests.prow_job_run_timestamp")

### [nit] Hardcoded retention constants
- where: `pkg/db/db.go:336-340`
- concern: `CleanupPartitions` uses hardcoded 100-day detach and 110-day drop thresholds. These should be configurable for different deployment environments.

### [nit] Seed data creates partitions for 190-day range
- where: `cmd/sippy/seed_data.go:74`
- concern: `time.Now().AddDate(0, 0, -190)` at daily granularity across 3 tables x N releases creates thousands of partitions on seed init. Could be slow.

### [nit] DB_PARTITIONS type name violates Go naming conventions
- where: `vendor/github.com/neisw/gopar/partitioning/partitions.go:23`
- concern: Should be `DBPartitions` per Go convention. This is in the vendored dependency.

### [question] ProwJobRunTest/ProwJobRunTestOutput excluded from AutoMigrate — future column additions?
- where: `pkg/db/db.go:128-129`
- concern: With these models commented out of `modelsToMigrate`, any future schema changes to these tables require manual migration SQL. Is there a plan to document this for contributors?

### [question] Orphan detection query from phase 4 plan — will it be implemented?
- where: `docs/plans/trt-1989-phase4-partitioned-tables.md` (monitoring section)
- concern: The plan doc includes orphan detection queries, but no code implements periodic monitoring. With FKs dropped, orphan rows can accumulate silently.

## Checked
- SQL injection safety in gopar: uses `pq.QuoteIdentifier`/`pq.QuoteLiteral` throughout
- Query optimization changes in views.go, functions.go: join drops are consistent with denormalized column availability
- Component Readiness queries (provider.go): parameter binding matches expanded placeholder lists
- Partition naming/sanitization logic in gopar handles edge cases (dots, spaces, long names)
- ProwJobRunProwPullRequest model changes: index tags added correctly
- e2e partition test coverage: lifecycle test is thorough
- `ensurePartitionsForReleases` in load.go: error handling is appropriate (returns on partition creation failure, warns on cleanup failure)
- Since previous review: release filter additions in `job_runs.go`, `job_queries.go`, `test_queries.go`, `pr_new_tests_worker.go` are correct — queries now include `prow_job_run_release` for partition pruning
- Since previous review: new benchmark tests in `postgres_benchmarking_test.go` (+491 lines) cover matview, API, and single-release matview scenarios — well structured
- Since previous review: `QueryTestAnalysis` now filters on `release` — all callers updated consistently

## Open questions
- Has the migration been tested on a database that already has golang-migrate version 1 applied (the old `test_analysis_by_job_by_dates` RANGE migration)?
- Is `force_custom_plan` measurably impacting non-partitioned query performance?
- What's the plan for getting `gopar` to a stable release or moving it under the org?
- Is the status filter addition in `TestOutputs` intentional? If so, is there a separate behavioral validation?
