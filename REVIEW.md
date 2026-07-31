---
pr: openshift/sippy#3714
title: "TRT-2762: Component Readiness: adding variant filter inflates regression count with unrelated results"
head_sha: a1ffbd396366d04f889fc2b3b98b8e9ad12a479c
base: main
reviewed_at: 2026-07-31T15:45:13Z
verdict: request-changes
refresh_log:
  - from_sha: 3a24e874bd6ce705a434982175d64a538dd2dcd2
    to_sha: a1ffbd396366d04f889fc2b3b98b8e9ad12a479c
    summary: >-
      PR was rebased onto latest main (86 commits of divergence). The
      previously-reviewed Postgres capability-filtering implementation
      (hasCapabilityIntersection in provider.go/provider_test.go) was
      entirely superseded by an independent main-branch refactor into
      cr_queries.go (buildDrilldownFilters). Makefile audit-level
      add+revert pair dropped during rebase (main moved to npx audit-ci).
      Full re-review performed; old findings tied to deleted code
      retired, new findings below.
---

## Summary

Two JS bugs fixed in CompReadyVars.jsx: (1) updateVarsFromView() didn't sync
test_filters (lifecycles/capabilities) from the selected view into React
state, so Generate Report lost the lifecycle filter and inflated regression
count 86 -> 400+; (2) replaceIncludeVariantsCheckedItems /
replaceCompareVariantsCheckedItems mutated state objects in place before
calling their setters, invisible to React's reference-equality change
detection. Both fixes are correct and match existing patterns in the file.
A third, smaller change extends cr_queries.go's buildDrilldownFilters to
apply a top-level reqOptions.Capabilities array-overlap filter, verified
correctly applied to both base/sample and failure/placeholder queries.
Main gap: the actual root-cause fix (test_filters sync) has no automated
test coverage anywhere, only manual browser verification per the PR
description.

## Findings

### [should-fix] No automated test coverage for the root-cause fix (test_filters sync)
- where: `sippy-ng/src/component_readiness/CompReadyVars.jsx:441-453`
- concern: This is the fix for the actual reported bug (86 -> 400+ spurious regressions). There is no unit test for `CompReadyVarsProvider`/`updateVarsFromView` anywhere in `sippy-ng/src/component_readiness/` (only `CompReadyUtils.test.jsx` exists and doesn't touch this provider). The PR's test plan relies entirely on manual browser verification. A state-sync omission like this is easy to reintroduce when a new filter category is added later, and this exact class of bug is what shipped.
- excerpt: |
    if (view.test_filters) {
      if (Object.hasOwn(view.test_filters, 'lifecycles'))
        setTestLifecycles(view.test_filters.lifecycles)
      else setTestLifecycles([])
      if (Object.hasOwn(view.test_filters, 'capabilities'))
        setTestCapabilities(view.test_filters.capabilities)
      else setTestCapabilities([])
    } else {
      setTestLifecycles([])
      setTestCapabilities([])
    }

### [should-fix] New top-level Capabilities SQL filter has no test coverage
- where: `pkg/api/componentreadiness/dataprovider/postgres/cr_queries.go:102-105`
- concern: The two related integration tests (`TestDrillDownBySecondaryCapability` and the PVC test in `test/integration/component_readiness_test.go`) only exercise the singular `reqOptions.TestIDOptions[0].Capability` drilldown field, a different code path (`innerClause`/first `outerClause` branch). No test sets `reqOptions.Capabilities` (plural, top-level array-overlap) directly, so this new branch is unverified.
- excerpt: |
    if len(reqOptions.Capabilities) > 0 {
        f.outerClause += " AND tow.capabilities && ?"
        f.outerArgs = append(f.outerArgs, pq.Array(reqOptions.Capabilities))
    }

### [question] cmd/sippy/seed_data.go Azure addition tests variant exclusion, not the reported lifecycle bug
- where: `cmd/sippy/seed_data.go:200-217, 260-271, 396-406`
- concern: The new Azure job/test verifies ordinary `Platform` variant exclusion from the default view, not the lifecycle-filter regression-count-inflation scenario actually described in the PR. No seed data or e2e assertion reproduces the specific reported bug (view with a lifecycle filter -> Generate Report -> regression count). Is that covered elsewhere, or was this intended as a stand-in regression guard?
- excerpt: |
    // Azure job: Platform:azure is NOT in the default seed view, so results
    // from this job should be filtered out when the default view is active.

### [question] Postgres backend still has no lifecycle filtering (pre-existing, survived rewrite)
- where: `pkg/api/componentreadiness/dataprovider/postgres/cr_queries.go` (buildDrilldownFilters / queryTestStatus)
- concern: Unlike BigQuery (`pkg/api/componentreadiness/dataprovider/bigquery/querygenerators.go:497-503`, sample-only `COALESCE(NULLIF(lifecycle,''),'blocking') IN UNNEST(@Lifecycles)`), the rewritten Postgres query layer still has no `reqOptions.Lifecycles` handling at all. Confirmed low severity: `MixedProvider.providerFor` (`pkg/api/componentreadiness/dataprovider/mixed/provider.go:36-40`) routes test-status queries to BigQuery unless `reqOptions.DataSource == DataSourcePostgres`, a dev/local/CI-seed-only path. This is not introduced by this PR but is worth a tracking issue since it would matter if `--data-provider=postgres` is ever run against production-scale data.
- excerpt: |
    func (p *MixedProvider) providerFor(reqOptions reqopts.RequestOptions) dataprovider.DataProvider {
        if reqOptions.DataSource == reqopts.DataSourcePostgres {
            return p.pg
        }
        return p.bq
    }

## Checked
- Both JS fixes (functional-updater spread; test_filters sync with Object.hasOwn) are correct and consistent with existing patterns (`includeVariantsCheckedItems` already used the object form).
- `compareVariantsCheckedItems` changed from `useState([])` (array abused as a map via direct index mutation) to `useState({})`; all three consumers (`CompReadyUtils.jsx:507` Object.entries, `IncludeVariantCheckboxList.jsx:35-36` `in` operator, `CompReadyTestPanel.jsx:249`) work correctly with either shape, no breakage from the type change.
- New `reqOptions.Capabilities` filter in `cr_queries.go` is applied uniformly to both base (`queryBaseTestStatusGA`) and sample (`queryTestStatusPrefixSum`) paths (both funnel through shared `queryTestStatus`), and to both the failure query (via `queryAndScan`'s `outerQuery` wrapper) and the placeholder query — matches BigQuery parity requirement (capability filter unconditional in `querygenerators.go:486-495`, unlike lifecycle which is sample-only by design).
- `reqOptions.Capabilities` threading confirmed end-to-end: `utils/queryparamparser.go:53-54` (`testCapabilities` query param) -> `TestFilters.Capabilities` (embedded in `RequestOptions`) -> `cr_queries.go`.
- Current PR diff against main is genuinely small (3 files, +69/-24 per `gh pr view`); the large diff between old/new head SHAs is entirely rebase noise from an unrelated main-branch Postgres query-layer refactor, not new PR content.
- Production routing (MixedProvider -> BigQuery by default) confirmed via `cmd/sippy/serve.go` wiring, consistent with prior review.
- `pq` import in `cr_queries.go` is a pre-existing dependency (`github.com/lib/pq`), not newly introduced.

## Open questions
- Is the lifecycle-filter-inflation scenario (the actual reported bug) covered by any e2e test, or only by manual verification?
- Any plan to add a frontend unit test for `updateVarsFromView`'s test_filters sync, given this is the fix for the actual reported regression?
- Any plan to file a tracking ticket for the Postgres-provider lifecycle-filter gap, or is `--data-provider=postgres` considered permanently dev-only and out of scope?
