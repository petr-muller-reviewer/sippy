---
pr: openshift/sippy#3829
title: "TRT-2833: Add integration test tier for PostgreSQL-backed API methods"
head_sha: 7b09025dcfa9f28e9caa26b1336eb32d5519a4b8
base: main
reviewed_at: 2026-07-27T11:12:47Z
verdict: needs-discussion
---

## Scope note

`gh pr diff` reports 1090 changed files / 130055 additions because the head branch
(`worktree-polished-dreaming-lagoon`) is based on a stale point of `main` and hasn't been
rebased — most of the reported diff is unrelated drift (vendor dependency churn,
`.js`→`.jsx` renames from other merged PRs). The actual authored diff, isolated against the
true merge-base (`1c5aaf7ea5ee2c928ab67d586c150fa25442c044`, unchanged since the prior
review), is 16 non-vendor files, ~1290 insertions / 57 deletions: `Makefile`,
`go.mod`/`go.sum`, doc files (`.apm/instructions/dev-commands.instructions.md`,
`.claude/rules/dev-commands.md`, `.cursor/rules/dev-commands.mdc`, `AGENTS.md`, `CLAUDE.md`,
`apm.lock.yaml`), `.gitignore`, `pkg/db/functions.go`, `pkg/db/query/build_clusters.go`,
`pkg/db/query/job_queries.go`, and three files under `test/integration/`. This review covers
that actual diff. GitHub's own diff view 406s on the full PR (too many files) — rebase
recommended before merge regardless of code correctness.

## What this PR does

- Adds `make integration`: spins up a real Postgres container via `testcontainers-go` (with
  Podman auto-detection), runs SQL-level tests against it — a tier between unit tests (no DB)
  and full e2e.
- `test/integration/util/testdb.go` / `schema.go` manage container lifecycle, per-test DB
  cloning (`CREATE DATABASE ... TEMPLATE`), and a hand-built integration schema (AutoMigrate
  models + the two custom SQL functions, skipping partitioning/matviews/triggers/GIN indexes).
- `test/integration/jobs_test.go` has 21 tests: `TestProwJobSimilarName` (+LIKE-wildcard,
  case-sensitivity cases), `TestVariantReports` (+5 variants: multi-job, zero-previous-runs,
  empty variants, boundary timestamp, multi-release), `TestJobReports` (+5 variants: bugs, PRs,
  zero-previous-runs, filters/sort/limit, multi-release).
- Fixes four real SQL bugs surfaced by these tests, all in `pkg/db/functions.go`,
  `build_clusters.go`, `job_queries.go`:
  1. `job_results` returned `previous_failures`; Go struct expects `previous_fails` — always
     scanned as zero. Renamed consistently across all three files.
  2. Period-boundary queries used `BETWEEN` for both windows, double-counting rows at the
     boundary. Previous-period window is now half-open `[start, boundary)`; current period
     stays inclusive `BETWEEN`.
  3. `open_bugs` used `COUNT(DISTINCT bug_jobs.bug_id)`, which counted all associated bugs
     regardless of status (the status filter lived on the `bugs` LEFT JOIN, but
     `bug_jobs.bug_id` is populated independent of that join succeeding). Changed to
     `COUNT(DISTINCT bugs.id)`, correctly NULLed by the status filter.
  4. That `LEFT JOIN bug_jobs`/`bugs` lived inside the `results` CTE alongside the
     `prow_job_runs` aggregation, fanning out rows per associated bug/PR and inflating
     `current_runs`/`current_passes`/etc. Fixed by extracting bug counting into an isolated
     `job_bugs` CTE, joined once at the outer `SELECT`.

Since the prior review (head `73738baf8`): `replaceDBName` was rewritten to use `net/url`
instead of manual byte-scanning (resolves a prior nit); a `.PHONY: integration` line was
added; bugs #3 and #4 above and the associated `TestJobReports_WithBugs` /
`TestJobReports_WithPullRequests` regression tests were added; test coverage roughly
quadrupled (3 → 21 tests) via a `runSpec`/`createRuns` helper refactor.

## Findings

### [should-fix] Stale branch inflates PR diff to 1090 files
- where: PR head vs `main` (merge-base `1c5aaf7ea` vs current `main`)
- concern: The PR as opened is unreviewable/unmergeable in its current form — GitHub itself
  refuses to render the diff (406, too many files). Needs a rebase onto current `main`.

### [should-fix] Hand-maintained schema list can drift from production AutoMigrate list
- where: `test/integration/util/schema.go:20-64` (`allModels` slice)
- concern: This list duplicates the set of models normally passed to `AutoMigrate` during real
  schema setup. If a new model is added to production migration logic later but not mirrored
  here, integration tests keep passing against a schema that no longer matches prod, silently
  losing coverage value. Unchanged since the prior review.
- excerpt: |
    allModels := []any{
        // Models normally managed by AutoMigrate in UpdateSchema
        &models.ReleaseDefinition{},
        ...
        // Models normally managed by migrations (partitioned tables).
        // Created here as regular tables for integration testing.
        &models.ProwJobRunTest{},
        ...
    }

### [nit] Integration test DB setup has no context/timeout (CodeRabbit-flagged, unaddressed)
- where: `test/integration/util/testdb.go` (`createTemplateDB`, `NewTestDB`)
- concern: Neither function accepts or threads a `context.Context` into its SQL calls
  (`adminDB.Exec`, `db.New`), despite `StartPostgresContainer` already receiving a `ctx`. A
  hung/unresponsive container can make `make integration` block indefinitely with no way to
  interrupt it. CodeRabbit raised this as a nitpick on 2026-07-26; it was not addressed in the
  latest push. Low severity for local/CI dev tooling, but worth a conscious decision.

### [question] CI wiring for `make integration` not visible in this diff
- where: n/a (no CI job config changes in scope)
- concern: `make integration` requires Docker/Podman-in-container capability. Confirm the Prow
  job environment for this repo supports that; otherwise this target is local-only. PR
  description's test-plan checkbox `[ ] CI green` is unchecked, though PR comments show Prow
  reported "all tests passed" as of 2026-07-27T02:55:00Z — worth reconciling.

### [nit] Podman socket detection duplicates existing tooling patterns
- where: `test/integration/util/testdb.go` (`configurePodmanIfNeeded`, `podmanSocketPath`)
- concern: Repo likely already has Podman-detection logic for e2e/devcontainer tooling in
  `scripts/`. Worth checking for reuse instead of re-implementing socket discovery in Go,
  though not required for correctness.

## Checked

- All four SQL fixes verified against Go struct tags: `previous_fails` matches
  `pkg/db/models/build_clusters.go:16`, `pkg/apis/api/types.go:209,239`. No other code
  references the old `previous_failures` column name for these query paths.
- `PostgresFunctions` are recreated via hash-based `syncSchema` (`pkg/db/db.go:405`) — function
  body fixes self-apply at next server startup, no migration needed.
- Fix #4 (row fan-out) is the most consequential: it would have silently skewed pass/fail
  percentages, average duration, and run counts for any job with multiple open bugs or
  multiple merged retested PRs. `TestJobReports_WithBugs` (5 bugs, 2 active) and
  `TestJobReports_WithPullRequests` (2 PRs) explicitly assert `CurrentRuns`/`CurrentPasses`
  are not inflated — correct regression coverage for this exact bug class.
- Fix #3 verified: `COUNT(DISTINCT bugs.id)` is correctly NULLed by the status-filtered
  `bugs` LEFT JOIN, unlike the prior `COUNT(DISTINCT bug_jobs.bug_id)`.
- Boundary-timestamp fix (#2) exercised directly: `TestVariantReports_BoundaryTimestamp` and
  boundary points already present in `TestJobReports`/`TestVariantReports` place runs exactly
  at `start`/`boundary`/`end`.
- `replaceDBName` now uses `net/url` (`u.Path = "/" + newDB`) instead of the previously
  hand-rolled byte-scanning — resolves prior nit.
- `sanitize()` in `testdb.go` restricts generated DB names to `[a-z0-9_]` before
  interpolation via `fmt.Sprintf` into `CREATE DATABASE` — safe given only `t.Name()` feeds it.
- `gofmt -l` clean on all changed Go files.
- go.mod/go.sum additions remain scoped to `testcontainers-go` + transitive deps (Docker/Moby
  client libs, `gopsutil`, etc.) plus incidental indirect bumps; author and CodeRabbit already
  confirmed the `golang.org/x/crypto` version predates this PR (tracked separately as
  TRT-2844) — correct scope discipline.
- `.PHONY: integration` added to the `Makefile` target — correct, avoids surprises if a file
  named `integration` exists.
- New `runSpec`/`createRuns`/`createSingleRun` test helpers correctly deduplicate what was
  previously copy-pasted per-test run-insertion logic.
- Docs updated in the same PR per project convention.

## Open questions

- Can you rebase this onto current `main` so the PR diff reflects only the intended ~1290-line
  change?
- Does the Prow CI config for this repo support Docker/Podman-in-container so `make
  integration` can actually run there, or is this local-only for now? Can the `[ ] CI green`
  test-plan item be checked off given Prow reported all tests passing?
- Any plan to keep `test/integration/util/schema.go`'s model list in sync with production
  `AutoMigrate` calls (comment pointing at source of truth, or a test diffing the two lists)?
- Was the CodeRabbit context-propagation nitpick on `testdb.go` intentionally deferred, or
  worth a quick follow-up given `StartPostgresContainer` already threads a `ctx`?
