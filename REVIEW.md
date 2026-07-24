---
pr: openshift/sippy#3797
title: "[WIP] TRT-2741: Add per-request BQ/PG toggle for Component Readiness"
head_sha: 51fc2188be3929b5b4c393fbe5f50ffb9123e381
base: main
reviewed_at: 2026-07-22T15:36:55Z
verdict: request-changes
---

## Summary

Adds a `dataSource` query param (`bigquery`|`postgres`) letting `MixedProvider` route Component Readiness queries per-request. Implements SQL push-down for PG (prefix-sum self-join over `test_cumulative_summaries`, GA-aligned path over `prow_ga_raw_test_data`), SQL-side variant-group aggregation, and a placeholder/existence query so grid cells with runs-but-no-failures render `NotSignificant`. Bundles an orthogonal `KeyWithVariants`/`ColumnIdentification` encoding refactor (JSON keys -> custom `Encode()`). Frontend: cookie-based BQ/PG toggle, `dataSource` propagated through query state, default base window 27->30 days. PR carries `do-not-merge/work-in-progress` label; own test-plan checklist has 2 unchecked items.

## Findings

### [blocking] MixedProvider bypasses dataSource toggle for variant/job-variant lookups
- where: `pkg/api/componentreadiness/dataprovider/mixed/provider.go:59-61,78-83`
- concern: `QueryUniqueVariantValues`, `QueryJobVariantValues`, and `LookupJobVariants` are hard-wired to `p.bq` regardless of `reqOptions.DataSource`, unlike every other method which routes via `providerFor(reqOptions)`. When a user selects `dataSource=postgres`, filter-dropdown population and job-variant lookups still hit BigQuery while report/test-status data comes from Postgres. Violates the project's BQ/PG parity convention unless intentionally documented as "reference data assumed identical across backends."
- excerpt: |
    func (p *MixedProvider) QueryUniqueVariantValues(ctx context.Context, field string, nested bool) ([]string, error) {
    	return p.bq.QueryUniqueVariantValues(ctx, field, nested)
    }
    ...
    func (p *MixedProvider) QueryJobVariantValues(ctx context.Context, jobNames, variantKeys []string) (map[string]map[string]string, error) {
    	return p.bq.QueryJobVariantValues(ctx, jobNames, variantKeys)
    }

    func (p *MixedProvider) LookupJobVariants(ctx context.Context, jobName string) (map[string]string, error) {
    	return p.bq.LookupJobVariants(ctx, jobName)
    }

### [blocking] New SQL push-down logic has zero test coverage
- where: `pkg/api/componentreadiness/dataprovider/postgres/cr_queries.go` (421 new lines), `pkg/api/componentreadiness/dataprovider/postgres/variants.go` (206 new lines)
- concern: No unit tests for pure-logic builders (`buildVariantFilterClause`, `buildVariantGroupMapping`, `buildColumnGroupMapping`) and no functional test against real Postgres per repo convention (`releasesync_functional_test.go` pattern). `queryTestStatusPrefixSum`/`queryBaseTestStatusGA` build 3-4 layers of nested `fmt.Sprintf` SQL and hand-reconstruct matching `[]any` arg slices in the same order with no compile-time or runtime check tying arg order to placeholder order. Traced both queries manually; ordering is correct today, but a future edit reordering a WHERE/HAVING clause would silently desync args from placeholders with no error, just wrong data — and nothing would catch it.
- excerpt: |
    failureArgs := make([]any, len(joinArgs))
    copy(failureArgs, joinArgs)
    failureArgs = append(failureArgs, minimumFailure, release, start, end)

### [blocking] Frontend toggle default state contradicts actual data source
- where: `sippy-ng/src/App.js:439-441,650,662`; `sippy-ng/src/component_readiness/CompReadyVars.js:139-141`; `sippy-ng/src/component_readiness/ComponentReadinessIndicator.js:61-63`
- concern: `testTableDBSource` defaults to `undefined` when no cookie exists. The toggle icon/tooltip logic treats anything not exactly `'bigquery'` as "Postgres active" (`testTableDBSource === 'bigquery' ? <ToggleOff/> : <ToggleOn/>`), but the actual data-fetch code treats anything not exactly `'postgres'` as bigquery (`cookies['testTableDBSource'] === 'postgres' ? 'postgres' : ''`). Net effect: first-time visitors see the UI claim Postgres is active while the app queries BigQuery.
- excerpt: |
    const testTableDBSourcePreference = cookies['testTableDBSource']
    const [testTableDBSource, setTestTableDBSource] = React.useState(
      testTableDBSourcePreference
    )
    // App.js:650 icon logic: testTableDBSource === 'bigquery' ? ... : ...
    // CompReadyVars.js:141 fetch logic: cookies['testTableDBSource'] === 'postgres' ? 'postgres' : ''

### [should-fix] Placeholder/existence query can misattribute capability on grouped cells
- where: `pkg/api/componentreadiness/dataprovider/postgres/cr_queries.go` (`placeholderOuterQuery`, `runFailureAndExistence`)
- concern: `placeholderOuterQuery` does `SELECT DISTINCT` over `(component, col_group_id)` but still carries a real test's `tow.capabilities`. `scanGroupedResults`/`runFailureAndExistence` key merged results by `TestID+Variants` only, not capabilities, so when multiple tests share a component+column with different capability sets, only one arbitrary capability set survives per synthetic placeholder row. On capability-grouped views this can attribute a `NotSignificant` placeholder to the wrong capability, or drop one entirely. Not covered by the PR's manual test plan (standard/cross-compare view checks, not capability-drilldown-specific).
- excerpt: |
    const placeholderOuterQuery = `SELECT DISTINCT
        'grid:' || tow.component AS test_id,
        ...
        tow.capabilities,
        cm.col_group_id AS variant_group_id,
        1 AS total_count, 1 AS success_count, 0 AS flake_count, ...`

### [should-fix] README/config docs not updated for new dataSource param
- where: `pkg/api/README.md`, `config/README.md`
- concern: Repo convention requires README updates in the same PR when API endpoints or config/params change. Verified neither file mentions `dataSource` or the BQ/PG toggle. `docs/features/` also has no update despite this adding a new query architecture (prefix-sum + GA raw tables) to a documented major feature.
- excerpt: |
    (grep for "dataSource" in pkg/api/README.md and config/README.md returns no hits)

### [nit] QueryJobVariants reqOptions param unused on both concrete providers
- where: `pkg/api/componentreadiness/dataprovider/bigquery/provider.go` (`_ reqopts.RequestOptions`), `pkg/api/componentreadiness/dataprovider/postgres/provider.go`
- concern: The signature change is threaded through the interface and both providers, but only `MixedProvider.providerFor()` actually uses it for routing. Dead API surface on both concrete implementations; a one-line comment would prevent future confusion.
- excerpt: |
    func (p *BigQueryProvider) QueryJobVariants(ctx context.Context, _ reqopts.RequestOptions) (crtest.JobVariants, []error) {

### [nit] Endpoint descriptions still say "BigQuery" only
- where: `pkg/server/server.go:2730,2739`
- concern: `/api/component_readiness` and `/api/component_readiness/test_details` endpoint `Description` strings still read "Reports component readiness from BigQuery" despite now supporting `dataSource=postgres`.

### [nit] testTableDBSource cookie repurposed without rename/comment
- where: `sippy-ng/src/App.js` (cookie previously scoped to `sippy-ng/src/tests/TestTable.js`)
- concern: Same cookie now also drives the Component Readiness data source. Functionally fine as one global toggle, but the name no longer reflects its scope; a rename or clarifying comment would help future readers.

### [nit] windowDays computation duplicated
- where: `pkg/api/componentreadiness/dataprovider/postgres/provider.go:242`, `pkg/api/componentreadiness/dataprovider/postgres/cr_queries.go:158`
- concern: `int(reqOptions.BaseRelease.End.Sub(reqOptions.BaseRelease.Start).Hours() / 24)` is duplicated verbatim in two files; extract to a shared helper to avoid drift.

### [question] Is BQ-only routing for variant/job-variant lookups intentional?
- Is the bypass in `MixedProvider.QueryUniqueVariantValues`/`QueryJobVariantValues`/`LookupJobVariants` (see blocking finding above) deliberate because this reference data is assumed identical across backends, or an oversight from routing only the report/status query paths?

### [question] Is the GA window day-truncation robust to future changes?
- `windowDays := int(End.Sub(Start).Hours() / 24)` relies on `End`/`Start` being rounded to specific times of day so the ~30.9999-day span truncates to exactly 30 (matching `utils.GAWindows`). Verified correct today but undocumented — is this guaranteed by an invariant elsewhere, or fragile to a future change in date rounding?

### [question] Should the placeholder/existence mechanism have a documented BQ-equivalence contract?
- BQ has no placeholder/existence logic (it returns per-test rows unconditionally; PG's push-down filters aggressively and needs a supplementary query to backfill "ran, no failures" cells). This looks like an intentional architectural divergence, but there's no test asserting BQ and PG produce identical `NotSignificant` cells for the same underlying data. Should there be one before this leaves WIP?

## Checked
- SQL injection: not found — user-controlled values go through bind params or a strict allow-list regexp (`dataSource` validated via `pkg/util/param/param.go:67`).
- Concurrency in `runFailureAndExistence` (goroutines via `WaitGroup.Go`): no shared mutable state during the parallel section, errors from both paths collected properly.
- HATEOAS `dataSource` propagation through test-details links (`linkinjector.go`, `test_details.go`, `utils.GenerateTestDetailsURL`): correct, only added when non-empty, covered by tests (`utils_test.go`).
- `gofmt -l` and `go vet ./pkg/api/componentreadiness/...`: clean on touched files.
- `sets.Set[string]` used correctly in new code per repo convention.
- `deserializeRowToTestStatus` (BQ) updated to populate `TestID`/`Variants` directly, matching PG's `scanGroupedResults` — consistent with the `KeyOrDie`->`Encode` refactor.
- `Encode()`/`DecodeColumnID()` refactor and `component_report_test.go` updates: well covered with table-driven tests.

## Open questions
- Is the BQ-only routing for `QueryUniqueVariantValues`/`QueryJobVariantValues`/`LookupJobVariants` intentional?
- Is the GA window day-truncation guaranteed correct by an invariant, or should it have an explicit comment/test?
- Should BQ/PG produce provably identical `NotSignificant` grid cells, and if so, is a cross-backend test planned?
