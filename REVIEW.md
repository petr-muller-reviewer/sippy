---
pr: openshift/sippy#3678
title: "Add variant_combinations table for efficient matview grouping"
head_sha: c4936c469e2f06ff163858c0d02c2faa467e466e
base: main
reviewed_at: 2026-06-25T15:08:03Z
verdict: approve
gate:
  decision: merge
  gated_at: 2026-06-25T13:51:54Z
  gated_head_sha: c4936c469e2f06ff163858c0d02c2faa467e466e
  reviewed_head_sha: c4936c469e2f06ff163858c0d02c2faa467e466e
---

## Summary

- Introduces `variant_combinations` lookup table mapping each unique `prow_jobs.variants` array to a serial integer ID.
- BEFORE INSERT/UPDATE trigger on `prow_jobs` auto-populates `variant_combination_id` via upsert.
- Denormalizes `variant_combination_id` into `test_daily_summaries` so the matview groups by integer directly, eliminating the two-stage aggregation (per-job pre_agg then per-variant-combo collapse with GROUP BY + SUM).
- Matview query simplified: outer GROUP BY and SUM removed since pre_agg already aggregates at variant_combination_id granularity.
- Benchmarked at 56% faster matview refresh on staging (158s to 69s).

## Gate

**Decision: merge.**

All prior findings are addressed or non-blocking. The CodeRabbit FK constraint concern was fixed by the author and confirmed. The nits below are cosmetic and do not gate merge. No human reviewer has outstanding blocking concerns. `reviewDecision` is `APPROVED`, e2e tests pass.

**Merge risk (Area 2):** No backward-incompatible changes. Model changes are additive only (new struct, new nullable fields). The `variants` column remains in the matview output via the `variant_combinations` join, so all downstream queries in `test_queries.go` and `misc_queries.go` are unaffected. On first deploy, `test_daily_summaries` will be truncated if rows lack `variant_combination_id`, which means a brief matview staleness window until the next refresh. This follows the same pattern as migration 000002 and is acceptable.

**Gating list:** No items gate merge. Missing `lgtm`/`approved` labels are a Prow workflow concern, not a code concern.

## Findings

### [should-fix] No defensive sort on variant arrays before deduplication
- where: `pkg/db/migrations/000003_create_variant_combinations.up.sql:12-23`, `pkg/testidentification/ocp_variants.go:129-138`
- concern: The unique index on `variant_combinations(variants)` uses PostgreSQL array equality, which is order-sensitive (`{a,b} != {b,a}`). There is no explicit sort at the database boundary. The deduplication correctness relies on `filterVariants` (in `ocp_variants.go:151-168`) always producing a canonical order derived from the fixed `importantVariants` slice. If that slice is ever reordered, or if variants are set through a different code path, the same logical variant set would produce a new `variant_combinations` row instead of matching the existing one. Adding a `sort()` in the trigger function or documenting the invariant would make the contract explicit.
- excerpt: |
    -- trigger function (no sort):
    INSERT INTO variant_combinations (variants)
    VALUES (NEW.variants)
    ON CONFLICT (variants) DO UPDATE SET variants = EXCLUDED.variants

    -- Go side (implicit order from importantVariants):
    func (v *openshiftVariants) IdentifyVariants(jobName string) []string {
        allVariants := v.jobVariants[jobName]
        return filterVariants(allVariants, importantVariants)
    }

### [nit] Down migration does not document GORM-managed table dependency
- where: `pkg/db/migrations/000003_create_variant_combinations.down.sql:1-2`
- concern: The down migration drops only the trigger and function. The `variant_combinations` table and `variant_combination_id` columns (on `prow_jobs` and `test_daily_summaries`) are GORM-managed and survive rollback. If someone runs the down migration without reverting the Go code, new prow_job inserts will not populate `variant_combination_id` (trigger gone), producing NULL variants in new matview entries. A comment in the down migration noting this GORM dependency would prevent confusion.
- excerpt: |
    DROP TRIGGER IF EXISTS trg_prow_jobs_variant_combination ON prow_jobs;
    DROP FUNCTION IF EXISTS set_variant_combination_id();

### [nit] Backfill queries in ensureVariantCombinationTrigger run unconditionally on every startup
- where: `pkg/db/db.go:565-576`
- concern: The INSERT INTO variant_combinations / UPDATE prow_jobs backfill queries execute on every startup even after the initial backfill when they return zero rows. The cost is negligible (GORM auto-creates a btree FK index, and PostgreSQL btree indexes include NULLs, so the WHERE IS NULL scan is fast). Wrapping the backfill in an IF EXISTS guard like the truncate check that follows would make intent clearer.
- excerpt: |
    INSERT INTO variant_combinations (variants)
    SELECT DISTINCT variants FROM prow_jobs
    WHERE variants IS NOT NULL AND variant_combination_id IS NULL
    ON CONFLICT (variants) DO NOTHING;

    UPDATE prow_jobs
    SET variant_combination_id = vc.id
    FROM variant_combinations vc
    WHERE prow_jobs.variants = vc.variants
      AND prow_jobs.variants IS NOT NULL
      AND prow_jobs.variant_combination_id IS NULL;

## Checked

- Matview semantic equivalence: pre_agg grouping by variant_combination_id instead of prow_job_id produces the same final result because variant_combination_id maps 1:1 to a variants array, and the old outer GROUP BY + SUM collapsed to this same granularity.
- Matview consumer queries (test_queries.go, misc_queries.go): `vc.variants` aliases to `variants` in the materialized view output, so all existing queries referencing `variants` on the matview continue to work.
- Trigger upsert pattern: `ON CONFLICT DO UPDATE SET variants = EXCLUDED.variants` with RETURNING is the standard PostgreSQL get-or-create pattern pre-PG19. `DO NOTHING` would return zero rows from RETURNING.
- GORM AutoMigrate ordering: `VariantCombination` is listed before `ProwJob` in the schema list, so the FK target table exists when GORM processes ProwJob.
- Collapsed matview: reads from the base matview which still has a `variants` column, so exclusion filters (`NOT ('never-stable' = any(variants))`) work unchanged.
- Window function partition (`PARTITION BY base.id, base.suite_name, base.release`): unchanged from before the PR, does not need variant_combination_id.
- ON CONFLICT clause in daily summary insert: keyed on `(test_id, prow_job_id, suite_id, release, summary_date)`, same as before. The new `variant_combination_id` is an update-target column, not a conflict key.
- Denormalization staleness: if prow_jobs.variants changes, existing test_daily_summaries rows retain the old variant_combination_id until re-aggregated. This is not a regression; the old matview had equivalent staleness via the prow_jobs join (stale until matview refresh).
- Variant array ordering: `filterVariants` (`ocp_variants.go:151-168`) produces a deterministic order governed by the fixed `importantVariants` slice, and `IdentifyVariants` is the sole code path that sets `prow_jobs.Variants`. The invariant holds today but is implicit (see should-fix finding).

## Open questions

- The unique index on `variant_combinations(variants)` relies on array element order for equality. Would it be worth adding a `sort()` in the PL/pgSQL trigger function to normalize the array before lookup, making the deduplication robust against upstream ordering changes?
- The down migration drops only the trigger function (SQL-managed) while leaving the variant_combinations table and FK columns (GORM-managed). Migration 000002 creates test_daily_summaries entirely in SQL and its down drops the table. Was the split intentional here, or would it be cleaner to also create variant_combinations in the SQL migration for consistency?
- Would it be worth adding an IF EXISTS guard around the backfill INSERT/UPDATE block (similar to the truncate guard) to make the "runs only once" intent explicit?
