---
pr: openshift/sippy#3928
title: "TRT-2914: CR Parity Gap: PG provider does not apply the IgnoreDisruption test filter"
head_sha: 5c8df6287ee9981b0814def75995e109a91b37c9
base: main
reviewed_at: 2026-08-24T10:27:13Z
verdict: approve
refresh_log:
  - from: ad96bf9ff33e3152db020efd9c2961d9cdbbb712
    to: 5c8df6287ee9981b0814def75995e109a91b37c9
    summary: Removed unit test file cr_queries_test.go (135 lines), replaced with integration test TestIgnoreDisruptionFilter in test/integration/component_readiness_test.go (92 lines), per project convention of preferring real-DB integration tests over isolated unit tests for query-building logic. PR merged.
---

## Findings

(none)

## Resolved
(none — no findings existed in the prior review)

## Checked
- `buildDrilldownFilters` change mirrors the existing BigQuery implementation (`pkg/api/componentreadiness/dataprovider/bigquery/querygenerators.go:453`), same literal `'Disruption'` capability string — parity maintained.
- New clause is parameter-free (`ANY(tow.capabilities)` against a string literal), so `outerArgs` correctly untouched — no placeholder/arg-order mismatch.
- `TestIgnoreDisruptionFilter` in `test/integration/component_readiness_test.go` (added in 5c8df6287, modeled after existing `TestCapabilitiesArrayOverlapFilter`) exercises the filter against a real Postgres instance: excludes tests with the `Disruption` capability when set, includes all tests when unset, and correctly excludes a test matching a combined capability+disruption case.
- `go build` and the test suite pass.
- PR is merged (state: MERGED); approved by neisw and self-approved by openshift-trt-agent[bot]; `/lgtm` and CI all-green per PR activity.

## Open questions
(none)
