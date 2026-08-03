---
pr: openshift/sippy#3859
title: "TRT-2836: Add integration tests for payload query functions"
head_sha: a017b4ded2329385a15ecad02eceb80571fb6399
base: main
reviewed_at: 2026-08-03T20:29:36Z
verdict: approve
---

## Summary

Test-only PR (no production code changes). Adds 53 integration tests covering all 11 functions in `pkg/db/query/payload_queries.go`, including 8 new tests for previously-untested `GetTestFailuresForPayloadStream`. Extends `test/integration/util/fixtures.go` with functional-options builders (`ReleaseTagOption`, `ProwJobRunOption`, `ReleasePullRequestOption`). CI green (lint, unit, e2e, build, verify).

Verified test assertions against actual SQL in `pkg/db/query/payload_queries.go`:
- `GetLastAcceptedByArchitectureAndStream`, `GetPayloadStreamPhaseCounts`, `GetPayloadAcceptanceStatistics`: `release_time < reportEnd` (exclusive) — tests correctly assert exclusion at exact boundary.
- `GetLastPayloadStatus`: `release_time <= reportEnd` (inclusive) — test correctly asserts inclusion at boundary.
- `GetLastPayloadTags`: no upper bound on `release_time` at all — test explicitly documents this instead of assuming a bug.
- Gap-math in `GetPayloadAcceptanceStatistics` test (2h/3h/6h → min/mean/max in seconds) checks out.

## Findings

### [should-fix] Unused fixture options added
- where: `test/integration/util/fixtures.go:129-135`
- concern: `WithPreviousReleaseTag` and `WithForced` are added but never used anywhere in this PR (or the rest of the repo). For a PR whose stated purpose is exhaustive coverage of existing query functions, dead helper code should either be dropped or backed by a test that uses it — `WithPreviousReleaseTag` in particular seems like it was meant for `GetPreviousPayload` tests but isn't used there.
- excerpt: |
    func WithPreviousReleaseTag(prev string) ReleaseTagOption {
    	return func(rt *models.ReleaseTag) { rt.PreviousReleaseTag = prev }
    }

    func WithForced(forced bool) ReleaseTagOption {
    	return func(rt *models.ReleaseTag) { rt.Forced = forced }
    }

### [nit] CreateReleaseJobRun doesn't follow the options pattern used elsewhere in the same diff
- where: `test/integration/util/fixtures.go:1408-1420`
- concern: `CreateProwJobRun`, `CreateReleaseTag`, and `CreateReleasePullRequest` all gained a `...Option` variadic parameter in this PR, but `CreateReleaseJobRun` still takes everything positionally. Minor inconsistency, not a functional issue given its current single call-site pattern.

### [question] No pure "nonexistent payload" empty-result test for the two GetTestFailuresFor* functions
- concern: `TestGetTestFailuresForPayload_NoFailures` covers an existing payload with no job runs, but there's no test for a `payloadTag`/release combo that doesn't exist at all. Probably fine (SQL naturally returns empty), but worth confirming it was considered rather than missed.

## Checked
- No mocking of DB clients — tests use real Postgres via testcontainers-go (`intutil.NewTestDB`), per project convention.
- Table-driven test used for `GetLastPayloadStatus` phase-streak cases, matches existing repo pattern.
- Functional-options pattern for fixture builders is idiomatic and non-breaking for existing call sites.
- Boundary conditions (`<` vs `<=` on `reportEnd`) verified line-by-line against `payload_queries.go` — all correct.
- `GetLastPayloadTags` "no upper bound" test comment matches actual query behavior (verified, not assumed).
- CI fully green (lint, unit, e2e, build, verify, security).

## Open questions
- Should `WithPreviousReleaseTag` / `WithForced` be dropped, or is there a follow-up test intended to use them?
