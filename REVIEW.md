---
pr: openshift/sippy#3839
title: "Extend partition start bound to cover outlier job start times"
head_sha: b184f2daeae0a017851c36b111d2217ec2f8e87d
base: main
reviewed_at: 2026-07-28T11:52:28Z
verdict: approve
---

## Summary

Fixes backup pod failures caused by a mismatch between BigQuery's completion-time filter and the start-time-based partition key. Adds `partitionStartDate()` which scans the fetched job set and extends the partition window to cover the earliest `StartTime`, instead of relying on a hardcoded grace period.

## Findings

### [nit] Doc comment says "floor" but means "default/ceiling"
- where: `pkg/dataloader/prowloader/prow.go:185`
- concern: "loadSince is used as a floor" is misleading. The function uses `loadSince - 1day` as the default start date and only moves it earlier (toward smaller values). `loadSince` acts as the upper bound on the default, not a lower bound. Suggest: "loadSince (minus a 1-day grace) is the default start date, extended earlier if any job's StartTime precedes it."
- excerpt: |
    // partitionStartDate computes the start of the date range for which partitions must
    // exist to accommodate the given prowJobs. loadSince is used as a floor, with a 1 day
    // grace period

### [nit] ensurePartitions doc says "one week" but default is 14 days
- where: `pkg/dataloader/prowloader/prow.go:205`
- concern: The comment says "otherwise looks back one week" but `resolveLoadSince()` falls back to `DefaultLookbackDays = 14`. Inaccurate inherited text worth correcting.
- excerpt: |
    //   - pl.loadSince if available, otherwise looks back one week, plus a 1 day grace period

### [question] Log when partition bound is extended beyond default grace
- where: `pkg/dataloader/prowloader/prow.go:193-201`
- concern: When `partitionStartDate` returns a date earlier than `loadSince - 1day`, it would help operators to see a log line noting the extension (e.g. the delta or earliest outlier time). Currently the only visibility is the existing log at line 214 which shows the final range but not whether it was extended.

## Checked
- `partitionStartDate` correctly skips zero-value `time.Time` via `!st.IsZero()`
- Range-over-index avoids copying `ProwJob` structs
- Non-BQ path (Prow JSON fetch) is also covered since `ensurePartitions` receives the full `prowJobs` slice regardless of source
- Test covers no-jobs, within-grace, zero-value, and outlier cases
- The 1-day grace period logic was moved from the `EnsurePartitions` call-site into `partitionStartDate`, consolidating it correctly
- O(n) scan over `prowJobs` is negligible compared to BQ query and DB operations

## Open questions
- Would it be worth logging a warning when the partition bound is extended beyond the default grace period, to give operators visibility into outlier jobs?
