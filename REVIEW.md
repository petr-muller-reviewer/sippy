---
pr: openshift/sippy#3884
title: "Combined CR query with materialized CTEs"
head_sha: 2d9f0e2ff49ed2466ac988b018187530012a8980
base: main
reviewed_at: 2026-08-20T16:08:18Z
verdict: approve
refresh_log:
  - from_sha: 5fdcc5e071b18d889475b1f6841536aa800da8de
    to_sha: 2d9f0e2ff49ed2466ac988b018187530012a8980
    summary: >-
      Consolidated refresh covering all activity 2026-08-08 to 2026-08-20.
      Author extracted the duplicated prow_jobs join and variant-filter/lookup
      construction into shared helpers (resolves two should-fix findings).
      Reviewer verified the blocking GROUP BY finding against both
      cumulative-summary write paths (pkg/db/cumulativesummary/cumulative_summary.go,
      pkg/dataloader/prowloader/pgwriter/pgwriter.go): every key, once
      created, persists (flat if inactive) on every later date, so the row
      shape the finding assumed can't occur — downgraded from blocking.
      CodeRabbit raised three further nitpicks (planner-hint scope, log
      levels, duplicate buildVariantFilterClause call, untyped source
      parameter in UNION branches); author addressed all of them (or
      explained why not applicable, for the planner-hint one, citing
      pre-existing TRT-2741 benchmark coverage) and CodeRabbit approved.
      Verdict changed from request-changes to approve.
---

## Findings

### [should-fix] base/GA-window branching logic duplicated between queryCombinedTestStatus and QueryBaseTestStatus
- where: `cr_queries.go:380-393` (`queryCombinedTestStatus`'s `baseIsGA` branch), `pkg/api/componentreadiness/dataprovider/postgres/provider.go` (`QueryBaseTestStatus`)
- concern: Both places independently decide `gaSpec` vs `prefixSumSpec` based on `baseMatchesGAWindow`. A future change to GA-window eligibility applied to one call site and missed in the other would make the combined/main-report path and the releasefallback path silently disagree on which base-query strategy to use for the same release.
- excerpt: |
    baseIsGA := p.baseMatchesGAWindow(ctx, baseRelease, baseRange)
    var baseSpec testStatusSpec
    if baseIsGA {
        baseWindowDays := baseRange.End.AddDays(-1).DaysSince(baseRange.Start)
        baseSpec = gaSpec(baseRelease, baseWindowDays)
    } else { ... baseSpec = prefixSumSpec(...) }
- status: still open at `2d9f0e2ff` — not addressed by any commit or comment in this PR. Not resolved, but not a merge blocker: pure duplication risk, no observed correctness issue.

### [should-fix] combined-query SQL text and bind-arg slice synchronized only by hand-written comments
- where: `cr_queries.go:432-444` (`fullSQL` fmt.Sprintf with 4 UNION ALL branches, `allArgs` built separately)
- concern (as originally filed): a `"? AS source, "` bind-parameter placeholder was injected per branch, and `allArgs` was assembled by hand in matching order (`sampleCTEArgs, baseCTEArgs, "S", minimumFailure, "S", "B", minimumFailure, "B"`). Nothing structurally paired branch order to arg order.
- status: PARTIALLY RESOLVED at `2d9f0e2ff`. CodeRabbit independently flagged a related issue (untyped `?` bind parameter in a `UNION ALL` select list can hit Postgres error 42P18 "could not determine data type of parameter") and proposed baking the source tag in as a typed literal (`'S'::text AS source`, `'B'::text AS source`) instead of a bind arg. The author applied this, which as a side effect drops `allArgs` from 6 hand-appended literals to 2 (`minimumFailure` for each side) — meaningfully less to keep in sync, though the remaining 2-arg-per-4-branch pairing is still hand-maintained, not structurally enforced. See CodeRabbit review comment 2026-08-19T14:33:00Z and the fix at PR review comment 2026-08-19T17:31:00Z (mstaeble) / 2026-08-19T17:37:01Z (coderabbitai, nitpick accepted).

## Resolved

### [blocking→verified-not-a-bug] prefixSumSpec GROUP BY drops prow_job_id/lifecycle, corrupting totals when a key vanishes across the window
- where: `pkg/api/componentreadiness/dataprovider/postgres/cr_queries.go:137` (GROUP BY), `cr_queries.go:255-` (prefixSumSpec CASE-WHEN)
- original concern: `buildInnerAggregation`'s `GROUP BY e.test_id, e.suite_id, vg.group_id` (no `prow_job_id`/`lifecycle`) is shared by `prefixSumSpec`, which computes `SUM(CASE WHEN date=lookupEnd ...) - SUM(CASE WHEN date=lookupStart ...)`. The prior self-join design paired each `(test_id, prow_job_id, suite_id, lifecycle)` key's own start/end rows before subtracting. Worry: a key present at `lookupStart` but absent at `lookupEnd` (e.g. a lifecycle reclassification) would still contribute its full `lookupStart` value to the group's `SUM`, with nothing at `lookupEnd` to offset it — potentially driving `total_count` negative or otherwise wrong for the whole group.
- worked_example (as originally filed, now shown to rest on a false premise — see verification): |
    Job's test lifecycle reclassifies 'blocking' -> 'informing' mid-window.
    Old key: prefix_sum_runs=500 at lookupStart, no row at lookupEnd (assumed: writer stops updating it).
    New key: prefix_sum_runs=50 at lookupEnd only.
    Old (self-join) result: 50 - 0 = 50 (correct; old key excluded, absent at lookupEnd).
    New (grouped CASE-WHEN) result, under the assumed row shape: SUM(end)=50, SUM(start)=500 -> total_count = 50 - 500 = -450.
- author_response: |
    (mstaeble, 2026-08-19T14:05:34Z) The self-join and the grouped CASE-WHEN only diverge when a
    key has a lookupStart row but no lookupEnd row. That state can't arise from the
    cumulative-summary loader: any key present on a given date is carried forward to every later
    date. More fundamentally, prefix sums are monotonic by definition, so a key present at start
    but absent at end is a data-consistency violation, not an input the read path should paper
    over. Handling it in the query would just mask a loader/data bug.
- verification: |
    (reviewer, 2026-08-19T16:35:12Z, re-confirmed 2026-08-20T16:08:18Z) Read both write paths
    end-to-end. The author is correct; the finding's premise does not hold under normal operation.

    1. `pkg/db/cumulativesummary/cumulative_summary.go` (batch Backfill path): `insertSQL` (lines
       77-99) rebuilds each day from a `FULL OUTER JOIN` of yesterday's row (`prev`) and today's
       daily totals (`tds`) on the full key (test_id, prow_job_id, suite_id, lifecycle). When a
       key exists in `prev` but has no matching `tds` row today (exactly the reclassification
       case), the join still emits a row via `COALESCE(prev.*, tds.*)` — carrying every prev field
       forward unchanged (+0 to all sums). The old key is never dropped; it persists flat forever.
       The comment above `deleteSQL` ("rows whose daily-total key changed ... are dropped instead
       of persisting forever") does not match what the SQL actually does — read literally, no key
       with a `prev` row is ever excluded from the next day's insert.

    2. `pkg/dataloader/prowloader/pgwriter/pgwriter.go` (incremental per-load path, the one that
       actually drives production ingestion): `ensureCumulativeSummaryRows` (line 527) creates a
       zero-baseline row for any key seen in the batch on-or-before the day being processed if one
       doesn't exist yet; `updateCumulativeSummaries` (line 550) then adds that batch's deltas.
       Both are called once per day for every day from the batch's earliest date through
       "tomorrow", so a key that stops generating new results still gets ensured+updated (flat)
       for every subsequent day the loader processes. `CarryForwardCumulativeSummaries`/
       `carryForwardRelease` (lines 586-624) separately handles full gap days (no batch activity
       system-wide) by copying the entire previous day's row set verbatim into each gap day.

    Net effect: once a `(test_id, prow_job_id, suite_id, lifecycle)` key has a row on any date, it
    has a row (flat if inactive) on every later date, under both write paths. So in the
    reclassification scenario, the "blocking" key's `prefix_sum_runs` is the same flat value at
    `lookupStart` and `lookupEnd` (contributes 0 to `SUM(end) - SUM(start)`), and the "informing"
    key's contribution is its own real start/end row values — the CASE-WHEN sum computes the
    correct total. The row shape the original finding assumed — present at `lookupStart`, absent
    at `lookupEnd` — is not reachable from either write path during normal operation.

    Residual caveat, not blocking: a full historical re-run of `Backfill` after retroactively
    relabeling `test_daily_totals` lifecycle values across history (a data-correction/reprocessing
    operation, not routine ingestion) could in principle produce a gap partway through, if `prev`
    itself was already recomputed under the new label when a later date is processed sequentially.
    Worth a one-line comment in the loader if that path is ever exercised, but not a reason to hold
    this PR.
- resolution: Downgraded from blocking to a documentation note; no SQL change made or required.
    Worth fixing the misleading comment on `deleteSQL` in `cumulative_summary.go:74` separately
    (unrelated to this PR).

### [should-fix→resolved] prow_jobs join template duplicated verbatim between queryTestStatusCTE and queryCombinedTestStatus
- where (original): `cr_queries.go:246-250` (inline in `queryTestStatusCTE`), `cr_queries.go:446-451` (`prowJobJoinTemplate` local to `queryCombinedTestStatus`)
- concern: The exact same JOIN predicate (`JOIN prow_jobs pj ON pj.id = e.prow_job_id AND pj.deleted_at IS NULL AND pj.variant_combination_id IN (%s) JOIN vg ON vg.vcid = pj.variant_combination_id`) existed in two places with no shared constant/helper.
- resolution: RESOLVED at `a895b2666` (2026-08-19) — extracted into `prowJobVariantJoin` (`variants.go`), used by both `queryTestStatusCTE` and `queryCombinedTestStatus`. Raised inline by petr-muller 2026-08-19T10:51:47Z; author replied "Done." 2026-08-19T14:27:25Z.

### [should-fix→resolved] queryCombinedTestStatus hand-rolls variant lookup/filter construction instead of reusing prepareVariantQuery
- where (original): `cr_queries.go:416-451`
- concern: `prepareVariantQuery` already encapsulated variant lookup, group mapping, filter-clause construction, and subquery building for the standalone path; `queryCombinedTestStatus` reimplemented this bundle inline for both sample and base sides.
- resolution: RESOLVED at `a895b2666` (2026-08-19) — both `prepareVariantQuery` and `queryCombinedTestStatus` now call the shared `resolveVariantFilter` (`variants.go`). Raised inline by petr-muller 2026-08-19T11:44:37Z (via LLM-assisted review, linked a proposed fix commit); author replied "Done." 2026-08-19T14:27:36Z. A follow-on nitpick surfaced in the new helper itself (`buildVariantFilterClause` still called twice — see below) and was also fixed.

### [nitpick→resolved] buildVariantFilterClause called twice with identical args inside resolveVariantFilter
- where: `variants.go` (`lookupVariantValues` internally called `buildVariantFilterClause`; `resolveVariantFilter` called it again with the same `includeVariants`)
- source: CodeRabbit, PR review 2026-08-19T14:33:02Z, nitpick.
- resolution: RESOLVED at `2d9f0e2ff` — `lookupVariantValues` now takes `filterClause`/`filterArgs` as parameters instead of computing them itself; `resolveVariantFilter` computes them once and passes them down. Purely a redundant-computation fix (pure, small function), not a correctness issue.

### [nitpick→not-applicable] planner hints (enable_nestloop/enable_sort off) applied globally, not scoped to the combined query
- where: `cr_queries.go:34` (`queryPlannerHints`), applied at both the standalone and combined query paths.
- source: CodeRabbit, PR review 2026-08-19T14:33:02Z (flagged Major/actionable): "A combined-query benchmark does not establish acceptable plans for the standalone paths."
- author_response: (mstaeble, 2026-08-19T17:31:00Z) The hints predate this PR — they originate from the TRT-2741 PostgreSQL CR provider work, which benchmarked the standalone prefix-sum and GA queries with these exact hints. The combined query deliberately reuses the same shared constant so the two paths stay consistent rather than drift.
- resolution: CodeRabbit accepted the clarification (2026-08-19T17:31:42Z): "The finding does not apply," recorded it as a durable learning for future reviews of this file. Reviewer note: consistent with what's visible in the current diff — this PR only reused an existing constant, it didn't introduce these hints or newly apply them to a previously-unhinted path.

### [nitpick→resolved] combined-query Info-level logs on every request
- where: `cr_queries.go` (`queryCombinedTestStatus` scan-complete log, `mergePlaceholders` per-side merge logs)
- source: CodeRabbit, PR review 2026-08-19T14:33:02Z, nitpick: "Every report request then emits three Info lines that report internal query metrics."
- resolution: RESOLVED at `2d9f0e2ff` — both logs changed from `log.Info` to `log.Debug`.

### [nitpick→resolved] untyped `?` bind parameter for UNION branch source tag
- where: `cr_queries.go:432-444`
- source: CodeRabbit, PR review 2026-08-19T14:33:00Z (self-raised in the same pass, not from this reviewer's original findings): an untyped `? AS source` parameter in a `UNION ALL` select list can produce Postgres error 42P18 ("could not determine data type of parameter") depending on how the planner resolves types across branches.
- resolution: RESOLVED at `2d9f0e2ff` — replaced with typed literals `'S'::text AS source` / `'B'::text AS source` baked directly into each branch's template invocation, removing the bind parameter (and 4 of the corresponding `allArgs` entries) entirely. This also meaningfully reduces (though doesn't eliminate) the still-open "SQL text and bind-arg slice synchronized by hand" should-fix finding above.

## Checked
- BigQuery provider parity: this PR only touches the Postgres provider (`pkg/api/componentreadiness/dataprovider/postgres/`); no BigQuery-side query logic changed, so no parity gap introduced by this PR itself.
- `gofmt`/lint on the modified files: no obvious formatting issues observed.
- The failure/placeholder UNION ALL branch templates (`failureBranchTemplate`, `placeholderBranchTemplate`) themselves are unchanged/shared, not duplicated.
- CI (`openshift-ci[bot]`): all required tests passed as of 2026-08-20T02:50:50Z.
- CodeRabbit automated review: APPROVED at 2026-08-19T17:37:05Z, after all its findings were resolved or explained.

## Open questions
- Was this rewrite validated against the old self-join query's output on a realistic dataset that includes a lifecycle transition or job variant-combination change within the base/sample window? (Still open — the write-path verification confirms the assumed carry-forward invariant holds, not that the rewrite was tested end-to-end against a dataset exercising it. Low priority given the invariant is now understood and documented here.)

## Timeline since original review (2026-08-08T13:24:11Z, `5fdcc5e07`)
- 2026-08-19: petr-muller posted an LLM-assisted review (COMMENTED, "fine to merge if you don't find them serious") with the blocking GROUP BY finding and two duplication nitpicks.
- 2026-08-19: mstaeble disputed the blocking finding's premise; extracted both duplicated bundles into shared helpers (`prowJobVariantJoin`, `resolveVariantFilter`) and marked the duplication comments "Done".
- 2026-08-19T14:33: CodeRabbit posted a CHANGES_REQUESTED automated review — one actionable (planner-hint scope) and two nitpicks (log levels, duplicate `buildVariantFilterClause` call) — plus, in the same pass, a further nitpick about the untyped source bind parameter.
- 2026-08-19T16:35: Reviewer independently verified the blocking finding against `cumulative_summary.go` and `pgwriter.go`; confirmed the author's rebuttal and downgraded the finding.
- 2026-08-19T17:31: mstaeble addressed all CodeRabbit findings — either fixing them (log levels, duplicate filter-clause call, typed source literal) or explaining why the planner-hint finding didn't apply (pre-existing TRT-2741 benchmark coverage).
- 2026-08-19T17:37: CodeRabbit accepted the planner-hint explanation and APPROVED.
- 2026-08-20T02:50: CI reported all required tests passed.
- PR remains OPEN, unmerged, no further unresolved feedback as of this refresh.
