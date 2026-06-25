---
pr: openshift/sippy#3679
title: "Move release metadata from BigQuery to PostgreSQL"
head_sha: 23f283ff45acde57208061fc51e47c79f5d5516d
base: main
reviewed_at: 2026-06-25T13:24:19Z
verdict: request-changes
gate:
  decision: hold
  gated_at: 2026-06-25T13:34:00Z
  gated_head_sha: 23f283ff45acde57208061fc51e47c79f5d5516d
  reviewed_head_sha: 23f283ff45acde57208061fc51e47c79f5d5516d
---

## Gate

**hold** — No new commits since the local review. Both `should-fix` findings are unaddressed in the current head. The PR also lacks `lgtm` and `approve` labels; the only inline reviewer (sosiouxme) left one comment that the author addressed by discussion but did not follow up with an approval.

**Findings disposition (Area 1)**

- `cmd/sippy/load.go:173` — `releaseConfigs, _ = api.GetReleasesFromDB(...)` — **not-addressed**. Silent error discard still present. An `--init-database` run against a DB where `release_definitions` is absent (migration not applied) or has a schema problem will silently deliver an empty release list to all downstream loaders with no operator signal.
- `cmd/sippy/seed_data.go:495` — `FirstOrCreate` instead of upsert — **not-addressed**. Re-seeding the dev database after any seed value change silently leaves stale data.
- `pkg/sippyserver/server.go:195` — caching concern from sosiouxme — **addressed-by-discussion**. Author gave a detailed rationale (7ms PG query vs. 50-200ms HTTP overhead, transitional callers). No code change; sosiouxme did not follow up with `/lgtm`. Borderline: technically unresolved from the reviewer's side, but the rationale is compelling and the concern is not blocking-severity.

**Merge risk (Area 2)**

- **Default loader list change** (`cmd/sippy/load.go:104`): `release-definitions` is now first in the default `--loader` list. Any invocation of `sippy load` without explicit `--loader` flags will now attempt to run the `release-definitions` loader, which hard-fails with a `CRITICAL error` if `bigqueryErr != nil`. Previous defaults never hard-failed on BQ absence; other BQ-dependent loaders check their own preconditions internally. Blast radius: any deployment running `sippy load` with defaults and without BQ access will now fail the entire load rather than skipping the BQ-dependent step. For the project's production deployments (BQ always configured) this is acceptable, but it narrows the operational envelope compared to before.
- **`QueryReleases` behavioral change in postgres provider**: previously derived the release list from actual `prow_jobs` rows (always self-consistent with the DB), now requires `release_definitions` to be pre-populated. An empty table returns an empty list with nil error — callers that expect releases to be derivable from prow data will silently receive none. This is intentional transitional behavior, but it is a behavioral change to an existing interface.
- No exported API surface changes, no CRD changes, no configuration schema changes beyond the default loader list.

**Process blockers**: PR has no `lgtm` or `approve` labels; CI state is `UNSTABLE` (e2e passed per bot comment, but `mergeStateStatus` may reflect missing labels).

**Gating list**
- `cmd/sippy/load.go:173` — silent error discard unaddressed (should-fix, REVIEW.md)
- `cmd/sippy/seed_data.go:495` — `FirstOrCreate` unaddressed (should-fix, REVIEW.md)
- Missing `lgtm` / `approve` labels (process)

## Summary

- Adds `release_definitions` PG table to store release metadata (GA dates, dev start, capabilities, product, status).
- Adds `release-definitions` loader that syncs BQ `Releases` table into PG.
- Postgres data provider `QueryReleases` now reads from `release_definitions` instead of deriving releases from `prow_jobs` + hardcoded map.
- `Server.getReleases` prefers PG, falls back to BQ when PG is empty or errors.
- Seeds release definitions for the dev database.

## Findings

### [should-fix] DB error silently discarded when reading releaseConfigs
- where: `cmd/sippy/load.go:173`
- concern: `releaseConfigs, _ = api.GetReleasesFromDB(context.Background(), dbc)` drops any DB error. If the table is missing (migration not applied), the DB is overloaded, or there is a schema mismatch, `releaseConfigs` stays empty with no log or warning. All downstream loaders that consume it (`prowLoader`, `releaseloader.New`, `featuregateloader.New`, the regression-cache loader, `ensurePartitionsForReleases`) silently operate on an empty release list.
- excerpt: |
    releaseConfigs := []sippyv1.Release{}
    if dbErr == nil {
        releaseConfigs, _ = api.GetReleasesFromDB(context.Background(), dbc)
    }

### [should-fix] seedReleaseDefinitions uses FirstOrCreate instead of upsert
- where: `cmd/sippy/seed_data.go:495`
- concern: `dbc.DB.Where("release = ?", release).FirstOrCreate(&def)` creates the row once and silently does nothing on subsequent seed runs. The sync loader correctly uses `clause.OnConflict` upsert. If a seed value changes (GA date offset, capabilities, product), re-seeding leaves stale data with no error or warning.
- excerpt: |
    if err := dbc.DB.Where("release = ?", release).FirstOrCreate(&def).Error; err != nil {
        return fmt.Errorf("failed to create release definition %s: %w", release, err)
    }

### [nit] GetReleaseRowsFromBigQuery duplicates GetReleasesFromBigQuery body verbatim
- where: `pkg/api/releases.go:501-527`
- concern: The new function copies the query string, client call, iterator loop, and all error-handling from the existing `GetReleasesFromBigQuery`. The only difference is omitting `transformRelease`. The existing function could call the new one and map `transformRelease` over the result; a future change to the BQ query path must otherwise land in both places.
- excerpt: |
    queryString := fmt.Sprintf("SELECT * FROM `%s` ORDER BY DevelStartDate DESC", client.ReleasesTable)
    q := client.Query(ctx, bqlabel.ReleaseAllReleases, queryString)
    it, err := q.Read(ctx)
    // ... identical iteration pattern

### [nit] syncReleaseDefinitions is non-transactional
- where: `pkg/dataloader/releasedefloader/releasedefloader.go:60-73`
- concern: Each upsert is a separate statement with no wrapping transaction. A mid-loop DB error leaves some rows updated and others not. Recoverable on the next run, but the table is inconsistent during the window. Wrapping in a transaction gives atomic all-or-nothing semantics.
- excerpt: |
    for _, def := range defs {
        err := dbc.DB.Clauses(clause.OnConflict{...}).Create(&def).Error
        if err != nil {
            return fmt.Errorf("upserting release definition %s: %w", def.Release, err)
        }
    }

### [nit] develStart for in-development releases is an arbitrary relative date
- where: `cmd/sippy/seed_data.go:479`
- concern: `develStart := now.AddDate(0, 0, m.gaDays-180)` computes development start as `now - 180 days` for releases with `gaDays == 0`. The value is semantically arbitrary and not tied to actual development start. If `FirstOrCreate` is ever changed to upsert, the stored date would shift on each seed run.
- excerpt: |
    develStart := now.AddDate(0, 0, m.gaDays-180)
    def := models.ReleaseDefinition{
        ...
        DevelopmentStartDate: &develStart,
    }

### [question] No log when PG returns empty with no error and falls back to BQ
- where: `pkg/sippyserver/server.go:195`
- concern: When `GetReleasesFromDB` returns `(nil, nil)` (empty table, no error), the code silently falls through to BQ with no log. Only the error path emits a warning. During first-boot or post-wipe scenarios, an operator monitoring logs sees no indication the BQ fallback is active.
- excerpt: |
    releases, err := api.GetReleasesFromDB(ctx, s.db)
    if err == nil && len(releases) > 0 {
        return releases, nil
    }
    if err != nil {
        log.WithError(err).Warn("error querying releases from database, trying bigquery fallback")
    }

## Checked

- Staging verification covered all key endpoints (`/api/releases`, `/api/releases/health`, `/api/component_readiness`) with 36 real releases synced from BQ.
- `forceRefresh` query parameter: frontend does not use it (grep returned empty), so removing it from `getReleases` is safe.
- `QueryReleaseDates` correctly ignores request options by design (PG derives dates from actual `prow_job_runs` min/max, not filtered by options); the `_` parameter is intentional.
- The `release-definitions` loader is correctly placed first in `loaderOrder` so downstream loaders can rely on an up-to-date table.
- BQ fallback in `getReleases` is reachable when PG is up but the table is empty — the flow falls through the `err == nil && len > 0` guard correctly.
- `DefinitionToRelease` vs `transformRelease`: parallel converters that produce the same `sippyv1.Release` shape from different source types; the `map[ReleaseCapability]bool` for capabilities is required by the existing API type, not a CLAUDE.md violation.

## Open questions

- Should `GetReleasesFromDB` return an error if the table exists but is empty (to distinguish "first run" from "DB problem")? Or is the silent empty-and-fallback behavior intentional for bootstrapping?
- The comment on `releaseConfigs` initialization says "On the first run the table may be empty; the release-definitions loader will populate it for subsequent runs" — but if `release-definitions` is in the default loader list, won't the first run also include it? Is the bootstrapping concern about the loader ordering within a single run?
