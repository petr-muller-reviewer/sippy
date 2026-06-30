---
pr: openshift/sippy#3698
title: "Use variant_combinations for query filtering and drop GIN index"
head_sha: 5d890c93462e80f1cd115a52a219fe8384d28be3
base: main
reviewed_at: 2026-06-30T10:11:42Z
verdict: approve
---

## Summary

Migrates all variant-based WHERE clauses from GIN-indexed array operators on `prow_jobs.variants` to integer `variant_combination_id` lookups against the small `variant_combinations` table (~2K rows). Drops the now-unused GIN index via migration 000004. Benchmarks show 1.5x-21x speedups across affected endpoints.

## Findings

### [nit] Identical excluded_vc CTE duplicated in two functions
- where: `pkg/db/query/test_queries.go:116,175`
- concern: The CTE `SELECT id FROM variant_combinations WHERE @excluded && variants` is copy-pasted identically in `TestReportsByVariant` and `TestReportExcludeVariants`. If the exclusion logic changes, both must be updated in lockstep.
- excerpt: |
    WITH excluded_vc AS (
        SELECT id FROM variant_combinations WHERE @excluded && variants
    ),

### [nit] Same variant subquery pattern repeated across 6 call sites
- where: `pkg/api/test_analysis.go:59,63,125,129`, `pkg/db/query/test_queries.go:271,275,304,308`
- concern: Each blocked/allowed variant appends an individual `WHERE variant_combination_id [NOT] IN (SELECT id FROM variant_combinations WHERE ? = any(variants))`. A shared GORM scope would centralize this. Not a bug, the current form is correct.
- excerpt: |
    jq = jq.Where("prow_jobs.variant_combination_id NOT IN (SELECT id FROM variant_combinations WHERE ? = any(variants))", bv)

## Checked

- NULL handling with `NOT IN`: the PR description addresses this explicitly. The trigger sets `variant_combination_id = NULL` only when `variants IS NULL`, and staging confirms zero matview rows have NULL `variant_combination_id`. The semantic change (old pattern included NULL rows, new pattern excludes them) is acceptable given these constraints.
- PlatformInfraSuccess double-counting risk: variant assignment uses a map key (`VariantPlatform`), so only one Platform variant per job is possible. The old query also used `unnest`, so behavior is equivalent.
- GIN index drop safety: remaining `unnest(prow_jobs.variants)` queries are unaffected because GIN indexes do not help `unnest()`. The `&&` operator is now only used against `variant_combinations` (different, small table).
- Per-variant subquery efficiency: N is typically small (~2 variants). Batching into `&&` would change AND to OR semantics for inclusion filters, so separate subqueries are semantically correct.
- Migration 000004 up/down: correct `DROP INDEX IF EXISTS` / `CREATE INDEX IF NOT EXISTS` pair.
- GORM model tag removal: `prow.go` tag change matches the migration.
- `buildCollapsedMatViewSQL` SQL injection: `CollapsedVariantExclusions` is a hardcoded `[]string` literal, not user input. Manual quoting is safe in this context.
- `PlatformInfraSuccess` mixed interpolation: `table` comes from a controlled switch on `period`, not user input. `sql.Named` for the other params is correct.

## Open questions

- The `NOT IN` pattern silently drops rows with NULL `variant_combination_id`. Would it be worth adding a `NOT NULL` constraint on `variant_combination_id` in the matview definitions (or a CHECK) to make this invariant explicit rather than relying on the trigger?
