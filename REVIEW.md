---
pr: openshift/sippy#3685
title: "Batch BigQuery label fetching and skip GCS for cached job runs"
head_sha: b8e53133bed6c8b3b8f3edcdbeed86d4579741af
base: main
reviewed_at: 2026-06-25T14:09:41Z
verdict: approve
gate:
  decision: merge
  gated_at: 2026-06-25T13:34:45Z
  gated_head_sha: cb42fa30b7722d680bf572e9334f4c05254916a4
  reviewed_head_sha: cb42fa30b7722d680bf572e9334f4c05254916a4
refresh_log:
  - from: cb42fa30b7722d680bf572e9334f4c05254916a4
    to: b8e53133bed6c8b3b8f3edcdbeed86d4579741af
    summary: "Author addressed should-fix: switched to if-init syntax so labelsCache is only set on success and err doesn't leak into function scope"
---

## Gate

**Merge.** No code changes since the review. The should-fix finding (silent label degradation on BQ prefetch failure) is a real concern but not a merge blocker: labels are decorative metadata, not routing-critical, and the error is recorded in `pl.errors`. No backward-incompatible API, config, or behavioral changes affect external consumers. CI passed.

Since refresh: the should-fix finding has been addressed in `b8e53133`. Gate verdict unchanged.

### Area 1 -- findings disposition

- **[should-fix] Prefetch error silently degrades labels**: addressed in `b8e53133`. Author switched to if-init syntax so `labelsCache` is only assigned on success.
- **[nit] No batching for large buildID arrays**: not addressed, not expected to be. No current risk.

### Area 2 -- merge risk

- **No exported API changes.** `prefetchLabels` and `labelsCache` are both unexported. No public types, functions, or struct fields were added, removed, or changed.
- **No configuration changes.** No flags, env vars, or config schema changes.
- **Behavioral change is internal only.** The reordering of `createOrUpdateProwJob` before the cache check and the bulk label prefetch change internal data-loading behavior. External consumers (API, frontend) see the same data. The only observable difference: on BQ label prefetch failure, jobs are now imported with empty labels instead of being skipped individually. This is a softer failure mode, not a harder one.
- **No blast radius beyond this service.** Changes are confined to the prow data loader; no shared library surface is affected.

## Findings

### Resolved

#### [should-fix] Prefetch error silently degrades labels for entire batch
- where: `pkg/dataloader/prowloader/prow.go:272-276`
- resolved in: `b8e53133` -- switched to if-init syntax (`if lc, err := ...; err == nil`), so `labelsCache` is only assigned on success. On failure, it stays at its zero value (`nil`), which is the same runtime behavior, but the code now clearly expresses intent and the `err` variable no longer leaks into function scope (the `err =` at old line 315 reverts to `:=`).

### [nit] No batching for large buildID arrays in BQ query
- where: `pkg/dataloader/prowloader/prow.go:1183`
- concern: All build IDs are passed in a single `IN UNNEST(@BuildIDs)` parameter. Staging tested 205k successfully, and production loads (16-21k) are well within BQ's 10MB parameter limit. No current risk, but no defensive batching exists for future growth scenarios (multi-day backfill, cluster expansion). Low priority.
- excerpt: |
    labels, err := GatherLabelsFromBQ(pl.ctx, pl.bigQueryClient, buildIDs, earliest)

## Checked
- Race condition on `labelsCache`: write at line 276 happens before worker goroutines spawn at line 287, Go memory model guarantees visibility. Safe.
- `createOrUpdateProwJob` called for cached runs: `prowJobCache` provides internal caching, so repeated calls for the same job spec are cheap in-memory lookups, not DB writes. Intentional design to keep job definitions current.
- Nil map read safety: Go returns zero value from nil map reads, no panic possible at line 999.
- `err` variable scoping after moving prefetch: if-init syntax at line 272 keeps `err` scoped to the if block; `err :=` at line 315 is independent. Clean.
- Log level change from Info to Debug for cached runs: reasonable for high-volume entries (~80% of jobs).

## Open questions
- On the prefetch error path: is importing runs with empty labels preferable to skipping them? The old code skipped individual runs on BQ label failure. If labels are decorative (not used for routing or decisions), silent degradation is fine. If they drive downstream logic, the error should be louder or block processing.
