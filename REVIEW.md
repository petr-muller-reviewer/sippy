---
pr: openshift/sippy#3518
title: "TRT-1989: schema migration for non gorm based tables"
head_sha: af8b425f6fff20781b22a54753206e1f4d7ccaa8
base: main
reviewed_at: 2026-05-13T22:20:44Z
verdict: needs-discussion
---

## Findings

### [should-fix] Two migrate instances created in baseline path
- where: `pkg/db/migrate/migrate.go:101-118`
- concern: When baseline stamping is needed, two separate `migrate.Migrate` instances are created — one for `Force()` and another for `Up()`. Each opens its own postgres driver. A single instance with `Force()` then `Up()` would avoid the redundant allocation and driver open.
- excerpt: |
    m, cleanup, err := newMigrate(gormDB)
    ...
    if err := m.Force(baselineVersion); err != nil {
    ...
    m, cleanup, err := newMigrate(gormDB)
    ...
    if err := m.Up(); err != nil && err != migrate.ErrNoChange {

### [should-fix] Redis DB isolation removed in e2e
- where: `scripts/e2e.sh:73`
- concern: Previously e2e used `redis://sippy-redis:6379/1` (logical DB 1) to isolate from dev's DB 0. Now uses default DB 0 with no isolation. Concurrent dev `sippy serve` and e2e runs would share the Redis keyspace, risking flaky tests. The explanatory comment was also removed.
- excerpt: |
    -    export REDIS_URL="${SIPPY_E2E_REDIS_URL:-redis://sippy-redis:6379/1}"
    +    export REDIS_URL="${REDIS_URL:-redis://sippy-redis:6379}"

### [should-fix] MCP silently ignores missing BigQuery creds for bigquery provider
- where: `mcp/server.py:343-345`
- concern: Previously, calling `sippy_serve` with `data_provider=bigquery` without credentials returned a clear early error. Now the error is discarded (`creds_path, _ = ...`), and the server starts without credentials, crashing later with an opaque error.
- excerpt: |
    -    creds_path, creds_err = _resolve_bigquery_creds(bigquery_credentials_file)
    +    creds_path, _ = _resolve_bigquery_creds(bigquery_credentials_file)
         if creds_path:
             args.extend(["--google-service-account-credential-file", str(creds_path)])
    -    elif data_provider == "bigquery":
    -        return f"BigQuery credentials required for data_provider=bigquery: {creds_err}"

### [nit] errors.Is vs == for sentinel errors
- where: `cmd/sippy/migrate.go:48`
- concern: Uses `err == gomigrate.ErrNilVersion` instead of `errors.Is(err, gomigrate.ErrNilVersion)`. While `golang-migrate` currently returns bare sentinels, `errors.Is` is idiomatic and future-proof.
- excerpt: |
    if err == gomigrate.ErrNilVersion {

### [nit] Same pattern in migrate.go
- where: `pkg/db/migrate/migrate.go:118,123,157,179,184`
- concern: Multiple uses of `err != migrate.ErrNoChange` and `err != migrate.ErrNilVersion` via `==`/`!=` instead of `errors.Is`.

### [question] Lint script behavior change for local developers
- where: `hack/go-lint.sh:6-9`
- concern: Old script ran golangci-lint directly if the binary was found (`command -v`). New script only runs directly when `CI=true`, otherwise always uses a container. Developers with golangci-lint installed locally will now be forced through podman/docker. Intentional?
- excerpt: |
    -if command -v golangci-lint &>/dev/null; then
    +if [ "$CI" = "true" ];
    +then

### [question] Scope of the PR
- where: (whole PR)
- concern: The PR bundles the migration infrastructure with several unrelated changes: MCP server rewrite (async to sync, removed tools), frontend symptom aggregation removal, variant registry tier changes, lint script changes, e2e test cleanups. These make independent review harder and increase merge risk. Was this intentional, or could these be split?

## Checked
- Migration SQL is idempotent (`CREATE TABLE IF NOT EXISTS`, `CREATE INDEX IF NOT EXISTS`) — correct for baseline case
- Baseline detection logic: checks `schema_migrations` absence + `test_analysis_by_job_by_dates` presence before stamping — sound
- Connection preservation: cleanup function only closes `iofs` source, not the underlying `*sql.DB` — correct
- E2e migration tests use isolated tracking table (`e2e_schema_migrations`) — no interference with production migrations
- `down` migration drops entire partitioned table including partitions — expected, consistent with `up`
- Embedded SQL via `//go:embed *.sql` in `migrations.go` — standard pattern
- CLI subcommands (`version`, `force`, `down`) wire flags correctly via `f.BindFlags`
- `go 1.25` to `go 1.25.0` — no functional change, just patch version pinning
- Vendor diff is solely from the `golang-migrate` dependency and its transitive deps

## Open questions
- Is there a reason the baseline stamping and the subsequent `Up()` use two separate `migrate.Migrate` instances instead of one?
- Was the Redis DB isolation removal in e2e intentional? What prevents keyspace collisions with concurrent dev servers?
- Should the MCP `sippy_serve` tool still validate BigQuery credentials when `data_provider=bigquery` is explicitly requested?
- Are the bundled changes (MCP rewrite, frontend, variant registry) intended for this PR, or should they be split out?
