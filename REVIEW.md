---
pr: openshift/sippy#3674
title: "Move release metadata from BigQuery to PostgreSQL"
head_sha: f4321802008b273bffb2ace4c101f2bbafab03f4
base: main
reviewed_at: 2026-06-24T14:12:26Z
verdict: request-changes
refresh_log:
  - old_sha: 35f162ad368c47cf97bf4797f4a64a7410187255
    new_sha: f4321802008b273bffb2ace4c101f2bbafab03f4
    summary: "Partition creation moved before loaders; seed_data uses capability constants; release-definitions added to loaderOrder. One silently-ignored-error instance resolved; first-run bootstrap concern worsened for partitions."
---

## Summary

Creates a `release_definitions` PostgreSQL table storing release metadata (GA dates, capabilities, previous-release chain) previously fetched from BigQuery at runtime. A new `ReleaseDefinitionLoader` syncs BQ rows to PG during load cycles. All consumers migrated from `v1.Release` to `models.ReleaseDefinition` across 39 files. Clean type propagation, thorough caller updates.

Since previous review:
- Partition creation block moved from after loaders back to before loaders (uses the initial `releaseDefs` read, eliminating the second `GetReleasesFromDB` call).
- `seed_data.go` now uses capability constants (`models.CapComponentReadiness`, etc.) instead of string literals.
- `loaderwithmetrics.go` adds `release-definitions` to the `loaderOrder` list so it runs first.

## Findings

### [blocking] ReleaseFallback middleware created with potentially nil dbc, causing panic

- where: `pkg/api/componentreadiness/component_report.go:280-281`
- concern: `initializeMiddleware()` passes `c.dbc` to `NewReleaseFallbackMiddleware` unconditionally, but `c.dbc` can be nil (the nil guard at line 283 for regressiontracker proves this). When `ReleaseFallback.QueryTestDetails` or `getFallbackBaseQueryStatus` runs, it calls `GetReleaseDatesFromDB(ctx, r.dbc, ...)` which dereferences `dbc.DB.WithContext(ctx)` and panics. The test passes nil dbc but only exercises `PreAnalysis`, so the panic path is untested.
- excerpt: |
    if c.ReqOptions.AdvancedOption.IncludeMultiReleaseAnalysis && c.ReqOptions.SampleRelease.PullRequestOptions == nil {
        c.middlewares = append(c.middlewares, releasefallback.NewReleaseFallbackMiddleware(c.dataProvider, c.dbc, c.ReqOptions, c.releaseConfigs))
    }
    if c.dbc != nil {
        c.middlewares = append(c.middlewares, regressiontracker.NewRegressionTrackerMiddleware(c.dbc, c.ReqOptions))

### [should-fix] First-run bootstrap: loaders and partition creation use empty release definitions

- where: `cmd/sippy/load.go:168-186`
- concern: `releaseDefs` is read from PG before the loader loop constructs loaders. On a fresh database the table is empty. All loaders are constructed with empty `releaseDefs`, making the first load cycle a no-op. The new commit moves partition creation from after loaders (where it re-queried and would have found populated data) to before loaders, so partitions are now also created from the empty initial read. On first run: zero loaders load data AND zero partitions are created.
- excerpt: |
    var releaseDefs []models.ReleaseDefinition
    if dbErr == nil {
        releaseDefs, _ = api.GetReleasesFromDB(context.Background(), dbc)
    }
    // Ensure partitions exist for all releases (only when InitDatabase is true)
    if f.InitDatabase && dbErr == nil {
        err = ensurePartitionsForReleases(dbc, releaseDefs)

### [nit] Redundant import alias

- where: `cmd/sippy/load.go:39`
- concern: `releasedefloader "github.com/openshift/sippy/pkg/dataloader/releasedefloader"` aliases the package to its own name. Drop the alias.
- excerpt: |
    releasedefloader "github.com/openshift/sippy/pkg/dataloader/releasedefloader"

## Resolved

### [should-fix] Errors from GetReleasesFromDB silently discarded (second call)

- where: previously `cmd/sippy/load.go:348`
- resolution: The second `GetReleasesFromDB` call (post-loaders, for partition creation) was removed. Partition creation now reuses the initial `releaseDefs` read. Only one silently-ignored error remains (line 173), which is defensible since the table may not exist on first migration.

## Checked

- All callers of changed functions (`GetReleasesFromDB`, `BuildReleasesResponse`, `NewReleaseFallbackMiddleware`, `NewComponentReportGenerator`, etc.) were updated to pass `[]models.ReleaseDefinition`.
- `GetReleaseDatesFromDB` produces the same output as the old BigQuery `GetReleaseDatesFromBigQuery` for releases with and without GA dates. Downstream code in `calculateFallbackReleases` safely handles nil Start/End via explicit nil checks.
- `ReleaseDefinition` model schema matches BigQuery source fields. Upsert in `syncReleaseDefinitions` uses proper ON CONFLICT with column updates.
- `HasCapability` on nil/empty `Capabilities` slice is safe (returns false, same as old `map[ReleaseCapability]bool` behavior).
- DB migration adds `ReleaseDefinition` to the auto-migrate list in `db.go`.
- Seed data function handles all synthetic releases and correctly uses `FirstOrCreate`. Now uses capability constants instead of string literals.
- `ReleaseRowToDefinition` conversion preserves all fields from the BQ row including nullable Patch, GADate, DevelStartDate.
- Tests updated throughout: `releasefallback_test.go`, `regressionallowances_test.go`, `queryparamparser_test.go`, `triage_test.go`, `utils_test.go`.
- `QueryReleaseDates` and `QueryReleases` cleanly removed from `DataProvider` interface and both provider implementations.
- `release-definitions` added to `loaderOrder` in `loaderwithmetrics.go`, ensuring deterministic execution order.

## Open questions

- Is the first-run no-op by design? If so, should the load command log a clear message ("no release definitions found, first load will be partial, re-run to load all data") rather than silently proceeding?
- The old Postgres provider's `QueryReleaseDates` derived time ranges from actual prow_job_runs data (MIN/MAX timestamps). The new `GetReleaseDatesFromDB` uses only GA dates. For BigQuery users this is identical behavior, but for Postgres-only users who relied on data-derived ranges, is this intentional?
- Moving partition creation before loaders means it no longer benefits from the release-definitions loader having populated the table. Was this intentional, or should partition creation remain after loaders?
