---
pr: openshift/sippy#3611
title: "TRT-1989: standardize lookback and partition begin grace"
head_sha: 43555f5c9c8256ee29759800247e5b314f65f8fe
base: main
reviewed_at: 2026-06-12T15:26:59Z
verdict: approve
refresh_log:
  - from: 49dd8c52414fe321daa4985dbcd8cc5fd6358bb4
    to: 43555f5c9c8256ee29759800247e5b314f65f8fe
    summary: "Author rebased onto main and added commit addressing mstaeble's nit: extracted resolveFrom() function to deduplicate loadSince-or-default pattern."
---

## Findings

### [should-fix] Stale doc comment says "one week" instead of 14 days
- where: `pkg/dataloader/prowloader/prow.go:197`
- concern: The `ensurePartitions` doc comment still says "otherwise looks back one week" but the behavior is now 14 days via `resolveLoadSince()`.
- excerpt: |
    //   - pl.loadSince if available, otherwise looks back one week

### [nit] Permalink in comment will rot
- where: `pkg/dataloader/prowloader/prow.go:208`
- concern: The comment links to a `blob/main/...#L473` GitHub permalink. As the file changes, this line number will drift. A reference to the function name (`fetchProwJobsFromOpenShiftBigQuery`) would be more durable.
- excerpt: |
    // https://github.com/openshift/sippy/blob/main/pkg/dataloader/prowloader/prow.go#L473 bq imports based on modified time which can include job_run_start_time a day earlier

### [resolved] Duplicated loadSince-or-default pattern
- where: `pkg/dataloader/prowloader/prow.go:126-131`, `pkg/dataloader/prowloader/prow.go:367`
- resolution: Author extracted standalone `resolveFrom(since *time.Time, to time.Time)`. `resolveLoadSince()` delegates to it, `getTestAnalysisByJobFromToDates` calls it directly. Two of three duplication sites eliminated. `bigqueryjobs.go:22-30` still inlines the pattern but has different semantics (side-effects on `pl.loadSince`).

### [question] No test for loadSince != nil in getTestAnalysisByJobFromToDates
- where: `pkg/dataloader/prowloader/prow_test.go:176`
- concern: All existing test cases pass `nil` for the new `loadSince` parameter. The `loadSince != nil` branch in the new-DB path (`lastDailySummary.IsZero()`) is untested. A test case with a non-nil `loadSince` and zero `lastDailySummary` would exercise the new code path.

## Checked
- `DefaultLookbackDays` is used consistently across all three fallback sites (bigqueryjobs, ensurePartitions, getTestAnalysisByJobFromToDates)
- The `-1` day grace on partition start is additive — it creates more partitions, never fewer, so no risk of breaking existing DBs
- `resolveLoadSince()` correctly replicates the prior inline logic
- Test signature change (`nil` third arg) preserves existing test semantics
- No exported API changes

## Since previous review
- Author rebased onto main (picked up ~15 unrelated merges) and added `43555f5c9 TRT-1989: update resolveFrom`.
- New commit extracts `resolveFrom()` as a standalone function, addressing mstaeble's inline nit and our "duplicated pattern" finding.
- `resolveLoadSince()` now delegates to `resolveFrom()`; `getTestAnalysisByJobFromToDates` calls `resolveFrom()` directly, replacing the inline `if/else`.
- mstaeble gave /lgtm, petr-muller approved with /hold to allow addressing the nit. LGTM was removed after the force-push. e2e tests passed on the new HEAD.

## Open questions
- Should the partition grace also apply to the BQ query time range in `fetchProwJobsFromOpenShiftBigQuery`, or is the 12-hour job-runtime adjustment (line 36) sufficient there?
