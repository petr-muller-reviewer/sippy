---
pr: openshift/sippy#3676
title: "Fix timezone bug in AdjustReleaseTime"
head_sha: e70471799a1f590a23cea4cc106151e90404d6e7
base: main
reviewed_at: 2026-06-24T14:04:02Z
verdict: approve
---

## Summary

One-line fix for an intermittent timezone bug in component readiness time window calculations. When `timeStr == "now"`, `ParseCRReleaseTime` set `relTime = time.Now()` (local timezone), then `AdjustReleaseTime` compared it against `time.Now().UTC()` by calendar date. After ~8pm EDT the UTC date advances while local date does not, causing the rounding-factor branch to be skipped and returning `23:59:59` for end times instead of `TruncateAligned`. The fix adds `relTime = relTime.UTC()` at the top of `AdjustReleaseTime`.

## Findings

### [nit] Root-cause call site still uses local time.Now()
- where: `pkg/util/utils.go:144`
- concern: `relTime = time.Now()` still uses local timezone. The fix in `AdjustReleaseTime` masks it correctly, but a future reader of line 144 gets no hint the timezone is wrong, and a future refactor removing the `relTime.UTC()` at line 179 (without knowing why it was added) would reintroduce the bug. The PR description says the fix should be at line 144 (`time.Now()` → `time.Now().UTC()`), but that's not what was done.
- excerpt: |
    case "now":
        relTime = time.Now()

## Checked

- `AdjustReleaseTime` fix (line 179) fires before both the day-comparison at line 191 and the `time.Date(relTime.Year(), ...)` extraction at lines 186/195 — normalization is complete before any day-sensitive operation.
- `releasedates.go:37` caller passes `*release.GADate` which is constructed as midnight UTC; the added `.UTC()` is a no-op there.
- The `ga` and `end` cases in `ParseCRReleaseTime` also benefit: if those times are ever non-UTC, `AdjustReleaseTime` now normalizes them.
- Tests in `TestParseCRReleaseTime` and `TestParseComponentReportRequest` compute expected values with `time.Now().UTC()` — they now agree with the UTC-normalized code path at all hours of the day.
- `TruncateAligned(now, factor, 0)` == `now.Truncate(factor)` when offset is 0, confirming the rounding test case is correct.

## Open questions

- Should line 144 also be changed to `time.Now().UTC()` to match the intent stated in the PR description and remove the latent inconsistency at the source?
