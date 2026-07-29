---
pr: openshift/sippy#3826
title: "TRT-2737: Remove test_analysis_by_job_by_dates table and BQ loader"
head_sha: d9996493ae47cfa02d77f7f7a348a3b3ec0873e1
base: main
reviewed_at: 2026-07-26T11:25:35Z
verdict: approve
refresh_log:
  - from: d9996493ae47cfa02d77f7f7a348a3b3ec0873e1
    to: d9996493ae47cfa02d77f7f7a348a3b3ec0873e1
    at: 2026-07-26T11:25:35Z
    summary: no code change; WIP label/title removed and ready-for-human-review added (hold label persists); CI e2e passed; CodeRabbit raised and withdrew two findings after author explanation (view-restore necessity, e2e partition coverage gap).
---

## Summary

Phase 2 of TRT-2737. Removes the now-dead `test_analysis_by_job_by_dates` table, its BigQuery loader (`loadDailyTestAnalysisByJob`), GORM model, BQ label constant, partition registration, and dead helpers (`LoadProwJobCache`, `LoadTestCache`) that were only used by the removed loader. Adds migration 000008 to drop the table, its dependent view, and any detached partitions. Updates the `RunMigrations` baseline-detection probe from the removed table to `prow_job_run_tests`. Corrects a stale comment on `plan_cache_mode=force_custom_plan`.

Since previous review: no code changes (head SHA unchanged). Author removed the `[WIP]` title prefix and the `do-not-merge/work-in-progress` label, and the PR gained `ready-for-human-review`; `do-not-merge/hold` is still present. CI ran `/test e2e` and all tests passed. CodeRabbit reviewed and raised two findings, both withdrawn after the author's explanation (see Resolved).

## Findings

### [question] Historical plan doc left unupdated
- where: `docs/plans/trt-1989-phase4-partitioned-tables.md`
- concern: Still extensively references `test_analysis_by_job_by_dates` / `TestAnalysisByJobByDate`. Likely fine to leave as a point-in-time planning record rather than living docs, but worth confirming with the team whether `docs/plans/` docs get updated/archived once their work completes (project convention requires docs+code in the same PR for docs that describe current behavior).

### [question] Migration lock duration on large prod table
- where: `pkg/db/migrations/000008_drop_test_analysis_by_job_by_dates.up.sql`
- concern: The old `plan_cache_mode` comment (now rewritten) mentioned 10k+ partitions on this table causing 17+ minute planner enumeration. Dropping the parent table plus that many partitions in one migration transaction could hold locks/take a while in prod. PR description confirms `make e2e` passed with the migration, but e2e likely has far fewer partitions than prod — worth confirming rollout timing/lock expectations are acceptable.

## Resolved

### [question] PR still marked WIP / on hold despite `approved` label
- where: PR labels
- concern: PR carried `do-not-merge/hold` and `do-not-merge/work-in-progress` plus a `[WIP]` title, alongside an `approved` label.
- resolution: Author removed the `[WIP]` title prefix and the `do-not-merge/work-in-progress` label; PR gained `ready-for-human-review`. `do-not-merge/hold` still present — merge readiness still gated on that, not on WIP status.

### [question] Down-migration should restore the dropped view (raised by CodeRabbit)
- where: `pkg/db/migrations/000008_drop_test_analysis_by_job_by_dates.down.sql`
- concern: CodeRabbit initially flagged that the down migration doesn't recreate `prow_test_analysis_by_variant_14d_view`.
- resolution: Author explained the view had no canonical definition to restore — it was dynamically created by application code removed in Phase 1 (#3747), which also removed the view's registration in `pkg/db/views.go` and its only consumer. CodeRabbit verified against #3747 and withdrew the finding. Consistent with this review's own "Checked" note that the view isn't tracked in `views.go`.

### [question] e2e partition test coverage gap for aggregate tables (raised by CodeRabbit)
- where: `test/e2e/db/partitions/partitions_test.go`
- concern: CodeRabbit noted `test_daily_totals` and `test_cumulative_summaries` aren't covered by the partition lifecycle e2e test.
- resolution: Author clarified this is a pre-existing gap from #3747 (which added those tables without e2e coverage), not a regression introduced by this removal-only PR. CodeRabbit agreed and withdrew the finding.

## Checked

- No remaining references to `TestAnalysisByJobByDate`, `LoadProwJobCache`, `LoadTestCache`, or `ProwLoaderTestAnalysis` anywhere in the Go codebase (grepped tree-wide).
- `down.sql` table/index definitions match the original `000001_create_partitioned_tables.up.sql` definitions exactly (columns, unique index, secondary index) — down migration is a faithful structural restore (data loss is expected/documented).
- `prow_test_analysis_by_variant_14d_view` is not tracked in `pkg/db/views.go` (`PostgresViews`/`PostgresMatViews`), so the `DROP VIEW IF EXISTS` in the migration is the only place this needed handling; no other view-registration update required.
- `RunMigrations` baseline-detection switch from `test_analysis_by_job_by_dates` to `prow_job_run_tests` is sound: both tables originate from the same 000001 baseline migration, and the new marker remains valid after this table is dropped (the old one would not have).
- All imports cleaned up correctly in `prow.go` (`civil`, `pgtype`, `pgx/v4`, `query` package) — no dangling/unused imports.
- Detached-partition cleanup in the up-migration (`DO $$ ... $$` block matching `test_analysis_by_job_by_dates_%`) correctly accounts for the partition lifecycle detaching old partitions before drop.
- Test coverage: `TestGetTestAnalysisByJobFromToDates` removed with its function; `test/e2e/db/partitions/partitions_test.go` updated consistently across all 5 table-name-list occurrences.
- PR description states `make lint`, `make test` (15055 tests), `make e2e`, and manual down-migration schema comparison all passed.

## Open questions

- Now that `[WIP]` and `do-not-merge/work-in-progress` are cleared, what's still blocking removal of `do-not-merge/hold`?
- Should `docs/plans/trt-1989-phase4-partitioned-tables.md` be updated or archived now that this phase is complete?
- Was migration lock/runtime duration on the prod-scale partitioned table validated beyond the e2e environment?
