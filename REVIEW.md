---
pr: openshift/sippy#3922
title: "TRT-2884: exclude InfraFailure-labeled runs from summary tables"
head_sha: c378683ee494191f4d4c096494a250eca62ccf53
base: main
reviewed_at: 2026-08-20T16:49:03Z
verdict: approve
---

## What this PR does
- Adds `pkg/db/infrafailure/infrafailure.go`: `RecordInfraFailure` (applies the InfraFailure label to a `prow_job_runs` row and subtracts its counts from summary tables, gated by a row-locked conditional UPDATE) and `SubtractNewInfraFailure` (idempotent variant used from the re-evaluator), plus shared `subtractFromSummaries`.
- Wires inline subtraction into `pkg/api/jobrunscan/reevaluate.go`'s `updatePostgresLabels`: reads+locks the `prow_job_runs` row (`SELECT ... FOR UPDATE`), then either runs `SubtractNewInfraFailure` (merged set newly carries the label) or preserves an existing InfraFailure label so the full-array label replace doesn't clobber it.
- Adds InfraFailure exclusion filters to `pgwriter.go`, `dailysummary.go`, and `test_queries.go`/`job_queries.go` so summary tables stay consistent with the label.
- Changes `LookupProwJobRunPartitionKeys` signature (int64 partition key, gosec G115 fix) and threads `context.Context` through the affected call paths.
- Adds `test/integration/infrafailure_test.go` (419 lines) covering the gap/idempotency scenarios.
- Went through multiple review rounds already (CodeRabbit: race condition, temp table, parenthesization, bind constant, int64 type — all addressed in `b676f7813`).

## Findings

### [nit] SubtractNewInfraFailure re-checks a fact already known from the row lock
- where: `pkg/api/jobrunscan/reevaluate.go:526-545`, `pkg/db/infrafailure/infrafailure.go:224-237`
- concern: `updatePostgresLabels` already does `SELECT labels ... FOR UPDATE` into `currentRun` and evaluates `slices.Contains(currentRun.Labels, infrafailure.LabelInfraFailure)` for the preserve-label branch. When `mergedHasInfraFailure` is true, it calls `infrafailure.SubtractNewInfraFailure(tx, ...)`, which independently re-runs `SELECT 1 FROM prow_job_runs WHERE id = ? AND labels @> ARRAY[?] LIMIT 1` on the same row inside the same transaction. The row lock guarantees this can't have changed since the first read, so it's a pure extra round-trip per re-evaluation call, not a correctness issue. `SubtractNewInfraFailure` could take the already-known containment as a parameter instead of re-querying it.
- excerpt: |
    mergedHasInfraFailure := slices.Contains(merged, infrafailure.LabelInfraFailure)
    ...
    if mergedHasInfraFailure {
        if err := infrafailure.SubtractNewInfraFailure(tx, int64(jobRun.ID)); err != nil {
    ...
    func SubtractNewInfraFailure(tx *gorm.DB, prowJobRunID int64) error {
        var found int
        res := tx.Raw(
            "SELECT 1 FROM prow_job_runs WHERE id = ? AND labels @> ARRAY[?] LIMIT 1",
            prowJobRunID, LabelInfraFailure).Scan(&found)

## Checked
- GORM `.Where()` OR-clause parenthesization: `clause.Where.Build` auto-wraps multi-clause `Expr` containing "or" in parens — correct.
- `RowsAffected` semantics on raw `SELECT ... Scan` in `SubtractNewInfraFailure` — behaves as intended for the idempotency check.
- `INNER JOIN tmp_prow_job_runs` batch-delta exclusion logic in the temp-table path — correct.
- Alleged partition-wise-join risk in `dailysummary.go`/`test_queries.go` — `prow_job_runs` is not a physically partitioned table, so join-pruning concern doesn't apply.
- `LookupProwJobRunPartitionKeys` int64 change addresses gosec G115 correctly.

## Open questions
- None outstanding — prior CodeRabbit rounds already covered race condition, temp table, parenthesization, bind constant, and int64 type concerns, all addressed in `b676f7813`.
