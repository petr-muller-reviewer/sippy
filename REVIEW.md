---
pr: openshift/sippy#3914
title: "TRT-2824: Improve Sippy's DB Seeding"
head_sha: 5e0b5af51b1c3f9b3d58b52ad02f9abce3f77715
base: main
reviewed_at: 2026-08-21T10:22:26Z
verdict: request-changes
refresh_log:
  - old_sha: f3fd12bb09a461c276de15f0081fba33b7c28656
    new_sha: 5e0b5af51b1c3f9b3d58b52ad02f9abce3f77715
    summary: "Reverted the queryTestDetails lateral join / JobLabels-JobSymptoms provider change entirely per reviewer feedback; added a 4.22-gcp-only seed view so two views share one TestRegression (multi-view regression seed coverage); skipped cache writes for views with prime_cache disabled. PR merged."
---

## What this PR does
- Expands `cmd/sippy/seed_data.go` to seed release payloads (`release_tags`, `release_job_runs`, `release_pull_requests`, `release_repositories`), bugs/Jira linkage, symptom-to-job-run linkage, and periodic-job test output text — areas that previously had no local seed data.
- Adds 2 new component-readiness views to `config/seed-views.yaml`, including a `4.22-amd64-vs-arm64` cross-compare view (replacing an earlier, disputed `4.22-flake-as-failure` view).
- Refactors `syncRegressions` to aggregate active regression IDs per release across all enabled views before closing stale ones (fixing a prior bug where a second view could re-close a first view's still-active regression).
- Adds a production change to `pkg/api/componentreadiness/dataprovider/postgres/provider.go`: a `LEFT JOIN LATERAL` in `queryTestDetails` populating `JobLabels`/`JobSymptoms` per test-details row, closing a BigQuery/Postgres parity gap for the test-details symptom viewer.
- Adds e2e tests (`test/e2e/release_payloads_test.go`, `test/e2e/test_outputs_test.go`) exercising the new seed data through existing API endpoints.
- Went through 3 rounds of CodeRabbit review and a staging walkthrough from `smg247`; several issues were fixed along the way (non-deterministic label assignment, triage-symptom mismatch, an unscoped triage lookup, the flake-as-failure view itself).
- Since previous review: reverted the `queryTestDetails` lateral join and `JobLabels`/`JobSymptoms` provider change entirely (reviewer feedback: "too invasive for this PR"), resolving both should-fix findings on that code below. Added a `4.22-gcp-only` seed view (Platform:gcp subset of `4.22-main`) so two views' regressions land on the same `TestRegression` record, exercising multi-view regression sharing in seed data. Skips cache writes for views with `prime_cache.enabled: false`. PR merged at `5e0b5af51`.

## Findings

### [should-fix] syncRegressions abandons close-loop for all releases if any later view's report generation fails
- where: `cmd/sippy/seed_data.go:989-1033`
- concern: The refactored close-loop runs once, after the per-view loop, using `releaseActiveIDs` accumulated so far. But `GetComponentReport` errors (`seed_data.go:990-995`) trigger an immediate `return`, before the close-loop is ever reached — so a later view's failure prevents closing stale regressions for releases whose views already succeeded earlier in the same run. The production `regressioncacheloader.go` pattern isolates errors per release; this seed implementation does not.
- excerpt: |
    report, reportErrs := componentreadiness.GetComponentReport(ctx, provider, dbc, reportOpts, "")
    if len(reportErrs) > 0 {
        for _, e := range reportErrs {
            rLog.WithError(e).Warn("report generation error")
        }
        return fmt.Errorf("error generating component report for view %s", view.Name)
    }

### [nit] New seeding helpers use plain Create() instead of this file's idempotent FirstOrCreate pattern
- where: `cmd/sippy/seed_data.go:1480` (and similar `Create()` calls in `seedReleasePayloads`/`seedBugsAndTriases`)
- concern: `dbc.DB.Create(&tag)` duplicates `ReleaseTag`/`ReleaseRepository`/`ReleaseJobRun`/`Bug` rows if the documented seed workflow is re-run against an already-seeded database, unlike the rest of this file which uses `FirstOrCreate` on a natural key for idempotency.
- excerpt: |
    if err := dbc.DB.Create(&tag).Error; err != nil {
        return fmt.Errorf("failed to create release tag %s: %w", tagName, err)
    }

### [nit] syncRegressions reimplements RegressionCacheLoader's close-rolled-off logic instead of reusing it
- where: `cmd/sippy/seed_data.go:961-1033` vs `pkg/dataloader/regressioncacheloader/regressioncacheloader.go` (`closeRolledOffRegressions`)
- concern: The same "diff active IDs against current regressions, close the rest" logic exists in production code. Duplicating it here risks the two drifting as the production logic evolves (e.g. batching or locking changes).

### [nit] Per-regression / per-job-run DB round trips in seedRegressionJobRuns and seedSymptomJobRunLinkage
- where: `cmd/sippy/seed_data.go:1582-1624`
- concern: One raw SQL query per regression plus one `Create()` per failed run, inside a loop, rather than batching. This is seed data (not a hot path), but run time scales linearly as more synthetic regressions are added, which has already happened multiple times in this PR's history.

## Checked
- Previously-flagged CodeRabbit issues are fixed as claimed: deterministic ordering before `Limit(4)` (9fcb28f), triage-symptom consistency (9fcb28f), triage lookup scoped by regression IDs instead of a global `Limit(2)` (0a6efda9).
- `sets.Set[uint]` used per repo convention (no hand-rolled `map[X]bool`) in `syncRegressions`.
- `gofmt -l` clean on modified files.
- The `4.22-amd64-vs-arm64` cross-compare view follows the production `5.0-x86-vs-multi-arm` pattern.
- New e2e tests hit endpoints/queries that already exist and correctly filter by seeded `release` values.
- `pkg/api/componentreadiness/dataprovider/postgres/provider.go` and `pkg/db/models/triage.go` are back to their pre-PR state (full revert verified by diff); no residual BigQuery/Postgres parity gap or lateral-join non-determinism remains.

## Open questions
- Is re-running seed-data against an already-seeded database (idempotency) a supported workflow for the new payload/bug/triage helpers, or is a fresh database always assumed?

## Resolved
### [should-fix] Lateral join in queryTestDetails picks an arbitrary regression's labels when a job run is tracked by more than one open regression
- resolution: Reverted entirely in `86eb22b7e` ("Address review: revert provider changes, add multi-view regression support") per reviewer feedback that the change was too invasive for this PR. `provider.go`/`triage.go` are back to pre-PR state.

### [should-fix] JobLabels/JobSymptoms have different semantics in the Postgres vs BigQuery TestDetailsQuerier
- resolution: Moot after the above revert; neither provider carries these fields anymore.
