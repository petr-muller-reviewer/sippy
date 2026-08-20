---
pr: openshift/sippy#3914
title: "Expand seed data coverage for release payloads, bugs, symptoms, test outputs, and CR views"
head_sha: f3b0903d28e71f9e985b85d46b0fedad4984ff05
base: main
reviewed_at: 2026-08-19T13:33:06Z
verdict: approve
---

## What this PR does
- Adds `seedReleasePayloads()` to `cmd/sippy/seed_data.go`: 6 `release_tags` (4.21/4.22, nightly/ci streams) with `release_job_runs`, `release_pull_requests`, `release_repositories`, and previous-accepted-tag linkage per release/stream.
- Adds `seedBugsAndTriages()`: 4 `bugs` with Jira-style URLs/statuses, linked to seeded tests/jobs; 2 `triage` records linked to regressions.
- Adds `seedSymptomJobRunLinkage()`: applies label definitions to failed job runs, populates `job_symptoms` on `regression_job_runs`, creates `triage_symptoms`.
- Adds `seedTestOutputsForPeriodicJobs()`: seeds `prow_job_run_test_outputs` for 4 failed periodic-job test cases.
- Adds 2 new component-readiness views to `config/seed-views.yaml` (`4.21-cross-compare`, `4.22-flake-as-failure`).
- Adds e2e tests (`test/e2e/release_payloads_test.go`, `test/e2e/test_outputs_test.go`) exercising the new seed data through existing API endpoints.
- Branch history already includes a fixup commit (`f3b0903d2`) addressing prior CodeRabbit feedback: previous-tag-linkage bug, non-deterministic `Limit()` queries, map-based label/symptom assignment races, multi-view regression-closing bug.

## Findings

None found. Reviewed the diff against `upstream/main` (668 insertions across 4 files), traced the new seeding logic line-by-line, and cross-checked against model definitions and existing API/query code exercised by the new e2e tests.

## Checked
- `sort.SliceStable` + `lastAccepted` map builds a correct per-release/stream previous-accepted chain (hand-traced all 6 synthetic payloads).
- Refactored `syncRegressions` aggregates active regression IDs per release across all enabled views before closing any — matches the production pattern in `pkg/dataloader/regressioncacheloader/regressioncacheloader.go` and fixes the prior "second view re-closes first view's still-active regression" bug.
- `sets.Set[uint]` used instead of hand-rolled `map[X]bool`, per repo convention.
- FK wiring in `seedReleasePayloads` (`ReleaseTagID` as string, composite FKs for `ProwJobRunTestOutput`) matches model/gorm tags.
- `gofmt -l` clean on the modified file.
- New e2e tests hit endpoints/queries that already exist and correctly filter by seeded `release` values.

## Open questions
- None.
