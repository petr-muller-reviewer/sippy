---
pr: openshift/sippy#3654
title: "Add composite index on (release, timestamp) to job runs matview"
head_sha: 6b430e24533c6f319d185a3c68b4fa73515a3fab
base: main
reviewed_at: 2026-06-23T12:21:39Z
verdict: approve
---

## Summary

Adds `AdditionalIndexes []string` to `PostgresView` for non-unique indexes managed via `syncSchema`. Uses it to create `(release, timestamp DESC)` on `prow_job_runs_report_matview`, matching the filter pattern in `JobsRunsReportFromDB` (`release = ?` + `timestamp < ?`). Staging benchmarks show COUNT drops from ~120ms to 15ms warm, paginated SELECT to sub-ms.

## Findings

No findings.

## Checked

- Index naming: `idx_<name>` (unique) vs `idx_<name>_<i>` (additional) -- no collision
- `syncSchema` call: correct `hashTypeMatViewIndex`, passes `matViewUpdated` for force-recreate on matview change
- Column validity: `release` (line 254) and `timestamp` (line 265, bigint epoch-ms) both exist in matview output
- No SQL injection: column expressions are hardcoded Go literals
- Query pattern match: `pkg/api/job_runs.go:115-119` filters `release = ?` and `timestamp < ?`
- Struct doc comment on `AdditionalIndexes` is clear and includes naming convention

## Open questions

- Orphaned index cleanup: removing entries from `AdditionalIndexes` leaves highest-numbered indexes in DB (never referenced by `syncSchema`). Not a problem today with one entry, but worth noting for future use.
