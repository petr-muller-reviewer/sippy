---
pr: openshift/sippy#3653
title: "use TIMENOW placeholder in job runs matview"
head_sha: e0dc60a5fb8ea2e446f15adaf00654a47fbdf629
base: main
reviewed_at: 2026-06-23T12:21:39Z
verdict: approve
---

## Summary

Replaces three `CURRENT_TIMESTAMP` references with `|||TIMENOW|||` in `prow_job_runs_report_matview` (`pkg/db/views.go`). Last matview using `CURRENT_TIMESTAMP` directly. Production behavior unchanged (`NOW()` = `CURRENT_TIMESTAMP`). Fixes seed data returning zero rows because wall-clock time was used instead of the pinnable placeholder. Regression from PR #3616.

## Findings

None.

## Checked

- Placeholder syntax matches all other matviews in the file
- `syncPostgresMaterializedViews` (lines 97-105) applies `|||TIMENOW|||` replacement to all matviews including this one
- `NOW()` and `CURRENT_TIMESTAMP` are equivalent in PostgreSQL; production semantics unchanged
- No remaining `CURRENT_TIMESTAMP` in matview definitions after this change
- No BigQuery parity concern: matview is PostgreSQL-only
- No CLAUDE.md convention violations
- Surrounding SQL syntax unaffected by substitution

## Open questions

None.
