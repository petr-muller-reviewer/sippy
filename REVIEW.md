---
pr: openshift/sippy#3716
title: "[WIP] TRT-2364: Fix timestamp and date type inconsistencies"
head_sha: 9ba64f3e40433fcbfea191c6e1c38c5454f8288e
base: main
reviewed_at: 2026-08-19T11:24:41Z
verdict: request-changes
refresh_log:
  - from: 0ce2e75bb0fd2c61816ab0fa32cb9eac94b24f0c
    to: ae5ec5d39cd554ea8661769ebe23d2a2079309d4
    summary: "Author addressed review feedback: fixed sets.NewString, reverted unintended files, added useStableJSONQueryParam, fixed test_analysis.go time.Now→reportEnd, added type:'date' to DataGrid columns, fixed GridToolbarFilterItem not-field preservation. Temporal/Safari and in-memory Filter findings resolved by author rationale."
  - from: ae5ec5d39cd554ea8661769ebe23d2a2079309d4
    to: 8906ae372d7f8f2c7d2cfd7c6fc38642b8e9bf6a
    summary: "Rebased on main (picks up #3719 job-runtime). Added nil guard to StripJobRunFilters, removed nil check from caller. Import reorder in GridToolbarFilterItem.js. CI passed."
  - from: 8906ae372d7f8f2c7d2cfd7c6fc38642b8e9bf6a
    to: c192dbd6fff9ce836ff7ce4107152e0cbd1870f5
    summary: "Full re-review performed (not an incremental refresh): branch was force-pushed/rebased onto a much newer main (old head not an ancestor of new head), pulling in the CRA->Vite .js->.jsx rename and 71 changed files total. New findings surfaced: a silently dropped OVERLAPS clause in jira.go changing JIRA incident query semantics, and dropped timestamp filter validation in splitJobAndJobRunFilters. Prior findings re-verified against current code."
  - from: c192dbd6fff9ce836ff7ce4107152e0cbd1870f5
    to: 9ba64f3e40433fcbfea191c6e1c38c5454f8288e
    summary: "Another rebase onto newer main (old head again not an ancestor of new head), but verified content-identical: diffed the PR's own commit against its parent at both points and every file the PR touches (jira.go, parameters.go, filterable.go, recent_test_failures.go, JobsDetail.jsx, etc.) is byte-identical except one now-redundant import line in pkg/api/tests.go (cloud.google.com/go/civil, already present via the new base). No findings changed. Activity: author requested @coderabbitai review (2026-08-17), CI e2e passed; no human review comments."
recommended_rereview:
  - at: 2026-08-20T16:50:06Z
    old_sha: 9ba64f3e40433fcbfea191c6e1c38c5454f8288e
    new_sha: 357750ad1313ee2b20e6ea8d49de2eb914d0df21
    reason: "Real (non-mechanical) content changes this time, >100 net lines across areas the existing review touched: pkg/api/job_runs.go, pkg/api/tests.go, pkg/db/functions.go, pkg/sippyserver/server.go, pkg/flags/postgres_benchmarking_test.go."
---

## Summary

Replaces epoch millisecond integers with proper time.Time, civil.Date, and RFC 3339/YYYY-MM-DD serialization across Go backend, PostgreSQL schema, and React frontend. Adds StripJobRunFilters to fix a pre-existing bug where timestamp filters caused errors on /api/jobs. Sets timezone=UTC on all PostgreSQL connections. Rewrites JobsDetail day bucketing with Temporal.PlainDate. Updates project coding standards (CLAUDE.md, APM instructions) with timestamp/date guidelines.

Since last review: branch was rebased onto a much newer `main` (old head `8906ae372` is not an ancestor of the new head `c192dbd6f`), which also pulled in the sippy-ng CRA→Vite migration (`.js` → `.jsx` renames). This warranted a full re-review rather than an incremental refresh. Two new issues surfaced during that re-review: a silently dropped `OVERLAPS` where-clause in `pkg/api/jira.go` that changes which JIRA incidents are returned, and dropped timestamp filter validation in `splitJobAndJobRunFilters`.

Since previous review: branch was rebased again (`c192dbd6f` → `9ba64f3e4`, old head again not an ancestor of new). Verified the PR's own diff is content-identical across the rebase except one now-redundant import line in `pkg/api/tests.go`; no findings changed as a result. Author requested a CodeRabbit review; CI e2e passed; no new human review activity.

## Re-review Recommended

### 2026-08-20T16:50:06Z — 9ba64f3e40433fcbfea191c6e1c38c5454f8288e..357750ad1313ee2b20e6ea8d49de2eb914d0df21
- PR title dropped its `[WIP]` prefix (now "TRT-2364: Fix timestamp and date type inconsistencies"), suggesting the author considers it ready.
- Old head is again not an ancestor of the new head (another rebase), but unlike the prior two rebases this one carries real content changes, not just mechanical rebase drift. `git diff --stat` between the two heads, restricted to the PR's own file list, shows 603 insertions / 336 deletions across 14 files:
  - `pkg/db/functions.go` (+185/-… ) — the `job_results` SQL function signature changed: params renamed (`release`→`p_release`, `start`→`p_start`, etc.) and several columns/params changed from `timestamp without time zone` to `timestamptz`; a `max(prow_job_runs.timestamp)::timestamp without time zone` cast was dropped.
  - `pkg/flags/postgres_benchmarking_test.go` (460 lines changed, largest single-file delta) — adds a new `sortedDateKeys[V any](m map[civil.Date]V) []string` helper alongside the existing `sortedMapKeys`, and reworks call sites to use it for `civil.Date`-keyed maps.
  - `pkg/sippyserver/server.go` (76 lines), `pkg/api/tests.go` (90 lines), `pkg/api/job_runs.go` (53 lines) — all areas this review's existing findings already touch (`splitJobAndJobRunFilters`, filter validation, timestamp/date handling).
  - Also touched: `cmd/sippy/seed_data.go`, `pkg/api/jobs.go`, `pkg/api/releases.go`, `pkg/db/query/test_queries.go`, `sippy-ng/src/build_clusters/BuildClusterDetails.jsx`, `sippy-ng/src/jobs/JobRunsTable.jsx`, `sippy-ng/src/jobs/JobTable.jsx`, `test/integration/job_runs_report_test.go`, `test/integration/jobs_test.go`.
- Why this exceeds "update in place": both the "significant new code in areas existing findings touched" trigger and the "force-pushed/rewritten large sections" trigger fired together. This is unlike the 2026-08-19 refresh, where a similarly-shaped rebase was verified byte-identical in content — this one is not: the SQL function signature and a new generic helper are genuinely new work, not rebase noise.
- Activity since 2026-08-19T11:24:41Z: 2026-08-20T01:41 openshift-merge-bot scheduled required e2e tests; 2026-08-20T02:28 openshift-ci reported all tests passed. No new inline review comments or PR reviews.
- Existing findings from the 2026-08-08 full re-review were not re-verified against this head; they may need re-checking once a full re-review runs, especially the `splitJobAndJobRunFilters`/`pkg/sippyserver/server.go`-related findings given `server.go` changed again.

## Findings

### [blocking] JIRA incident query silently drops a WHERE clause, changing which incidents match
- where: `pkg/api/jira.go:26`
- concern: The prior code had two AND-ed `.Where(...)` clauses: a COALESCE'd `OVERLAPS` (for the `end` column display value) and a second, un-COALESCE'd `(start_time, resolution_time) OVERLAPS (?, ?)` that actually filtered rows. For open/unresolved incidents (`resolution_time IS NULL`), that second clause evaluated to SQL NULL, which excludes the row under `AND` semantics — so open incidents were never returned. This PR's timestamp-type migration collapsed the query to a single `.Where` using the COALESCE'd clause, so open incidents now match whenever active in the queried range. This looks like an accidental behavior change riding along with a type-only refactor: the commit describes type fixes, not incident-filtering semantics, and there's no test on `GetJIRAIncidentsFromDB`/`CalendarEvent`.
- excerpt: |
    // before (main):
    .Where(`(start_time, COALESCE(resolution_time, '`+startOfNextDay+`')) OVERLAPS (?, ?)`, start, end).
    .Where(`(start_time, resolution_time) OVERLAPS (?, ?)`, start, end).
    // after (this PR):
    .Where(`(start_time, COALESCE(resolution_time, ?)) OVERLAPS (?, ?)`, startOfNextDay, start, end).

### [should-fix] splitJobAndJobRunFilters dropped timestamp validation and its error return
- where: `pkg/sippyserver/parameters.go:115-131`
- concern: The previous epoch-ms implementation parsed and validated the `timestamp` filter value here, returning an error the caller turned into an HTTP 400. The new version just routes `timestamp`/`cluster` filter items to `jobRunsFilter` with no parsing, and the function no longer returns an error (both call sites in `server.go` now have nothing to check). A malformed timestamp value is no longer rejected up front — it now fails later and less cleanly, inside `pkg/filter` (unhandled DB type-cast error on Postgres, or a sentinel string that fails query execution on BigQuery via `makeTimestampParam`).
- excerpt: |
    func splitJobAndJobRunFilters(fil *filter.Filter) (*filter.Filter, *filter.Filter) {
    	...
    	for _, f := range fil.Items {
    		switch f.Field {
    		case "timestamp":
    			jobRunsFilter.Items = append(jobRunsFilter.Items, f)
    		case "cluster":
    			jobRunsFilter.Items = append(jobRunsFilter.Items, f)
    		default:
    			jobFilter.Items = append(jobFilter.Items, f)
    		}
    	}
    	return jobFilter, jobRunsFilter
    }

### [should-fix] Postgres timestamp filter path has no validation/logging counterpart to the new BigQuery path
- where: `pkg/filter/filterable.go:163-183` (`FilterFieldToSQL`/`FilterItemToSQL`) vs `pkg/filter/filterable.go:212-219` (`makeTimestampParam`)
- concern: The new BigQuery arithmetic-filter path parses and validates timestamp values via `makeTimestampParam`, logging a structured error (field + value) before substituting a sentinel on failure. The Postgres arithmetic path (`FilterItemToSQL`) has no equivalent — a malformed timestamp value goes straight through as a raw bind parameter with no field/value context logged when it eventually fails as a DB type-cast error. Project convention (CLAUDE.md) calls for parity between the two providers; this is a narrow but real gap in error observability.
- excerpt: |
    makeTimestampParam := func() []bigquery.QueryParameter {
    	t, err := time.Parse(time.RFC3339Nano, f.Value)
    	if err != nil {
    		log.Errorf("Failed to parse timestamp filter value %q for field %s: %v", f.Value, f.Field, err)
    		return makeParam("NOT A TIMESTAMP: " + f.Value)
    	}
    	return makeParam(t)
    }

### [should-fix] RecentTestFailure.Filterable not migrated to the new timestamp type system
- where: `pkg/apis/api/recent_test_failures.go:31-45`
- concern: Every other `Filterable` implementation touched by this PR was migrated so `GetFieldType` returns `ColumnTypeTimestamp` for time fields and `GetTimestampValue` is implemented. `RecentTestFailure` was missed: `GetFieldType` still falls through to `ColumnTypeNumerical` for `first_failure`/`last_failure`/`last_pass`, and `GetNumericalValue` presumably still calls `.Unix()` further down. Currently dormant — `GetRecentTestFailures` filters via direct SQL, not the in-memory `filter.Filter()` path — but it's an inconsistency with the rest of the migration and will silently no-op an RFC3339 filter/sort value if the in-memory path is ever exercised against this type (`filterNumerical` would `strconv.ParseFloat` an RFC3339 string and fail).
- excerpt: |
    func (r RecentTestFailure) GetFieldType(param string) ColumnType {
    	switch param {
    	case "test_name", "suite_name", "jira_component":
    		return ColumnTypeString
    	default:
    		return ColumnTypeNumerical
    	}
    }

### [should-fix] jobRunFields / splitJobAndJobRunFilters still duplicates field ownership across packages (unresolved from prior review round)
- where: `pkg/filter/filterable.go:59` and `pkg/sippyserver/parameters.go:115-131`
- concern: Flagged in the previous review round and still present. Two independent hardcoded lists of job-run-only fields (`timestamp`, `cluster`) live in different packages with no shared constant; the two `switch` case bodies in `splitJobAndJobRunFilters` are now byte-identical (`case "timestamp": ... case "cluster": ...` both just append to `jobRunsFilter.Items`), which could at least collapse to `case "timestamp", "cluster":`. Adding a new job-run-only field requires remembering to update both packages, or it silently misroutes to the wrong query.
- excerpt: |
    var jobRunFields = sets.NewString("timestamp", "cluster")

### [nit] RFC3339 timestamp parsing duplicated with divergent error handling
- where: `pkg/filter/filterable.go:212` (`makeTimestampParam`, BigQuery) and `pkg/filter/filterable.go:652` (`filterTimestamp`, in-memory/Postgres-model path)
- concern: `time.Parse(time.RFC3339Nano, ...)` is implemented independently in both places. One logs and substitutes a sentinel value on failure; the other returns a bare `fmt.Errorf`. A future format change (e.g. also accepting date-only values) has to be applied in both places or the exact BigQuery/Postgres divergence this PR is fixing elsewhere comes back.
- excerpt: |
    comparison, err := time.Parse(time.RFC3339Nano, filter.Value)
    if err != nil {
    	return false, fmt.Errorf("failed to parse timestamp filter value %q: %w", filter.Value, err)
    }

### [nit] JobsDetail.jsx still uses the global Temporal API with no runtime polyfill
- where: `sippy-ng/src/jobs/JobsDetail.jsx:57-58,106,116`
- concern: Re-verified this review round: `JobsDetail.jsx` is unreferenced anywhere else in `sippy-ng/src` (confirmed via grep), consistent with the prior review's finding that this page is unreachable dead code (no route wires to it). `Temporal` is still used with only an ESLint `/* global Temporal */` suppression and no `@js-temporal/polyfill` dependency, so it would crash on load in any current browser if the page ever became reachable again. Downgraded from the prior round's "resolved" status to a nit rather than dropped entirely, since the code still ships in the bundle even if unreachable.
- excerpt: |
    /* global Temporal */
    const day = Temporal.PlainDate.from(result.start)

### [nit] JobsDetail day-bucketing indices kept in sync only by convention
- where: `sippy-ng/src/jobs/JobsDetail.jsx:106`
- concern: The column-header loop walks backward from `endDate` to build day labels, while each result's bucket index is computed forward via `resultDate.until(endDate).days`. They currently agree only because both are anchored to `endDate`; nothing derives them from a single shared sequence. A future change to one (e.g. reordering columns oldest-first) without updating the other would silently misplace results under the wrong date column with no compiler or test signal.

## Checked

- SQL filter generation (orFilterToSQL, andFilterToSQL) correctly removes epoch extraction and passes timestamptz directly
- civil.Date.Scan handles time.Time from PostgreSQL correctly for CountByDate.Date
- Frontend timestamp filter values correctly changed from epoch ms to ISO 8601 strings throughout
- StripJobRunFilters is a correctness improvement (old code would have errored or silently ignored timestamp filters on /api/jobs)
- DataGrid valueGetter correctly returns Date objects for sorting/filtering
- App.jsx timezone-Z stripping hack correctly removed (civil.Date serializes as YYYY-MM-DD)
- releasedates.go gaTime := release.GADate.In(time.UTC) is correct (civil.Date.In returns midnight in the given timezone)
- splitJobAndJobRunFilters signature change (dropping error return) is internally consistent with its callers (both discard the old error), even though it removes validation (see should-fix finding above)
- JobsDetail.jsx confirmed unreferenced from any route/import in sippy-ng/src (dead code, matches prior review round's conclusion)
- toBQStr path is not affected (only used with Test objects, no ColumnTypeTimestamp fields)
- test_analysis.go fix: time.Now() replaced with reportEnd, added upper bound (date <= reportEnd)
- useStableJSONQueryParam hook correctly stabilizes object references via JSON serialization comparison
- GridToolbarFilterItem.jsx preserves the not field when updating filter model
- App.jsx useEffect dependency array fix ([isLoaded]) is correct
- type:'date' added to Regressed Since and Last Failure DataGrid columns
- File renames from .js to .jsx (JobsDetail, GridToolbarFilterItem, App, JobTable, PullRequestsTable, etc.) are extension-only, no behavioral changes riding along with the rename itself

## Open questions

- Was dropping the second `OVERLAPS` where-clause in `GetJIRAIncidentsFromDB` (jira.go:26) intentional? If so, was it validated against the calendar UI showing open incidents correctly? If not, the clause needs to be restored (likely as a second COALESCE'd `.Where` or folded into the single clause).
- Was removing timestamp filter validation from `splitJobAndJobRunFilters` (and its error return) intentional, or just a side effect of dropping the old epoch-ms parsing code? Should equivalent RFC3339 validation be added back so bad input still gets a clean 400 instead of a raw DB/BigQuery error?
- Would it make sense to consolidate the job-run field set (`timestamp`, `cluster`) into a single exported constant shared between `pkg/filter` and `pkg/sippyserver`, eliminating the now fully-duplicated switch cases?
- Is `RecentTestFailure`'s `Filterable` implementation intentionally left on the old numerical/epoch model, or was it missed during the migration?
