---
pr: openshift/sippy#3884
title: "Combined CR query with materialized CTEs"
head_sha: 5fdcc5e071b18d889475b1f6841536aa800da8de
base: main
reviewed_at: 2026-08-08T13:24:11Z
verdict: request-changes
---

## Findings

### [blocking] prefixSumSpec GROUP BY drops prow_job_id/lifecycle, corrupting totals when a key vanishes across the window
- where: `pkg/api/componentreadiness/dataprovider/postgres/cr_queries.go:150` (GROUP BY), `cr_queries.go:272-289` (prefixSumSpec CASE-WHEN)
- concern: `buildInnerAggregation`'s `GROUP BY e.test_id, e.suite_id, vg.group_id` (no `prow_job_id`/`lifecycle`) is shared by `prefixSumSpec`, which computes `SUM(CASE WHEN date=lookupEnd ...) - SUM(CASE WHEN date=lookupStart ...)`. The prior self-join design paired each `(test_id, prow_job_id, suite_id, lifecycle)` key's own start/end rows before subtracting. With the new grouping, a key present at `lookupStart` but absent at `lookupEnd` (e.g. a lifecycle reclassification — `pkg/db/cumulativesummary/cumulative_summary.go` deletes+reinserts under a new key on reclassification) still contributes its full `lookupStart` value to the group's `SUM`, with nothing at `lookupEnd` to offset it. This can drive `total_count` negative or otherwise wrong for the whole group, not just the reclassified test.
- excerpt: |
    // buildInnerAggregation, cr_queries.go:150
    GROUP BY e.test_id, e.suite_id, vg.group_id%s

    // prefixSumSpec, cr_queries.go:277-284
    selectCols: `SUM(CASE WHEN e.date = ? THEN e.prefix_sum_runs ELSE 0 END)
      - SUM(CASE WHEN e.date = ? THEN e.prefix_sum_runs ELSE 0 END) AS total_count, ...`
- worked_example: |
    Job's test lifecycle reclassifies 'blocking' -> 'informing' mid-window.
    Old key: prefix_sum_runs=500 at lookupStart, no row at lookupEnd (writer stops updating it).
    New key: prefix_sum_runs=50 at lookupEnd only.
    Old (self-join) result: 50 - 0 = 50 (correct; old key excluded, absent at lookupEnd).
    New (grouped CASE-WHEN) result: SUM(end)=50, SUM(start)=500 -> total_count = 50 - 500 = -450.
    Via `WHERE agg.total_count > 0` this can also silently drop real data (MissingSample/MissingBasis)
    or understate counts when other jobs in the same group are positive.

### [should-fix] prow_jobs join template duplicated verbatim between queryTestStatusCTE and queryCombinedTestStatus
- where: `cr_queries.go:246-250` (inline in `queryTestStatusCTE`), `cr_queries.go:446-451` (`prowJobJoinTemplate` local to `queryCombinedTestStatus`)
- concern: The exact same JOIN predicate (`JOIN prow_jobs pj ON pj.id = e.prow_job_id AND pj.deleted_at IS NULL AND pj.variant_combination_id IN (%s) JOIN vg ON vg.vcid = pj.variant_combination_id`) exists in two places with no shared constant/helper. A future edit to the join (e.g. an added filter predicate) is easy to apply in one copy and miss in the other.
- excerpt: |
    JOIN prow_jobs pj ON pj.id = e.prow_job_id
                    AND pj.deleted_at IS NULL
                    AND pj.variant_combination_id IN (%s)
                JOIN vg ON vg.vcid = pj.variant_combination_id

### [should-fix] combined-query SQL text and bind-arg slice synchronized only by hand-written comments
- where: `cr_queries.go:460-478` (`fullSQL` fmt.Sprintf with 4 UNION ALL branches, `allArgs` built separately)
- concern: `sourcePrefix := "? AS source, "` injects a placeholder per branch, and `allArgs` is assembled by hand in matching order (`sampleCTEArgs, baseCTEArgs, "S", minimumFailure, "S", "B", minimumFailure, "B"`). Nothing structurally pairs branch order to arg order; reordering, adding, or removing a UNION branch requires remembering to update `allArgs` in lockstep, and a mismatch produces silently-wrong bindings or a runtime arg-count error with no test coverage to catch a reordering mistake.
- excerpt: |
    fullSQL := fmt.Sprintf("WITH vg(vcid, group_id) AS (%s),\ncm(group_id, col_group_id) AS (%s),\n%s,\n%s\n%s\nUNION ALL\n%s\nUNION ALL\n%s\nUNION ALL\n%s", ...)
    var allArgs []any
    allArgs = append(allArgs, sampleCTEArgs...)
    allArgs = append(allArgs, baseCTEArgs...)
    allArgs = append(allArgs, "S", minimumFailure)
    allArgs = append(allArgs, "S")
    allArgs = append(allArgs, "B", minimumFailure)
    allArgs = append(allArgs, "B")

### [should-fix] base/GA-window branching logic duplicated between queryCombinedTestStatus and QueryBaseTestStatus
- where: `cr_queries.go:397-410` (`queryCombinedTestStatus`'s `baseIsGA` branch), `pkg/api/componentreadiness/dataprovider/postgres/provider.go:294-306` (`QueryBaseTestStatus`)
- concern: Both places independently decide `gaSpec` vs `prefixSumSpec` based on `baseMatchesGAWindow`. A future change to GA-window eligibility applied to one call site and missed in the other would make the combined/main-report path and the releasefallback path silently disagree on which base-query strategy to use for the same release.
- excerpt: |
    baseIsGA := p.baseMatchesGAWindow(ctx, baseRelease, baseRange)
    var baseSpec testStatusSpec
    if baseIsGA {
        baseWindowDays := baseRange.End.AddDays(-1).DaysSince(baseRange.Start)
        baseSpec = gaSpec(baseRelease, baseWindowDays)
    } else { ... baseSpec = prefixSumSpec(...) }

### [should-fix] queryCombinedTestStatus hand-rolls variant lookup/filter construction instead of reusing prepareVariantQuery
- where: `cr_queries.go:416-451`
- concern: `prepareVariantQuery` (used by the standalone `queryTestStatusCTE` path) already encapsulates variant lookup, group mapping, filter-clause construction, and subquery building. `queryCombinedTestStatus` reimplements this bundle inline for both the sample and base sides. A bug fix to `buildVariantFilterClause` semantics or similar is easy to apply to `prepareVariantQuery` and forget to mirror here, since the two implementations aren't visibly linked.
- excerpt: |
    sampleLookup, err := lookupVariantValues(ctx, p.dbc, sampleIncludeVariants, dbGroupBy)
    ...
    sampleFilterClause, sampleFilterArgs := buildVariantFilterClause(sampleIncludeVariants)
    sampleVarSubquery := "SELECT vc.id FROM variant_combinations vc"
    if sampleFilterClause != "" { sampleVarSubquery += " WHERE " + sampleFilterClause }

## Checked
- BigQuery provider parity: this PR only touches the Postgres provider (`pkg/api/componentreadiness/dataprovider/postgres/`); no BigQuery-side query logic changed, so no parity gap introduced by this PR itself (though the blocking finding is Postgres-specific and worth checking against BigQuery's equivalent aggregation for the same bug).
- `gofmt`/lint on the modified file: no obvious formatting issues observed.
- The failure/placeholder UNION ALL branch templates (`failureBranchTemplate`, `placeholderBranchTemplate`) themselves are unchanged/shared, not duplicated.

## Open questions
- Is the dropped `prow_job_id`/`lifecycle` from `GROUP BY` (cr_queries.go:150) intentional — e.g. is there a reason lifecycle reclassification can't happen within a lookup window in practice — or is this a regression introduced by collapsing the old self-join into CASE-WHEN aggregation?
- Was this rewrite validated against the old self-join query's output on a realistic dataset that includes a lifecycle transition or job variant-combination change within the base/sample window?
- Given the duplication between `queryTestStatusCTE`'s inline join template and `queryCombinedTestStatus`'s `prowJobJoinTemplate`, would it make sense to extract a single shared helper now while both call sites are fresh in mind?
