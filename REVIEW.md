---
pr: openshift/sippy#3674
title: "Move release metadata from BigQuery to PostgreSQL"
head_sha: 35f162ad368c47cf97bf4797f4a64a7410187255
base: main
reviewed_at: 2026-06-24T11:25:01Z
verdict: request-changes
---

## Summary

Creates a `release_definitions` PostgreSQL table storing release metadata (GA dates, capabilities, previous-release chain) previously fetched from BigQuery at runtime. A new `ReleaseDefinitionLoader` syncs BQ rows to PG during load cycles. All consumers migrated from `v1.Release` to `models.ReleaseDefinition` across 39 files. Clean type propagation, thorough caller updates.

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

### [should-fix] First-run bootstrap: all downstream loaders receive empty release definitions

- where: `cmd/sippy/load.go:168-173`
- concern: `releaseDefs` is read from PG before the loader loop constructs loaders. On a fresh database the table is empty. All loaders are appended to a slice and run together afterward, so the `release-definitions` loader populates the table too late. The prow loader's fallback (`if len(releases) == 0`) silently produces zero releases, making the first load cycle a no-op. The comment acknowledges this ("for subsequent runs"), but the first run silently loads nothing with no error.
- excerpt: |
    var releaseDefs []models.ReleaseDefinition
    if dbErr == nil {
        releaseDefs, _ = api.GetReleasesFromDB(context.Background(), dbc)
    }

### [should-fix] Errors from GetReleasesFromDB silently discarded

- where: `cmd/sippy/load.go:173,348`
- concern: Both calls use `_ =` to ignore errors. The first (line 173) is somewhat defensible since the table may not exist on first migration. The second (line 348) runs inside `if f.InitDatabase && dbErr == nil` after the DB was successfully migrated and loaders completed. Ignoring an error here masks real database problems: `ensurePartitionsForReleases` gets an empty slice, creates zero partitions, and logs no warning.
- excerpt: |
    releaseDefs, _ = api.GetReleasesFromDB(context.Background(), dbc)
    ...
    releaseDefs, _ := api.GetReleasesFromDB(context.Background(), dbc)
    err = ensurePartitionsForReleases(dbc, releaseDefs)

### [nit] Redundant import alias

- where: `cmd/sippy/load.go:39`
- concern: `releasedefloader "github.com/openshift/sippy/pkg/dataloader/releasedefloader"` aliases the package to its own name. Drop the alias.
- excerpt: |
    releasedefloader "github.com/openshift/sippy/pkg/dataloader/releasedefloader"

## Checked

- All callers of changed functions (`GetReleasesFromDB`, `BuildReleasesResponse`, `NewReleaseFallbackMiddleware`, `NewComponentReportGenerator`, etc.) were updated to pass `[]models.ReleaseDefinition`.
- `GetReleaseDatesFromDB` produces the same output as the old BigQuery `GetReleaseDatesFromBigQuery` for releases with and without GA dates. Downstream code in `calculateFallbackReleases` safely handles nil Start/End via explicit nil checks.
- `ReleaseDefinition` model schema matches BigQuery source fields. Upsert in `syncReleaseDefinitions` uses proper ON CONFLICT with column updates.
- `HasCapability` on nil/empty `Capabilities` slice is safe (returns false, same as old `map[ReleaseCapability]bool` behavior).
- DB migration adds `ReleaseDefinition` to the auto-migrate list in `db.go`.
- Seed data function handles all synthetic releases and correctly uses `FirstOrCreate`.
- `ReleaseRowToDefinition` conversion preserves all fields from the BQ row including nullable Patch, GADate, DevelStartDate.
- Tests updated throughout: `releasefallback_test.go`, `regressionallowances_test.go`, `queryparamparser_test.go`, `triage_test.go`, `utils_test.go`.
- `QueryReleaseDates` and `QueryReleases` cleanly removed from the `DataProvider` interface and both provider implementations.

## Open questions

- Is the first-run no-op by design? If so, should the load command log a clear message ("no release definitions found, first load will be partial, re-run to load all data") rather than silently proceeding?
- The old Postgres provider's `QueryReleaseDates` derived time ranges from actual prow_job_runs data (MIN/MAX timestamps). The new `GetReleaseDatesFromDB` uses only GA dates. For BigQuery users this is identical behavior, but for Postgres-only users who relied on data-derived ranges, is this intentional?
