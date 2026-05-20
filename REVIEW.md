---
pr: openshift/sippy#3541
title: "Trt 1989 migration indexes"
head_sha: b252569d945494e4299d4cd8553669532da9b7c8
base: main
reviewed_at: 2026-05-20T10:15:07Z
verdict: approve
---

## Findings

### [should-fix] Composite index names misleading for timestamp-first tables
- where: `pkg/db/models/prow.go:108-110`
- concern: `prow_job_run_tests` and `prow_job_run_test_outputs` lead with timestamp in the composite index (field declaration order), but the index is named `..._release_timestamp`. The plan doc explains timestamp-first is intentional for these tables (materialized view queries filter primarily on timestamp ranges). Name should be `..._timestamp_release` to match actual column order.
- excerpt: |
    ProwJobRunTimestamp time.Time `gorm:"index:idx_prow_job_run_tests_release_timestamp"`
    ProwJobRunRelease string `gorm:"index:idx_prow_job_run_tests_release_timestamp"`

### [should-fix] Backfill SQL does not account for zero-value timestamps
- where: `docs/plans/trt-1989-partitioning-prep.md:150-164`
- concern: Backfill conditions check `IS NULL OR = ''` for release (string), but new timestamp columns default to Go's zero `time.Time` (`0001-01-01 00:00:00+00`), not NULL. Backfill SQL needs a timestamp condition like `prow_job_run_timestamp = '0001-01-01 00:00:00+00'` or `prow_job_run_timestamp < '2000-01-01'`. Applies to all tables with denormalized timestamps.

### [nit] varchar(10) removal from ProwJob.Release is a no-op but undocumented
- where: `pkg/db/models/prow.go:20`
- concern: Changed from `gorm:"varchar(10);index"` to `gorm:"index"`. GORM AutoMigrate won't alter the existing column type, so this is safe. But it's a schema intent change bundled silently with the partitioning work. Fine as-is, just noting.
- excerpt: |
    Release     string         `gorm:"index"`

### [question] CASCADE behavior with explicit join table lacking gorm.Model
- where: `pkg/db/models/prow.go:74-79`
- concern: `ProwJobRunProwPullRequest` is a pure join table (composite PK, no `gorm.Model`, no `DeletedAt`). `ProwJobRun.PullRequests` has `constraint:OnDelete:CASCADE`. CASCADE should work at the DB FK level, but has this been tested? GORM's soft-delete interplay with explicit join tables without `DeletedAt` can sometimes cause surprises.

### [question] NOT NULL constraints deferred
- where: `pkg/db/models/prow.go:43,108,110,132,134`
- concern: All denormalized columns are nullable (Go zero values). Phase 3 queries using these columns need to guard against empty/zero values until backfill completes and NOT NULL constraints are added. Is there a plan to add `gorm:"not null"` after backfill, or will the partitioning migration enforce this?

## Checked
- BigQuery loader feeds through the same `processGCSBucketJobRun` path; parity maintained
- All creation sites for ProwJobRun, ProwJobRunTest, ProwJobRunTestOutput, ProwJobRunAnnotation, ProwJobRunProwPullRequest populate denormalized fields
- `SetupJoinTable` is registered before `AutoMigrate` in `db.go`
- `ProwJobRunProwPullRequest` is listed after both parent tables in `modelsToMigrate`
- Transaction wrapping ProwJobRun + pull request join table inserts ensures atomicity
- Seed data and test fixtures updated with new fields
- Only one creation site each for ProwJobRun (prow.go:961), ProwJobRunTestOutput (prow.go:1273), ProwJobRunAnnotation (prow.go:947)
- Index column order matches stated intent in plan doc for each table

## Open questions
- The index on `prow_job_run_tests` and `prow_job_run_test_outputs` intentionally leads with timestamp per the plan doc. Should the index names reflect this (`..._timestamp_release`) to avoid confusion?
- Has the CASCADE delete on the explicit `ProwJobRunProwPullRequest` join table been tested with an integration test?
- What's the plan for adding NOT NULL constraints on the denormalized columns after backfill?
