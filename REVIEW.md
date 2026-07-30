---
pr: openshift/sippy#3849
title: "[WIP] TRT-2752: Add lifecycle as a dimension in summary tables"
head_sha: a6737647e0a31484a34317cce246c01769149ca2
base: main
reviewed_at: 2026-07-30T17:25:22Z
verdict: needs-discussion
---

## Summary

Adds `lifecycle` (`blocking`/`informing`) as a key column in `test_daily_totals` and `test_cumulative_summaries` (migration `000011`). Daily/cumulative aggregation now buckets by lifecycle instead of collapsing both into one row per (test, job, suite, date). Every self-join on `test_cumulative_summaries` gets `AND s.lifecycle = e.lifecycle` to prevent the row split from becoming a Cartesian product across the join. New integration test `TestMixedLifecycleRowsProduceCorrectCounts` covers exactly that scenario. Chat tool schema docs updated to match.

## Findings

### [should-fix] Postgres CR provider does not apply the existing `Lifecycles` filter
- where: `pkg/api/componentreadiness/dataprovider/postgres/cr_queries.go:210-242` (`queryTestStatusPrefixSum`)
- concern: BigQuery's provider already supports filtering sample results by lifecycle via `reqOptions.Lifecycles` (`pkg/api/componentreadiness/dataprovider/bigquery/querygenerators.go:497-503`, `AND COALESCE(NULLIF(lifecycle,''),'blocking') IN UNNEST(@Lifecycles)`). This PR gives Postgres everything needed to do the same (lifecycle column, correctly joined) but never references `reqOptions.Lifecycles`; the final `GROUP BY e.test_id, e.suite_id, vg.group_id` still combines blocking+informing into one total. A caller setting `testLifecycles` gets filtered results from BigQuery and unfiltered results from Postgres. CLAUDE.md requires BigQuery/Postgres parity for data-provider query-logic changes. May be intentionally deferred given the `[WIP]` label — worth confirming with the author rather than assuming it's an oversight.
- excerpt: |
    FROM test_cumulative_summaries e
    LEFT JOIN test_cumulative_summaries s
        ON s.release = e.release AND s.test_id = e.test_id
        AND s.prow_job_id = e.prow_job_id AND s.suite_id = e.suite_id
        AND s.lifecycle = e.lifecycle AND s.date = ?
    ...
    GROUP BY e.test_id, e.suite_id, vg.group_id

### [question] Primary key column reorder on `test_cumulative_summaries` implies a full index rebuild
- where: `pkg/db/migrations/000011_add_lifecycle_to_summaries.up.sql:16-19`
- concern: Beyond adding `lifecycle`, the migration reorders the PK from `(date, release, ...)` to `(release, date, ...)`, aligned with the table's `LIST by release, RANGE by date` partitioning. Reasonable, but this means a full PK rebuild on what is likely one of the largest tables in the schema, not just an additive column change. Worth confirming the rollout plan (locking/duration) for production, separate from the correctness of the change.
- excerpt: |
    ALTER TABLE test_cumulative_summaries
        DROP CONSTRAINT IF EXISTS test_cumulative_summaries_pkey;
    ALTER TABLE test_cumulative_summaries
        ADD PRIMARY KEY (release, date, test_id, prow_job_id, suite_id, lifecycle);

### [nit] PR description references the wrong migration number
- where: PR description ("migration 000010") vs `pkg/db/migrations/000011_add_lifecycle_to_summaries.{up,down}.sql`
- concern: Stale text, likely left over from before a rebase over the already-merged `000010_drop_test_analysis_by_job_by_dates`. Code is correct; description text only.
- excerpt: |
    Adds `lifecycle` (blocking/informing) as a key column in `test_daily_totals`
    and `test_cumulative_summaries`, with migration 000010

## Checked
- All self-joins on `test_cumulative_summaries` (`cr_queries.go`, `cumulative_query.go`, `feature_gates.go`, `test_queries.go` ×3) got the `s.lifecycle = e.lifecycle` fix.
- `UpdateDateForRelease` (`cumulative_summary.go`) correctly threads `lifecycle` through the `FULL OUTER JOIN` and `COALESCE(prev.lifecycle, tds.lifecycle, 'blocking')`.
- `pkg/api/test_analysis.go` (untouched) only `SUM()`s `test_daily_totals` without a self-join or lifecycle grouping — the new row-per-lifecycle split re-combines into the same totals as before, not a bug.
- `prow_job_run_tests.lifecycle` is `NOT NULL DEFAULT 'blocking'` (migration 000009), so the direct `pjrt.lifecycle` insert in `dailysummary.go`'s `buildInsertSQL` cannot produce a `NULL` that would violate the new `NOT NULL` constraint on `test_daily_totals.lifecycle`.
- `up.sql`/`down.sql` are consistent: down restores the original `idx_test_daily_totals_unique` index and original PK column order.
- `gofmt -l` clean on all modified Go files.
- New integration test directly targets the join-fix bug (mixed-lifecycle rows previously inflating counts via Cartesian self-join) with clear before/after math.

## Open questions
- Is the missing `reqOptions.Lifecycles` filter in the Postgres CR provider intentional scope-narrowing for this PR, or a follow-up gap?
- What's the planned rollout approach for the `test_cumulative_summaries` PK rebuild on production (table size, expected lock duration)?
- Is this PR still blocked by the `do-not-merge/work-in-progress` label for a reason beyond what's visible in the diff?
