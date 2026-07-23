---
pr: openshift/sippy#3808
title: "TRT-2815: Add partition pruning to payload test failure queries"
head_sha: 6c82ce5e7c7a9ae604ca8665a7506008720f4b66
base: main
reviewed_at: 2026-07-23T11:45:52Z
verdict: approve
---

## Summary

`GetTestFailuresForPayload` and `payloadTestFailuresMatView` join `prow_job_run_tests` (LIST-partitioned by `prow_job_run_release`, RANGE-sub-partitioned by `prow_job_run_timestamp`, ~3,500 partitions) without any predicate on those partition keys, so the planner scans all partitions and `/api/payloads/test_failures` times out (>60s planning, >90s execution). Fix passes the already-loaded `ReleaseTag.Release`/`ReleaseTag.ReleaseTime` into the raw query as bind params, and adds equivalent join predicates to the matview definition, to enable partition pruning (2-5 partitions, ~6ms planning, ~115ms execution per PR benchmarks).

## Findings

No blocking or should-fix findings. Two low-severity notes below.

### [nit] ambiguous positional string/time params
- where: `pkg/db/query/payload_queries.go:50`
- concern: `func GetTestFailuresForPayload(db *gorm.DB, payloadTag, release string, releaseTime time.Time)` has two adjacent `string` params (`payloadTag`, `release`) with generic names; a future caller could pass the wrong field (e.g. `payload.Architecture` instead of `payload.Release`) without a compile error. Current single call site (`pkg/api/releases.go:234`) is correct.
- excerpt: |
    func GetTestFailuresForPayload(db *gorm.DB, payloadTag, release string, releaseTime time.Time) ([]models.PayloadFailedTest, error) {

### [question] no committed regression test for the pruning behavior
- where: `pkg/db/query/payload_queries.go`, `pkg/db/views.go:280-327`
- concern: verification described in the PR body (`EXPLAIN ANALYZE` on staging, row-count parity) is manual/staging-only. Is there an existing functional-test harness (real-DB, gated on env vars per `releasesync_functional_test.go` pattern) where a regression test asserting partition-scan count or row-count parity would fit, to prevent someone from loosening these predicates later and silently reintroducing the full scan?
- excerpt: |
    AND pjrt.prow_job_run_release = ?
    AND pjrt.prow_job_run_timestamp >= ?

## Checked

- `prow_job_run_tests.prow_job_run_release` is populated from `ProwJob.Release` (`pkg/dataloader/prowloader/prow.go:1116`), matching `ReleaseTag.Release` string semantics (e.g. `"4.15"`) — the new equality bind is comparing like-for-like values.
- `prow_job_run_tests.prow_job_run_timestamp` and `prow_job_runs.timestamp` are both set from the same source (`pj.Status.StartTime`, denormalized at insert time) — confirms the matview's new `pjrt.prow_job_run_timestamp = pjr.timestamp` equality join in `pkg/db/views.go:310` is a correctness-neutral pruning hint, not a lossy filter.
- Partition scheme confirmed in `pkg/db/migrations/000001_create_partitioned_tables.up.sql`: LIST by `prow_job_run_release` → RANGE by `prow_job_run_timestamp`, matching the PR's stated pruning strategy.
- The `>=` (not exact/bounded) filter on `releaseTime` in the raw query is intentional and reasonable: job runs against a payload can only occur at or after its `release_time`, and there's no natural upper bound since failures should remain visible indefinitely.
- No BigQuery equivalent of this matview/query exists, so the "provider parity" convention in CLAUDE.md doesn't apply here.
- Single call site (`pkg/api/releases.go:234`) updated correctly; no other callers of `GetTestFailuresForPayload` in the repo.
- No injection risk: `release`/`releaseTime` come from an already-fetched trusted `ReleaseTag` row, not directly from request input; both new params are passed as bind params, not interpolated.

## Open questions

- Given no committed regression test, would you consider adding one against a real (seeded) partitioned-table fixture, or is the staging `EXPLAIN ANALYZE` verification considered sufficient given the narrow scope of this fix?
