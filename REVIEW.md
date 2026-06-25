---
pr: openshift/sippy#3685
title: "Batch BigQuery label fetching and skip GCS for cached job runs"
head_sha: cb42fa30b7722d680bf572e9334f4c05254916a4
base: main
reviewed_at: 2026-06-25T13:30:42Z
verdict: approve
---

## Findings

### [should-fix] Prefetch error silently degrades labels for entire batch
- where: `pkg/dataloader/prowloader/prow.go:272-276`
- concern: If `prefetchLabels` fails (BQ outage, credential expiry), `labelsCache` is set to `nil` and all workers continue. Every `pl.labelsCache[buildID]` lookup returns zero value from nil map, so all job runs import with empty labels. Old per-job code returned the error, causing that job to be skipped entirely (logged as "couldn't import job X, continuing"). New behavior is a silent data quality change: downstream consumers cannot distinguish "no labels in BQ" from "failed to query BQ." Consider initializing to an empty map on error, or logging a per-batch warning that labels were unavailable.
- excerpt: |
    labelsCache, err := pl.prefetchLabels(prowJobs)
    if err != nil {
        pl.errors = append(pl.errors, errors.Wrap(err, "error pre-fetching labels from BigQuery"))
    }
    pl.labelsCache = labelsCache

### [nit] No batching for large buildID arrays in BQ query
- where: `pkg/dataloader/prowloader/prow.go:1183`
- concern: All build IDs are passed in a single `IN UNNEST(@BuildIDs)` parameter. Staging tested 205k successfully, and production loads (16-21k) are well within BQ's 10MB parameter limit. No current risk, but no defensive batching exists for future growth scenarios (multi-day backfill, cluster expansion). Low priority.
- excerpt: |
    labels, err := GatherLabelsFromBQ(pl.ctx, pl.bigQueryClient, buildIDs, earliest)

## Checked
- Race condition on `labelsCache`: write at line 276 happens before worker goroutines spawn at line 287, Go memory model guarantees visibility. Safe.
- `createOrUpdateProwJob` called for cached runs: `prowJobCache` provides internal caching, so repeated calls for the same job spec are cheap in-memory lookups, not DB writes. Intentional design to keep job definitions current.
- Nil map read safety: Go returns zero value from nil map reads, no panic possible at line 999.
- `err` variable shadowing after moving prefetch: `:=` at line 272 introduces `labelsCache` and `err`, then `err =` at line 315 correctly reuses it. No shadowing issue.
- Log level change from Info to Debug for cached runs: reasonable for high-volume entries (~80% of jobs).

## Open questions
- On the prefetch error path: is importing runs with empty labels preferable to skipping them? The old code skipped individual runs on BQ label failure. If labels are decorative (not used for routing or decisions), silent degradation is fine. If they drive downstream logic, the error should be louder or block processing.
