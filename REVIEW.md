---
pr: openshift/sippy#3874
title: "[WIP] TRT-2847: Fix aggregate test details variant filter excluding valid jobs"
head_sha: 29203eb82aca1a36b707bd43f3761ae61886d54f
base: main
reviewed_at: 2026-08-07T17:12:38Z
verdict: request-changes
---

## What this PR does

- Fixes `queryBaseAggregateTestDetails` (fallback path used when `prow_job_run_tests` has no per-run data for the base release) returning zero rows for older releases (e.g. 4.21 in the 5.0-main view).
- Root cause: the view's `includeVariants` (JobTier, CGroupMode, ContainerRuntime, Owner) was AND-ed wholesale into the SQL `variant_combinations` array-overlap filter, but older releases' `variant_combinations` rows lack those newer keys, so the filter always failed and returned 0 rows.
- Fix: `filterVariantsByDBGroupBy` (`variants.go`) strips `includeVariants` down to only `dbGroupBy` keys before building the SQL WHERE clause, called from `queryBaseAggregateTestDetails` (`provider.go:532`).
- PR is CLOSED (WIP label), never merged via GitHub (`mergedAt`/`mergeCommit` both null), but its single commit `29203eb82` is already present on `main`.

## Findings

### [blocking] Stripped includeVariants keys are never re-applied, dropping JobTier/Owner/etc. filtering entirely in the aggregate fallback path
- where: `pkg/api/componentreadiness/dataprovider/postgres/variants.go:171-183`, `pkg/api/componentreadiness/dataprovider/postgres/provider.go:532`, `pkg/api/componentreadiness/dataprovider/postgres/provider.go:667-713` (`processAggregateRows`), `pkg/api/componentreadiness/dataprovider/postgres/provider.go:84-105` (`matchRequestedVariants`)
- concern: `filterVariantsByDBGroupBy` removes JobTier/Owner/CGroupMode/ContainerRuntime from the SQL filter, but nothing re-applies them afterward. `processAggregateRows` → `matchRequestedVariants` only checks (a) `reqOptions.TestIDOptions[].RequestedVariants` (a narrow single-test override) and (b) `filterByDBGroupBy`, which only labels the output key, not an exclusion filter. `matchesIncludeVariants` (`provider.go:119`) does the correct check but is wired into an unrelated code path (`provider.go:786`), not into `processAggregateRows`. No earlier stage narrows candidate jobs by these keys either — `reqOptions.VariantOption.IncludeVariants` comes straight from view config (`config/views.yaml`, e.g. `JobTier: [blocking, informing, standard]`, `Owner: [...]`, neither ever in `db_group_by`). Net effect: aggregate base stats will now silently include jobs of any JobTier/Owner/CGroupMode/ContainerRuntime value, not just view-allowed ones — trading a false-negative (0 rows) for a potential false-positive (wrong-tier jobs silently included) whenever a release has a mix of allowed/disallowed values among jobs that otherwise match on dbGroupBy dimensions. This also pushes Postgres out of parity with BigQuery's `NewBaseTestDetailsQueryGenerator` (`bigquery/querygenerators.go` ~line 900), which passes the full unstripped `IncludeVariants` — a parity requirement per project CLAUDE.md.
- excerpt: |
    func filterVariantsByDBGroupBy(includeVariants map[string][]string, dbGroupBy sets.Set[string]) map[string][]string {
        filtered := make(map[string][]string, dbGroupBy.Len())
        for k, v := range includeVariants {
            if dbGroupBy.Has(k) {
                filtered[k] = v
            }
        }
        return filtered
    }

### [blocking] Empty-but-non-nil DBGroupBy silently disables variant filtering entirely
- where: `pkg/api/componentreadiness/dataprovider/postgres/variants.go:175` (`filterVariantsByDBGroupBy`), `pkg/api/componentreadiness/dataprovider/postgres/provider.go:613-618,651-656` (`buildAggregatePrefixSumQuery`, `buildAggregateGAQuery`), `pkg/api/utils.go:14-27` (`VariantsStringToSet`)
- concern: `VariantsStringToSet` returns a non-nil empty `sets.Set[string]{}` (via `sets.New[string]()`) whenever the `dbGroupBy` query param is absent/empty, not nil. `filterVariantsByDBGroupBy` then strips `includeVariants` down to `{}` since `dbGroupBy.Has(k)` is false for every key. Both SQL builders guard with `if len(includeVariants) > 0`, treating the now-empty map as "no filter requested" rather than "filter matches nothing" — so a caller that intended to filter to e.g. `Platform:aws`/`Network:ovn` instead gets results summed across every platform/network/tier/owner combination in the release, silently, with no error. This is distinct from the JobTier/Owner leak above: it applies to *all* `includeVariants` keys, including ones present in `dbGroupBy` in other call paths, whenever `dbGroupBy` itself is empty.
- excerpt: |
    func filterVariantsByDBGroupBy(includeVariants map[string][]string, dbGroupBy sets.Set[string]) map[string][]string {
        filtered := make(map[string][]string, dbGroupBy.Len())
        for k, v := range includeVariants {
            if dbGroupBy.Has(k) {
                filtered[k] = v
            }
        }
        return filtered
    }
    // buildAggregatePrefixSumQuery / buildAggregateGAQuery:
    if len(includeVariants) > 0 {
        filterClause, filterArgs := buildVariantFilterClause(includeVariants)
        ...
    }

### [should-fix] Integration test only covers the false-negative fix, not the potential false-positive regression
- where: `test/integration/component_readiness_test.go:1855-1907` (new subtest "non-dbGroupBy includeVariants keys do not exclude valid jobs")
- concern: the new test seeds one job whose variant_combination lacks JobTier/Owner/CGroupMode and asserts it's no longer wrongly excluded. There's no test seeding two jobs with identical dbGroupBy dims but different JobTier values (one allowed, one not) asserting only the allowed one contributes — exactly the scenario the blocking finding above would break silently.
- excerpt: |
    result, errs := provider.QueryBaseJobRunTestStatus(context.Background(), opts)
    require.Empty(t, errs)
    totalRows := 0
    for _, rows := range result {
        totalRows += len(rows)
    }
    require.Equal(t, 1, totalRows, "non-dbGroupBy variant keys should not exclude valid jobs")

### [should-fix] Fix applied only at one call site, not in shared variant-filter infrastructure
- where: `pkg/api/componentreadiness/dataprovider/postgres/provider.go:532` (call site), `pkg/api/componentreadiness/dataprovider/postgres/cr_queries.go:142` (`mergeRequestedVariants`, sibling path not covered)
- concern: `filterVariantsByDBGroupBy` is only invoked from `queryBaseAggregateTestDetails`. The sibling per-run path `queryTestDetails`, and `cr_queries.go`'s `queryTestStatus` (via `mergeRequestedVariants`), don't get the same treatment. If either later hits the same "older release missing variant_combinations keys" gap this PR fixes, the identical over-constraining bug reappears there since the fix isn't centralized in `buildVariantFilterClause` or `mergeRequestedVariants`.
- excerpt: |
    includeVariants = filterVariantsByDBGroupBy(includeVariants, reqOptions.VariantOption.DBGroupBy)
    // provider.go:532 — the only call site

### [nit] Three near-identical "filter map by dbGroupBy" implementations
- where: `pkg/api/componentreadiness/dataprovider/postgres/provider.go:74-81` (`filterByDBGroupBy`), `pkg/api/componentreadiness/dataprovider/postgres/variants.go:175-182` (`filterVariantsByDBGroupBy`), `pkg/api/componentreadiness/dataprovider/postgres/variants.go:76-83` (inline loop in `lookupVariantValues`)
- concern: three copies of the same "keep only keys present in dbGroupBy" loop exist across two files, differing only in map value type (`string` vs `[]string`). A future behavior change — e.g. how an empty `dbGroupBy` should be handled, per the blocking finding above — has to be replicated in all three or it silently diverges.
- excerpt: |
    func filterByDBGroupBy(variants map[string]string, dbGroupBy sets.Set[string]) map[string]string { ... }
    func filterVariantsByDBGroupBy(includeVariants map[string][]string, dbGroupBy sets.Set[string]) map[string][]string { ... }
    // plus an inline third copy in lookupVariantValues

### [nit] make() capacity hint sized by the wrong collection
- where: `pkg/api/componentreadiness/dataprovider/postgres/variants.go:176`
- concern: `make(map[string][]string, dbGroupBy.Len())` sizes the map by `dbGroupBy`'s length rather than `includeVariants`'s (the collection actually being filtered/iterated). Harmless (map still grows correctly) but the capacity hint doesn't reflect the real upper bound. Same pattern already exists in `lookupVariantValues`'s `filtered` map, so at least it's consistent with existing style.
- excerpt: |
    filtered := make(map[string][]string, dbGroupBy.Len())
    for k, v := range includeVariants {

### [question] Is dropping JobTier/Owner filtering in this path actually safe?
- where: `pkg/api/componentreadiness/dataprovider/postgres/variants.go:171-174` (comment)
- concern: the code comment says the over-constraint "can" cause problems but doesn't explain why fully dropping the constraint (rather than making it tolerant of absent-in-DB values) is safe. If it's structurally true that JobTier/Owner are never meaningfully populated for jobs that reach this aggregate-fallback path, that reasoning should be stated explicitly in the comment; otherwise the blocking finding above stands.

### [question] PR was closed without merging, but its commit is already on main
- where: PR metadata (`state: CLOSED`, `mergedAt: null`, `mergeCommit: null`) vs `git log` showing commit `29203eb82` on `main`
- concern: was this landed intentionally outside the normal PR merge flow (e.g. cherry-picked or pushed directly), and should this PR be reopened/linked, or closed as superseded?

## Checked
- SQL query builders (`buildAggregatePrefixSumQuery`, `buildAggregateGAQuery`) — correctly consume the now-filtered `includeVariants` via existing `buildVariantFilterClause`; no injection risk (parameterized `?` placeholders).
- Unit test `TestFilterVariantsByDBGroupBy` (`variants_test.go`) — solid table-driven coverage of the new pure function, follows project convention.
- The non-aggregate per-run path (`queryTestDetails`, `provider.go:380-421`) intentionally still uses the full unstripped `includeVariants` — this fix is correctly scoped to only the aggregate-fallback path used for older releases whose `variant_combinations` rows lack newer keys.
- No other call sites of `filterVariantsByDBGroupBy` introduced; single call site at `provider.go:532`.
- Could not run `gofmt`/`go vet` (no Go toolchain in review sandbox) — diff style otherwise matches surrounding code.

## Open questions
- Does any release actually have a mix of allowed/disallowed JobTier (or Owner) values among jobs that share the same dbGroupBy dimensions? If yes, this fix as written will silently include disallowed jobs in base aggregate stats.
- Should `processAggregateRows` re-apply `matchesIncludeVariants` (guarded on the key being present in the row's own variants, to avoid resurrecting the zero-rows bug) instead of dropping the constraint universally in SQL?
- Can `reqOptions.VariantOption.DBGroupBy` actually be empty for requests that reach `queryBaseAggregateTestDetails` in production? If so, `includeVariants` filtering is silently disabled for those requests today.
- Why is this PR closed unmerged while its commit already exists on `main`?
