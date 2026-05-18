---
pr: openshift/sippy#3334
title: "Trt-1989: partition management"
head_sha: dd5ebf5b038eb247d60a11ad350f5edd949a1194
base: master
reviewed_at: 2026-05-18T16:00:20Z
verdict: needs-discussion
---

## Summary

Adds PostgreSQL partition lifecycle management for three large tables (prow_job_run_tests, prow_job_run_test_outputs, test_analysis_by_job_by_dates). Introduces migration helpers, partition CRUD utilities, schema utilities, FK management, and table rename tooling. Changes ProwJobRunTest and ProwJobRunTestOutput models from gorm.Model to composite PK (id, created_at).

## Findings

### [blocking] OldestDate used where NewestDate intended in preparePartitions
- where: `pkg/dataloader/prowloader/prow.go:443`
- concern: Copy-paste error. The end-of-range display uses `stats.OldestDate.Time` when the condition checks `stats.NewestDate.Valid`. This renders the partition range incorrectly.
- excerpt: |
    if stats.NewestDate.Valid {
        endRange = stats.OldestDate.Time.Format("2006-01-02")
    }

### [blocking] Model PK change needs migration sequencing clarity
- where: `pkg/db/models/prow.go:86-92,109-115`
- concern: Removing gorm.Model and switching PK from (id) to (id, created_at) is a breaking schema change. The comments say "Do not update until after partitions have been enabled" but this PR updates the model. If GORM auto-migrate runs against the existing non-partitioned table, this will fail or corrupt the PK. The PR needs to clarify the deployment order: is the partition migration expected to run first, or is auto-migrate disabled for these tables?
- excerpt: |
    // Do not update until after partitions have been enabled
    // Remove gorm.model on Partitioned tables so we can manage the primary key and index
    type ProwJobRunTest struct {
        ID        uint      `gorm:"primaryKey;autoIncrement"`
        CreatedAt time.Time `gorm:"primaryKey;index"`

### [should-fix] fmt.Printf in production code
- where: `pkg/dataloader/prowloader/prow.go:431,445`
- concern: Uses fmt.Printf instead of log.Infof. Bypasses structured logging, won't appear in log aggregation. Every other function in this PR uses logrus.
- excerpt: |
    fmt.Printf("  Total: %d partitions (%s)\n", stats.TotalPartitions, stats.TotalSizePretty)
    fmt.Printf("  Range: %s to %s\n", startRange, endRange)

### [should-fix] Partition failure blocks all data loading
- where: `pkg/dataloader/prowloader/prow.go:232-243`
- concern: Any partition management error (including detaching a 95-day-old partition) aborts the entire load cycle. A failure to clean up old data should not prevent loading new data. Consider separating aging failures from prepare failures, or at minimum logging and continuing for aging errors.
- excerpt: |
    for _, config := range partitionConfigs {
        err := pl.updatePartitions(config)
        if err != nil {
            pl.errors = append(pl.errors, err)
            return
        }
    }

### [should-fix] Incomplete composite index on ProwJobRunTest
- where: `pkg/db/models/prow.go:94,101`
- concern: `idx_pjrt_run_test_status` has priority:1 on ProwJobRunID and priority:3 on Status, with no priority:2 column. Either a column is missing from the composite index or the priorities are wrong.
- excerpt: |
    ProwJobRunID uint `gorm:"index;index:idx_pjrt_run_test_status,priority:1"`
    Status   int `gorm:"index;index:idx_prow_job_run_tests_test_id_status;index:idx_pjrt_run_test_status,priority:3"`

### [should-fix] OnDelete:CASCADE on partitioned table FK
- where: `pkg/db/models/prow.go:106`
- concern: ProwJobRunTestOutput still declares `constraint:OnDelete:CASCADE` but the PR is explicitly removing FK cascades for partitioned tables. If the table is partitioned, PostgreSQL cannot enforce cascading deletes across partitions. This constraint declaration may be silently ignored or cause errors.
- excerpt: |
    ProwJobRunTestOutput *ProwJobRunTestOutput `gorm:"constraint:OnDelete:CASCADE;"`

### [nit] Dev note left in unrelated code
- where: `pkg/dataloader/prowloader/prow.go:1359`
- concern: Comment reads like a personal observation, not a code comment. Should be tracked as an issue if it matters, or removed.
- excerpt: |
    // interesting that we rely on created_at here which is when we imported the test, not when the test ran

### [nit] Documentation-only tests that verify nothing
- where: `pkg/db/utils_test.go:230-480`, `pkg/db/partitions_test.go:207-540`
- concern: Multiple test functions (TestSyncIdentityColumn, TestMigrateTableDataRange, TestVerifyPartitionCoverage, TestRenameTables, TestColumnDeduplication, etc.) contain only comments and t.Log. They document behavior but assert nothing. Either write real integration tests or move this to godoc.

### [nit] Very large new files
- where: `pkg/db/partitions.go` (2116 lines), `pkg/db/utils.go` (2159 lines)
- concern: Each file is 2100+ lines. FK operations, sequence operations, table rename logic, and column verification could be split into separate files for maintainability.

### [question] DeleteTestsByName single-transaction concern
- where: `pkg/db/migration.go:682-731`
- concern: Deletes from 6 tables in a single transaction. For production-sized data (millions of rows matching a LIKE pattern), this could hold locks for extended periods. Is this intended for one-off maintenance only, or could it run during normal operations?

## Checked
- SQL injection prevention: pq.QuoteIdentifier used consistently for dynamic identifiers, sql.Named for values, escapeForLike for LIKE patterns
- Partition name validation (isValidPartitionName) properly restricts format to prevent accidental drops
- Transaction handling with proper rollback in defer across all mutation functions
- Retention policy validation (90-day minimum, 75% partition threshold, 80% storage threshold)
- NextDay removal is safe: only caller was the inline partition creation in loadDailyTestAnalysisByJob which is also removed
- strings import removal is correct: no other usage in prow.go

## Open questions
- What is the expected deployment sequence? Does the partition migration (MigrateToPartitionedTable + Finalize) run before this code deploys, or does this PR need to handle the non-partitioned case?
- Is the idx_pjrt_run_test_status index intentionally missing a priority:2 column, or should TestID be included?
- Should aging errors (detach/drop old partitions) really block the entire load, or can they be logged and continued past?
