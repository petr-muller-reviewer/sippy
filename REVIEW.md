---
pr: openshift/sippy#3902
title: "TRT-2867: Move /api/jobs/runs/reevaluate to async workflow via sippy-daemon"
head_sha: bd2a6ff45aac01c5e93c5338fee315756e82b416
base: main
reviewed_at: 2026-08-17T22:08:28Z
verdict: request-changes
---

## What this PR does

- Moves `POST /api/jobs/runs/reevaluate` from synchronous execution to an async batch workflow: non-dry-run requests are enqueued as River (PostgreSQL-backed queue) jobs and processed by sippy-daemon.
- Adds `pkg/sippyserver/workqueue` (Submitter, StatusQuerier) plus `workqueue_batches`/`workqueue_batch_items` migration to track batch/item state against River job rows.
- Wires a River worker (`ReevaluateWorker`) into sippy-daemon that delegates to the existing `jobrunscan.ReEvaluator.reEvaluateOne` logic.
- Adds `GET /api/jobs/runs/reevaluate/{batch_id}` for polling batch status (counts + per-item state).
- dry_run requests keep the old synchronous path (`reEvaluateSynchronous`), unchanged apart from the new branch in `jsonReEvaluateJobRunSymptoms`.
- Updates docs describing async processing, polling, batching, dry-run.

## Findings

### [blocking] async ReEvaluator's symptom cache is never populated in sippy-daemon
- where: `cmd/sippy-daemon/main.go:202`, `pkg/api/jobrunscan/reevaluate_worker.go:52-60`
- concern: `setupRiverProcess()` constructs `evaluator := jobrunscan.NewReEvaluator(...)` and registers `NewReevaluateWorker(evaluator)`, but nothing calls `evaluator.RefreshSymptomCache()` on that instance. `RefreshSymptomCache()` is only called on a *separate*, short-lived `ReEvaluator` created in `reEvaluateAsync` (`pkg/sippyserver/job_run_scan.go:238-239`), purely to compute the `symptomHash` used for River job dedup — that instance is discarded after the request. `ReevaluateWorker.Work` calls `w.evaluator.CachedSymptoms()`, which is nil on the daemon's evaluator, so every job returns `"symptom cache not initialized"` and fails after `MaxAttempts=3`. As wired, no async re-evaluation job can ever succeed.
- excerpt: |
    // cmd/sippy-daemon/main.go
    evaluator := jobrunscan.NewReEvaluator(bqClient, gcsClient, f.GoogleCloudFlags.StorageBucket, dbc, cacheClient, artifactMgr, false)
    workers := river.NewWorkers()
    river.AddWorker(workers, jobrunscan.NewReevaluateWorker(evaluator))
    // no RefreshSymptomCache() call anywhere on `evaluator`

    // pkg/api/jobrunscan/reevaluate_worker.go
    func (w *ReevaluateWorker) Work(ctx context.Context, job *river.Job[ReevaluateJobRunArgs]) error {
        symptoms := w.evaluator.CachedSymptoms()
        if symptoms == nil {
            return fmt.Errorf("symptom cache not initialized")
        }

### [should-fix] async path skips ValidateReEvalRequest, allowing malformed build IDs to reach the queue
- where: `pkg/sippyserver/job_run_scan.go:223-236`
- concern: `reEvaluateSynchronous` (line 207) calls `apijobrunscan.ValidateReEvalRequest(buildIDs)` before doing any work. `reEvaluateAsync` only checks `len(buildIDs)==0` and `>MaxJobRunsPerBatch`; it never validates individual IDs. A request with a non-numeric build ID is accepted, enqueued as a River job, and only fails later inside the worker/BigQuery query — burning a queue slot and surfacing as an opaque per-item failure in batch status instead of an immediate 400.
- excerpt: |
    if len(buildIDs) == 0 {
        failureResponse(w, http.StatusBadRequest, "prow_job_build_ids is required")
        return
    }
    if len(buildIDs) > apijobrunscan.MaxJobRunsPerBatch {
        failureResponse(w, http.StatusBadRequest, fmt.Sprintf("maximum %d job runs per batch", apijobrunscan.MaxJobRunsPerBatch))
        return
    }
    // no ValidateReEvalRequest(buildIDs) call here

### [should-fix] buildBatchItems pairs River job IDs to item keys purely by array index
- where: `pkg/sippyserver/workqueue/submitter.go:56, 73, 105-115`
- concern: `s.riverClient.InsertMany(ctx, items)` returns `[]*rivertype.JobInsertResult` which `buildBatchItems` zips against `itemKeys` by position (`itemKeys[i]`). This assumes River's underlying bulk `INSERT ... SELECT FROM unnest(...) ... ON CONFLICT ... RETURNING` (no `ORDER BY`) preserves the input array order in its `RETURNING` output. That's true today for the vendored implementation, but it's an implicit ordering contract on an unordered SQL statement, not something documented/enforced by the River client. If it ever doesn't hold (e.g. under conflict handling or a differing query plan), a batch item silently reports the wrong build's re-evaluation state.
- excerpt: |
    func buildBatchItems(batchID uuid.UUID, results []*rivertype.JobInsertResult, itemKeys []string) []BatchItem {
        items := make([]BatchItem, len(results))
        for i, r := range results {
            items[i] = BatchItem{
                BatchID:    batchID,
                RiverJobID: r.Job.ID,
                ItemKey:    itemKeys[i],
            }
        }
        return items
    }

### [nit] batch status response has no HATEOAS links
- where: `pkg/sippyserver/workqueue/status.go:21-30`, `pkg/sippyserver/job_run_scan.go:264-288`
- concern: Project convention (CLAUDE.md) requires HATEOAS in API responses. The sync re-eval response is annotated via `InjectReEvalHATEOASLinks` (job_run_scan.go:219), and the batch-submit response includes a manual `links.status` (job_run_scan.go:258), but `jsonReEvaluateBatchStatus` returns the raw `BatchStatusResponse` with no `links` field, so a client polling status has no self/related links.
- excerpt: |
    type BatchStatusResponse struct {
        BatchID   uuid.UUID    `json:"batch_id"`
        Status    BatchStatus  `json:"status"`
        Total     int          `json:"total"`
        Completed int          `json:"completed"`
        Failed    int          `json:"failed"`
        Running   int          `json:"running"`
        Pending   int          `json:"pending"`
        Items     []ItemStatus `json:"items"`
    }

## Checked
- dry_run path (`reEvaluateSynchronous`) is unchanged and still validates input and injects HATEOAS links.
- `Submitter.Submit` correctly enqueues River jobs before creating batch/item tracking rows, with a documented harmless-orphan-jobs failure mode if the transaction fails.
- River job uniqueness (`ByArgs` + `ByPeriod`) via `ProwJobBuildID`+`SymptomHash` looks sound for dedup semantics.
- Migration for `workqueue_batches`/`workqueue_batch_items` not reviewed for schema-level issues beyond existence.

## Open questions
- How was the async path tested end-to-end, given `evaluator.RefreshSymptomCache()` is never invoked on the daemon's `ReEvaluator`? Did any test exercise `ReevaluateWorker.Work` against a live symptom cache, or only in isolation with a pre-populated cache?
- Is the per-index pairing in `buildBatchItems` guaranteed by River's `InsertMany` API contract, or would it be safer to key off something returned per-item (e.g. matching on `Args`) instead of the SQL fetch order?
