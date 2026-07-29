---
pr: openshift/sippy#3845
title: "TRT-2752: Add test lifecycle column to prow_job_run_tests"
head_sha: 4f2cfc6f941482eeba05ec4319029b7451571068
base: main
reviewed_at: 2026-07-29T22:51:57Z
verdict: approve
---

## Summary

Ingestion-only (Phase 1) change. Parses `lifecycle` JUnit XML attribute ("blocking"/"informing"), threads it through `junit.TestCase` -> `extractTestCases` -> `types.TestCaseEntry` -> `prowJobRunTestRow` -> COPY temp table -> INSERT into `prow_job_run_tests`. Adds migration 000009 (`ALTER TABLE ... ADD COLUMN lifecycle TEXT NOT NULL DEFAULT 'blocking'`), matching BQ's `COALESCE(NULLIF(lifecycle, ''), 'blocking')` semantics. No read-path/filter changes yet.

## Findings

### [nit] normalizeLifecycle accepts unrecognized values silently
- where: `pkg/dataloader/prowloader/prow.go:1719-1727`
- concern: any non-empty string is lowercased and stored as-is; no validation against the known `blocking`/`informing` enum. Matches BQ's equally permissive `COALESCE(NULLIF(lifecycle, ''), 'blocking')`, so it's parity-consistent rather than a regression, but there's no guard against garbage values ending up in the column.
- excerpt: |
    func normalizeLifecycle(raw string) string {
        if raw == "" {
            return "blocking"
        }
        return strings.ToLower(raw)
    }

### [question] should unrecognized lifecycle values be logged/flagged?
- where: `pkg/dataloader/prowloader/prow.go:1719-1727`
- concern: if a test source ever emits a typo'd or unexpected lifecycle value, it will be silently persisted without any log signal. Worth confirming whether that's acceptable for Phase 1 or should be tightened before Phase 2 (filtering) relies on the column.

## Checked
- Full field propagation traced end-to-end: XML attr -> `TestCaseEntry` -> `prowJobRunTestRow` -> `testCols` (COPY) -> `INSERT ... SELECT FROM tmp_job_run_tests` in `writeJobRunBatch`. No dropped field.
- `prow_job_run_tests` is a partitioned parent table (`pkg/db/migrations/000001_create_partitioned_tables.up.sql`); `ADD COLUMN ... DEFAULT` is metadata-only on PG 11+ and propagates to partitions, confirming the PR description's claim.
- Migration numbering (000009) sequential, no collision with other in-flight/merged migrations on `main`.
- `strings` import already present in `prow.go`; no new unused/missing import.
- `TestExtractTestCases` updated with `Lifecycle` on all existing cases plus a new case covering informing/blocking/empty-default in one suite.
- `TestNormalizeLifecycle` table-driven, covers empty/blocking/informing/unknown/mixed-case.
- `gorm:"default:blocking"` tag on `models.ProwJobRunTest.Lifecycle` is redundant with the SQL default but harmless.
- Existing `Lifecycle` usages elsewhere (`test_details.go`, BQ `querygenerators.go`) are pre-existing/unrelated to this column (BQ-side and JobTier-variant based); this PR doesn't need to touch them since it's ingestion-only.
- No README/docs update required — internal schema/ingestion change, no API or config surface touched.

## Open questions
- Should `normalizeLifecycle` log a warning (or reject) when given a value outside `{blocking, informing}`, given Phase 2 will presumably filter/query on this column?
