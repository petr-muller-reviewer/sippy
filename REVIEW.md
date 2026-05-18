---
pr: openshift/sippy#3532
title: "TRT-1989: model changes to prep for partitioning"
head_sha: 171fc894547e0fdb0dc9e38a82cdd18754c8a845
base: master
reviewed_at: 2026-05-15T15:14:33Z
verdict: needs-discussion
---

## Findings

### [blocking] Expression index tags reference nonexistent column
- where: `pkg/db/models/prow.go:96`
- concern: `ProwJobRunTimestamp` on `ProwJobRunTest` has `gorm:"expression:DATE(timestamp AT TIME ZONE 'UTC')"`. The `timestamp` column exists on `prow_job_runs`, not on `prow_job_run_tests`. This will either fail at AutoMigrate time or create an expression index referencing the wrong column. Should reference `prow_job_run_timestamp`.
- excerpt: |
    ProwJobRunTimestamp time.Time `gorm:"expression:DATE(timestamp AT TIME ZONE 'UTC')"`

### [blocking] Same expression index bug on ProwJobRunTestOutput
- where: `pkg/db/models/prow.go:120`
- concern: Same issue as above. `ProwJobRunTestTimestamp` references `timestamp` in its expression, but the column on `prow_job_run_test_outputs` would be `prow_job_run_test_timestamp`.
- excerpt: |
    ProwJobRunTestTimestamp time.Time `gorm:"expression:DATE(timestamp AT TIME ZONE 'UTC')"`

### [should-fix] Missing index on ProwJobRunTest.ProwJobID
- where: `pkg/db/models/prow.go:94`
- concern: The comment says this field avoids a join to get ProwJobID. If queries will filter or join on this column, it needs `gorm:"index"`. The adjacent `ProwJobRunID` has one. Without it, queries on one of the largest tables will table-scan.
- excerpt: |
    ProwJobID uint

### [question] No backfill for existing rows
- where: `pkg/db/models/prow.go:42-98`
- concern: AutoMigrate adds nullable columns. All existing rows get NULL for the new fields. Is backfill intentionally deferred? Partitioning will need these populated for all rows.

### [nit] PR description says ProwJobRunStartTime, code uses ProwJobRunTimestamp
- where: PR description
- concern: Minor mismatch between the PR description field name and the actual code. Could confuse future searches.

## Checked
- Loader threads release and timestamp correctly through prowJobRunTestsFromGCS and extractTestCases
- Seed data populates all new fields consistently
- Test fixtures updated to include new required fields
- No existing queries modified; purely additive schema change
- Naming convention across tables (hierarchical prefix nesting) is consistent and intentional

## Open questions
- Have you tested that AutoMigrate succeeds with the expression index tags as written? The `timestamp` reference looks like it targets the wrong table's column.
- Is the backfill for existing rows planned as a separate migration, or will it be part of the partitioning PR?
- Should `ProwJobRunTest.ProwJobID` have an index tag given its stated purpose of avoiding joins?
