---
pr: openshift/sippy#3828
title: "TRT-2814: Eliminate prow_job_runs_report_matview"
head_sha: 7cdc7cb15292280a1f1d41e229583dea85226b7f
base: main
reviewed_at: 2026-07-31T15:45:17Z
verdict: approve
refresh_log:
  - from: 7cdc7cb15292280a1f1d41e229583dea85226b7f
    to: 7cdc7cb15292280a1f1d41e229583dea85226b7f
    summary: "No code change. Title dropped [WIP] prefix and do-not-merge/work-in-progress label was removed (resolves prior open question). CodeRabbit resolved its comment thread and approved (2026-07-31T15:28:26Z). Required e2e test run scheduled by merge-bot and passed (2026-07-31T15:37:17Z)."
---

## Context

Full re-review superseding the prior REVIEW.md/.html (which was pinned to head `f18213b02`, not an ancestor of the current head). Diff gathered fresh via `gh pr diff 3828` / `gh pr view 3828` against current head `7cdc7cb15`.

## What this PR does

- Replaces `prow_job_runs_report_matview` (~90s refresh) with a two-phase live query in `JobsRunsReportFromDB` (`pkg/api/job_runs.go`): phase 1 paginates `prow_job_runs` JOIN `prow_jobs` with conditional PR joins; phase 2 enriches the returned page (test-name arrays, PR data, annotations) concurrently via `errgroup`.
- `test_failures`/`test_flakes` read directly from stored columns on `prow_job_runs` (populated at insert time by the prow loader, TRT-2834) instead of a runtime CTE.
- Refactors `pkg/filter`: exports `FilterItemToSQL` (column-expression based) and `FilterFieldToSQL` (type-aware, array/timestamp handling), replacing unexported `orFilterToSQL`/`andFilterToSQL`. `Filter.ToSQL` now uses `FilterFieldToSQL` uniformly for AND and OR link operators.
- Consolidates ILIKE wildcard escaping into exported `filter.EscapeLikeMetachars`, applied at all 12 ILIKE-pattern-construction sites across `pkg/filter` and `pkg/db/query/cumulative_query.go` — fixes previously-unescaped `%`/`_` in user filter values acting as SQL wildcards.
- Adds a 1387-line integration test suite (`test/integration/job_runs_report_test.go`): pagination (incl. tied-sort-key and no-sort determinism), all filter operators/dispatch paths, sorting, test-name EXISTS-subquery filters, PR/annotation enrichment, multi-PR dedup, unsortable-field rejection.
- Adds `sortable: false` to hidden test-name array columns in the frontend (`JobRunsTable.jsx`), paired with a backend `ValidationError` for unsortable sort fields.

## Findings

### [nit] `FilterItemToSQL` OperatorIsEmpty is not array-aware
- where: `pkg/filter/filterable.go:771-776` (new `FilterItemToSQL`, `OperatorIsEmpty`/`OperatorIsNotEmpty` cases)
- concern: Unlike `FilterFieldToSQL`'s `isEmptyFilter` (which special-cases arrays with `IS NULL OR ARRAY_LENGTH(...) = 0`), `FilterItemToSQL` emits a plain `"%s IS NULL"`/`"%s IS NOT NULL"`. This is documented in the function's doc comment as intentional ("no type-aware transformations... use FilterFieldToSQL when the column may be an array"). None of the current `columnAliases` callers in `job_runs.go` are array-typed, so it's not a live bug. A future caller adding an array-typed column alias and using `isEmpty`/`isNotEmpty` would silently get wrong results.
- excerpt: |
    case OperatorIsEmpty:
        sql = fmt.Sprintf("%s IS NULL", column)
    case OperatorIsNotEmpty:
        sql = fmt.Sprintf("%s IS NOT NULL", column)

### [should-fix] Stale reference to dropped matview in chat/AI-agent tool
- where: `chat/sippy_agent/tools/database_query.py` (not touched by this PR)
- concern: This file documents `prow_job_runs_report_matview` as a queryable table for the sippy-chat natural-language DB tool and includes an example query against it (`FROM prow_job_runs_report_matview m, LATERAL unnest(m.variants) ...`). Confirmed by grep this is the only remaining reference to the matview outside files this PR already updates. Once merged, the chat tool will generate SQL against a nonexistent table. Project convention (CLAUDE.md) is docs/code in the same PR; this is arguably in-scope.
- excerpt: |
    * **`prow_job_runs_report_matview`**: Pre-joined and aggregated data about job runs. Excellent for job pass/fail rates.

### [question] Has the test_failures/test_flakes backfill run against production?
- where: N/A (operational, cross-PR dependency on TRT-2834, already merged per repo history)
- concern: `test_failures`/`test_flakes` are now read directly from stored columns rather than computed live. The loader populates them at insert time going forward; pre-existing rows need a one-time backfill. If it hasn't run against production, job runs older than the backfill would show zero failures/flakes after this ships — a visible regression from the matview's live-computed values. The PR's staging verification (20,446 rows, correctly ordered non-zero failure counts) suggests staging is backfilled; production status is unconfirmed from the diff alone.

### [question] `%q`-based identifier quoting reachable from job-runs filters
- where: `pkg/filter/filterable.go` (`FilterFieldToSQL`), reached via `pkg/api/job_runs.go` (`applyJobRunFilters` default/fallback branch for fields not in `columnAliases`)
- concern: `field := fmt.Sprintf("%q", f.Field)` quotes a client-supplied filter field name using Go string-escaping, not SQL identifier-escaping. Pre-existing pattern, not introduced by this PR, but now also reachable from the new job-runs filter path for any field not in `columnAliases` (e.g. `cluster`, `labels`, `url`). Worth confirming `f.Field` is validated against an allowlist upstream (e.g. via `apitype.JobRun{}`'s `GetFieldType`) before reaching this point.

## Checked

- NOT/HasEntry semantics: old `field IS NULL OR ? != ALL(field)` vs new `NOT(? = ANY(COALESCE(field,'{}')))` — verified equivalent for NULL and non-NULL cases; covered by new `TestListFilteredJobIDs` cases (`hasEntry`/`NOT hasEntry` on `variants`).
- IsEmpty/IsNotEmpty NOT-wrapping in the `FilterFieldToSQL` path (`isEmptyFilter` self-wraps via `WrapNot`) — truth table matches the old `optNot(!f.Not)` logic in `orFilterToSQL`, just expressed differently.
- Duplicate-row fix for multi-PR-linked runs: `LEFT JOIN (SELECT DISTINCT ON(prow_job_run_id) prow_job_run_id, prow_pull_request_id FROM ... ORDER BY prow_job_run_id, prow_pull_request_id DESC)` deterministically dedups; matches PR description's claimed bugfix. Same ordering used consistently in both `addPRJoin` and `enrichJobRunsWithPRData`.
- Pagination determinism: `q.Order("prow_job_runs.id DESC")` always appended after the user's sort field (GORM `.Order()` calls accumulate across invocations), acting as a tiebreaker rather than overriding requested sort. Exercised by `TestJobRunsReport_PaginationStableWithTiedSortKey` and `TestJobRunsReport_PaginationDeterministicWithoutSort`.
- Conditional PR join wiring: joined pre-COUNT only when needed for filtering (avoids inflating/duplicating count rows), post-COUNT when only needed for sorting; enrichment (`enrichJobRunsWithPRData`) skips re-fetching PR data when it was already pulled inline via `needs.needsPRJoin()`.
- Concurrency safety of the 3 parallel `errgroup` enrichment functions writing disjoint struct fields of the same `[]apitype.JobRun` slice — safe under the Go memory model (distinct fields are distinct memory locations), no lock needed; comment above the block explains why.
- ILIKE-escaping fix applied consistently across all named sites; all SQL parameterized via `?` placeholders in both GORM `.Where()` and raw `.Raw()`/`.Exec()` calls; no string-interpolated user filter values found (only static status-code constants via `fmt.Sprintf`).
- `sets.New[string]()` used per project convention (`prColumns`); `gofmt`-clean; `//nolint:gosec` usage on `uint`→`int` ID conversions consistent with existing codebase patterns.
- `test_grid_url`/`brief_name`/`timestamp` column aliases correctly routed through `columnAliases` map (Postgres disallows referencing SELECT aliases in WHERE); non-aliased real columns fall through to the generic `FilterFieldToSQL` path.
- Deleted `Test_BenchmarkJobRunsReportMatview` and the matview definition/registration (`pkg/db/views.go`) are consistent — no other in-repo Go/SQL references to the dropped matview.

## Open questions
- Has the test_failures/test_flakes backfill script been run against production (not just staging)?
- Any plan to update `chat/sippy_agent/tools/database_query.py` in this PR or as a fast-follow?
- Is `f.Field` guaranteed to be allowlist-validated before reaching the `%q`-quoted fallback path in `FilterFieldToSQL`, for fields not covered by `columnAliases`?

## Resolved since previous pass
- [question] "Is the `do-not-merge/work-in-progress` label still accurate?" — resolved: label removed and `[WIP]` dropped from title as of 2026-07-31T15:28Z-ish (title change observed at refresh time). CodeRabbit also independently resolved its comment thread and approved. Required e2e CI run passed.
