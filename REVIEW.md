---
pr: openshift/sippy#3687
title: "Remove unused columns from CR BigQuery queries"
head_sha: 0d400af1c4a9aae4531175e41b93a9a8b00b8c45
base: main
reviewed_at: 2026-06-25T13:30:46Z
verdict: approve
---

## Findings

No findings.

## Checked
- `file_path` remains in GROUP BY (line 623) after removal from SELECT: valid BigQuery SQL, intentional per PR description to preserve aggregation semantics.
- `deserializeRowToTestStatus` still handles `test_suite` (line 805) and `capabilities` (line 824): correct, this deserializer serves `BuildComponentReportQuery` which still selects those columns. The test details query uses `deserializeRowToJobRunTestReportStatus` (line 1051) which never referenced the removed columns.
- Both deserializers use name-based column matching (`fieldSchema.Name`), not positional indexing: removing columns from SELECT cannot cause index-shift bugs.
- PostgreSQL `testDetailRow.Capabilities` was selected but never read in the processing loop: safe to remove.
- Provider parity maintained: neither BQ nor PG test details queries return the removed columns. Test status queries (which use these fields) are untouched.
- Removed TODO comments and debug log lines were stale references to already-resolved or irrelevant concerns.

## Open questions
- None.
