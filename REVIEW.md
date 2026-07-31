---
pr: openshift/sippy#3714
title: "TRT-2762: Component Readiness: adding variant filter inflates regression count with unrelated results"
head_sha: 3a24e874bd6ce705a434982175d64a538dd2dcd2
base: main
reviewed_at: 2026-07-25T13:06:27Z
verdict: approve
refresh_log:
  - from_sha: 54c1db77bce2caecfdf524b8c2eb7b28cf719da6
    to_sha: 3a24e874bd6ce705a434982175d64a538dd2dcd2
    summary: >-
      Commit 652b348ee addressed the should-fix (stale test_filters state) and
      map[string]bool nit via an independent panel review + author response.
      Lifecycle-filter gap remains but confirmed low-severity (Postgres path
      not used in production; MixedProvider routes test-status queries to
      BigQuery there). Two commits (50e51aef9 add, 3a24e874b revert) made a
      net-no-op Makefile change requested then reverted by a human reviewer.
---

## Summary

Two JS fixes (state mutation in replace-variant functions; missing test_filters sync in updateVarsFromView) are correct. Postgres capability filtering is correct and now has unit test coverage. Since previous review: an independent review panel (`/deep-review`) ran, author addressed all its actionable feedback in 652b348ee, which also fixed both open findings from this review. Remaining lifecycle-filter gap is real but downgraded to low-severity: confirmed the Postgres provider's `queryTestStatus` path is not reached in production (see Checked).

## Findings

### [question] Postgres backend does not implement lifecycle filtering
- where: `pkg/api/componentreadiness/dataprovider/postgres/provider.go:349-503`
- concern: `reqOptions.Lifecycles` is never passed to `queryTestStatus`, no SQL predicate exists, matching BigQuery's `COALESCE(NULLIF(lifecycle, ''), 'blocking') IN UNNEST(@Lifecycles)`. Originally flagged as blocking. Downgraded after verification: `cmd/sippy/serve.go` wires `MixedProvider` when BigQuery credentials are configured (the production case), and `MixedProvider` routes `QueryBaseTestStatus`/`QuerySampleTestStatus` to BigQuery, not Postgres (`pkg/api/componentreadiness/dataprovider/mixed/provider.go:55-61`). The pure-Postgres `queryTestStatus` path is only reached via `--data-provider=postgres`, which in practice is local dev/devcontainer/CI-seed only. A separate `/deep-review` panel independently reached the same conclusion ("pre-existing, not introduced by this PR, Postgres provider is for local dev/testing only") and the author declined to fix it in this PR, which is a reasonable scope call. Still worth a tracking issue since the gap is real and would matter if `--data-provider=postgres` is ever run against production-scale data.
- excerpt: |
    func (p *PostgresProvider) queryTestStatus(ctx context.Context, release string, start, end time.Time,
        _ crtest.JobVariants, includeVariants map[string][]string,
        dbGroupBy map[string]bool, capabilities []string) (map[string]crstatus.TestStatus, []error) {
    // reqOptions.Lifecycles is never forwarded here, no SQL lifecycle predicate exists

## Resolved (since previous review, in 652b348ee)

### [should-fix] Stale lifecycle/capability state when switching to a view without test_filters — FIXED
- where: `sippy-ng/src/component_readiness/CompReadyVars.jsx:432-443`
- resolution: Added explicit `else setTestLifecycles([])` / `else setTestCapabilities([])` branches, plus an outer `else` clearing both when `view.test_filters` is absent entirely. Also switched guards from truthy checks to `Object.hasOwn(...)`, matching the surrounding code style and closing the related consistency question from the first review.
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

### [nit] compareVariantsCheckedItems array/object type mismatch — FIXED (bonus, beyond original finding)
- where: `sippy-ng/src/component_readiness/CompReadyVars.jsx:190-207`
- resolution: `useState([])` changed to `useState({})`; both replace functions switched to React's functional updater form (`setX((prev) => ({...prev, [variant]: checkedItems}))`), eliminating a theoretical stale-closure race the first review didn't flag but an independent panel did.

### [nit] map[string]bool hand-rolled set violates project coding standard — NOT ADDRESSED, downgraded
- where: `pkg/api/componentreadiness/dataprovider/postgres/provider.go:97-107, 370-376`
- status: still present as originally reported (`capSet := make(map[string]bool, ...)`, `hasCapabilityIntersection(testCaps []string, requestedCaps map[string]bool)`). Not raised by the independent `/deep-review` panel either. Left as a style nit, not blocking; `hasCapabilityIntersection` now has a godoc comment and table-driven unit tests (`provider_test.go`), so the code is otherwise clean.

## Checked
- Spread-into-new-object fix for replaceIncludeVariantsCheckedItems and replaceCompareVariantsCheckedItems is correct (now functional-updater form, strictly better)
- BigQuery already had capability filtering (UNNEST(@Capabilities) in SQL) pre-PR; no BigQuery-side gap for capabilities
- reqOptions.Capabilities correctly threaded to both QueryBaseTestStatus and QuerySampleTestStatus
- Azure seed job (Platform:azure) correctly excluded from default view; seed data design is sound
- hasCapabilityIntersection now covered by table-driven unit tests in provider_test.go (6 cases: match, no-match, empty test caps, empty requested caps, multi-overlap, nil test caps)
- Production wiring: cmd/sippy/serve.go builds MixedProvider when BigQuery is configured; MixedProvider routes test-status queries to BigQuery, not Postgres — confirms the lifecycle-filter gap only affects the standalone `--data-provider=postgres` path (dev/devcontainer/CI-seed)
- Makefile churn (audit-level raised then reverted across 50e51aef9/3a24e874b) is a net no-op; reverted per human reviewer (stbenjam) request in inline PR comment
- ci/prow/lint is currently failing on HEAD, but author's comment confirms it reproduces on main independent of this PR (react-router npm audit advisory, no patch available yet) — not a regression introduced by this PR

## Open questions
- (resolved) Why is lifecycle filtering omitted from queryTestStatus — see Findings above; low severity, worth a follow-up issue.
- Any plan to file a tracking ticket for the Postgres-provider lifecycle-filter gap, or is `--data-provider=postgres` considered permanently dev-only and out of scope?
