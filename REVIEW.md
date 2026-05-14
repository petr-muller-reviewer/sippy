---
pr: openshift/sippy#3498
title: "TRT-2647: rhcos10 default for 5.0"
head_sha: 8f534523fdd5649708777bdf4750306f52adebb4
base: master
reviewed_at: "2026-05-14T17:49:43Z"
verdict: comment
refresh_log:
  - from: 8f534523fdd5649708777bdf4750306f52adebb4
    to: 8f534523fdd5649708777bdf4750306f52adebb4
    summary: "No code changes. Incorporated review discussion: dgoodwin wants regression tracking on, petr-muller raised update-job OS detection gap, neisw acknowledged both."
---

# PR #3498 — TRT-2647: rhcos10 default for 5.0

**Author:** neisw (Forrest Babcock)
**Assignee:** petr-muller
**Labels:** approved, do-not-merge/hold, ready-for-human-review
**Lines:** +1818 / -1900

## What This PR Does

Changes the default OS variant for OCP 5.0 and main/master-branch jobs from `rhcos9` to `rhcos10`. Updates component readiness views to match: removes explicit `OS: rhcos9` filter from 5.0 main views (so both OS variants show up), reworks the 5.0 OS comparison view, and removes the now-obsolete 4.22 OS comparison view.

## Files Changed

- `pkg/variantregistry/ocp.go` — Core logic: `setOS()` fallback for 5.0/main now returns `rhcos10`
- `pkg/variantregistry/ocp_test.go` — Seven test expectations updated to `rhcos10`
- `config/views.yaml` — View config: removed OS filter from 5.0 main views, reworked comparison view, removed 4.22 comparison view
- `pkg/variantregistry/snapshot.yaml` — Mechanical `rhcos9` to `rhcos10` for 5.0/main jobs (~1600 lines)

## Activity Since Previous Review

No new commits. Discussion since 2026-05-07:

- **dgoodwin** (2026-05-08): Flagged relaxed thresholds on `config/views.yaml`, asked if intentional. "I would probably keep regression tracking on as well." Separately: "Everything else looks good. Holding until we hear it's flipped sounds good."
- **neisw** (2026-05-12): Responded on `config/views.yaml` citing TRT-2647 Jira card guidance to "minimally monitor that 9 doesn't get worse than 10."
- **dgoodwin** (2026-05-12): Agreed: "yes tracking would be useful here."
- **petr-muller** (2026-05-12): Asked on `pkg/variantregistry/ocp.go`: "we'll need the special treatment for update jobs as discussed on slack, right?"
- **neisw** (2026-05-12): Acknowledged: "details details..."
- CI: e2e tests triggered and passed (2026-05-12).

## Findings

### Question: Update jobs need special OS detection

petr-muller raised that update jobs may need special treatment for OS variant detection (discussed on Slack). neisw acknowledged but hasn't addressed it in code yet. This is a gap in the current `setOS()` logic — update jobs that cross OS boundaries (e.g. upgrading from rhcos9 to rhcos10) may not be correctly classified by the current fallback.

### Partially Resolved: Relaxed thresholds and disabled regression tracking

The `5.0-rhcos10-vs-rhcos9` comparison view has significantly relaxed settings compared to the old `5.0-techpreview-rhcos9-vs-rhcos10`:

- `regression_tracking: false` — regressions won't be tracked/filed as Jira bugs
- `pity_factor`: 5 to 10 — more forgiving of failures
- `minimum_failure`: 3 to 4 — needs more failures before flagging
- `include_multi_release_analysis: false`
- `pass_rate_required_new_tests: 90` (added)

dgoodwin flagged this and asked if intentional. neisw cited the Jira card guidance to "minimally monitor." dgoodwin then agreed that regression tracking specifically should be enabled. Consensus: **re-enable `regression_tracking: enabled: true`**. The other relaxed thresholds appear intentional per the Jira card's "minimal monitoring" directive.

### OK: Core logic change

The `setOS` fallback at `ocp.go:1315-1316` is clean and correct. Explicit rhcos pattern matching earlier in the function still correctly handles jobs that reference an OS in their name.

### OK: Removing OS filter from 5.0 main views

`OS: rhcos9` removed from `5.0-main`, `5.0-rosa`, `5.0-main-mass-failure`. Views now pick up all OS variants by default. Answers the author's open question from the PR description — simpler than listing both.

### OK: Comparison view direction

Renaming to `5.0-rhcos10-vs-rhcos9` and swapping base/compare is semantically correct — the new default (rhcos10) should be the base.

### OK: 4.22 comparison view removal

`4.22-techpreview-rhcos9-vs-rhcos10` removed. No longer needed for a 4.x release.

### OK: Tests and snapshot

Tests and snapshot are mechanical and consistent with the logic change.
