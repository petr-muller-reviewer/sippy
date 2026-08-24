---
pr: openshift/sippy#3913
title: "TRT-2895: Force close regressions"
head_sha: 96bfb7a25322a5c26e241143f3c4a94ee234e142
base: main
reviewed_at: 2026-08-24T15:59:59Z
verdict: request-changes
refresh_log:
  - old_head_sha: a9599901ce96483a03d09e3532c40feebd7e3d84
    new_head_sha: 96bfb7a25322a5c26e241143f3c4a94ee234e142
    at: 2026-08-24T15:59:59Z
    summary: "Single commit (TRT-2895: Use test_failures > 0 in force-close gap query) switches queryRegressionFailureGaps' WHERE clauses from test_failed = true to test_failures > 0; no findings resolved, none newly introduced."
---

## What this PR does

- Adds force-close for test regressions tied to a resolved triage: `ForceCloseRegressions` and a dry-run `ForceClosePreview` in `pkg/api/componentreadiness/regressiontracker.go`, exposed via `jsonForceCloseRegressions`/`jsonForceClosePreview` in `pkg/sippyserver/server.go`.
- Force-closing is a single atomic `UPDATE ... WHERE id IN (subquery on triage_regressions) AND closed IS NULL AND opened < closeTime ... RETURNING id`, stamping `force_closed=true`, `force_closed_by`, `force_closed_reason`, `force_closed_by_triage_id`, and a `closed` timestamp derived from the triage's resolution time.
- `ResolveTriages`'s auto-resolution `NOT EXISTS` check now treats `force_closed` regressions as inactive (`closed IS NULL OR (closed > ? AND force_closed = false)`), so a triage whose regressions are all force-closed can auto-resolve.
- New `ForceCloseResult`/`ForceClosePreview`/`ForceClosePreviewRegression` response types carry HATEOAS `Links` via `InjectForceCloseHATEOASLinks`/`InjectForceClosePreviewHATEOASLinks`.
- Adds integration coverage in `test/integration/regression_forceclose_test.go` and shared fixtures in `test/integration/util/fixtures.go`; removes the now-redundant `000013_add_force_close_to_regressions` SQL migration.

Since previous review: one targeted commit switches `queryRegressionFailureGaps`' failure-detection predicate from `test_failed = true` to `test_failures > 0` in both grouped queries (correctness fix on the underlying column semantics, not a structural change). No prior findings resolved or newly introduced.

## Findings

### [blocking] force-close scoped only by the given triage, ignoring multi-triage regression linkage
- where: `pkg/api/componentreadiness/regressiontracker.go:379-382`
- concern: `TestRegression.Triages` is many-to-many via `triage_regressions` (`pkg/db/models/triage.go:49,224`), but the force-close UPDATE's eligibility subquery is `SELECT test_regression_id FROM triage_regressions WHERE triage_id = ?`, scoped only to the triage ID passed in. If a regression is also linked to a different, still-open triage B, force-closing via triage A sets `force_closed=true` globally on it. Since `ResolveTriages` (line 233) excludes `force_closed` rows from its open-regression check, triage B can then auto-resolve on the next run using triage A's unrelated resolution time, even though nobody resolved triage B and its issue may still be open. Confirmed against the current query shape (this survived the rewrite from select-then-update to a single atomic UPDATE).
- excerpt: |
    Where("test_regressions.id IN (?)",
        prs.dbc.DB.Table("triage_regressions").
            Select("test_regression_id").
            Where("triage_id = ?", triageID)).
    Where("test_regressions.closed IS NULL").
    Where("test_regressions.opened < ?", closeTime).

### [should-fix] ResolveTriages mislabels force-closed triages as auto-resolved rollouts
- where: `pkg/api/componentreadiness/regressiontracker.go:266`
- concern: since the `NOT EXISTS` subquery at line 233 now treats force-closed regressions as inactive, a triage whose only regressions were force-closed can auto-resolve through this path. Line 266 unconditionally sets `ResolutionReason = models.RegressionsRolledOff`, mislabeling a deliberate human force-close as an automatic rollout and discarding the real force-close reason from the resulting triage's resolution record.
- excerpt: |
    triage.ResolutionReason = models.RegressionsRolledOff

### [should-fix] force-close endpoint has stricter auth behavior than sibling triage-write endpoints
- where: `pkg/sippyserver/server.go:488`
- concern: `jsonForceCloseRegressions` hard-401s whenever `getUserForRequest()` returns empty, unlike `jsonCreateTriage`/`jsonUpdateTriage`/`jsonDeleteTriage` in the same file, which proceed and merely log the (possibly empty) user. `getUserForRequest` (server.go:2233) returns `""` whenever `X-Forwarded-User` is unset and `DEV_MODE != "1"`, so a developer who can create/update/delete triages locally gets an unexpected 401 specifically on force-close. This may be intentional (audit requirement) but is an undocumented policy inconsistency across otherwise-parallel endpoints.

### [should-fix] deleting a triage leaves stale force_closed_by_triage_id references
- where: `pkg/db/models/triage.go:247`
- concern: `ForceClosedByTriageID` has no FK/cascade handling, and `jsonDeleteTriage` (server.go:2061) never clears it. Deleting a triage that previously force-closed regressions leaves `test_regressions.force_closed_by_triage_id` pointing at a nonexistent triage row; the `force_close`/`triage` HATEOAS links built from that stale ID 404, and the audit trail can no longer be resolved.

### [nit] duplicated HATEOAS URL-building logic
- where: `pkg/api/componentreadiness/regressiontracker.go:388`
- concern: `forceCloseTriageLinks`/`regressionDetailLink` hand-roll the same URL format strings already defined and used via `injectHATEOASLinks`/`InjectRegressionHATEOASLinks` in `pkg/api/componentreadiness/triage.go` (`triageLink`/`regressionLink`, ~lines 793/797). Two independent copies of the same format strings now exist; a route rename is likely to update only one.

### [nit] duplicated "load triage, check Resolved" validation
- where: `pkg/api/componentreadiness/regressiontracker.go:354`
- concern: `ForceCloseRegressions` and `ForceClosePreview` duplicate an identical block that loads the triage, wraps the error, and checks `Resolved.Valid`. A future change to this validation (e.g. also checking `ResolutionReason`) needs to be applied in two places by hand.

### [nit] redundant paired queries in failure-gap lookup
- where: `pkg/api/componentreadiness/regressiontracker.go:286`
- concern: `queryRegressionFailureGaps` runs two near-identical grouped queries against `regression_job_runs` (`MAX(start_time) ... <= resolutionTime` then `MIN(start_time) ... > resolutionTime`) that could be one conditional-aggregation query. Doubles DB round trips on every preview call. (2026-08-24: both queries' failure predicate was corrected from `test_failed = true` to `test_failures > 0`; the duplication itself is unchanged.)

### [nit] ad hoc, uncapped trailing-JSON body validation
- where: `pkg/sippyserver/server.go:503`
- concern: the decode-twice-to-`io.EOF` trick for rejecting trailing JSON is reinvented ad hoc for this one endpoint, with no `http.MaxBytesReader`-style size cap, and no sibling write endpoint (`jsonCreateTriage`, `jsonUpdateTriage`) performs this check. Leaves body-validation behavior inconsistent across the API, and an unbounded body is fully read into the decoder regardless.

## Checked
- The rewritten single-UPDATE force-close (subquery + `closed IS NULL` + `opened < closeTime`, `RETURNING id`) closes the prior TOCTOU race between select and update — the eligibility check and the write are now one atomic statement.
- New response types (`ForceCloseResult`, `ForceClosePreview`, `ForceClosePreviewRegression`) now populate `Links`, addressing the prior missing-HATEOAS finding.
- Handlers no longer echo raw `err.Error()` to callers and handle `gorm.ErrRecordNotFound` with 404; user identity is no longer logged.
- No SQL injection concerns; all queries are parameterized gorm calls.
- New integration tests in `test/integration/regression_forceclose_test.go` exercise the happy path and several edge cases, but do not appear to cover the multi-triage-sharing scenario in the blocking finding above.

## Open questions
- Is a regression ever expected to be linked to more than one triage in practice, or is `triage_regressions` many-to-many purely a schema affordance that's unused today? If genuinely unused, the blocking finding may be low-probability in practice, but the code doesn't defend against it and the schema explicitly allows it.
- Is the force-close 401-on-missing-user behavior intentional and stricter than other triage-write endpoints by design (e.g. because it's destructive/audited), or an oversight from copying a different code path?
- Should `ResolveTriages` record a different resolution reason (or preserve the force-close reason) when a triage auto-resolves purely because its regressions were force-closed, rather than because they rolled off?
