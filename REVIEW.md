---
pr: openshift/sippy#3719
title: "Return average prow job runtime over current/prev weeks"
head_sha: 5e4b91173b6f451679b000438edb66f2b2457a51
base: main
reviewed_at: 2026-07-01T12:13:49Z
verdict: approve
---

## Findings

No findings.

## Checked
- AVG(duration) correctness with LEFT JOIN to bug_jobs: join is on prow_jobs.id, so all runs of a given job are multiplied by the same factor (number of bug associations), preserving AVG correctness.
- Nanosecond-to-minutes divisor (60000000000.0 = 60e9 ns = 1 minute): correct.
- coalesce(..., 0) for NULL averages: consistent with all other aggregates in the same query (counts use the same pattern). Go field is `int` (not `*int`), so NULL would map to 0 anyway.
- RETURNS TABLE signature updated to include both new columns.
- Final SELECT in the outer query includes both new columns.
- GetNumericalValue switch cases added for both new param names.
- JSON tags match SQL column names.
- Cross-file impact: GORM scans by struct tag name, not column position. No explicit column enumeration elsewhere.
- BigQuery parity: job_results is a PostgreSQL-only aggregation function with no BigQuery equivalent. The CLAUDE.md parity rule does not apply.
- No callers of GetNumericalValue break with the addition.
- gofmt: struct field alignment is consistent with adjacent fields in that section.

## Open questions
- The fields use `int` (not `*int`), so jobs with zero runs in a period show `0` minutes rather than `null` in JSON. This is consistent with how other zero-value fields behave in this struct, but worth confirming the author is comfortable with that for duration specifically (0 minutes is a meaningful value, unlike 0 passes which clearly means "no data").
