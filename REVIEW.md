---
pr: openshift/sippy#3836
title: "Make /api/jobs/runs/reevaluate async with task ID + polling"
head_sha: 9c3992c266326ea1edccc05fbca61102488abf21
base: main
reviewed_at: 2026-07-29T00:11:18Z
verdict: approve
---

## What this PR does
- Converts `POST /api/jobs/runs/reevaluate` to async-by-default: creates an in-memory task, launches a background goroutine, returns 202 with task ID + HATEOAS polling link.
- Adds `GET /api/jobs/runs/reevaluate/{task_id}` for polling status/progress/results.
- Preserves old synchronous behavior behind `?sync=true` (50-run cap); async cap raised to 500.
- New `TaskStore` (`pkg/api/jobrunscan/reevaluate.go`): RWMutex map, TTL-based cleanup (default 1h), semaphore-based concurrency cap (`MaxConcurrentTasks = 5`), panic recovery in the background goroutine, WaitGroup-tracked shutdown via `Stop()`.
- Rewrites `sippy-ng/src/jobs/ReEvaluateSymptoms.jsx` to submit the whole batch once and poll every 2s instead of client-side-pooled per-ID requests.
- Docs (`pkg/api/README.md`, `docs/features/job-analysis-symptoms.md`) updated in the same PR.

## Findings

### [should-fix] No handler-level tests
- where: `pkg/sippyserver/job_run_scan.go` (jsonReEvaluateJobRunSymptoms, jsonGetReEvaluationTask)
- concern: `TaskStore` and `ValidateReEvalRequest` are well unit-tested, but the actual HTTP surface (202/400/429/503 on POST, 200/404 on GET, HATEOAS link injection, sync-vs-async branching) has no test coverage. This is the contract clients depend on.

### [nit] Dead ref in frontend
- where: `sippy-ng/src/jobs/ReEvaluateSymptoms.jsx:106,222`
- concern: `taskIDRef.current = taskID` is set in `startPolling` but never read anywhere else. Either intended for a guard that got dropped, or leftover and removable.
- excerpt: |
    const taskIDRef = useRef(null)
    ...
    taskIDRef.current = taskID
    pollingActiveRef.current = true

### [nit] Frontend poll timeout shorter than backend task timeout
- where: `sippy-ng/src/jobs/ReEvaluateSymptoms.jsx` (`MAX_POLL_COUNT = 300`, `POLL_INTERVAL_MS = 2000`) vs `pkg/sippyserver/job_run_scan.go` (`context.WithTimeout(context.Background(), 30*time.Minute)`)
- concern: frontend gives up polling after ~10 minutes and shows "timed out", but the server keeps processing for up to 30 minutes. For a large async batch (up to the new 500-run cap) that legitimately runs past 10 minutes, the UI shows failure while the task actually completes server-side unobserved. Not a correctness bug, but a UX/tuning mismatch worth aligning.

### [question] Retry can create orphaned duplicate tasks
- where: `sippy-ng/src/jobs/ReEvaluateSymptoms.jsx` handleReEvaluate retry loop (retries on `TypeError` only)
- concern: if the POST succeeds server-side (202, task created) but the client fails to read/parse the response body (connection drop after headers), that surfaces as a `TypeError` too, and the retry resubmits the same batch, creating a second orphaned task. Self-expires via TTL so low impact — is this an accepted tradeoff?

### [question] Global concurrency cap vs. new 500-run async ceiling
- where: `pkg/api/jobrunscan/reevaluate.go` (`MaxConcurrentTasks = 5`)
- concern: cap is process-wide across all users, tasks process one buildID at a time sequentially per goroutine. With the async cap raised to 500 runs/request, is 5 concurrent tasks system-wide sized correctly for expected concurrent usage, or could legitimate concurrent submissions from multiple users routinely hit 429?

## Checked
- `go build -mod=vendor ./pkg/...`, `go vet ./pkg/api/jobrunscan/... ./pkg/sippyserver/...` — clean.
- `go test ./pkg/api/jobrunscan/...` and `-race` on `TestTaskStoreConcurrentAccess` — pass.
- `TaskStore.Get()` returns a deep copy (including `CompletedAt` pointer and `Links` map) — no shared-memory races with callers; verified via `TestTaskStoreGetReturnsCopy` / `TestTaskStoreGetDeepCopiesCompletedAt`.
- `Create()` returns an immutable `TaskCreated` snapshot, not a pointer into the map — handler builds the 202 response from this before launching the goroutine, avoiding a data race.
- Panic recovery, `Stop()` idempotency (`sync.Once`), and stuck-task cleanup by `CreatedAt` (not just `CompletedAt`) are present and tested — addresses goroutine-leak/panic risk for a long-running background task pattern.
- `google/uuid` already present in `go.mod`/vendor prior to this PR (was indirect, now used directly) — no new dependency risk.
- Capability gating: POST keeps `WriteEndpointsCapability`, GET is read-only `LocalDBCapability` — consistent with existing conventions.
- Docs updated in the same PR per project convention.

## Open questions
- Is the 10-minute frontend poll ceiling intentional, or should it be raised (or made configurable) to match the 30-minute backend task timeout for large batches?
- Is `MaxConcurrentTasks = 5` based on a specific resource budget (memory/BigQuery/GCS rate limits), or a placeholder that should be revisited under real usage?
- Any plan to add handler-level tests (`httptest`) for the new POST/GET endpoints before or shortly after merge?
