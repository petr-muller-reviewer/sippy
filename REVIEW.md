---
pr: openshift/sippy#3816
title: "TRT-2822: Fix all no-unused-vars linter warnings in sippy frontend and enable error-level enforcement"
head_sha: 43233802798a53daadc6e439114696b791b06412
base: main
reviewed_at: 2026-07-25T21:32:53Z
verdict: approve
refresh_log:
  - from_sha: 43233802798a53daadc6e439114696b791b06412
    to_sha: 43233802798a53daadc6e439114696b791b06412
    at: 2026-07-25T21:32:53Z
    summary: No code changes. smg247 commented /retest; ci/prow/lint now passes (was previously failing on npm audit react-router CVE against a stale main base). Resolves the open question about the lint failure.
---

## Summary

Mechanical cleanup across 64 sippy-ng files fixing all 136 `no-unused-vars` ESLint warnings (mostly `_`-prefixing unused params/vars per existing `argsIgnorePattern`/`varsIgnorePattern: '^_'`), adds `caughtErrorsIgnorePattern: '^_'`, and flips the rule from `warn` to `error`. No intended behavior change.

## Findings

### [should-fix] Dead functions/state prefixed with `_` instead of deleted
- where: `sippy-ng/src/component_readiness/CompReadyEnvCapabilities.jsx`, `CompReadyEnvCapability.jsx`, `CompReadyEnvCapabilityTest.jsx`, `CompReadyVars.jsx`, `TestDetailsReport.jsx`, `sippy-ng/src/releases/PayloadStream.jsx`, `sippy-ng/src/bugs/BugButton.jsx`
- concern: `_cancelFetch` is defined but never called in 5 files (confirmed via grep — no other occurrence of `cancelFetch` in each file). `PayloadStream.jsx` has a full dead tab-switching implementation (`_currentTab`/`_setCurrentTab`/`_handleTabChange`) — verified the `Tabs` component has no `onChange`/`value` wired to it. `BugButton.jsx` has an entirely unused `[_open, _setOpen]` state pair. The PR already sets a precedent of deleting genuinely dead code (in `ComponentReadiness.jsx`, `copyPopoverEl`/`linkToReport`/`copyLinkToReport`/`currentPath` were removed outright) — these should get the same treatment rather than being underscore-prefixed, since they're not "intentionally unused parameters" but vestigial dead code that will confuse future readers.
- excerpt: |
    // PayloadStream.jsx
    const [_currentTab, _setCurrentTab] = useState(0)
    function _handleTabChange(_event, newValue) {
      console.warn('Setting new value ' + newValue)
      _setCurrentTab(newValue)
    }

### [nit] Inconsistent destructuring style for one unused-prop rename
- where: `sippy-ng/src/pull_requests/PullRequestsTable.jsx`
- concern: `const { classes } = props` became `const _classes = props.classes` instead of `const { classes: _classes } = props`, the pattern used everywhere else in this PR for destructured unused values. Functionally identical, just stylistically inconsistent.
- excerpt: |
    const _classes = props.classes

## Resolved

### [question] CI lint job is failing — unrelated to this PR's changes?
- where: `ci/prow/lint` check on PR #3816
- concern: The lint job failed, but `npx eslint .` completed cleanly in the log (no errors printed) — the failure was downstream in `npm audit --omit=dev` flagging pre-existing `react-router` CVEs (GHSA-qwww-vcr4-c8h2), matching the unrelated recent main-branch commits about npm audit thresholds.
- resolution: Confirmed unrelated to this PR. After a `/retest` (smg247, 2026-07-25T18:26:29Z) re-ran against a newer `main` base (`1c5aaf7ea5ee2c928ab67d586c150fa25442c044`), `ci/prow/lint` now passes. All CI checks are green except `ci/prow/e2e` (pending) and `tide` (pending merge-label gate).

## Checked
- All renamed `theme`/`props`/`index`/`event` params were only renamed where ESLint had already confirmed they were unused in the function body — safe.
- `ComponentReadiness.jsx` dead-code removal (`copyPopoverEl`, `linkToReport`, `copyLinkToReport`, `currentPath`) — grepped the PR-head file, no remaining references, no orphaned callers.
- `eslint.config.mjs` change (`caughtErrorsIgnorePattern: '^_'`, `warn`→`error`) is straightforward and matches stated goal.
- `catch (e)` → `catch (_e)` renames in `helpers.jsx`/`FeatureGates.jsx` are consistent and low-risk.
- No test changes — appropriate for a pure lint/rename cleanup with no behavioral change.

## Open questions
- Should the `_cancelFetch` functions (6 occurrences), `PayloadStream`'s dead tab-switching code, and `BugButton`'s unused `open` state be deleted outright instead of underscore-prefixed, for consistency with the dead-code removal already done in `ComponentReadiness.jsx`?
