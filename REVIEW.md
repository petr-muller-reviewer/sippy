---
pr: openshift/sippy#3834
title: "TRT-2834: Add stored test_flakes column to prow_job_runs"
head_sha: 38d0afd420db8ec1acf6a23e60f3287e66327c9f
base: main
reviewed_at: 2026-07-28T23:55:25Z
verdict: approve
---

## Summary

- Adds denormalized `test_flakes` int column to `prow_job_runs` (GORM `AutoMigrate`, `NOT NULL DEFAULT 0`), mirroring existing `test_failures`.
- Postgres prow loader (`pkg/dataloader/prowloader/prow.go`) now returns and stores a flake count alongside failure count from `prowJobRunTestsFromGCS`, keyed on `sippyprocessingv1.TestStatusFlake` (13).
- `cmd/sippy/seed_data.go` updates both `test_failures` and `test_flakes` in one parameterized `UPDATE`.
- Prerequisite for TRT-2814 (retiring `prow_job_runs_report_matview`). Column unused/unconsumed by any reader in this PR.
- Backfill script for existing rows is provided only in the PR description, not committed to the repo.

## Findings

### [nit] Backfill script not committed to repo
- where: PR description (not a file)
- concern: Project convention keeps docs/runbooks in the same PR as the code; a PL/pgSQL backfill script this important (250k+ rows in prod) living only in the PR description means it's lost from repo history once the PR is merged and the description scrolls out of view.
- excerpt: |
    (backfill script only in gh pr view body, not in a tracked file)

### [nit] No unit test coverage for new flake-counting branch
- where: `pkg/dataloader/prowloader/prow.go:1690-1697`
- concern: `prowJobRunTestsFromGCS`/`buildJobRunResult` have no existing unit tests (pre-existing gap), and this PR adds counting logic without a table-driven test asserting failure vs. flake counts split correctly. Low risk given the pattern mirrors existing `test_failures` code exactly, but this column is about to become load-bearing for TRT-2814.
- excerpt: |
    switch tc.Status {
    case int(sippyprocessingv1.TestStatusFailure):
        failures++
    case int(sippyprocessingv1.TestStatusFlake):
        flakes++
    }

## Checked
- `ProwJobRun` is in the `AutoMigrate` model list (`pkg/db/db.go:152`) and `prow_job_runs` itself is not one of the partitioned tables (only `prow_job_run_tests`/`prow_job_run_test_outputs` are) — the "metadata-only ADD COLUMN" claim in the description holds.
- `writeJobRunBatch`'s INSERT into `prow_job_runs` is insert-only (no `ON CONFLICT`), so no upsert path could silently skip populating `test_flakes`.
- Status constants (`TestStatusFailure`=12, `TestStatusFlake`=13) match usage; consistent with prior commit's move away from magic numbers.
- `seed_data.go` UPDATE correctly parameterizes both status codes instead of hardcoding `12`.
- No BigQuery data provider populates `prow_job_runs` test counts — this is a Postgres-only ingestion table, so no cross-provider parity gap.
- `gofmt -l` clean on all three changed files.

## Open questions
- Is the backfill script going to be committed anywhere (e.g. `scripts/`) for future reference, or is it intentionally one-off/throwaway?
- Any plan to add regression coverage for the counting logic before TRT-2814 starts depending on `test_flakes` being accurate?
