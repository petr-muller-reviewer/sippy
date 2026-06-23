---
pr: openshift/sippy#3612
title: "Proposed Implementation for Spot Check Jobs"
head_sha: 0b71bbed5549bbdb8c3d784a3fefd7ab7e641f76
base: main
reviewed_at: 2026-06-12T13:23:26Z
verdict: needs-discussion
---

## Findings

### [should-fix] Component name casing mismatch between variant registry and BQ fallback
- where: `pkg/variantregistry/ocp.go:768` vs `pkg/api/componentreadiness/dataprovider/bigquery/provider.go:385`
- concern: The variant setter uses `"Node / kubelet"` (lowercase k) but the BQ COALESCE fallback produces `'Node / Kubelet'` (uppercase K). During transition when both paths can fire, COALESCE prefers the variant value, creating two distinct groups for the same component depending on which branch fires. Pick one casing and use it everywhere.
- excerpt: |
    // ocp.go:768
    {[]string{"-cpu-partitioning"}, "Node / kubelet", "CPU Partitioning"},
    // provider.go:385
    WHEN LOWER(jobs.prowjob_job_name) LIKE '%%cpu-partitioning%%' THEN 'Node / Kubelet'

### [should-fix] SpotCheckSample always-on fallback causes unnecessary BQ queries
- where: `pkg/api/componentreadiness/utils/queryparamparser.go:227-235`
- concern: When no view provides `SpotCheckSample`, it defaults to the sample release dates. This means `SpotCheckJobs` middleware is instantiated and its `Query` runs a BQ query on every component report for all 61 views, not just the one with `spot_check_sample` configured. The middleware `Analyze` correctly skips non-spot-check tests, but the BQ query cost is incurred unconditionally. Consider only falling back for test details drill-down requests with a `spotcheck:` test ID.
- excerpt: |
    if opts.SpotCheckSample == nil && !opts.SampleRelease.Start.IsZero() && !opts.SampleRelease.End.IsZero() {
        opts.SpotCheckSample = &reqopts.Release{
            Name:  opts.SampleRelease.Name,
            Start: opts.SampleRelease.Start,
            End:   opts.SampleRelease.End,
        }
    }

### [nit] Duplicate comment in initializeMiddleware
- where: `pkg/api/componentreadiness/component_report.go:280,287`
- concern: Both lines say `// Initialize all our middleware applicable to this request.` — the first was added by this PR, the second is pre-existing. Remove one.

### [nit] syntheticTestNameFromID does not restore casing
- where: `pkg/api/componentreadiness/middleware/spotcheckjobs/spotcheckjobs.go:306-313`
- concern: `syntheticTestNameFromID` reconstructs the display name from the lowercased test ID without title-casing. The displayed name will be `[spot-check] node / kubelet / cpu partitioning` instead of properly cased. Compare with `syntheticTestName` which receives original-case values.

### [nit] Design doc committed with stale content
- where: `docs/plans/spot-check-jobs.md`
- concern: The 439-line design doc describes an `AnalysisComplete` flag that does not exist in the implementation (replaced by `Analyze` return value), references specific line numbers that have shifted, and uses "what will change" framing. If this is intended as living documentation, update it to reflect the actual implementation. If it was a planning artifact, consider removing it.

### [question] Postgres provider silently returns empty
- where: `pkg/api/componentreadiness/dataprovider/postgres/provider.go:799-814`
- concern: Both spot-check methods return `nil, nil` — no error, no results, no log. If someone runs Component Readiness against Postgres with a spot-check-enabled view, spot-check tests will silently not appear. A `log.Warn` or a comment explaining "BigQuery-only, intentionally no-op" would help debugging.

### [question] Only 5.0-main view has spot_check_sample
- where: `config/views.yaml:12-14`
- concern: Only 1 of 61 views has `spot_check_sample` configured. Is this intentional for initial rollout, or should other release views (4.18-main, 4.19-main, etc.) also get the spot-check window? The PR description mentions spot-check jobs exist across many releases, and the snapshot.yaml changes show spot-check variants assigned to jobs from 4.12 through 4.22.

## Checked
- Analysis middleware refactoring preserves behavioral equivalence: `NewTestPassRate` -> `AllTestsPassRate` -> `FisherExact` reproduces the exact control flow of the old `assessComponentStatus` including confidence initialization, MinimumFailure early return, pity factor, and MissingBasis override.
- Middleware ordering in `initializeMiddleware` is correct: synthetic injection (SpotCheckJobs) -> data adjustment (ReleaseFallback, RegressionTracker, RegressionAllowances) -> analysis (NewTestPassRate, AllTestsPassRate, FisherExact) -> decoration (LinkInjector).
- `Analyze` first-responder-wins semantics in `List.Analyze` correctly short-circuits on first `handled=true`.
- SpotCheckJobs.Analyze status ladder covers all cases: 0 runs, 1 failure (pending retry), 2 failures (significant), 3+ failures (extreme), any passes (not significant).
- BigQuery queries use parameterized values for user-controlled inputs; variant names go through `param.Cleanse` consistent with existing patterns.
- BQ transition strategy (`IN ('spotcheck', 'rare')` + COALESCE fallback) is sound.
- `validateSpotCheckVariants` prevents adding spotcheck tier without component/capability.
- Frontend changes are minimal: `isSpotCheck` conditional hides basis columns/stats without restructuring components. Null-safe since `=== 'spot_check'` is false for undefined.
- Test coverage: unit tests for each new middleware, spot-check analyze logic, variant matching, query filtering.
- `assessComponentStatus` is fully removed from both `component_report.go` and `test_details.go`.
- Regression tracking works with spot-check tests because `RegressionTracker.PostAnalysis` only checks `ReportStatus` value, not basis data.

## Open questions
- Is the always-on SpotCheckSample fallback (queryparamparser.go:227-235) intentional? It means every CR report request for every view runs a spot-check BQ query, even when the view has no spot_check_sample configured.
- Should other release views beyond 5.0-main also get spot_check_sample configured?
- Is `"Node / kubelet"` the correct OCPBUGS component name? The casing is unusual (lowercase k).
