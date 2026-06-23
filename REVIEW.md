---
pr: openshift/sippy#3643
title: "TRT-2695: retroactively re-evaluate symptoms (API)"
head_sha: 075eebdf4c73b21219fb72323d44d8b8a5d7370f
base: main
reviewed_at: 2026-06-23T20:22:55Z
verdict: needs-discussion
refresh_log:
  - old_sha: f4d91701b05f4c5ab531032946535e2aa861f836
    new_sha: 075eebdf4c73b21219fb72323d44d8b8a5d7370f
    summary: "5 commits: fixed BQ date filter inconsistency, lifted Manager to struct field (reuses server-wide instance), variadic set init, structured logging, sets.New[string]() migration. Author confirmed frontend-worker-pool approach for batching."
  - old_sha: 075eebdf4c73b21219fb72323d44d8b8a5d7370f
    new_sha: 075eebdf4c73b21219fb72323d44d8b8a5d7370f
    summary: "No code changes. Discussion between mstaeble and sosiouxme on orchestration: mstaeble prefers backend orchestration over frontend worker pool, sosiouxme argues idempotency + frontend retry is simpler. Both agree the core logic is what matters; orchestration mechanism can iterate."
---

## Summary

Adds `POST /api/jobs/runs/reevaluate` endpoint. Re-runs symptom definitions against completed job run artifacts and updates BQ, GCS, and PostgreSQL. Delete-then-insert strategy for idempotency, preserves manually-applied labels. Also includes APM/tooling updates (em-dash removal, testing guidelines, sets convention, agentic commands).

Since previous review (f4d91701b..075eebdf4, 5 commits):
- Fixed `>=` to `=` in `queryNonSymptomLabels` date filter, matching `clearBQLabels`.
- `jobartifacts.Manager` now a field on `ReEvaluator`, injected via `NewReEvaluator()`, reuses server's `s.jobartifactsManager`.
- `mergeLabels` uses `sets.New[string](manualLabels...)` directly.
- All `sets.NewString()` calls migrated to `sets.New[string]()`, `.List()` to `.UnsortedList()`. Tests updated with order-independent `sameStrings` helper.
- Structured logging throughout (`log.WithField`/`log.WithFields` instead of `Warnf`/`Debugf`).

## Findings

### [should-fix] Synchronous API with no cancellation safety
- where: `pkg/api/jobrunscan/reevaluate.go:123-127`, `pkg/sippyserver/job_run_scan.go:174-205`
- concern: The endpoint processes all job runs synchronously. The request `ctx` is passed to BQ and GCS calls in `clearAndWrite` that respect cancellation, but the artifact scanning in `getJobRunFiles` (`query.go:134`) uses `context.Background()`. If a proxy kills the connection, a race develops: the delete in `clearBQLabels` may complete (ctx still alive), then a subsequent BQ insert or GCS write fails with `context.Canceled`, leaving the job run with cleared-but-not-rewritten data. The author's comment (2026-06-23) describes the intended architecture: the frontend will run a pool of workers making single-run requests, reporting progress as they return. This reduces the timeout risk (single-run takes seconds) but the cancellation hazard remains even for a single run: if ctx cancels between the BQ delete and insert, the label state is inconsistent. Consider either (a) using a detached context for the write path so client disconnect doesn't corrupt state, or (b) an async pattern. Discussion update (2026-06-23): mstaeble raised concerns about concurrent requests and frontend cancellation, arguing backend orchestration is simpler for consistency. sosiouxme responded that operations are "idempotent-ish" (BQ streaming buffer may cause harmless duplicates), and on cancellation: backend halts at context checkpoint, frontend can retry. Both agree true consistency across 3 storage backends is impractical, and the orchestration mechanism can iterate post-merge.
- excerpt: |
    for _, buildID := range prowJobBuildIDs {
        result := r.reEvaluateOne(ctx, buildID, symptoms)
        results = append(results, result)
    }

### [nit] Context not propagated to GORM DB queries
- where: `pkg/api/jobrunscan/reevaluate.go:137,373`
- concern: `loadActiveSymptoms` and `loadLabelDefinitions` use `r.db.DB.Find(...)` without `.WithContext(ctx)`. Request cancellation won't propagate to these DB queries.
- excerpt: |
    res := r.db.DB.Order("id").Find(&all)

### [nit] DisallowUnknownFields is stricter than peer handlers
- where: `pkg/sippyserver/job_run_scan.go:186`
- concern: No other handler in this file uses `DisallowUnknownFields`. A client sending a future field gets 400 here but not from other endpoints. Deliberate hardening is fine but the inconsistency is worth noting. `MaxBytesReader` at line 184 is a nice touch absent elsewhere.
- excerpt: |
    req.Body = http.MaxBytesReader(w, req.Body, 1<<20)
    dec := json.NewDecoder(req.Body)
    dec.DisallowUnknownFields()

### [nit] nil-db guard in production code is a test accommodation
- where: `pkg/api/jobrunscan/reevaluate.go:368`
- concern: `loadLabelDefinitions` checks `r.db == nil` solely because `TestBuildOutputs` constructs `ReEvaluator{gcsBucket: "test-bucket"}` with nil db.
- excerpt: |
    if ids.Len() == 0 || r.db == nil {
        return nil, nil
    }

### [question] Streaming buffer skip can produce duplicate BQ rows
- where: `pkg/api/jobrunscan/reevaluate.go:449-453`
- concern: `clearBQLabels` swallows streaming buffer errors and continues to insert new labels, producing duplicates. Author confirmed (2026-06-22) this is acceptable: labels are collapsed on display, and failure to delete only means stale labels from changed symptoms may persist. Should the API README mention this limitation?
- excerpt: |
    if strings.Contains(err.Error(), "streaming buffer") {
        log.WithError(err).WithField("buildID", buildID).Warn("symptom reEval: BQ delete hit streaming buffer, skipping")
        return nil
    }

## Resolved

### [was: should-fix] BQ date filter inconsistency between delete and query
- resolved_in: 075eebdf4
- detail: `queryNonSymptomLabels` now uses `= @startDate`, matching `clearBQLabels`.

### [was: should-fix] New Manager worker pool created per job run
- resolved_in: 3d811abca
- detail: `Manager` is now a field on `ReEvaluator`, injected via constructor. Handler passes `s.jobartifactsManager` (server-wide instance).

### [was: nit] mergeLabels unnecessary loop over manualLabels
- resolved_in: 102e28e3a
- detail: Now uses `sets.New[string](manualLabels...)` directly.

## Checked

- `BulkInsertJobRunLabels` extraction is a clean refactor; existing annotator becomes thin wrapper.
- HATEOAS links follow project conventions (per-result symptom links, self link).
- `sets.New[string]()` used consistently throughout (migrated from `sets.NewString()`).
- GCS cleanup under `artifacts/job_labels/` is safe: only cloud function and re-evaluator write there; annotator writes BQ only.
- `WriteHTMLSummaryToBucket` writes `label-summary.html` inside the `artifacts/job_labels/` prefix, correctly cleaned up by `clearGCSLabels`.
- Matcher construction with `(0, 0, 1)` args correct for single-match detection.
- Nil `ContentMatcher` for `MatcherTypeFile` correctly treated as file-existence match.
- CEL matcher type excluded with logged warning.
- Symptoms without labels filtered out in `filterRelevantSymptoms`.
- Unit tests cover validation, filtering, merging, URL parsing, matcher construction, output building.
- Functional tests gated behind env vars following `releasesync_functional_test.go` pattern.
- Route registration with `LocalDBCapability` + `WriteEndpointsCapability` matches security model.
- PostgreSQL label update uses diff-based merge to avoid streaming buffer dependency.
- `release_job_runs` update correctly conditional on existence.
- Regression data left to regression cache loader (no direct update needed).
- APM/tooling changes are mechanical.
- Structured logging migration is correct and consistent.
- `sets.New[string]()` + `.UnsortedList()` migration handled correctly; test comparisons updated to `sameStrings` with sort.

## Open questions

- The cancellation hazard remains even for single-run requests: `clearBQLabels` uses request `ctx` but `getJobRunFiles` uses `context.Background()`. If ctx cancels between the BQ delete and subsequent writes, label state is inconsistent. The author's frontend-worker-pool design mitigates the timeout risk but doesn't eliminate this race. mstaeble agrees backend orchestration would be better but both reviewers accept iterating post-merge. A minimal fix: use a detached context for the write path so client disconnect doesn't corrupt state.
- Should the streaming buffer skip be surfaced in the per-run result rather than silently logged?
