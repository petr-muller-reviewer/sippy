---
pr: openshift/sippy#3825
title: "[WIP] TRT-2741: Add per-request BQ/PG toggle for Component Readiness"
head_sha: 0faeda5b03b42da0a1c927706b4560d2bf471005
base: main
reviewed_at: 2026-07-26T12:30:44Z
verdict: needs-discussion
refresh_log:
  - from: b8c7a98b07742f749fed68188a3ed7bd99d13c8f
    to: 0faeda5b03b42da0a1c927706b4560d2bf471005
    summary: Force-pushed, small targeted diff (3 files, +78/-48) confined to postgres/provider.go and its tests — replaced hand-rolled map[string]bool with sets.Set[string] for filterByDBGroupBy (per project convention), and tightened variants_test.go assertions to check actual valuesClause/group contents instead of only counts. No findings resolved; no new findings surfaced. No PR comments/reviews since prior review.
  - from: 0faeda5b03b42da0a1c927706b4560d2bf471005
    to: 0faeda5b03b42da0a1c927706b4560d2bf471005
    summary: No code change. mstaeble left an inline reply on docs/database-tuning.md (2026-07-26T12:07:18Z) defending the random_page_cost rationale text against a CodeRabbit nit; CodeRabbit's analysis chain agreed no doc change was needed and withdrew the comment, then submitted an APPROVED review (2026-07-26T12:07:52Z). Unrelated to any existing finding.
---

## Summary

Adds a `dataSource` query param + cookie-based UI toggle to switch Component Readiness between BigQuery and PostgreSQL per request. Rewrites the PG test-status query as a prefix-sum over `test_cumulative_summaries` with SQL-level variant grouping via VALUES-clause joins, adds GA base-window routing to `prow_ga_raw_test_data` (1/30/90 day windows), restructures test_details with a MATERIALIZED CTE (work_mem planner fix), migrates `TestStatus.Variants` from `[]string` to `map[string]string`, adds `KeyWithVariants.Encode()`/`DecodeColumnID()` null-byte-separated key encoding, indexes `FindOpenRegression` by testID, merges `CompareVariants` into `IncludeVariants` for PG parity, and bumps the UI default base window from 27 to 30 days. PR itself documents accepted BQ/PG parity gaps (lastFailure scoping/InfraFailure inclusion, lifecycle filter, job scope) — not re-flagged here.

Since previous review: `filterByDBGroupBy` now takes `sets.Set[string]` instead of a hand-rolled `map[string]bool`, matching project convention; `variants_test.go` assertions were tightened to verify actual VALUES-clause and group contents rather than just counts. Neither change addresses any existing finding below.

## Findings

### [should-fix] EncodeVariants not provably collision-free
- where: `pkg/apis/api/componentreport/crtest/types.go:116-133`
- concern: Pairs are built as `key+":"+value` joined with `\x00`, with no escaping of `:` or embedded `\x00` in keys/values. Two distinct variant maps can encode identically (e.g. `{"A":"B:C"}` vs `{"A:B":"C"}`). A value containing a literal `\x00` would also corrupt `DecodeColumnID`'s split. Low likelihood given the fixed variant vocabulary, but unenforced and untested.
- excerpt: |
    pairs = append(pairs, key+":"+value)
    ...
    strings.Join(pairs, "\x00")

### [should-fix] Base window default (27→30 days) not documented
- where: `sippy-ng/src/component_readiness/CompReadyVars.jsx`, `sippy-ng/src/component_readiness/ReleaseSelector.jsx`
- concern: User-facing default behavior change, applied consistently in both files, but not mentioned in README.md/sippy-ng/README.md. Project convention requires config/behavior changes to be documented in the same PR.

### [should-fix] `mixed` provider dispatch has no test coverage
- where: `pkg/api/componentreadiness/dataprovider/mixed/provider.go`
- concern: `providerFor`'s BQ/PG routing is the core mechanism this PR introduces, but there is no test file for the `mixed` package. `queryparamparser_test.go` also has no case for parsing `dataSource`.

### [should-fix] No test pinning `DataSource` in fallback cache key
- where: `pkg/api/componentreadiness/middleware/releasefallback/releasefallback.go:337,352`
- concern: `fallbackTestQueryReleasesGeneratorCacheKey` appears to include `DataSource`, but no regression test verifies this. If it were ever dropped, BQ and PG requests could collide in cache and silently serve the wrong source's data.

### [nit] Shared mutable variant map aliased across TestStatus rows
- where: `pkg/api/componentreadiness/dataprovider/postgres/cr_queries.go:416-436`
- concern: `scanRows` sets `ts.Variants = variantMap` reusing the same map instance (`groupMapping.groupToVariants[variantGroupID]`) for every row in a variant group. No current mutation path found, but it's a latent hazard if any downstream code ever mutates `TestStatus.Variants` in place. Consider `maps.Clone` or documenting the shared-ownership invariant.

### [nit] GA window boundary logic untested directly
- where: `pkg/api/componentreadiness/dataprovider/postgres/cr_queries.go:203`, `pkg/api/componentreadiness/dataprovider/postgres/provider.go:230-257`
- concern: `baseMatchesGAWindow` (1/30/90-day detection) and `queryBaseTestStatusGA`'s independent re-derivation of `windowDays` are algebraically consistent but only exercised transitively — no direct unit test for boundary conditions (off-by-one on GA date, non-matching windows).

### [nit] Encode/Decode roundtrip tests only cover alphanumeric values
- where: `pkg/apis/api/componentreport/crtest/types_test.go`
- concern: `TestColumnEncodeDecodeRoundTrip`/`TestEncodeStability` don't cover colon- or null-byte-containing values, which is exactly the scenario the encoding's collision-freedom depends on (see EncodeVariants finding above).

### [nit] Invalid dataSource silently falls back to BigQuery
- where: `pkg/util/param/param.go:67`, `pkg/api/componentreadiness/utils/queryparamparser.go:93-95`
- concern: Invalid `dataSource` values are logged and reset to `""` (→ BigQuery) rather than surfacing a 400, inconsistent with how other params reject invalid input. Low impact since the regex allowlist (`^(bigquery|postgres)$`) already blocks garbage before it's user-visible.

### [question] Unrelated GA/cumulative-summary infra bundled into this PR
- where: `pkg/db/models/prow.go`, `pkg/db/query/feature_gates.go`, `pkg/dataloader/gateststatus/loader.go`
- concern: These support the GA-window/cumulative-summary feature rather than the dataSource toggle itself. No correctness issues found, but worth asking whether a split makes the review/merge easier given the PR is already large (44 files) and WIP.

## Checked
- SQL construction: all user-controlled variant keys/values go through `?` placeholders; only integer values (vcid, group IDs) are `fmt.Sprintf`'d into VALUES clauses — no injection risk.
- Prefix-sum date-range math (`AddDays(-1)` half-open interval convention) traced correctly across sample/GA/placeholder query call sites.
- `FindOpenRegression` testID indexing: correct, it's a pre-filter not the full match, no collision risk; well tested in `regressiontracker_test.go`.
- Cookie-based dataSource UI toggle: written only from hardcoded string constants, validated server-side by regex, safe default on missing/corrupt cookie.
- Provider interface parity (BigQuery/Postgres/Mixed): verified via `go build ./pkg/...` and `var _ DataProvider = &X{}` assertions.
- HATEOAS: `dataSource` correctly propagated into generated links (test_details "latest" link, linkinjector), covered by `utils_test.go`.
- `TestStatus.Variants` `[]string`→`map[string]string` change: safe, that struct is explicitly internal/non-serialized and never crosses the API/cache boundary directly.

## Open questions
- Is the `EncodeVariants` collision scenario (colon/null-byte in variant values) actually reachable given the real variant vocabulary, or should it be defensively escaped anyway?
- Should the 27→30 day base window default change get a README/config doc note before merge?
- Any plan to add `mixed` package tests and a `dataSource` cache-key regression test before this leaves WIP?
- Would splitting the GA-window/cumulative-summary DB-layer changes into a separate PR from the dataSource toggle make review easier?
