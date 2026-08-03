---
pr: openshift/sippy#3847
title: "[WIP] Populate base statistics in test_details from aggregate tables"
head_sha: 232123b3864cc5f35c4efd07ad832f69e9511156
base: main
reviewed_at: 2026-07-30T17:52:05Z
verdict: needs-discussion
---

## Summary

Adds a fallback in `PostgresProvider.QueryBaseJobRunTestStatus` (postgres/provider.go) that queries aggregate tables (`test_cumulative_summaries` prefix-sum delta, or `prow_ga_raw_test_data` for GA-window releases) when the normal per-run query (`prow_job_run_tests`) returns nothing for the base release. Threads a new `*string` explanation return value through `TestDetailsQuerier.QueryBaseJobRunTestStatus` → `getJobRunTestStatus` → `internalGenerateTestDetailsReport` so the UI can explain why per-run links are missing. Aggregate rows are marked as such implicitly (empty `ProwJobRunID`), and `assessTestStats` skips building a `JobRunStats` entry when `ProwJobRunID == ""`. BigQuery/Mixed providers and `releasefallback` middleware updated only for the new signature; no BQ behavior change. PR carries `do-not-merge/work-in-progress` label.

## Findings

### [blocking] all-or-nothing fallback gate breaks multi-test cache priming
- where: `pkg/api/componentreadiness/dataprovider/postgres/provider.go:168-186`
- concern: `QueryBaseJobRunTestStatus` triggers the aggregate fallback only when the per-run query's combined result across the *entire* `reqOptions.TestIDOptions` set is empty (`if len(result) > 0 { return result, nil, nil }`). `GenerateTestDetailsReportMultiTest` (test_details.go:87-178, used for cache-priming all test details for a view) calls this once for potentially hundreds of test IDs at once. If even one test/job in that batch has any per-run data for the base release (e.g. a job that started reporting to `prow_job_run_tests` partway through a still-transitioning release), the fallback is skipped for *every other* test in the batch, regressing them to all-zero base stats — the exact bug this PR is meant to fix. Needs confirmation that target releases are always fully-backfilled-or-fully-absent, or the gate needs to be per-test/per-job rather than batch-wide.
- excerpt: |
    result, errs := p.queryTestDetails(...)
    if len(errs) > 0 {
        return result, nil, errs
    }
    if len(result) > 0 {
        return result, nil, nil
    }
    result, source, errs := p.queryBaseAggregateTestDetails(ctx, reqOptions)

### [should-fix] "IsAggregate" flag from PR description does not exist; empty ProwJobRunID used as implicit sentinel
- where: `pkg/api/componentreadiness/test_details.go:562` (skip logic), `pkg/api/componentreadiness/dataprovider/postgres/provider.go:396-409` (aggregate row construction, ProwJobRunID never set)
- concern: PR/commit description states aggregate rows are "marked with `IsAggregate=true`," but no such field was added to `crstatus.TestJobRunRows`. Instead, `assessTestStats` infers "this is a synthetic row" from `ProwJobRunID == ""`. Verified this is safe today: Postgres per-run rows always set `ProwJobRunID` from `pjr.id` (provider.go:355), and BigQuery always sets it from `junit.prowjob_build_id`, which survives an inner join and can't be empty for real rows (querygenerators.go:306-311, 612, 1083-1084). But it's an undocumented, implicit contract rather than the explicit flag the author described — a future data source that legitimately omits a run ID would be silently miscategorized with no compile-time signal.
- excerpt: |
    *testStats = testStats.AddTestCount(jobRow.Count, flakeAsFailure)
    if jobRow.ProwJobRunID != "" {
        *jobRunStatsList = append(*jobRunStatsList, c.getJobRunStats(jobRow))
    }

### [should-fix] no end-to-end test coverage for explanation propagation or JobRunStats exclusion
- where: `pkg/api/componentreadiness/component_report_test.go:1571-1638`, `test/integration/component_readiness_test.go:1594-1112` (new `TestQueryBaseJobRunTestStatus_AggregateFallback`)
- concern: new integration tests thoroughly cover `PostgresProvider.QueryBaseJobRunTestStatus` in isolation, but nothing exercises the full pipeline — no test confirms `baseExplanation` actually reaches `testdetails.Report.Analyses[*].Explanations`, or that aggregate rows are excluded from `JobStats.BaseJobRunStats` while still contributing to `BaseStats` totals. The `component_report_test.go` edit adds `ProwJobRunID` to all fixture rows specifically to dodge the new skip branch, so that branch is unexercised at the unit level. The multi-test all-or-nothing concern above also has no covering test (all new fallback tests use a single `TestIDOptions` entry).
- excerpt: |
    report := tc.generator.internalGenerateTestDetailsReport("", nil, nil, nil, baseStats, sampleStats, tc.generator.ReqOptions.TestIDOptions[0])

### [nit] processAggregateRows duplicates queryTestDetails row-processing logic
- where: `pkg/api/componentreadiness/dataprovider/postgres/provider.go:347-411` vs `393-471`
- concern: batch-fetching job variants, `RequestedVariants` matching, `filterByDBGroupBy`, `JiraComponentID` conversion, and result-map assembly are copy-pasted nearly verbatim between `processAggregateRows` and `queryTestDetails`. Extracting a shared helper would remove ~50 duplicated lines and prevent the two paths drifting apart.

### [nit] new prefix-sum/GA query builders bypass existing shared query abstraction
- where: `pkg/api/componentreadiness/dataprovider/postgres/provider.go:261-345` vs `pkg/api/componentreadiness/dataprovider/postgres/cr_queries.go:107-268`
- concern: `cr_queries.go` already factors the prefix-sum-vs-GA split into a `testStatusSpec`/`queryTestStatus` abstraction used by `queryTestStatusPrefixSum`/`queryBaseTestStatusGA`. `buildAggregatePrefixSumQuery`/`buildAggregateGAQuery` reimplement the same shape with hand-written SQL instead of reusing/extending that abstraction. Not wrong, but doubles the surface area a future schema change to these tables must touch.

### [nit] unexplained nolint suppression
- where: `test/integration/component_readiness_test.go:109`
- concern: `//nolint:unparam` added to `createGARawData` with no comment on why the flagged parameter can't be removed; project convention favors comments that explain "why", not just suppressing the linter.

### [question] is Postgres-only fallback intentionally exempt from data-provider parity rule?
- where: `pkg/api/componentreadiness/dataprovider/bigquery/provider.go:93-111`, `pkg/api/componentreadiness/dataprovider/interface.go:27`
- concern: the interface signature change touches BigQuery/Mixed providers, but the actual fallback behavior is Postgres-only (BQ always returns `nil` explanation). Presumably intentional since this targets a Postgres-specific historical backfill gap, but worth stating explicitly in the PR description.

## Checked
- gofmt clean on all touched files.
- Prefix-sum delta pattern (self-join on `test_cumulative_summaries` with `COALESCE`, `HAVING SUM(...) > 0`) matches the existing pattern in `queryTestStatusPrefixSum`/`cr_queries.go` — not a new risk.
- Only `total_count` delta is checked in the `HAVING` filter (not success/flake deltas) — matches existing precedent in `cr_queries.go:163`, not a regression introduced here.
- Frontend (`sippy-ng/src/component_readiness/TestDetailsReport.jsx`) already renders `explanations` generically; no frontend change needed.
- No `docs/features/*` doc references this code path; no README references the changed API; docs-in-same-PR rule not triggered.
- Confirmed via BigQuery query-generator trace that `ProwJobRunID` is effectively always non-empty for legitimate per-run rows on both providers, so the `ProwJobRunID != ""` filter is a no-op for BQ and only filters the new Postgres synthetic rows it targets.

## Open questions
- Can a base release realistically have partial per-run backfill (some jobs/tests present in `prow_job_run_tests`, others not) within the same request batch, or is the per-release backfill always all-or-nothing? This determines whether the blocking finding above is a real bug or a non-issue.
- Is there a plan to add the `IsAggregate` field described in the PR/commit message, or was the empty-`ProwJobRunID` sentinel a deliberate simplification?
- Is a BigQuery-side equivalent of this fallback planned, or is this permanently Postgres-only (dev/local use) by design?
