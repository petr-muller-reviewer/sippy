---
pr: openshift/sippy#4150
title: "TRT-2631: Prep for BDD tests"
head_sha: a237d3036ae1dee6cf8be7586fe11e95917b3bce
base: main
reviewed_at: 2026-10-27T17:50:54Z
verdict: approve
---

# Review

## Verdict

Approve: no defect found in the merge-base diff; the remaining risk is test coverage of the new race guards, not the code itself.

The PR is narrow and internally consistent. It installs Playwright's Chromium runtime libraries in the CI build image, adds stable `data-testid` hooks for the upcoming BDD suite, and hardens Component Readiness against overlapping report fetches. I traced the guard through the success, error, and abort paths of each page, checked that the moved early returns no longer skip hooks, and confirmed that rendering the sidebar during the initial main-page load cannot touch uninitialized context (`App.jsx` gates routes on `/api/releases`, and `CompReadyVars` initializes `views`, `allJobVariants`, and `dbGroupByVariants` to `[]`). The triage changes (`null` regressions, form/list sync on same-id reload, inline delete errors) remove real crashes. CI is green on the reviewed SHA for unit, lint, e2e, build, and images.

## What this PR does

- Adds the X11/ATK/NSS/cairo/pango/alsa runtime packages Playwright's Chromium needs to `Dockerfile.buildroot`, so browser-driven tests can run in the CI build image.
- Adds `data-testid` attributes across Component Readiness: component/capability/test links, report cells, generate-report buttons, toolbar search/red-only controls, triage buttons and submit, force-close buttons, and the regressed-tests modal tabs.
- Adds `createLatestRequest` (`CompReadyUtils.jsx`) and applies it to the main report, capabilities, capability, capability-test, and test-details fetches, so a superseded response can no longer replace a newer selection or clear its loading state.
- Moves the `fetchError` and `CompReadyCancelled` returns below every hook in `ComponentReadiness.jsx` and `CompReadyEnvCapability.jsx`, fixing the "rendered fewer hooks" unmount; also hardens `gotFetchError` against non-string errors.
- Makes triage rendering tolerant of `regressions: null` (`UpdateTriagePanel`, `TriagedRegressionTestList`), keeps the update form and regression list in sync with a reloaded record (`UpsertTriageModal`, `TriagedRegressionTestList`), keeps the triage page mounted on same-id reload so force-close state survives, and reports delete failures inline instead of replacing the page.

## Findings

No findings.

## Checked

- Merge-base diff `bc130e0247f7fb0b6bda1788d72f39b214b37aa4..a237d3036`, 27 files, +311/−72, matches the PR's reported 311/72.
- Stale-guard semantics in all five pages: stale success, stale error, and stale `finally` are dropped; the newest request still sets `isLoaded`, including after `AbortError` replaces the shared `abortController`.
- Hook ordering: every `useState`/`useEffect`/`useQueryParam`/`useContext` call precedes the moved returns in `ComponentReadiness.jsx`, `CompReadyEnvCapability.jsx`, `CompReadyEnvCapabilityTest.jsx`, `CompReadyEnvCapabilities.jsx`, and `TestDetailsReport.jsx`.
- Safety of rendering `Sidebar`/`CompReadyMainInputs` while the main report is still loading: releases are loaded before routes render (`App.jsx` `if (!isLoaded) return Loading`), and all list props used by `CompReadyMainInputs`/`ReleaseSelector` start as `[]`/`{}`.
- `UpsertTriageModal` prop sync against all three call sites: `Triage.jsx` passes `triage` (update mode), `TestDetailsReport.jsx` and `RegressedTestsPanel.jsx` pass only `regressionIds`, so the new effect is a no-op in add mode.
- `CompReadyTestCell` `environment`/`testId` plumbing: `CompCapTestRow` is the only caller and now passes `columnNames[idx]` and `testCols.test_id`.
- `TriagedRegressionTestList` behavior for `undefined` (event-filled lists in `TriagedTestsPanel`) versus `null` (empty triage) and the new unit test for the `null` case.
- `data-testid` uniqueness within a rendered page, including the regressed-tests modal where `RegressedTestsTabPanel` renders children only for the active tab, so `triage-regressions` is not duplicated.
- `TriagePotentialMatches` removal of the second `setIsModalOpen(true)`: `findPotentialMatches` already sets it on entry, and the link-success path now closes the dialog and clears selection explicitly.
- PR checks on `a237d3036`: `ci/prow/unit`, `lint`, `e2e`, `build`, `images`, `integration`, `verify`, `security` all pass.

## Open questions

- `Triage.jsx` deliberately has no request-generation guard after `789e5a0e1`, so a slow load for the previous id can still be applied after navigating from `/triages/42` to `/triages/43`. Was that tradeoff only about keeping the force-close message mounted, or would a guard keyed on `id` (not a global sequence) preserve that behavior too?
- The guard itself is unit-tested (`createLatestRequest`), but nothing drives an out-of-order response through the page components. Will the BDD suite cover "slow older response arrives after a newer one" for at least the main report and test-details pages, or should a component-level regression test be added here?
- The main-route loading view now duplicates the provider stack plus `Sidebar` from the loaded layout. Was extracting a shared wrapper considered, so a future layout change cannot let the loading and loaded views diverge?
