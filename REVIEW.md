---
pr: openshift/sippy#3728
title: "TRT-2768: Import /payload job results from PRs into postgres"
head_sha: ee1291b1526abda5275063f597dcb4e98ae739f9
base: main
reviewed_at: 2026-07-20T15:27:45Z
verdict: approve
refresh_log:
  - from: d5763a01aef34537cb3c36a371780739aee6092d
    to: ee1291b1526abda5275063f597dcb4e98ae739f9
    summary: >
      Author (dgoodwin) and human reviewer (mstaeble) traded feedback; CodeRabbit
      also reviewed. Addressed: introduced models.ReleasePresubmits constant used
      everywhere (resolves prior magic-string finding), added soft-delete guards
      and partition/index filters to the PG query, added a result limit (with a
      follow-up crash fix), fixed include_successes to exclude flakes (matches old
      BQ behavior), replaced sha param with latest_sha_only bool, added indexes on
      ProwPullRequest(org,repo,number) and prow_job_run_prow_pull_requests(pr_id),
      used TestStatus constants instead of magic numbers in seed data, and seeded
      competing SHAs for PR 99001 to actually exercise the latest_sha_only filter.
      API README still not updated for this endpoint.
---

## Summary

Transforms /payload sub-jobs into presubmit-style records during BQ import, ports `/api/pull_requests/test_results` from BigQuery to PostgreSQL (with `latest_sha_only` filter, default 2-week range, result limit), adds seed data and e2e tests. Since the first review pass, the author addressed feedback from a human reviewer (mstaeble), CodeRabbit, and this review.

## Findings

### [should-fix] API endpoint modified but pkg/api/README.md not updated
- where: `pkg/api/prtestresults.go` (whole file), `pkg/api/README.md` (untouched)
- concern: CLAUDE.md rule: "When API endpoints are added, removed, or modified, update `pkg/api/README.md`." Response schema changed (removed `prowjob_build_id`/`success`/`flaked`/`failure_content`, added `prow_job_run_id`/`status`/`output`), parameters changed (`sha` → `latest_sha_only`, `limit` added), data source switched from BQ to PG, date params now optional with 14-day default. Still not reflected in the README as of `ee1291b15`.

### [nit] No query-time deduplication for cross-suite test duplicates
- where: `pkg/api/prtestresults.go:58-110`
- concern: If the same test name appears in multiple suites within one job run, both rows are returned. The old BQ code had the same limitation (ROW_NUMBER partitioned by testsuite), so this is not a regression. Noting for awareness.

## Resolved (since d5763a01)

### [should-fix] "Presubmits" hardcoded across 34 locations with no shared constant
- resolution: commit `cb70960bb` ("Use a Presubmits release in database and consts") introduced `models.ReleasePresubmits` in `pkg/db/models/releases.go` and replaced hardcoded `"Presubmits"` strings in `pkg/api/prtestresults.go`, `pkg/dataloader/prowloader/prow.go`, `pkg/variantregistry/ocp.go`, `pkg/api/job_runs.go`, and `cmd/sippy/seed_data.go`. Also added a `ReleaseDefinition` model with a `CapPullRequests` capability constant, seeded via `seedReleaseDefinitions`, addressing the human reviewer's separate comment about coordinating with #3679's release-definitions infrastructure.

### [should-fix, from mstaeble] Missing indexes and soft-delete guards on the PG query
- resolution: commits `76cc3bdad` (partition filters + soft-delete guards), `ed84adb65` (composite index on `ProwPullRequest(org, repo, number)`), `376bf6c1e` (index on `prow_job_run_prow_pull_requests(prow_pull_request_id)`). The JOIN on `prow_job_run_prow_pull_requests` now includes `jrpr.prow_job_run_release` and timestamp bounds so the existing composite index can be used; `pp.deleted_at IS NULL AND pjr.deleted_at IS NULL AND pj.deleted_at IS NULL` added to the WHERE clause.

### [should-fix, from mstaeble/CodeRabbit] No result limit or pagination
- resolution: commits `bbc381f18` (add `limit` param, default/cap 10000) and `5662a6bbc` (fix crash: `limit` wasn't in the param regexp allowlist, so `param.SafeRead` rejected it).

### [nit, from mstaeble] include_successes returns flakes, diverging from old BQ behavior
- resolution: commit `dd24908e7` restricts the "successes" branch of `include_successes` to `status = TestStatusSuccess` only (previously any status matched the name pattern, unintentionally including flakes). E2e test `TestPRTestResultsIncludeSuccesses` updated to assert `statuses["flake"] == 0`.

### [question, from mstaeble] Is the sha filter exact-match or does the API need "latest run only" semantics?
- resolution: commit `0312ea604` replaced the `sha` query param with a `latest_sha_only` bool. Seed data (`cmd/sippy/seed_data.go`) now creates a second, older SHA for PR 99001 (`oldSHAPR`) linked only to the earliest run, so the latest-SHA-only assertion actually exercises the filter instead of trivially passing (CodeRabbit had flagged that every seeded run previously shared one SHA).

### [nit] Magic numbers for test status in seed data
- resolution: commit `75765d4a0` replaced raw ints (12, 1, 13) with `v1.TestStatusFailure`/`v1.TestStatusSuccess`/`v1.TestStatusFlake` constants in `seedPresubmitData`. Commit `cfd007f53` also fixed a stale comment mismatched to the wrong TestID (flagged by mstaeble: "Is this comment accurate? ... TestID used is networkTestID").

## Checked

- `includeSuccesses` filter now correctly excludes flakes, matching old BQ semantics; validated by e2e test.
- /payload sub-job name stabilization: `strings.Replace` with PR-number prefix, fallback to `payload-pr-` + releaseJobName. Handles the expected patterns and logs a warning on fallback. Unchanged since last review.
- Variant override in `createOrUpdateProwJob`: replaces first release-matching variant with `models.ReleasePresubmits`, clears TestGridURL for payload presubmits. Logic is correct for single-release-variant case.
- `processProwJob` ordering: /payload early-return before synthetic release check is intentional (payload jobs should always route to Presubmits, not synthetic overrides).
- Endpoint capability change from `ComponentReadinessCapability` to `LocalDBCapability`: correct since endpoint now uses PG.
- New `limit` param wired through `param.SafeRead`'s regexp allowlist (`uintRegexp`) after the crash fix; `PrintPRTestResultsJSON` silently falls back to the default on a parse error rather than returning 400 (CodeRabbit raised this: "Return 400 for invalid or excessive limits" — not addressed, but low severity given the safe default).
- E2e tests cover default failures, include_successes (successes only, no flakes), multi-PR isolation, latest_sha_only filtering with competing SHAs, default date range, missing params, and empty results.

## Open questions

- CodeRabbit's suggestion to return HTTP 400 for malformed/out-of-range `limit` values (instead of silently defaulting) was not addressed — worth a follow-up if strict client-facing validation matters here, otherwise fine to leave as-is.
- The 30-day max date range guard was removed. Is there any concern about very large date ranges against PG, or does partition pruning make that safe enough?
