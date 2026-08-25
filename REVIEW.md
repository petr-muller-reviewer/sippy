---
pr: openshift/sippy#3926
title: "TRT-2883: Fix spurious MissingSample/MissingBasis for tests below MinimumFailure"
head_sha: 67da3e982705e7f7d4002999918fa516c7b03da6
base: main
reviewed_at: 2026-08-21T15:14:28Z
verdict: approve
---

## What this PR does

- Fixes a PG/BigQuery parity bug (TRT-2883): PostgreSQL SQL-side filtered out tests below `MinimumFailure` entirely, so tests that only ran on one side (sample-only/base-only) or crossed the threshold between sides were silently dropped from both result maps instead of surfacing as `MissingSample`/`MissingBasis`. BigQuery has no such SQL-side filter, so it never had the bug.
- Standalone path (`queryTestStatusCTE`, feeds `QueryBaseTestStatus`/release-fallback): drops the SQL-level `MinimumFailure` filter entirely (new `testBranchTemplate` replaces `failureBranchTemplate`), relying solely on the existing Go-side check in `component_report.go`.
- Combined path (`queryCombinedTestStatus`): keeps the `>= MinimumFailure` `failureBranchTemplate` per side, and adds `belowThresholdRescueBranchTemplate`, a LEFT JOIN against a new narrow `keysCTETemplate` projection of the other side, to rescue below-threshold rows when either the other side has no matching row, or the other side's matching row is itself at/above threshold.
- `keysCTETemplate` materializes `(unique_id, variant_group_id, fail_count)` for each side so the rescue join doesn't hash the full-width status CTE twice.
- `minimumFailure` removed from `variantQuerySetup`/`prepareVariantQuery` since only the combined path needs it now (passed as a direct SQL arg, not a struct field).
- Integration tests updated/added: existing `test1/aws` expectation flipped (now rescued), new tests for both-sides-below-threshold (correctly excluded), one-sided-below-threshold (correctly rescued on both sample and base), and a `TestGenerateReport_MinimumFailureThreshold` addition covering the reverse-direction (base below/sample above) and both-below cases end-to-end through Fisher-exact analysis.

## Findings

### [should-fix] Duplicated column list across three near-identical branch templates
- where: `pkg/api/componentreadiness/dataprovider/postgres/cr_queries.go:187-253`
- concern: `testBranchTemplate`, `failureBranchTemplate`, and `belowThresholdRescueBranchTemplate` all repeat the identical `SELECT ... FROM %s pa JOIN tests t ... LEFT JOIN suites su ...` column list and joins, differing only in the `WHERE`/extra `LEFT JOIN` clause. A future change to the output columns (e.g. adding a field consumed by the row scanner) requires editing three string constants in lockstep; missing one produces a column-count/order mismatch against the shared scan struct, likely surfacing as a scan error or silently wrong data rather than a compile error.
- excerpt: |
    const testBranchTemplate = `SELECT
            %spa.unique_id AS test_id, t.name AS test_name,
            COALESCE(su.name, '') AS test_suite, pa.component, pa.capabilities,
            pa.variant_group_id, pa.total_count, pa.success_count, pa.flake_count, pa.last_failure
        FROM %s pa
        JOIN tests t ON t.id = pa.test_id
        LEFT JOIN suites su ON su.id = pa.suite_id`

### [question] Same invariant enforced via two independent mechanisms
- where: `pkg/api/componentreadiness/dataprovider/postgres/cr_queries.go:274-310` (standalone) vs `:410-506` (combined)
- concern: the standalone path drops the SQL filter entirely and relies purely on the Go-side `MinimumFailure` check, while the combined path keeps the SQL filter and adds a bespoke LEFT-JOIN rescue branch. Both fix the same underlying bug (TRT-2883) but share no code. If `MinimumFailure` semantics change later (e.g. the Go-side check is extended to inspect both sides), a maintainer has to remember to update both paths independently. Is this divergence intentional/documented anywhere beyond the code comments, or worth a shared test/assertion tying the two behaviors together?

### [question] queryTestStatusCTE now returns unfiltered base-side rows for release-fallback
- where: `pkg/api/componentreadiness/dataprovider/postgres/cr_queries.go:274-310`
- concern: `queryTestStatusCTE` feeds `QueryBaseTestStatus`, used by the release-fallback middleware, which caches per-release `BaseStatus` maps across every fallback release queried. With the SQL-side floor removed, low-count/noisy tests are now included, so the per-release result set (and cached map) is larger, bounded only by the Go-side check downstream. Not a correctness bug — no consumer found assuming pre-filtering — but worth confirming this doesn't meaningfully grow fallback-cache memory in practice for high-fanout fallback scenarios.

## Checked
- Argument-binding order for `belowThresholdRescueBranchTemplate` (two placeholders per side: `< MinimumFailure` then `>= MinimumFailure` for the other side) matches the `allArgs` append order in `queryCombinedTestStatus` (cr_queries.go:504-508).
- `gofmt -l` clean on the changed file.
- BigQuery provider parity: BigQuery has no SQL-side `MinimumFailure` filter today, so this PR brings Postgres in line with BigQuery's existing (correct) behavior rather than diverging further — no BigQuery-side change needed.
- New integration tests cover all four quadrants: both above threshold, one above/one below, both below, and one-sided (no counterpart row) — including a full `TestGenerateReport_MinimumFailureThreshold` case exercising Fisher-exact analysis on a rescued below-threshold base row.
- `variantQuerySetup.minimumFailure` field removal is a clean, fully-referenced-checked deletion (was only used by the now-removed standalone SQL filter).

## Open questions
- Is the split between "no SQL filter, rely on Go-side check" (standalone) and "SQL filter + rescue join" (combined) meant to be permanent, or would unifying both paths onto one strategy be worth a followup?
- For very large components/variant combinations, has the rescue join's cost against the materialized keys CTE been checked against the old combined-query plan, or is this considered a wash given the narrower row width?
