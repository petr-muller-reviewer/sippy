---
pr: openshift/sippy#3652
title: "drop unused array_agg columns from job runs matview"
head_sha: 8c47faa998d0e77b7844025133535c821fb493b0
base: main
reviewed_at: 2026-06-23T12:14:38Z
verdict: approve
---

## Findings

None.

## Checked
- `failed_test_ids` and `flaked_test_ids` are not referenced in the outer SELECT of `jobRunsReportMatView`
- No Go struct (`JobRun` in `pkg/apis/api/types.go`) maps to these column names
- Grep across entire codebase (Go, SQL, TS, JS) finds zero references to either column name
- SQL syntax after removal is correct (no dangling commas)
- Matview auto-syncs via `syncPostgresMaterializedViews` hash comparison; no separate migration needed
- No BigQuery equivalent of this matview exists, so no parity concern
- API surface unchanged; no documentation update required

## Open questions
- None
