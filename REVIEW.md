---
pr: openshift/sippy#3722
title: "TRT-2741: Expand variant keys and decouple daily summaries from variant_combination_id"
head_sha: fa1cbef53a79e54b095bf12e534955924b8b270e
base: main
reviewed_at: 2026-07-03T12:29:03Z
verdict: approve
---

## Summary

Drops `variant_combination_id` from `test_daily_summaries` and resolves it at matview refresh time via JOIN to `prow_jobs`. Adds 8 new variant keys to `importantVariants`. Introduces a migration MANIFEST with `make verify-migrations` to prevent concurrent-PR version collisions.

## Findings

### [should-fix] Postgres provider fieldMap parity gap widened by new variant keys
- where: `pkg/api/componentreadiness/dataprovider/postgres/provider.go:253-262`
- concern: The `fieldMap` hard-codes only 4 legacy keys (`platform`, `network`, `arch`, `upgrade`). Any other field returns `[]string{}`. BigQuery handles all variant keys dynamically via `UNNEST`. This pre-existing gap is widened by adding 8 new keys to `importantVariants`. CLAUDE.md requires parity between providers.
- excerpt: |
    fieldMap := map[string]string{
        "platform": "Platform",
        "network":  "Network",
        "arch":     "Architecture",
        "upgrade":  "Upgrade",
    }
    variantKey, ok := fieldMap[field]
    if !ok {
        return []string{}, nil
    }

### [question] Down migration truncates daily summaries instead of backfilling
- where: `pkg/db/migrations/000004_drop_daily_summary_variant_combination.down.sql:1`
- concern: TRUNCATE destroys all daily summary data on rollback. Since `prow_job_id` remains in the table and `prow_jobs.variant_combination_id` is intact, a backfill UPDATE would preserve data. The PR description acknowledges this tradeoff, but a slow-but-safe rollback may be preferable to hours of data unavailability.
- excerpt: |
    TRUNCATE test_daily_summaries;
    ALTER TABLE test_daily_summaries ADD COLUMN IF NOT EXISTS variant_combination_id BIGINT;

## Checked

- pre_agg CTE JOIN correctness: groups by `pj.variant_combination_id`, `tds.test_id`, `tds.suite_id`, `tds.release`; produces same aggregation as old design
- Daily summary INSERT SQL: correctly drops `variant_combination_id` from SELECT, GROUP BY, and valueColumns in lockstep
- Migration 000004 up: drops dependent matviews before column drop, matviews recreated by `syncPostgresMaterializedViews`
- MANIFEST mechanism: genuinely prevents silent merge of duplicate migration versions (golang-migrate only detects at runtime, too late)
- verify-migrations.sh: handles edge cases (empty lines, comments, zero-padded numbers) correctly
- Retroactive variant resolution: intentional design, fixes 49% stale `variant_combination_id` values
- `ensureVariantCombinationTrigger`: TRUNCATE removal is safe since column no longer exists after migration
- TestDailySummary struct: field removal matches migration and SQL changes
- No remaining references to `test_daily_summaries.variant_combination_id` in codebase

## Open questions

- The fieldMap parity gap in the Postgres provider predates this PR. Is fixing it in scope here, or should it be a follow-up?
- Is the TRUNCATE in the down migration a deliberate choice for speed, or would a backfill-based rollback be acceptable?
