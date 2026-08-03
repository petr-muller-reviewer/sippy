---
pr: openshift/sippy#3847
title: "TRT-2821: Fall back to aggregate tables for base stats in test_details"
head_sha: 9d2a7f80a7d94d35263eb75888f51b1e7cdea92a
base: main
reviewed_at: 2026-08-03T21:41:28Z
verdict: needs-discussion
---

## Summary

Reworks test_details to return pre-aggregated `crstatus.TestDetailsSummary` (per job/test, with optional `[]JobRunDetail`) instead of raw `TestJobRunRows`, via new `SummarizeTestJobRuns` (crstatus/summarize.go) applied identically by both BigQuery and Postgres providers. Adds a Postgres-only fallback (`queryBaseAggregateTestDetails`) that queries `test_cumulative_summaries` (prefix-sum, non-GA) or `prow_ga_raw_test_data` (GA releases) when the per-run query returns nothing for the base release, populating base stats without individual run links. Bumps BQ cache key V2->V3 and fixes `TestKeyStr` json serialization (`json:"-"` removed) since the cached type shape changed and grouping must survive the Redis round-trip. Simplifies test_details.go report generation via a new `splitByTestKey` helper.

This is a rework of an earlier WIP revision (head `232123b`) reviewed previously; that review's "IsAggregate flag doesn't exist" and "no end-to-end coverage" findings are resolved in this revision. The "all-or-nothing fallback gate" finding still applies.

## Findings

### [should-fix] all-or-nothing fallback gate for multi-test batches
- where: `pkg/api/componentreadiness/dataprovider/postgres/provider.go:233-249` (`QueryBaseJobRunTestStatus`)
- concern: the aggregate fallback triggers only when `len(result) == 0` across the entire `reqOptions.TestIDOptions` batch. `GenerateComponentTestDetailsReportMultiTest` can call this with multiple test IDs (e.g. cache-priming a view). If even one test/job in that batch has per-run data, the fallback is skipped for every other test in the batch, silently leaving them at zero base stats instead of falling back. Carried over from the prior review round (previously marked blocking against the WIP revision); still unresolved. Likely a non-issue if a release's per-run backfill is truly all-or-nothing, but that assumption isn't asserted or tested.
- excerpt: |
    result, errs := p.queryTestDetails(...)
    if len(errs) > 0 {
        return result, errs
    }
    if len(result) > 0 {
        return result, nil
    }
    log.WithField("release", reqOptions.BaseRelease.Name).
        Info("no per-run base test details found, falling back to aggregate tables")
    return p.queryBaseAggregateTestDetails(ctx, reqOptions)

### [nit] processAggregateRows duplicates queryTestDetails row-processing logic
- where: `pkg/api/componentreadiness/dataprovider/postgres/provider.go:407-472` vs `196-228`
- concern: job-variant batch-fetch, `RequestedVariants` matching, `filterByDBGroupBy`, `JiraComponentID` conversion, and result-map assembly are copy-pasted nearly verbatim between `processAggregateRows` and `queryTestDetails`. Extracting a shared helper would remove the duplication and prevent the two paths drifting apart on future filtering changes.

### [nit] new prefix-sum/GA query builders bypass existing shared query abstraction
- where: `pkg/api/componentreadiness/dataprovider/postgres/provider.go:321-405` vs `pkg/api/componentreadiness/dataprovider/postgres/cr_queries.go` (`testStatusSpec`/`queryTestStatus`)
- concern: `cr_queries.go` already factors the prefix-sum-vs-GA split into a shared abstraction used by `queryTestStatusPrefixSum`/`queryBaseTestStatusGA`. `buildAggregatePrefixSumQuery`/`buildAggregateGAQuery` reimplement the same shape with hand-written SQL instead of reusing/extending it. Doubles the surface area a future schema change to these tables must touch.

### [nit] unexplained nolint suppression
- where: `test/integration/component_readiness_test.go:1055`
- concern: `//nolint:unparam` added to `createGARawData` with no comment explaining why the flagged parameter can't be removed; project convention favors comments that explain "why", not bare suppressions.

### [nit] TestDetailsSummary/JobRunDetail lack json tags, inconsistent with sibling type
- where: `pkg/apis/api/componentreport/crstatus/types.go:51-73`
- concern: `TestJobRunStatuses` uses explicit snake_case json tags; the new `TestDetailsSummary`/`JobRunDetail` structs have none (default Go field names), except `JobRuns` which has `json:",omitempty"`. Not a correctness issue since this type only round-trips through the internal Redis cache (confirmed via `GetDataFromCacheOrGenerate[TestJobRunStatuses]`) and never reaches the frontend directly — cosmetic inconsistency only.

### [question] is Postgres-only fallback intentionally exempt from data-provider parity convention?
- where: `pkg/api/componentreadiness/dataprovider/bigquery/provider.go:88-127`
- concern: BigQuery/Mixed providers only get the type-signature change (`TestJobRunRows` -> `TestDetailsSummary`), no aggregate-fallback logic. Presumably intentional since BQ retains full history and doesn't have the older-release backfill gap Postgres has, but project convention calls for parity between providers on data-provider changes — worth confirming this asymmetry is deliberate and permanent.

## Checked
- gofmt clean; `sets.New[uint]()` used instead of hand-rolled `map[uint]bool` per project convention (postgres/provider.go, processAggregateRows).
- `SummarizeTestJobRuns` accumulates raw counts with `AddTestCount(row.Count, false)` (policy-independent) and callers recompute `SuccessRate` via `Stats.Add(summary.Stats, faf)` with the real `FlakeAsFailure` setting — verified this doesn't reintroduce a double-aggregation bug.
- No SQL injection risk: all query args go through gorm `?` placeholders including `IN (?)` slice expansion for `testIDs`.
- `Explanations: []string{}` replacing the old conditional "base details not available" message is correct — aggregate fallback now supplies real base stats (just no job-run links), and other middleware (regressionallowances, regressiontracker, releasefallback) still append their own explanations independently; nothing else relied on the removed message.
- New end-to-end integration tests (`TestTestDetailsReport_AggregateBaseStats`, `TestTestDetailsReport_LastFailureTracking`, `TestTestDetailsReport_FlakeAsFailure`) exercise `GenerateTestDetailsReport` with aggregate-only base data — closes the "no end-to-end coverage" gap from the prior WIP-revision review.
- `TestQueryBaseJobRunTestStatus_AggregateFallback` covers prefix-sum, GA, precedence-over-per-run, variant filtering, multi-job, zero-delta HAVING exclusion, and no-suite ownership.

## Open questions
- Can a base release realistically have partial per-run backfill within the same request batch (some jobs/tests present in `prow_job_run_tests`, others not), or is per-release backfill always all-or-nothing? Determines whether the all-or-nothing gate finding is a real bug.
- Is a BigQuery-side equivalent of this fallback planned, or is this permanently Postgres-only by design?
