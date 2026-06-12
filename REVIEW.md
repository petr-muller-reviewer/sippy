---
pr: openshift/sippy#3611
title: "TRT-1989: standardize lookback and partition begin grace"
head_sha: 49dd8c52414fe321daa4985dbcd8cc5fd6358bb4
base: main
reviewed_at: 2026-06-12T10:42:28Z
verdict: approve
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

### [nit] Duplicated loadSince-or-default pattern
- where: `pkg/dataloader/prowloader/prow.go:364-370`, `pkg/dataloader/prowloader/bigqueryjobs.go:22-30`
- concern: `resolveLoadSince()` centralizes the pattern for `ensurePartitions`, but `getTestAnalysisByJobFromToDates` and `fetchProwJobsFromOpenShiftBigQuery` still inline the same `if loadSince != nil` logic. Three copies of the same default. Not a bug — all use `DefaultLookbackDays` — but a future change to the default would need to touch all three sites.

### [question] No test for loadSince != nil in getTestAnalysisByJobFromToDates
- where: `pkg/dataloader/prowloader/prow_test.go:176`
- concern: All existing test cases pass `nil` for the new `loadSince` parameter. The `loadSince != nil` branch in the new-DB path (`lastDailySummary.IsZero()`) is untested. A test case with a non-nil `loadSince` and zero `lastDailySummary` would exercise the new code path.

## Checked
- `DefaultLookbackDays` is used consistently across all three fallback sites (bigqueryjobs, ensurePartitions, getTestAnalysisByJobFromToDates)
- The `-1` day grace on partition start is additive — it creates more partitions, never fewer, so no risk of breaking existing DBs
- `resolveLoadSince()` correctly replicates the prior inline logic
- Test signature change (`nil` third arg) preserves existing test semantics
- No exported API changes

## Open questions
- Would it make sense to refactor `getTestAnalysisByJobFromToDates` to accept a resolved `time.Time` (from `resolveLoadSince()`) instead of `*time.Time`, eliminating one copy of the pattern?
- Should the partition grace also apply to the BQ query time range in `fetchProwJobsFromOpenShiftBigQuery`, or is the 12-hour job-runtime adjustment (line 36) sufficient there?
