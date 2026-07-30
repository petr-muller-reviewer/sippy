---
pr: openshift/sippy#3818
title: "TRT-2764: Resolved conflicts from #3721 - Fix search bar not completing search"
head_sha: 412fd3cede6cfd2686f1bbb1a38141288bdecbd5
base: main
reviewed_at: 2026-07-25T12:26:40Z
verdict: approve
---

## Summary

Rebase/conflict-resolution of #3721 onto post-`.js`→`.jsx` `main`. Fixes toolbar search on table pages getting stuck in a loading spinner after Enter, caused by `requestSearch` mutating `filterModel`/`currentFilters` in place and calling `setFilterModel` with the same object reference (React bails out of re-render on referential equality). Fix applies an immutable update (`{ ...filterModel, items: newItems }`) across ~11 table components, adds a `searchField` prop + `useEffect` to `GridToolbar.jsx` that repopulates the search box from an existing single positive `contains` filter (URL refresh case), removes a redundant `onBlur` search trigger, and fixes an unrelated `columnField` mismatch (`releaseTag` vs `release_tag`) in `ReleasePayloadJobRuns.jsx`.

## Findings

### [should-fix] Uneven test coverage across mechanically-identical components
- where: `sippy-ng/src/jobs/JobTable.jsx`, `sippy-ng/src/tests/FeatureGates.jsx`, `sippy-ng/src/releases/PayloadStreamsTable.jsx`, `sippy-ng/src/releases/ReleasePayloadTable.jsx`, `sippy-ng/src/releases/PayloadStreamTestFailures.jsx`, `sippy-ng/src/releases/PayloadTestFailures.jsx`, `sippy-ng/src/releases/ReleasePayloadPullRequests.jsx`, `sippy-ng/src/repositories/RepositoriesTable.jsx`
- concern: The same immutable-`requestSearch` fix was hand-applied to ~11 components but only `JobRunsTable.jsx` and `GridToolbar.jsx` got new tests. Low risk since the pattern is mechanically identical and was spot-checked in this review, but a future edit to one of the untested components could silently regress without any test catching it.

### [nit] JobRunsTable.test.jsx tests a duplicated copy of the logic, not the real closure
- where: `sippy-ng/src/jobs/JobRunsTable.test.jsx:1-20`
- concern: The test file defines its own standalone `requestSearch` function ("We test the pure logic here because the full component requires extensive infrastructure") rather than importing/exercising the actual closure inside `JobRunsTable.jsx`. If the real `requestSearch` in the component changes and the copy isn't updated to match, the test keeps passing while the component regresses.
- excerpt: |
    function requestSearch(filterModel, searchField, searchValue) {
      const newItems = filterModel.items.filter(
        (f) => f.columnField !== searchField
      )
      ...
    }

### [question] Why does TestTable.jsx get an extra dedup guard the others don't?
- where: `sippy-ng/src/tests/TestTable.jsx:1013-1032`
- concern: `TestTable`'s `requestSearch` short-circuits (skips `setSearching(true)`/refetch) when the new search value equals the existing filter value; no other component in this PR has the equivalent guard. Unclear if this is `TestTable`-specific (e.g. papering over a double-fire path unique to that component) or whether the same protection should exist elsewhere for consistency.
- excerpt: |
    const existingFilter = filterModel.items.find(
      (f) => f.columnField === 'name' && f.operatorValue === 'contains'
    )
    if (existingFilter && existingFilter.value === searchValue) {
      return
    }

### [question] PR body test-plan checkboxes unchecked despite "Validation" claiming green
- where: PR description
- concern: The description's "Validation" section claims ESLint 0 errors and Vitest 32/32 passing, but the "Test plan" checkboxes (`make lint`, `make test`, manual verification) are all unchecked. Likely just unedited template boilerplate, but worth confirming CI is actually green before merge.

## Checked

- Immutable-update pattern (`{ ...filterModel, items: newItems }`) applied consistently across all ~11 touched components — no stragglers, no leftover in-place mutation found in the diff.
- `GridToolbar.jsx` new `useEffect`: guards on `initializedFromFilter` ref (fires once), requires exactly one matching filter for `searchField`, checks `operatorValue === 'contains'`, `not !== true`, truthy `value`. `not` matches the existing filter-model convention used in `GridToolbarFilterItem.jsx`/`GridToolbarFilterMenu.jsx` — not a new/invented field.
- `ReleasePayloadJobRuns.jsx` field-name fix (`releaseTag` → `release_tag`): confirmed `release_tag` is the actual grid column field (`field: 'release_tag'`) and used consistently elsewhere in the same file (filter removal, `searchField` prop) — the fix is correct.
- New `GridToolbar.test.jsx` covers all four population-guard branches (single positive match, negated filter, multiple filters for field, wrong operator) plus interaction behavior (Enter key, search button, no-op on blur, clear button).
- Removed `onBlur` handler on the search `TextField` — intentional, avoids the redundant second search that caused the double-search/stuck-loading symptom described in the PR's preserved commit history.

## Open questions

- Is the `TestTable.jsx`-only dedup guard (skip re-search when value unchanged) fixing a `TestTable`-specific bug, or should the same guard be added to the other ~10 components for consistency?
- Can `JobRunsTable.test.jsx` be changed to exercise the actual component closure (e.g. by extracting `requestSearch` to an exported/testable helper) instead of a hand-duplicated copy of the logic?
- Confirm CI (`make lint`, `make test`) is green — the PR's own test-plan checkboxes are unchecked.
