---
pr: openshift/sippy#3829
title: "[WIP] TRT-2833: Add integration test tier for PostgreSQL-backed API methods"
head_sha: 73738baf8303a4e7d9e14922d690fcae68123411
base: main
reviewed_at: 2026-07-26T12:22:39Z
verdict: needs-discussion
---

## Scope note

`gh pr diff` reports 1090 changed files / 129530 additions because the head branch
(`worktree-polished-dreaming-lagoon`) is stale relative to `main` and hasn't been rebased —
most of the reported diff is unrelated drift (vendor deps, `.js`→`.jsx` renames from other
merged PRs). The true authored diff, isolated against the actual merge-base
(`1c5aaf7ea5ee2c928ab67d586c150fa25442c044`), is 16 non-vendor files (~760 lines):
`Makefile`, `go.mod`/`go.sum`, doc files (`.apm/instructions/dev-commands.instructions.md`,
`.claude/rules/dev-commands.md`, `.cursor/rules/dev-commands.mdc`, `AGENTS.md`, `CLAUDE.md`,
`apm.lock.yaml`), `.gitignore`, `pkg/db/functions.go`, `pkg/db/query/build_clusters.go`,
`pkg/db/query/job_queries.go`, and three new files under `test/integration/`. This review
covers that actual diff.

## What this PR does

- Adds a `make integration` tier: spins up a real Postgres container via `testcontainers-go`
  (with Podman auto-detection) and runs SQL-level tests against it.
- `test/integration/util/testdb.go` manages container lifecycle and per-test DB cloning via
  `CREATE DATABASE ... TEMPLATE`.
- `test/integration/util/schema.go` builds a minimal integration schema (skips partitioning,
  matviews, triggers, GIN indexes) plus the two custom SQL functions (`job_results`,
  `test_results`).
- `test/integration/jobs_test.go` adds `TestProwJobSimilarName`, `TestVariantReports`,
  `TestJobReports`.
- Fixes two real SQL bugs surfaced by the new tests:
  1. `job_results` returned column `previous_failures`; Go struct expects `previous_fails` —
     always scanned as zero. Renamed consistently.
  2. Period-boundary queries used `BETWEEN` for both windows, double-counting rows at the
     boundary timestamp. Previous-period window changed to half-open `[start, boundary)` in
     `pkg/db/functions.go`, `pkg/db/query/build_clusters.go`, `pkg/db/query/job_queries.go`.

## Findings

### [should-fix] Stale branch inflates PR diff to 1090 files
- where: PR head vs `main` (merge-base `1c5aaf7ea` vs current `main` at `84f2bdfa8`)
- concern: The PR as opened is unreviewable/unmergeable in its current form — GitHub itself
  refuses to render the diff (406, too many files). Needs a rebase onto current `main` before
  this can be reviewed on GitHub or merged.

### [should-fix] Hand-maintained schema list can drift from production AutoMigrate list
- where: `test/integration/util/schema.go:20-64` (`allModels` slice)
- concern: This list duplicates the set of models normally passed to `AutoMigrate` during real
  schema setup (`UpdateSchema`). If a new model is added to production migration logic later but
  not mirrored here, integration tests keep passing against a schema that no longer matches
  prod, silently losing coverage value.
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

### [question] CI wiring for `make integration` not visible in this diff
- where: n/a (no `.ci-operator.yaml` / Prow job config changes in scope)
- concern: `make integration` requires Docker/Podman-in-container capability. Confirm the Prow
  job environment for this repo supports that before removing WIP status; if not, this target
  will only ever run locally.

### [nit] `replaceDBName` hand-rolls DSN string splicing
- where: `test/integration/util/testdb.go` (`replaceDBName`)
- concern: Manual byte-scanning to replace the DB name segment of a DSN works for the
  testcontainers-produced format tested here, but is more fragile than parsing with `net/url`
  and replacing `.Path`. Low risk since the DSN shape is controlled by testcontainers, but a
  simplification opportunity.

### [nit] Podman socket detection duplicates existing tooling patterns
- where: `test/integration/util/testdb.go` (`configurePodmanIfNeeded`, `podmanSocketPath`)
- concern: Repo likely already has Podman-detection logic for e2e/devcontainer tooling in
  `scripts/`. Worth checking for reuse instead of re-implementing socket discovery in Go, though
  not required for correctness.

## Checked

- `previous_fails` rename is consistent across `functions.go`, `build_clusters.go`,
  `job_queries.go`, and matches existing Go struct tags (`pkg/db/models/build_clusters.go`,
  `pkg/apis/api/types.go`). No other code references the old column names for these two
  query paths.
- `PostgresFunctions` are recreated via hash-based `syncSchema` (`pkg/db/db.go:405`) — the
  function-body fix self-applies at next server startup, no migration needed.
- Half-open interval fix (`>= $1 AND < $2` for previous period) correctly stops double-counting
  boundary rows without changing current-period (still `BETWEEN`) semantics.
- `TestVariantReports` and `TestJobReports` place runs exactly at `start`, `boundary`, and `end`
  timestamps, directly exercising the boundary-double-count bug being fixed.
- `TestJobReports` explicitly asserts `report.PreviousFails`, exercising the exact
  failures/fails column-name bug fixed here.
- `sanitize()` in `testdb.go` restricts generated DB names to `[a-z0-9_]` before they're
  interpolated via `fmt.Sprintf` into `CREATE DATABASE` — safe given only `t.Name()` feeds it,
  but the interpolation is only safe because of this step.
- `gofmt -l` clean on all changed Go files.
- go.mod/go.sum additions are scoped to `testcontainers-go` + transitive deps (Docker/Moby
  client libs, `gopsutil`, etc.) plus incidental indirect bumps (`logrus`, `mergo`,
  `klauspost/compress`, `golang.org/x/*`); nothing suspicious.
- Docs updated in the same PR per project convention (`.apm/instructions/`, mirrored
  `.claude/rules/`, `.cursor/rules/`, regenerated `AGENTS.md`/`CLAUDE.md`).

## Open questions

- Can you rebase this onto current `main` so the PR diff reflects only the intended ~760-line
  change?
- Does the Prow CI config for this repo support Docker/Podman-in-container so `make integration`
  can actually run there, or is this local-only for now?
- Is there a plan to keep `test/integration/util/schema.go`'s model list in sync with production
  `AutoMigrate` calls (e.g. a comment pointing at the source of truth, or a test that diffs the
  two lists)?
