---
pr: openshift/sippy#3534
title: "TRT-1989: add go_partman"
head_sha: 299278a70d2cd97fff21a975868c333d192d8e76
base: master
reviewed_at: 2026-05-18T00:39:27Z
verdict: approve
---

## Summary

Adds automatic partition management for `test_analysis_by_job_by_dates` using the `go_partman` library. Wrapper in `pkg/db/partitionmanager/`, integrated into load (Maintain + per-date EnsurePartition) and serve (background ticker) paths. Vendor patch fixes go_partman's multi-tenant assumption for non-tenant tables. E2E tests cover creation, idempotency, retention, data routing, and SQL injection rejection.

## Findings

### [nit] dropPartition in go_partman uses unquoted identifiers
- where: `vendor/github.com/jirevwe/go_partman/queries.go:105`
- concern: `DROP TABLE IF EXISTS %s.%s;` interpolates schema and table name without quoting. The values come from `pg_tables` so they are database-controlled, not user-controlled, but this diverges from sippy's own `EnsurePartition` which quotes identifiers. Worth noting in vendor patch doc or upstream issue.
- excerpt: |
    var dropPartition = `DROP TABLE IF EXISTS %s.%s;`

### [nit] Maintain() + EnsurePartition() dual strategy lacks explanatory comment
- where: `pkg/dataloader/prowloader/prow.go:379-406`
- concern: `Maintain()` creates future partitions and drops expired ones, then `EnsurePartition()` handles the specific historical date being imported. The two-tier strategy is correct but non-obvious. A brief comment at line 379 explaining why both are needed would help future readers.
- excerpt: |
    if pl.partMgr != nil {
        if err := pl.partMgr.Maintain(ctx); err != nil {
            log.WithError(err).Warn("partition manager maintenance failed, will create partitions on demand")
        }
    }
    ...
    if err := partitionmanager.EnsurePartition(pl.dbc.DB, "test_analysis_by_job_by_dates", dateToImport, nextDay); err != nil {

### [question] context.Background() in serve.go partition manager startup
- where: `cmd/sippy/serve.go:261`
- concern: Creates a new `context.Background()` rather than deriving from the command's context. If the cobra command has signal-handling context, the partition manager won't respond to graceful shutdown signals through context cancellation, only via `defer partMgr.Stop()`. This may be intentional (Stop() is the primary shutdown path) but worth confirming.
- excerpt: |
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    if startErr := partMgr.Start(ctx); startErr != nil {

### [question] 365-day retention default
- where: `pkg/db/partitionmanager/manager.go:36`
- concern: `RetentionPeriod: 365 * 24 * time.Hour` -- is this aligned with the existing data lifecycle for `test_analysis_by_job_by_dates`? If no prior pruning policy existed, 365 days is reasonable, but should be confirmed against actual usage patterns.
- excerpt: |
    RetentionPeriod: 365 * 24 * time.Hour,

## Checked

- `EnsurePartition` validates table names via `validIdentifier` regex and dates via `time.Parse` before DDL interpolation -- safe against SQL injection
- Vendor patch `SIPPY_VENDOR_PATCHES.md` thoroughly documents the empty-string tenant fix with guard-condition analysis across all code paths
- Graceful degradation: partition manager failures are warnings, never fatal, in both load and serve paths
- E2E tests cover partition creation, idempotency, data routing through parent table, retention-based drop, invalid table name rejection, and invalid date rejection
- `dropPartmanState` cleanup in tests properly records errors via `t.Logf` instead of silently ignoring them (addressed in latest commit)
- `go_partman` embedded web assets placeholder resolves the `//go:embed web/dist` build failure without pulling in unused UI code

## Open questions

- Is `context.Background()` in `serve.go:261` intentional, or should the partition manager derive its context from the cobra command's context for signal-aware shutdown?
- Does the 365-day retention period match the expected data lifecycle? Is there an existing pruning policy this should align with?
