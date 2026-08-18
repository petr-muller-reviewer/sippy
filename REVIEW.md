---
pr: openshift/sippy#3908
title: "Trt 2709 partitioning phase2 post migration"
head_sha: a34ab96af5dc9adb78bead1d7f1627eb41d90da4
base: main
reviewed_at: 2026-08-18T22:53:19Z
verdict: request-changes
---

## What this PR does

- Post-migration follow-up for the phase-2 table partitioning work (depends on #3907): scopes several queries to `prow_job_release` so Postgres can prune partitions instead of scanning across releases.
- Rewrites the `job_results()` Postgres stored function to add release/time-window scoping to its CTEs.
- Adds `query.CurrentActiveRelease` lookups in several call sites (autocomplete, releases, tests count) to determine which release to scope to.
- Changes `CleanupPartitions` to use a uniform 100-day cutoff for both detach and drop instead of a staggered 100/110-day window.
- Adds a two-step lookup pattern (`LookupProwJobRunPartitionKeys` + full row fetch) in `jobrunscan/reevaluate.go` to support querying a partitioned table by its partition key first.
- Ships `pkg/db/migrations/000001_create_partitioned_tables.up.sql` recreating the partitioned schema; drops the `Labels` GIN index annotation from the GORM model.

## Findings

### [blocking] CleanupPartitions lost its detach/drop safety window
- where: `pkg/db/db.go:364-394`
- concern: The doc comment still claims "Drops detached partitions older than 110 days" and "provides a 10-day safety window between detachment and permanent deletion", but the code now calls `DetachOldPartitions(100, ...)` immediately followed by `DropDetachedPartitions(100, ...)` with the same cutoff. `GetPartitionsForRemoval` matches by partition date range, not by a detach timestamp, so a partition that just crossed the 100-day threshold is detached and then permanently dropped in the same `CleanupPartitions()` call. There is no longer any window to notice and recover from a `DetachOldPartitions` bug before data is gone.
- excerpt: |
    // CleanupPartitions performs the full partition lifecycle cleanup:
    // 1. Detaches partitions older than 100 days
    // 2. Drops detached partitions older than 110 days
    //
    // This provides a 10-day safety window between detachment and permanent deletion.
    func (d *DB) CleanupPartitions(dryRun bool) (detached, dropped int, err error) {
    	detached, err = d.DetachOldPartitions(100, dryRun)
    	...
    	dropped, err = d.DropDetachedPartitions(100, dryRun)

### [blocking] jobRunsCount and testIDsCount scoped inconsistently
- where: `pkg/api/tests.go:414-440`
- concern: `jobRunsCount` is now filtered to a single release (`CurrentActiveRelease`), but the paired `testIDsCount` computation just below still iterates `release_definitions` and aggregates across every release. These two counts feed related Prometheus gauges (via `RefreshMetricsDB`); one becomes single-release, the other stays global, producing internally inconsistent metrics. This also contradicts the PR's apparent goal of making these filters result-preserving/partition-pruning only.
- excerpt: |
    var jobRunsCount int64
    err = dbc.DB.Table("prow_job_runs").
    	Where("prow_job_release = ?", release).
    	Where("timestamp > ? AND deleted_at IS NULL", today.AddDays(-lookbackDays).In(time.UTC)).
    	Count(&jobRunsCount).
    	Error
    ...
    var releases []string
    err = dbc.DB.Table("release_definitions").
    	Pluck("release", &releases).
    	Error

### [should-fix] job_results() last_pass CTE now bounded to report window
- where: `pkg/db/functions.go:122-128`
- concern: The rewritten `lp` CTE adds `AND prow_job_runs.timestamp BETWEEN p_start AND p_endstamp`, which the original (unbounded) `last_pass` CTE did not have. A job that hasn't passed since before `p_start` now reports `last_pass = NULL` instead of its true last-passing timestamp, silently breaking the "how long has this job been failing" signal for jobs that have been broken for a long time.
- excerpt: |
    lp AS (
        SELECT prow_job_runs.prow_job_id, max(prow_job_runs.timestamp)::timestamp without time zone as last_pass
        FROM prow_job_runs
        WHERE overall_result = 'S' AND prow_job_release = p_release
          AND prow_job_runs.timestamp BETWEEN p_start AND p_endstamp
        GROUP BY prow_job_runs.prow_job_id
    )

### [should-fix] ProwJobRunCount changed from all-time to 14-day window
- where: `pkg/db/query/job_queries.go:76-86`
- concern: `ProwJobRunCount` now hardcodes `timestamp > NOW() - INTERVAL '14 days'`, whereas the previous implementation (`ProwJobRunIDs`) counted all-time runs. This value feeds the `< 20` similar-job threshold check in `pkg/api/job_runs.go:684`, which falls back to a prior release when the count is low. A job with a long healthy history but low run frequency in the last 14 days (e.g. weekly periodics) now spuriously looks like it has insufficient data and takes a fallback path it never took before, changing risk-analysis job matching behavior.
- excerpt: |
    func ProwJobRunCount(dbc *db.DB, prowJobID uint, release string) (int, error) {
    	var count int64
    	q := dbc.DB.Table("prow_job_runs").
    		Where("prow_job_id = ?", prowJobID).
    		Where("prow_job_release = ?", release).
    		Where("timestamp > NOW() - INTERVAL '14 days'")

### [should-fix] ErrRecordNotFound from partition-key lookup miscategorized as eval error
- where: `pkg/api/jobrunscan/reevaluate.go:180-200`
- concern: `LookupProwJobRunPartitionKeys` failing with `gorm.ErrRecordNotFound` is unconditionally mapped to `ReEvalEvalError`, while the very next lookup (fetching the full row) correctly distinguishes `ErrRecordNotFound` → `ReEvalMissingError` from other errors. A build ID with no matching `prow_job_runs` row — a common, benign case for the scanner — now surfaces as an "eval error" instead of "missing", which likely affects downstream alerting/retry logic that treats the two statuses differently.
- excerpt: |
    partKeys, err := query.LookupProwJobRunPartitionKeys(r.db, jobRunID)
    if err != nil {
    	result.Status = ReEvalEvalError
    	result.Error = fmt.Sprintf("looking up partition keys for job run %s: %v", buildID, err)
    	return result
    }
    ...
    if res.Error != nil {
    	if errors.Is(res.Error, gorm.ErrRecordNotFound) {
    		result.Status = ReEvalMissingError
    	} else {
    		result.Status = ReEvalEvalError
    	}

### [should-fix] Cluster autocomplete silently returns empty list on CurrentActiveRelease failure
- where: `pkg/api/autocomplete.go:79-93`
- concern: If `query.CurrentActiveRelease` errors (e.g. `release_definitions` has no OCP release row, or a DB error), the failure is only logged at `Warn` and `clusterRelease` stays `""`. The query then runs `Where("prow_job_release = ?", "")`, which matches nothing, so the endpoint returns HTTP 200 with an empty list instead of surfacing the real backend failure. This masks a real problem as "no clusters found".
- excerpt: |
    clusterRelease := release
    if clusterRelease == "" {
    	var err error
    	clusterRelease, err = query.CurrentActiveRelease(dbc)
    	if err != nil {
    		log.WithError(err).Warn("could not determine current development release for cluster autocomplete")
    	}
    }
    q = q.Table("prow_job_runs")...Where("prow_job_release = ?", clusterRelease)

### [should-fix] GIN index on prow_job_runs.labels not recreated in new partitioned migration
- where: `pkg/db/migrations/000001_create_partitioned_tables.up.sql:22-30`
- concern: The `Labels` GORM tag no longer requests a GIN index in `pkg/db/models/prow.go`, and this migration's index list does not recreate one either. `pkg/api/componentreadiness/dataprovider/postgres/provider.go:412` still runs an array-containment query (`pjr.labels @> ARRAY['InfraFailure']`) against this column. Without the index this falls back to a sequential/partition scan — a latent performance regression that worsens as data grows.
- excerpt: |
    CREATE TABLE IF NOT EXISTS prow_job_runs (
        id BIGINT GENERATED BY DEFAULT AS IDENTITY,
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        deleted_at TIMESTAMP WITH TIME ZONE,
        ...
    -- no GIN index on labels in the index block that follows

### [should-fix] GetLastUpdateTime zero-time fallback not guarded by all callers
- where: `pkg/api/releases.go:673-684`
- concern: When no job runs exist for the active release in the last 14 days, `GetLastUpdateTime` returns the `COALESCE(..., '0001-01-01')` fallback with `err == nil`. `pkg/sippyserver/metrics/metrics.go` checks `IsZero()` before using the value, but `pkg/mcp/tools/releases.go:55-56` and `pkg/sippyserver/server.go:1357` (`jsonReleasesReportFromDB`) assign the value straight into `BuildReleasesResponse` without that guard. A data gap in the last 14 days for the active release surfaces as "last updated: 0001-01-01" in the API/MCP response instead of the previously-correct global last-update time.
- excerpt: |
    if err := dbc.DB.Raw("SELECT COALESCE(MAX(created_at), '0001-01-01') FROM prow_job_runs WHERE prow_job_release = ? AND timestamp > NOW() - INTERVAL '14 days'", rel).
    	Scan(&lastUpdated).Error; err != nil {
    	return time.Time{}, fmt.Errorf("query last update time: %w", err)
    }
    return lastUpdated, nil

## Checked
- Overall pattern of scoping queries to `CurrentActiveRelease` for partition pruning is sound where both sides of a comparison are scoped consistently.
- Two-step partition-key lookup pattern in `reevaluate.go` (fetch keys, then fetch full row) is a reasonable approach for querying partitioned tables without a full scan.
- New partitioned-table migration's column/type definitions match the previous unpartitioned schema for the tables inspected.

## Open questions
- `pkg/db/db.go`: was the detach/drop cutoff collapse to 100/100 intentional, or should the doc comment and the 110-day drop cutoff be restored? The comment still describes the old two-phase safety window.
- `pkg/api/tests.go`: should `testIDsCount` also be scoped to `CurrentActiveRelease`, or was leaving it global intentional (e.g. for a global test-ID cardinality metric)? If intentional, the doc/comment should say so since it now differs in scope from `jobRunsCount`.
- `pkg/db/functions.go`: is the `last_pass` time-bounding in the `lp` CTE deliberate (e.g. to bound the function's own runtime via partition pruning), accepting that jobs failing since before `p_start` will show `last_pass = NULL`?
- `pkg/db/query/job_queries.go`: is the 14-day window on `ProwJobRunCount` meant to change the `< 20` threshold's semantics in `findReleaseMatchJobNames`, or should that threshold be re-tuned/documented for the new window?
