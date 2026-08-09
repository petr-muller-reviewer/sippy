---
pr: openshift/sippy#3871
title: "Port feature gate promotion logic from o/api to sippy"
head_sha: 0ff0c810357220fcf3d8fb1ac95f198d7fd58a62
base: main
reviewed_at: 2026-08-08T13:23:43Z
verdict: approve
---

## Summary

Ports feature-gate promotion evaluation logic (previously in o/api) into sippy under
`pkg/api/featuregatepromotion/`. Introduces canonical `filter.Filter` definitions
(`GateTestFilter`, `InstallTestFilter`, `CapabilityRegressionsFilter` in `filters.go`)
reused both to build HATEOAS links (`pkg/sippyserver/server.go`) and to drive promotion
computation (`promotion.go`). Adds `/api/feature_gates` and `/api/feature_gates/{gate}`
endpoints, README docs, and seed-data fixtures for capability-regression tests.

## Findings

### [should-fix] Capability-regression criteria defined twice, must be kept in lockstep
- where: `pkg/api/featuregatepromotion/filters.go:38-53`, `pkg/api/featuregatepromotion/promotion.go:216`
- concern: The set of tests counted as "capability regressions" is defined twice: once
  declaratively as `CapabilityRegressionsFilter` (used only to build the `gate_job_tests`
  HATEOAS link) and once as hand-written raw SQL in `getCapabilityRegressions` (used to
  actually compute regressions for promotion evaluation). Both encode the same conditions
  (lifecycle=blocking, working %% < 92, runs >= 1, excluded test-name substrings) independently.
  A future change to one (e.g. adjusting the pass-rate threshold or excluded-name list) that
  misses the other will silently desync the advertised link from the actual computation.
- excerpt: |
    // filters.go — declarative filter, used only for the HATEOAS link
    func CapabilityRegressionsFilter(featureGate string) filter.Filter {
    	return filter.Filter{
    		Items: []filter.FilterItem{
    			{Field: "lifecycle", Operator: filter.OperatorEquals, Value: "blocking"},
    			{Field: "current_working_percentage", Operator: filter.OperatorArithmeticLessThan, Value: "92"},
    			...
    		},
    	}
    }

    -- promotion.go — raw SQL, actually computes regressions
    AND e.lifecycle = 'blocking'
    AND t.name NOT LIKE '%' || ? || '%'
    ...
    HAVING ... < 92

## Checked
- `lifecycle` is a pre-existing, well-supported filter field (`pkg/api/tests.go:463`), backed by
  a real `lifecycle text NOT NULL DEFAULT 'blocking'` column (`pkg/db/models/prow.go`).
- No BigQuery parity gap: `featuregatepromotion` is Postgres-only; the HATEOAS link it feeds
  always resolves through `/api/tests` -> `jsonTestsReportFromDB` (Postgres), never the
  BigQuery-backed `/api/tests/v2` route, so the BQ "lifecycle filter unsupported" guard in
  `pkg/api/tests.go` is never hit here.
- `e.lifecycle = 'blocking'` in the raw SQL join (`promotion.go:216`) is unambiguous — only one
  `e`-aliased table in scope — and correctly narrows both sides of the existing self-join on
  `s.lifecycle = e.lifecycle`.
- README (`pkg/api/README.md`) updated in the same PR to document the new endpoints and the
  shared-filter design intent.
- Full line-by-line diff scan across all changed/added files, cross-file tracing of
  `CapabilityRegressionsFilter` / `getCapabilityRegressions` callers, and an independent
  verification pass; no other correctness, parity, or convention issues found.

## Open questions
- Would it be worth extracting the capability-regression predicate (lifecycle, threshold,
  excluded-name list) into a single source of truth consumed by both the `filter.Filter` and
  the SQL builder, so future criteria changes can't update one path without the other?
