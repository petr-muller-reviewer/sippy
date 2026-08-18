---
pr: openshift/sippy#3907
title: "Trt 2709 partitioning phase2 query partitioning"
head_sha: 63c97d78939add0e89432350b2fd1035b0cb6d32
base: main
reviewed_at: 2026-08-17T15:05:35Z
verdict: request-changes
---

## Summary

Adds partition-pruning filters (`prow_job_release`, timestamp bounds) across
many queries so Postgres can skip partitions on large tables. Several of
these filters narrow the effective query scope compared to previous
behavior, and downstream consumers of the narrowed results were not
updated to match, or the narrowing has externally-visible correctness
implications.

## Findings

### [blocking] job_results last_pass window narrowed
- where: `pkg/db/functions.go:122-126`
- concern: the `lp` CTE computing `last_pass` (max last-passed timestamp) is now bounded to `prow_job_runs.timestamp BETWEEN p_start AND p_endstamp` (the reporting window), whereas previously it looked at the full history for the release. A job whose last pass predates the window now reports `last_pass = NULL` instead of the true last-pass timestamp. This is user-visible as the "Last pass" column in the Jobs UI.
- excerpt: |
    lp AS (
        SELECT prow_job_runs.prow_job_id, max(prow_job_runs.timestamp)::timestamp without time zone as last_pass
          ...
          AND prow_job_runs.timestamp BETWEEN p_start AND p_endstamp

### [blocking] ProwJobRunCount hardcoded 14-day window breaks fallback threshold
- where: `pkg/db/query/job_queries.go:69-78`
- concern: `ProwJobRunCount` (used in `pkg/api/job_runs.go` `findReleaseMatchJobNames`, summed into `totalJobRuns`) now hardcodes `timestamp > NOW() - INTERVAL '14 days'`, replacing what was previously an all-time count via `ProwJobRunIDs`. The summed total feeds `if totalJobRuns < 20` in `pkg/api/job_runs.go:582` which triggers a fallback to the prior release. A job with hundreds of historical runs but fewer than 20 in the last 14 days will now spuriously look data-starved and trigger the fallback, changing test-rate comparison behavior unrelated to partition pruning.
- excerpt: |
    func ProwJobRunCount(dbc *db.DB, prowJobID uint, release string) (int, error) {
    	var count int64
    	q := dbc.DB.Table("prow_job_runs").
    		Where("prow_job_id = ?", prowJobID).
    		Where("prow_job_release = ?", release).
    		Where("timestamp > NOW() - INTERVAL '14 days'")

    // pkg/api/job_runs.go:582
    if totalJobRuns < 20 {
    	// falls back to prior release

### [should-fix] cluster autocomplete silently filters to empty release
- where: `pkg/api/autocomplete.go:80-91`
- concern: `query.CurrentActiveRelease(dbc)` can return `("", nil)` (no matching row in `release_definitions`, e.g. under-populated dev/seed DB). On error the code only logs a warning and continues; on the `("", nil)` case there's no error at all. Either way `clusterRelease` ends up `""` and gets applied as `Where("prow_job_release = ?", "")`, silently returning an empty list with HTTP 200 instead of surfacing that no release could be determined.
- excerpt: |
    clusterRelease, err = query.CurrentActiveRelease(dbc)
    if err != nil {
    	log.WithError(err).Warn("could not determine current development release for cluster autocomplete")
    }
    ...
    Where("prow_job_release = ?", clusterRelease).

### [should-fix] jobRunsCount and testIDsCount lose scope parity
- where: `pkg/api/tests.go:414-436`
- concern: `jobRunsCount` is now scoped to a single `CurrentActiveRelease`, but `testIDsCount` (computed afterward via `release_definitions` loop) still spans every release. These two values are exported together as related Prometheus gauges (`matViewUniqueNumberOfJobRuns` / `matViewUniqueNumberOfTests` in `pkg/sippyserver/server.go`). After this change they no longer represent comparable scopes, silently changing the semantics of the job-runs metric.
- excerpt: |
    release, err := query.CurrentActiveRelease(dbc)
    ...
    err = dbc.DB.Table("prow_job_runs").
    	Where("prow_job_release = ?", release).
    	Where("timestamp > ? AND deleted_at IS NULL", ...).
    	Count(&jobRunsCount).
    ...
    err = dbc.DB.Table("release_definitions").
    	Pluck("release", &releases)
    // testIDsCount later aggregated across all `releases`

## Checked
- Diffed full PR against upstream/main; traced call sites of every function whose query semantics changed.
- Verified the top findings against actual downstream consumers, not just the diff (job_runs.go fallback logic, sippyserver.go metric exports).
- General shape of partition-pruning filters (prow_job_release + timestamp bounds) is consistent with the stated goal (trt-2709) and matches the pattern used in prior commits on this branch.

## Open questions
- For `job_results.last_pass`: was bounding it to the reporting window intentional (e.g. to match partition pruning), or should it use an unbounded/wider lookup and accept the extra partition scan?
- For `ProwJobRunCount`: should the 14-day window be a parameter (matching the caller's actual comparison window) rather than hardcoded, given it's reused in a `< 20` threshold that assumed all-time counts?
- For cluster autocomplete: should an unresolved `CurrentActiveRelease` return an error response instead of silently filtering to an empty release?
- For `tests.go`: was narrowing `jobRunsCount` to the current release intentional, and if so should `testIDsCount` be narrowed to match, or should the metric names/docs be updated to reflect the new asymmetric scope?
