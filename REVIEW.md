---
pr: openshift/sippy#3588
title: "TRT-2693: releasefallback panic"
head_sha: eb357be47cb2b9570446c4d8471886161e7bb027
base: main
reviewed_at: 2026-06-09T12:47:44Z
verdict: approve
---

## Findings

### [nit] unit test for nil-date case in FindStartEndTimesForRelease
- where: `pkg/api/componentreadiness/utils/utils.go:38-40`
- concern: The nil-date guard is the root cause fix. A unit test covering `FindStartEndTimesForRelease` with a `ReleaseTimeRange` that has nil `Start`/`End` would be a valuable regression test. Not blocking since the logic is trivially correct.
- excerpt: |
    if r.Start == nil || r.End == nil {
        return nil, nil, fmt.Errorf("release %s has no GA date", release)
    }

## Checked
- `EnqueueAsync` correctly calls `wg.Add(1)` before spawning the goroutine and `wg.Done()` inside it, matching the caller's `wg.Wait()`/`close(errCh)` pattern in `component_report.go:362-367`. Fixes real deadlock risk in `RegressionTracker.Query`/`QueryTestDetails` which previously did bare `errCh <- err` from a synchronous context.
- Bare `errCh <- bsErr` at `releasefallback.go:246-248` is inside a goroutine already tracked by `wg.Add(1)`/`defer wg.Done()` (lines 230-232), so it does not need `EnqueueAsync`.
- PR sample exclusion (`PullRequestOptions == nil` guard at `component_report.go:280`) correctly prevents fallback processing that is meaningless for PR samples and was part of the crash path.
- `QueryTestDetails` early return after `EnqueueAsync(wg, errCh, errs...)` at `releasefallback.go:198-199` is correct — avoids proceeding with nil/empty `timeRanges`.
- `getTestFallbackReleases` (line 377) has its own `End != nil && Start != nil` guard, which provides defense-in-depth for a separate code path. No redundancy issue.
- Variable rename `errs` to `bsErrs` in `QueryTestDetails` goroutine avoids shadowing the outer `errs`. Clean.
- Typo fix ("degredation" -> "degradation") and stale TODO removal are fine housekeeping.

## Open questions
- Would you consider adding a unit test for `FindStartEndTimesForRelease` covering the nil-date case? It's the exact crash scenario and would be a cheap regression guard.
