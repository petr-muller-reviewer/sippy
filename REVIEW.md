---
pr: openshift/sippy#3651
title: "[WIP] Add 14-day time bound to failed-tests matviews"
head_sha: d6528a046cd4f01ad85228784a4ca55eaa0dafca
base: main
reviewed_at: 2026-06-25T13:27:11Z
verdict: approve
---

## Findings

No findings.

## Checked
- The `prowJobFailedTestsMatView` template is used by both `prow_job_failed_tests_by_day_matview` and `prow_job_failed_tests_by_hour_matview`; the 15-day interval applies correctly to both via `|||TIMENOW|||` substitution.
- The only consumer (`PrintJobAnalysisJSONFromDB` in `pkg/api/job_analysis.go`) uses a 14-day default window (`PeriodToDates` returns 14 days for the `"default"` period). Custom `start`/`end` params can be passed via the API, but the matview query results are joined against `results.ByPeriod` which is populated from the time-bounded `sumResults` query — data outside the window is discarded by the `if _, ok` check at line 123.
- The 15-day interval (14 + 1 buffer) is consistent with the pattern used by `testAnalysisByJobMatView` at line 419 which also uses `|||TIMENOW||| - '14 days'::interval`.
- The added comment at `pkg/api/job_analysis.go:112` accurately documents the new constraint for future readers.
- `|||TIMENOW|||` replacement logic at `pkg/db/views.go:151,189` handles this correctly — same mechanism as all other matviews.

## Open questions
- The PR is marked WIP with a `do-not-merge/work-in-progress` label. The alternative PR #3647 removes the matviews entirely. Worth confirming which approach the team wants before merging either.
