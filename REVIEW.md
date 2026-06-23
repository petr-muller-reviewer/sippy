---
pr: openshift/sippy#3612
title: "OCPQE-32065: Proposed Implementation for Spot Check Jobs"
head_sha: 3e9c92baf29ccd3205c08327505d70a8e3d0d3b1
base: main
reviewed_at: 2026-06-23T20:24:26Z
verdict: needs-discussion
---

## Findings

### [should-fix] Cache key does not include SpotCheckJobSamples
- where: `pkg/api/componentreadiness/component_report.go:182-190`
- concern: `GeneratorCacheKey` cherry-picks fields from `RequestOptions` but omits `SpotCheckJobSamples`. Two requests with identical base/sample releases and variants but different spot-check sample configurations produce the same cache key. The first cached result is silently returned for the second request.
- excerpt: |
    type GeneratorCacheKey struct {
        ReportModified *time.Time
        BaseRelease    reqopts.Release
        SampleRelease  reqopts.Release
        VariantOption  reqopts.Variants
        AdvancedOption reqopts.Advanced
        TestFilters    reqopts.TestFilters
        TestIDOptions  []reqopts.TestIdentification
    }

### [should-fix] Default SpotCheckJobSamples fallback causes BQ queries for 60 of 61 views
- where: `pkg/api/componentreadiness/utils/queryparamparser.go:237-251`
- concern: When a view does not define `spot_check_job_samples`, the fallback creates a default `spotcheck-30d` entry if sample release dates are valid. Since all 60 views without explicit spot-check config have valid dates, `SpotCheckJobs` middleware is instantiated and its `Query` runs a BigQuery scan for every component report. The comment says "so spot-check middleware runs on drill-down requests too," but this also fires for top-level report requests. Gate the fallback on the presence of a `spotcheck-` prefixed `TestID` in `TestIDOptions` (drill-down only), or skip middleware creation at the report level for views without explicit config.
- excerpt: |
    if len(opts.SpotCheckJobSamples) == 0 && !opts.SampleRelease.Start.IsZero() && !opts.SampleRelease.End.IsZero() {
        opts.SpotCheckJobSamples = []reqopts.SpotCheckJobSampleOpts{
            {
                Name: "spotcheck-30d",
                ...
            },
        }
    }

### [should-fix] Appending 'rare' to spotCheckIncludeVariants slice may mutate caller's map entry
- where: `pkg/api/componentreadiness/dataprovider/bigquery/provider.go:477-480,637-640`
- concern: `values := spotCheckIncludeVariants[group]` gets a slice header pointing to the map entry's backing array. `append(values, "rare")` writes into the backing array if there is spare capacity. While the map entry's `len` stays unchanged so the mutation does not accumulate across calls, any concurrent reader of the backing array past the original length could observe the stale `"rare"`. Copy before appending: `values = append([]string(nil), spotCheckIncludeVariants[group]...)`.
- excerpt: |
    values := spotCheckIncludeVariants[group]
    if group == "JobTier" {
        values = append(values, "rare")
    }

### [nit] Spot-check sample resolution duplicated in two files
- where: `pkg/api/componentreadiness/utils/queryparamparser.go:177-195` vs `pkg/dataloader/regressioncacheloader/regressioncacheloader.go:508-524`
- concern: Nearly identical 17-line loops resolve `view.SpotCheckJobSamples` into `reqopts.SpotCheckJobSampleOpts`. If resolution logic changes, both must be updated. Extract a shared helper in `utils`.

### [nit] Dead code in MinimumFailure early-return path
- where: `pkg/api/componentreadiness/middleware/fisherexact/fisherexact.go:83-86`
- concern: At line 77, `status` is set to `crtest.NotSignificant` (0). At line 83, `if status <= crtest.SignificantTriagedRegression` checks 0 <= -200, always false. The explanation append is unreachable. Pre-existing (copied from old `assessComponentStatus`), but the refactoring is a good opportunity to clean it up.
- excerpt: |
    status = crtest.NotSignificant
    ...
    if effectiveMinimumFailure != 0 &&
        (testStats.SampleStats.Total()-samplePass) < effectiveMinimumFailure {
        if status <= crtest.SignificantTriagedRegression {
            testStats.Explanations = append(testStats.Explanations, ...)
        }

### [nit] Duplicate comment in initializeMiddleware
- where: `pkg/api/componentreadiness/component_report.go:280,287`
- concern: Both lines say `// Initialize all our middleware applicable to this request.` Remove one.

## Checked
- Analysis middleware refactoring preserves behavioral equivalence: NewTestPassRate -> AllTestsPassRate -> FisherExact reproduces the exact control flow of the old `assessComponentStatus` including confidence initialization, MinimumFailure early return, pity factor, and MissingBasis override.
- Middleware ordering is correct: synthetic injection (SpotCheckJobs) -> data adjustment (ReleaseFallback, RegressionTracker, RegressionAllowances) -> analysis (NewTestPassRate, AllTestsPassRate, FisherExact) -> decoration (LinkInjector).
- `List.Analyze` first-responder-wins semantics correctly short-circuits on first `handled=true`.
- SpotCheckJobs.Analyze status ladder covers all cases: 0 runs (MissingSample), 1 failure (MissingSample/pending retry), 2 failures (SignificantRegression), 3+ failures (ExtremeRegression), any passes (NotSignificant).
- Go 1.25 loop variable semantics — no loop variable capture bugs in goroutine closures.
- `param.Cleanse` is a no-op for all standard variant names (Platform, Architecture, Network, JobTier), so the cleanV change in QueryJobRuns and BuildComponentReportQuery is safe.
- BQ queries use parameterized values for user-controlled inputs; variant names go through `param.Cleanse` consistent with existing patterns.
- BQ transition strategy (`IN ('spotcheck-30d', 'rare')` + COALESCE fallback) is sound.
- Frontend `isSpotCheck` conditionals are null-safe (`=== 'spot_check'` is false for undefined).
- Test coverage is solid: unit tests for each middleware's Analyze, spot-check status ladder, variant matching, query filtering.
- Synthetic test ID round-trip is correct for current capability names ("CPU Partitioning", "Scaling") — neither contains hyphens.
- Postgres provider stubs return `nil, nil` — acceptable as BigQuery-only feature; comment documents intent.
- Regression tracking works with spot-check tests because `RegressionTracker.PostAnalysis` only checks `ReportStatus` value, not basis data.

## Open questions
- Is the always-on `SpotCheckJobSamples` fallback (queryparamparser.go:237-251) intentional for top-level report requests, or should it be limited to drill-down requests with a `spotcheck-` test ID?
- Should `GeneratorCacheKey` include `SpotCheckJobSamples` to prevent cache collisions between spot-check and non-spot-check results?
