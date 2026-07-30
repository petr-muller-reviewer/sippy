---
pr: openshift/sippy#3854
title: "TRT-2865: Add CI job to run the integration tests"
head_sha: c9757f0718cc2a192f72db13b978a61b5b904300
base: main
reviewed_at: 2026-07-30T23:15:50Z
verdict: request-changes
---

## Summary

Adds an external-Postgres mode for `test/integration` (`SIPPY_INTEGRATION_DSN`) so the suite can run in OpenShift CI pods without a container runtime, plus `scripts/integration.sh` (modeled on `scripts/e2e.sh`) and a `make ci-integration` target. Falls back to the existing testcontainers-go path when the env var is unset. Also fixes template-DB cleanup to unmark `IS_TEMPLATE` before dropping, so repeated runs against the same Postgres instance don't fail.

## Findings

### [should-fix] Missing `.apm/instructions` update for new env var / Makefile target
- where: `.apm/instructions/dev-commands.instructions.md` (not touched by this PR)
- concern: The PR adds `SIPPY_INTEGRATION_DSN` and `make ci-integration`, but the repo's own convention (mirrored into `CLAUDE.md`/`AGENTS.md`) is that new env vars, CLI flags, and Makefile targets get documented in `.apm/instructions/`, with `make apm` re-run afterward. Currently only `make integration` is documented there.
- excerpt: |
    Run integration tests: `make integration`

### [question] Template DB name collision if external Postgres is shared across concurrent runs
- where: `test/integration/util/testdb.go:129-140,151-192`
- concern: `createTemplateDB` uses a fixed name (`template_integration`) and drops/recreates it on every `TestMain` run. If the CI Postgres sidecar is ever shared across concurrent PR job runs (rather than one-per-pod, as implied by the PR description), two runs could race on dropping/creating/marking the same template DB.
- excerpt: |
    _, _ = adminDB.Exec(fmt.Sprintf("ALTER DATABASE %s IS_TEMPLATE = false", templateDB))
    if _, err := adminDB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", templateDB)); err != nil {

### [nit] Dead code path after `set -e` triggers
- where: `scripts/integration.sh:85-87`
- concern: `EXIT_CODE=$?` after the final `make integration` is unreachable when that command fails, since `set -e` exits the script immediately (the `EXIT` trap still captures the correct `$?` from the failing command). Not a bug, just slightly misleading — the trap's `ARG=$?` is what actually matters.
- excerpt: |
    make integration
    EXIT_CODE=$?

## Checked
- `PostgresContainer.Terminate` nil-checks `container` correctly for the external-Postgres path; no double-free / nil-deref risk.
- `ALTER DATABASE ... IS_TEMPLATE = false` before `DROP DATABASE` correctly models real Postgres semantics (a template DB can't be dropped while so marked); errors from the ALTER are intentionally swallowed since the DB may not exist yet.
- `scripts/integration.sh`'s caller-provided-DSN fast path bypasses devcontainer/host detection entirely and calls `make integration` directly — correct for the CI sidecar case.
- `gofmt -l` on both changed Go files reports no issues.
- Unquoted shell variables (`$DOCKER`, `$PSQL_CONTAINER`, `$PSQL_PORT`) match the existing style in `scripts/e2e.sh` — consistent with convention, not a regression.
- `podman stop -i` / `podman rm -i` is a legitimate podman flag (`--ignore`), same usage as in `scripts/e2e.sh`.
- Test coverage: PR description states all 133 integration tests pass against both testcontainers and external-Postgres paths, plus full unit suite — consistent with the "don't mock storage clients" project convention (external-Postgres path can't reasonably be unit tested without a real DB).

## Open questions
- Is the CI Postgres sidecar guaranteed to be one-per-job-pod, or could it ever be a shared/long-lived instance across concurrent PR checks? That changes whether the fixed `template_integration` name is safe.
- Should `scripts/e2e.sh` adopt the same DSN password-masking (`${DSN%%@*}@***`) added here in `scripts/integration.sh`, for consistency in CI logs?
