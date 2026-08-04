---
pr: openshift/sippy#3828
title: "TRT-2814: Eliminate prow_job_runs_report_matview"
head_sha: 7cdc7cb15292280a1f1d41e229583dea85226b7f
base: main
reviewed_at: 2026-08-04T11:17:03Z
verdict: approve
---

## What this PR does

- Replaces `prow_job_runs_report_matview` (~90s refresh, blocked data loading) with a two-phase live query in `JobsRunsReportFromDB` (`pkg/api/job_runs.go`): phase 1 paginates `prow_job_runs` JOIN `prow_jobs` with conditional PR joins (added pre-COUNT only if needed for filtering, post-COUNT if only needed for sorting); phase 2 enriches the returned page (test-name arrays, PR data, annotations) concurrently via `errgroup`.
- `test_failures`/`test_flakes` read directly from stored columns on `prow_job_runs` (populated at insert time by the prow loader, TRT-2834) instead of a runtime CTE.
- Refactors `pkg/filter`: exports `FilterItemToSQL` (column-expression based, used for `columnAliases` fields that aren't real base-table columns) and `FilterFieldToSQL` (type-aware, array/timestamp handling), replacing unexported `orFilterToSQL`/`andFilterToSQL`. `Filter.ToSQL` now uses `FilterFieldToSQL` uniformly for AND and OR link operators.
- Consolidates ILIKE wildcard escaping into exported `filter.EscapeLikeMetachars`, applied at all ILIKE-pattern-construction sites across `pkg/filter` and `pkg/db/query/cumulative_query.go` — fixes previously-unescaped `%`/`_` in user filter values silently acting as SQL wildcards.
- Adds a 1387-line integration test suite (`test/integration/job_runs_report_test.go`): pagination (incl. tied-sort-key and no-sort determinism), all filter operators/dispatch paths, sorting, test-name EXISTS-subquery filters, PR/annotation enrichment, multi-PR dedup, unsortable-field rejection.
- Adds `sortable: false` to hidden test-name array columns in the frontend (`JobRunsTable.jsx`), paired with a backend `ValidationError` for unsortable sort fields.
- Removes now-dead `apiRunResults`/`sort`/`limit` in-memory-sort code that predated the matview-based implementation and was unused by it.

## Findings

### [should-fix] Stale reference to dropped matview in chat/AI-agent tool
- where: `chat/sippy_agent/tools/database_query.py:113,132,236` (not touched by this PR)
- concern: This file documents `prow_job_runs_report_matview` as a queryable table for the sippy-chat natural-language DB tool, describes it as the preferred table for job pass/fail rates, and includes an example query against it (`FROM prow_job_runs_report_matview m, LATERAL unnest(m.variants) ...`). Verified by grep this is the only remaining reference to the matview outside files this PR already updates. Once merged, the chat tool will generate SQL against a nonexistent table for any job-pass/fail-rate question. Project convention (CLAUDE.md: "documentation and code belong in the same PR") suggests this is in-scope, or at minimum a fast-follow.
- excerpt: |
    * **`prow_job_runs_report_matview`**: Pre-joined and aggregated data about job runs. Excellent for job pass/fail rates.

### [nit] `FilterItemToSQL`'s IsEmpty/IsNotEmpty is not array-aware
- where: `pkg/filter/filterable.go` (new `FilterItemToSQL`, `OperatorIsEmpty`/`OperatorIsNotEmpty` cases)
- concern: Unlike `FilterFieldToSQL`'s `isEmptyFilter` (which special-cases arrays with `IS NULL OR ARRAY_LENGTH(...) = 0`), `FilterItemToSQL` emits a plain `"%s IS NULL"`/`"%s IS NOT NULL"`. Documented as intentional in the function's doc comment. None of the current `columnAliases` entries used from `job_runs.go` are array-typed, so this isn't a live bug, but a future alias for an array column combined with `isEmpty`/`isNotEmpty` would silently misbehave (NULL semantics differ from "empty array").
- excerpt: |
    case OperatorIsEmpty:
        sql = fmt.Sprintf("%s IS NULL", column)
    case OperatorIsNotEmpty:
        sql = fmt.Sprintf("%s IS NOT NULL", column)

### [question] Has the test_failures/test_flakes backfill run against production?
- where: N/A — operational, cross-PR dependency on TRT-2834 (already merged per repo history)
- concern: `test_failures`/`test_flakes` now come from stored columns populated at insert time rather than a live-computed CTE. Pre-existing rows need a one-time backfill; if that hasn't run in production, job runs older than the backfill would show zero failures/flakes post-merge — a visible regression vs. the matview's always-live values. Staging verification in the PR description (20,446 rows, correctly ordered non-zero counts) suggests staging is backfilled; production status isn't confirmed by the diff.

### [question] `%q`-based identifier quoting reachable from job-runs filters on non-aliased fields
- where: `pkg/filter/filterable.go` (`FilterFieldToSQL`), reached via `pkg/api/job_runs.go` `applyJobRunFilters`'s fallback branch for any filter field not in `columnAliases` (e.g. `cluster`, `labels`, `url`, `test_failures`)
- concern: `field := fmt.Sprintf("%q", f.Field)` quotes a client-supplied filter field name via Go string-escaping, not SQL-identifier-escaping. This is a pre-existing pattern (not introduced here), but the new job-runs filter dispatch now routes more field names through it. Traced that `f.Field` values originate from the frontend's fixed column definitions rather than arbitrary user text, so this looks safe in practice, but worth the author confirming there's no path where an arbitrary string reaches `FilterItem.Field`.

## Checked

- NOT/HasEntry semantics: old `field IS NULL OR ? != ALL(field)` vs new `NOT(? = ANY(COALESCE(field,'{}')))` — verified equivalent for NULL and non-NULL array values; covered by `TestListFilteredJobIDs`'s new `hasEntry`/`NOT hasEntry` cases on `variants`.
- Conditional PR-join wiring: the sort-only join is added to the same `*gorm.DB` object *after* `.Count()` runs but *before* the `ORDER BY pull_request_author` clause (added earlier, inside `applyJobRunFilters`) is rendered into SQL — confirmed this is safe because GORM accumulates clauses into a map and only serializes at execution time; call order of clause-adding methods doesn't matter, only the final clause set at `Scan()` time. Exercised by `TestJobRunsReport_SortByPRFieldDoesNotInflateTotalRows`.
- `q.Order(...)` in `pkg/filter/filterable.go:366` is called without `q = q.Order(...)` reassignment (pre-existing code, untouched by this PR). Verified against vendored GORM source that after the first chained call from a root session (`clone` drops to 0), `getInstance()` returns the same `*DB` pointer, so the bare call does mutate `q` in place here — not a bug, just fragile-looking.
- Duplicate-row fix for multi-PR-linked runs: `LEFT JOIN (SELECT DISTINCT ON(prow_job_run_id) ... ORDER BY prow_job_run_id, prow_pull_request_id DESC)` deterministically dedups; same tie-break ordering used consistently in both the inline `addPRJoin` and the separate `enrichJobRunsWithPRData` raw query. Covered by `TestJobRunsReport_MultiplePRsOnOneRunReturnsSingleRun`.
- Pagination determinism: `q.Order("prow_job_runs.id DESC")` is appended after any user-requested sort field (GORM `.Order()` calls accumulate), acting as a stable tiebreaker rather than overriding the requested sort. Exercised by `TestJobRunsReport_PaginationStableWithTiedSortKey` and `TestJobRunsReport_PaginationDeterministicWithoutSort`.
- Enrichment concurrency: the 3 parallel `errgroup` goroutines each write disjoint struct fields of the same `[]apitype.JobRun` slice elements — safe under the Go memory model (distinct fields are distinct memory addresses), no lock needed; the code comments call this out.
- `enrichJobRunsWithPRData` is skipped in Phase 2 when `needs.needsPRJoin()` already pulled PR columns inline — no redundant query.
- ILIKE-escaping fix applied consistently at every construction site found via grep; all SQL parameterized via `?` placeholders in both GORM `.Where()` and raw `.Raw()` calls; no string-interpolated user filter *values* found (only static status-code integer constants interpolated via `fmt.Sprintf`).
- No BigQuery counterpart exists for this report (only one Go file references `JobsRunsReportFromDB`), so the repo's "provider parity" convention doesn't apply here.
- No dangling references to the removed `escapeLikeMetachars`/`orFilterToSQL`/`andFilterToSQL`/`applyIlikeFilter` identifiers anywhere in `pkg/`.
- `sets.New[string]()` used per project convention (`prColumns`); `//nolint:gosec` usage on `uint`→`int` ID conversions consistent with existing codebase patterns.
- `pkg/api/README.md` and `docs/features/` have no existing references to this endpoint/matview, so no doc update was strictly required by the repo's doc-update rule for this specific change (aside from the chat-tool doc noted above).

## Open questions
- Has the test_failures/test_flakes backfill script been run against production (not just staging)?
- Any plan to update `chat/sippy_agent/tools/database_query.py` in this PR or as a fast-follow, given it still tells the chat agent to query the now-removed matview?
- Is `FilterItem.Field` guaranteed to originate only from a fixed/allowlisted set of frontend column names, never arbitrary user text, given it reaches the `%q`-quoted fallback path in `FilterFieldToSQL`?
