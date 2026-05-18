---
pr: openshift/sippy#3532
title: "TRT-1989: model changes to prep for partitioning"
head_sha: 9039e19b54d847f5ba5add5441ee1d8b61951638
base: master
reviewed_at: 2026-05-18T18:07:16Z
verdict: needs-discussion
refresh_log:
  - old_sha: 171fc894547e0fdb0dc9e38a82cdd18754c8a845
    new_sha: 9039e19b54d847f5ba5add5441ee1d8b61951638
    summary: "Author removed broken gorm expression index tags and varchar tags from new fields (1 commit, 1 file, 7 lines changed)"
---

## Findings

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

## Resolved

### [blocking] Expression index tags reference nonexistent column
- where: `pkg/db/models/prow.go:96`
- resolved_by: `9039e19b5` — removed the `expression:DATE(timestamp AT TIME ZONE 'UTC')` tags entirely from `ProwJobRunTimestamp` and `ProwJobRunTestTimestamp`.

### [blocking] Same expression index bug on ProwJobRunTestOutput
- where: `pkg/db/models/prow.go:120`
- resolved_by: `9039e19b5` — same commit, removed the expression tag.

## Checked
- Loader threads release and timestamp correctly through prowJobRunTestsFromGCS and extractTestCases
- Seed data populates all new fields consistently
- Test fixtures updated to include new required fields
- No existing queries modified; purely additive schema change
- Naming convention across tables (hierarchical prefix nesting) is consistent and intentional
- New commit `9039e19b5` also removed `varchar(10)` GORM tags from all release fields (these are not valid GORM type tags for string fields)

## Open questions
- Is the backfill for existing rows planned as a separate migration, or will it be part of the partitioning PR?
- Should `ProwJobRunTest.ProwJobID` have an index tag given its stated purpose of avoiding joins?
