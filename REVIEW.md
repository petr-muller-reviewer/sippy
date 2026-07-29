---
pr: openshift/sippy#3843
title: "TRT-2821: Use prefix_max_last_failure instead of LATERAL join for CR timestamps"
head_sha: 1db815f048d87126d7754cc777310063cbed793b
base: main
reviewed_at: 2026-07-29T11:38:10Z
verdict: request-changes
---

## Summary

Replaces a `LEFT JOIN LATERAL` against `prow_job_run_tests` (per-partition scan) with an inline `MAX(e.prefix_max_last_failure)` sourced from `test_cumulative_summaries`, unifying sample (prefix-sum) and base/GA paths behind a new `testStatusSpec.lastFailureExpr` field. Adds integration tests for multi-job MAX, mixed NULL/non-NULL, and GA no-timestamp behavior. Postgres-only change; BigQuery data provider untouched.

## Findings

### [blocking] LastFailure unbounded below dateRange.Start when minFail=0
- where: `pkg/api/componentreadiness/dataprovider/postgres/cr_queries.go:153-179`
- concern: `prefix_max_last_failure` (`pkg/db/cumulativesummary/cumulative_summary.go:148`, `GREATEST(prev.prefix_max_last_failure, tds.last_failure_timestamp)`) is an all-time cumulative running max, never reset at window start. The query bounds it only from above (`e.date = lookupEnd`); unlike `totalExpr`/`successExpr`/`flakeExpr` there is no diff against a start-of-window value. Correctness currently relies entirely on the `HAVING SUM(failures) >= minimumFailure` clause guaranteeing an in-window failure exists (monotonicity then guarantees the max is at least that recent). But `minFail` is user-controlled and validated only as `v >= 0` (`pkg/api/componentreadiness/utils/queryparamparser.go:384-385`, default 3). With `minFail=0`, `HAVING` degrades to `SUM(total) > 0` — no in-window failure required — so `LastFailure` can surface a timestamp from long before `dateRange.Start` for a test with zero failures in the actual reporting window. The old LATERAL join was explicitly scoped by `pjrt.prow_job_run_timestamp >= rangeStart AND < rangeEnd` and did not have this gap.
- excerpt: |
    failureInner := fmt.Sprintf(`
        SELECT
            e.test_id, e.suite_id, vg.group_id AS variant_group_id,
            SUM(%s) AS total_count,
            SUM(%s) AS success_count,
            SUM(%s) AS flake_count,
            %s AS last_failure
        ...
        HAVING SUM(%s) > 0
            AND SUM(%s) - SUM(%s) - SUM(%s) >= ?`,

### [should-fix] BigQuery/Postgres parity not addressed
- where: `pkg/api/componentreadiness/dataprovider/bigquery/querygenerators.go:437` vs `pkg/api/componentreadiness/dataprovider/postgres/cr_queries.go:238`
- concern: Project convention (`.claude/rules/backend.md`) requires parity between providers for query-logic changes. BigQuery's `last_failure` (`MAX(CASE WHEN junit_data.success_val = 0 THEN junit_data.prowjob_start ELSE NULL END)`) is strictly scoped to the date-filtered `junit_data` CTE — i.e. window-bound today. Postgres's new expression is not window-bound (see blocking finding above). This PR doesn't mention or address BigQuery; if the minFail=0 gap is real, the two providers will diverge in reported `LastFailure` for that configuration. Even absent that edge case, the PR should state explicitly whether this is an intentional postgres-only optimization with no observable semantic difference, or something that should also land on BigQuery.
- excerpt: |
    lastFailureExpr: "MAX(e.prefix_max_last_failure)",   // postgres, not window-bound
    MAX(CASE WHEN junit_data.success_val = 0 THEN junit_data.prowjob_start ELSE NULL END) AS last_failure  // bigquery, window-bound

### [nit] lastFailureExpr windowing behavior undocumented
- where: `pkg/api/componentreadiness/dataprovider/postgres/cr_queries.go:113-130`
- concern: The `testStatusSpec` field comment and the `queryTestStatus` doc comment don't note that `lastFailureExpr` is NOT windowed the same way `totalExpr`/`successExpr`/`flakeExpr` are (those diff `e` against `s`; this one doesn't, by necessity since MAX isn't invertible). A future maintainer editing this could reasonably assume all four expressions share the same windowing semantics.

## Checked
- Build passes (`go build ./...`), unit tests pass (`go test ./pkg/api/componentreadiness/dataprovider/postgres/...`).
- Removed `lastFailureLateral`, `spec.release`, `spec.dateRange` and the now-unused `sippyv1` import are fully cleaned up — no dangling references.
- Verified via monotonicity argument that in the default/common case (minFail >= 1), MAX(e.prefix_max_last_failure) at lookupEnd is guaranteed >= any in-window failure timestamp, so the change is safe for the default configuration.
- GROUP BY collapsing across multiple prow_job_id per variant group: MAX aggregation across jobs is correct — a job with only stale (pre-window) failures can't out-rank a job with an actual in-window failure, since the in-window job's own cumulative max is necessarily >= rangeStart.
- New GA-path assertion (`ts.LastFailure.IsZero()`) and multi-job/mixed-NULL integration tests are sound and match existing test conventions.

## Open questions
- Is `minFail=0` a realistic/supported configuration in production usage, or effectively unreachable in practice? If unreachable, this should probably be enforced (e.g. `v >= 1`) or explicitly documented as a known limitation rather than left as a latent correctness gap.
- Should the BigQuery provider receive the equivalent optimization (there's presumably an analogous LATERAL-like cost in BigQuery, or is BigQuery's current window-scoped `last_failure` already cheap enough that no change is needed there)?
- Is there a reason not to add a lower-bound guard (e.g. `CASE WHEN e.prefix_max_last_failure >= ?rangeStart THEN e.prefix_max_last_failure END`) to make `lastFailureExpr` robust regardless of `minFail`, rather than relying on the HAVING clause as an implicit invariant?
