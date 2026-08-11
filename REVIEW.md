---
pr: openshift/sippy#3864
title: "TRT-2893: Add mcpchecker JUnit suite to Sippy allowlist"
head_sha: 1be1421c3c8d8fc6975a2e5bead58e24d9af53c6
base: main
reviewed_at: 2026-08-11T10:55:23Z
verdict: approve
refresh_log:
  - from: 1be1421c3c8d8fc6975a2e5bead58e24d9af53c6
    to: 1be1421c3c8d8fc6975a2e5bead58e24d9af53c6
    summary: No code changes. PR title/Jira reference changed from OCPMCP-308 to TRT-2893; openshift-ci-robot posted a jira-lifecycle-plugin comment flagging the referenced Jira issue (TRT-2893) has no target version set for the "5.0.0" branch target.
---

## What this PR does
- Adds `"mcpchecker"` to the static `testSuites` allowlist in `pkg/db/suites.go`, consumed by `IsSuiteImportable`.
- Unblocks the prowloader (`pkg/dataloader/prowloader/prow.go:1076,1086`) from skipping JUnit testcases whose suite name is `mcpchecker`.
- Adds two focused unit tests in `pkg/db/suites_test.go` (mcpchecker importable; known suite still importable + unknown suite still rejected).
- Adds a design/spike doc `docs/plans/ocpmcp-308-eval-dashboard-sippy-spike.md` describing problem, approach, test plan, and dependency on OCPMCP-108 (upstream job emitting `junit_mcpchecker.xml`).

## Findings

### [question] runtime verification of actual suite name
- where: `pkg/db/suites.go:63`
- concern: The allowlist entry assumes the mcpchecker JUnit output uses the literal suite name `mcpchecker`. This can't be verified from this PR alone since it depends on OCPMCP-108 (the Prow job in `openshift/release`) actually emitting that suite name. The PR's own plan doc lists this as a follow-up verification step.
- excerpt: |
    "prowjob-junit",
    "mcpchecker",
    "OLM-Catalog-Validation",

## Checked
- No collision with existing entries in `testSuites`; addition is a single new literal string.
- `IsSuiteImportable` is a pure lookup over `testSuiteSet` (a `sets.Set[string]`) — no other code path needs updating for a new allowlisted name.
- `populateTestSuitesInDB` iterates `testSuites` to seed the `suites` table, so the new entry will be created on next migration/startup like existing suites.
- No BigQuery-side counterpart to `IsSuiteImportable`/`testSuites` exists (only consumed by the Postgres-oriented `pkg/dataloader/prowloader/prow.go`), so the project's provider-parity rule doesn't apply here.
- New tests are small and directly exercise the added behavior plus a known/unknown suite regression check.
- Docs addition (`docs/plans/...`) is additive and doesn't require README/config doc updates — no API, config option, or setup step changed.

## Open questions
- Has the actual mcpchecker JUnit output been confirmed to use suite name `mcpchecker` exactly (case/spelling), e.g. via a real job run or local `mcpchecker result convert junit` invocation?
- The Jira bot flagged that TRT-2893 has no target version set for the "5.0.0" branch target — is that a blocker for merge or just a Jira hygiene item to fix separately?
