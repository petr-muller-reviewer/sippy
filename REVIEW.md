---
pr: openshift/sippy#3614
title: "Optimize test report matview refresh with pre-aggregation CTE"
head_sha: 332faf57ca67c2749a45948b16f974efa37ee185
base: main
reviewed_at: 2026-06-12T10:45:50Z
verdict: approve
---

## Summary

Adds a `pre_agg` CTE to `testReportMatView` that groups `prow_job_run_tests` by `(prow_job_id, test_id, suite_id, prow_job_run_release)` before joining dimension tables. Collapses ~60M rows to ~10M before expensive joins. Refresh time drops from 25-35 min to ~6 min on production. Also casts `SUM()` results to `::bigint` to preserve column types, adds upper-bound timestamp filter for future partition pruning, and removes useless `ORDER BY`.

## Findings

No blocking or should-fix findings.

### [nit] BETWEEN boundary overlap counts rows at BOUNDARY in both windows
- where: `pkg/db/views.go:244-251`
- concern: `BETWEEN |||START||| AND |||BOUNDARY|||` and `BETWEEN |||BOUNDARY||| AND |||END|||` both include the boundary timestamp, so rows at exactly that instant are double-counted. This is pre-existing behavior from the original query, not introduced by this PR, but worth noting for a future fix.
- excerpt: |
    COUNT(*) FILTER (WHERE status = 1  AND prow_job_run_timestamp BETWEEN |||START||| AND |||BOUNDARY|||) AS previous_successes,
    ...
    COUNT(*) FILTER (WHERE status = 1  AND prow_job_run_timestamp BETWEEN |||BOUNDARY||| AND |||END|||) AS current_successes,

### [nit] Mixed predicate style in pre_agg CTE
- where: `pkg/db/views.go:255`
- concern: The `WHERE` clause uses `>= ... AND ... <=` while the `FILTER` clauses use `BETWEEN`. Semantically identical but slightly inconsistent within the same CTE. Cosmetic only.
- excerpt: |
    prow_job_run_timestamp >= |||START||| AND prow_job_run_timestamp <= |||END|||

## Checked

- Pre-aggregation math is correct: grouping by all outer join keys, so `SUM(COUNT(*))` == `COUNT(*)` over the full set
- `::bigint` casts are necessary because `SUM(bigint)` returns `numeric` in PostgreSQL
- Upper-bound `<= |||END|||` filter is a no-op today (END resolves to NOW()) but correctly enables future partition pruning
- ORDER BY removal is correct: matview physical order guarantees nothing to readers
- `test_ownerships` fan-out behavior is identical to original (same `LEFT JOIN`, same `GROUP BY` columns)
- NULL `suite_id` handling: PostgreSQL `GROUP BY` groups all NULLs together, matching original behavior
- Both 7d and 2d matviews use the same template, so both benefit
- EXCEPT-based correctness verification was done in both directions on two datasets (prod-like + staging)
- CodeRabbit comment about benchmark scripts is moot: scripts were removed from the repo in commit 93003d6bd

## Open questions

- The BETWEEN boundary overlap (rows at exactly `|||BOUNDARY|||` counted in both windows) is pre-existing. Is this intentional, or something to track as a separate fix?
