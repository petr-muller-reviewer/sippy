---
pr: openshift/sippy#3832
title: "TRT-2802: Replace git clone with GitHub API and COPY protocol in feature gate loader"
head_sha: 2b29be4122947322a45689285f593c59eda0d4ad
base: main
reviewed_at: 2026-07-29T10:59:57Z
verdict: approve
refresh_log:
  - from: 7781b8389b699c29c98eb3728238aaeef4eefcb1
    to: 2b29be4122947322a45689285f593c59eda0d4ad
    summary: >-
      Pure rebase onto newer main (old SHA unreachable from new — force-push).
      No diff in pkg/dataloader/featuregateloader/ between old and new SHA;
      go.mod/go.sum and cmd/sippy/load.go churn is unrelated rebase noise from
      other merged PRs (bugloader.New signature, testcontainers deps, etc).
      No new PR comments/reviews since last review; CI re-ran after rebase and
      passed (e2e). Findings unchanged.
---

## Summary

Replaces `go-git` clone of `openshift/api` in the feature gate loader with GitHub Contents API calls (~3m10s -> ~7s), switches DB writes from GORM `CreateInBatches` to pgx COPY-into-temp-table + `INSERT ... ON CONFLICT` (via pre-existing `db.CopyToTempTable` helper), and drops `go-git` + 16 transitive deps (~117k vendored lines). Only 5 non-vendor files change: `cmd/sippy/load.go`, `go.mod`/`go.sum`, `pkg/dataloader/featuregateloader/featuregateloader.go` (+test).

Since previous review: PR was rebased onto current `main` (force-push, old SHA no longer an ancestor). The feature-gate-loader diff itself is byte-identical; only unrelated rebase noise moved (other loaders' `New()` signatures in `load.go`, transitive dep churn in `go.mod`/`go.sum` from unrelated merged PRs). CI re-ran post-rebase and passed. No reviewer comments.

## Findings

### [should-fix] No functional test for the new COPY/upsert SQL path
- where: `pkg/dataloader/featuregateloader/featuregateloader.go:228-277` (`upsertFeatureGates`)
- concern: This is new SQL (raw COPY + `INSERT ... ON CONFLICT`) that has not previously run in production. The project's convention (see `releasesync_functional_test.go`) is to cover DB-touching code with a functional test that skips unless credentials/env vars are supplied, rather than relying solely on manual staging verification. None exists for this path.
- excerpt: |
    cleanup, err := db.CopyToTempTable(l.ctx, conn, "tmp_feature_gates", featureGates, featureGateTempCols)
    ...
    upsertTag, err := conn.Exec(l.ctx, `
        INSERT INTO feature_gates (release, topology, feature_set, feature_gate, status, created_at, updated_at)
        SELECT release, topology, feature_set, feature_gate, status, NOW(), NOW()
        FROM tmp_feature_gates
        ON CONFLICT (release, topology, feature_set, feature_gate) DO UPDATE SET
            status     = EXCLUDED.status,
            updated_at = NOW()
    `)

### [should-fix] GITHUB_TOKEN env var undocumented
- where: `pkg/dataloader/featuregateloader/featuregateloader.go:41-51` (`New`)
- concern: New optional env var `GITHUB_TOKEN` affects auth/rate limits for this loader. Project convention requires documenting new env vars in `config/README.md` or root `README.md` in the same PR; not present in this diff.
- excerpt: |
    githubToken:    os.Getenv("GITHUB_TOKEN"),

### [should-fix] New GitHub client duplicates and bypasses existing rate-limit-aware client
- where: `pkg/dataloader/featuregateloader/featuregateloader.go:41-51,192-218` (`New`, `doGet`) vs `pkg/dataloader/prowloader/github/github.go`
- concern: `pkg/dataloader/prowloader/github/github.go` already implements a GitHub client for this codebase with GitHub App installation-token auth (preferred) falling back to `GITHUB_TOKEN` (`newGHAuthClient`), plus explicit `RateLimits()` checking and `IsWithinRateLimitThreshold()` so callers can back off before hitting 403s. The new loader reimplements a bare `net/http.Client` that only reads `GITHUB_TOKEN` directly, has no rate-limit awareness, and no retry/backoff on non-200/404 responses (a 403 rate-limit response is just a generic error, and that release's fetch is dropped for the cycle with no retry). Both loaders share the same `GITHUB_TOKEN`/`api.github.com` quota with zero coordination — `listDirectory` here adds ~9 calls/cycle to a budget `prowloader` actively monitors, invisibly to it. Loader's own footprint is small (`listDirectory` is api.github.com-rate-limited at ~9 calls/cycle; `downloadFile` targets `raw.githubusercontent.com`, a separate, non-`api.github.com`-quota'd path), so this is unlikely to cause problems in practice, but it's an unnecessary duplication of already-solved infrastructure.
- excerpt: |
    githubToken:    os.Getenv("GITHUB_TOKEN"),
    ...
    resp, err := l.httpClient.Do(req) //nolint:gosec
    ...
    if resp.StatusCode == http.StatusNotFound {
        return nil, fmt.Errorf("not found: %s", targetURL)
    }
    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("HTTP %s from %s", resp.Status, targetURL)
    }

### [nit] `//nolint:gosec` on shared `doGet` lacks rationale comment
- where: `pkg/dataloader/featuregateloader/featuregateloader.go:204`
- concern: `doGet` is used both for the hardcoded-constant listing URL and the allowlist-checked download URL. The gosec (G107 variable URL in HTTP request) suppression is justified given `isAllowedDownloadURL` gates the download path, but a one-line comment on the suppression would help future readers who see the shared helper without the context of the caller-side check.
- excerpt: |
    resp, err := l.httpClient.Do(req) //nolint:gosec

### [nit] GitHub auth header uses legacy `token` scheme
- where: `pkg/dataloader/featuregateloader/featuregateloader.go:200-202`
- concern: `"token "+l.githubToken` still works for classic PATs but GitHub now recommends `Bearer` for newer token types (fine-grained PATs, OAuth/App tokens). Not a functional bug today, just something to revisit if the token type used in this env ever changes.
- excerpt: |
    req.Header.Set("Authorization", "token "+l.githubToken)

## Checked

- `parseFeatureGateFilename` regex (`featureGateFilenameRe`) reproduces old suffix-splitting behavior exactly for both legacy (`featureGate-{topology}-{featureSet}.yaml`) and versioned (`featureGate-{n}-{m}-{topology}-{featureSet}.yaml`) filenames; all pre-existing test cases pass unchanged.
- `isAllowedDownloadURL` correctly restricts `download_url` (untrusted value from GitHub API response) to `https://raw.githubusercontent.com` and `https://objects.githubusercontent.com`; covered by `TestIsAllowedDownloadURL` including scheme rejection, unknown host, and malformed URL cases.
- `dbErr` guard added before constructing `featuregateloader.New(ctx, dbc, releaseConfigs)` in `cmd/sippy/load.go:359-361` is consistent with the existing pattern used by every other loader branch in the same function (`bugs`, `test-mapping`, `ga-test-status`, etc.) — not a bug, just following convention.
- Partial-failure handling: a failed release fetch in `getFeatureGatesFromGitHub` is recorded in `l.errs` and the loop continues to the next release; if all releases fail, `upsertFeatureGates` short-circuits on the empty slice (no wasted COPY) while errors still surface via `Errors()`.
- `fetchFeatureGatesForBranch` aborts the branch on first download/unmarshal error, discarding any already-parsed files for that release — matches old `filepath.Walk` behavior, not a regression.
- `db.CopyToTempTable` (pre-existing helper, not touched by this PR) validates table/column identifiers against a regex before use in raw SQL — no injection risk from the fixed `featureGateTempCols` definitions.
- Composite unique constraint semantics preserved: old GORM `OnConflict` on `(release, topology, feature_set, feature_gate)` updating only `status` matches the new `ON CONFLICT ... DO UPDATE SET status, updated_at`.
- Storing `ctx context.Context` on the `FeatureGateLoader` struct is not an anti-pattern here: the shared `DataLoader` interface's `Load()` method (`pkg/dataloader/dataloader.go:3`, called via `pkg/dataloader/loaderwithmetrics/loaderwithmetrics.go:81`) takes no arguments, so a constructor-injected context is the established convention in this package (`gateststatus.GATestStatusLoader`, `testownershiploader.TestOwnershipLoader` do the same). Each loader is constructed fresh per CLI invocation and used once, so no staleness/reuse risk.
- Rate-limit exposure from this loader's own request volume is low: `listDirectory` (api.github.com, rate-limited) is ~9 calls/hourly cycle; `downloadFile` targets `raw.githubusercontent.com`, which is not part of the `api.github.com` rate-limit bucket.

## Open questions

- Is a functional test (skipped without credentials, per `releasesync_functional_test.go` pattern) planned as a fast-follow for the COPY/upsert path, or intentionally out of scope for this PR?
- Should `GITHUB_TOKEN` be documented in `config/README.md`, or is it considered ops/deployment-only config not covered by that doc?
- Was reusing `pkg/dataloader/prowloader/github.Client` (GitHub App auth + rate-limit checking) considered instead of a new bare HTTP client, given both now share the same `GITHUB_TOKEN`/`api.github.com` quota with no coordination?
