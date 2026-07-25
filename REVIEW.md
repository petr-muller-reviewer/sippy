---
pr: openshift/sippy#3821
title: "TRT-2799: syncPRStatus: Fix SHA mismatch bug (MergeCommitSHA vs Head.SHA)"
head_sha: 89c0433878f44acc6bc42df45f0262a9bd8e9ede
base: main
reviewed_at: 2026-07-25T21:25:28Z
verdict: approve
---

## Summary

`syncPRStatus` compared `prow_pull_requests.SHA` (PR head SHA) against GitHub's `MergeCommitSHA`
(synthetic merge commit) — these almost never match, leaving 82% of rows with `merged_at IS NULL`.
Fix: new `ListRecentlyMergedPRs` returns `Head.SHA` instead; `syncPRStatus` rewritten from a
per-row loop with per-row `Save()` into a batch pipeline (distinct org/repo -> fetch merged PRs ->
COPY to temp table -> bulk UPDATE/DELETE via raw SQL).

## Findings

### [should-fix] Partial progress discarded on rate-limit error mid-loop
- where: `pkg/dataloader/prowloader/prow.go:840-856`
- concern: The old per-PR loop called `Save()` immediately per row, so a later rate-limit error
  still left earlier rows durably updated. The new code accumulates all repos' results into
  `allMerged` in memory and only writes once at the end via the temp-table/bulk-UPDATE path. If
  `ListRecentlyMergedPRs` hits a rate-limit error partway through the `repos` loop, the function
  returns early and everything collected from repos already processed is discarded — nothing is
  persisted this cycle. Low severity since this loader runs periodically and unprocessed rows are
  simply retried next cycle, but it is a real behavior change from the prior incremental-save
  approach.
- excerpt: |
    for _, r := range repos {
        merged, err := pl.githubClient.ListRecentlyMergedPRs(r.Org, r.Repo)
        if err != nil {
            if pl.githubClient.IsWithinRateLimitThreshold() {
                return err
            }
            continue
        }
        ...
    }

### [nit] Inconsistent join key between the two bulk statements
- where: `pkg/dataloader/prowloader/prow.go:895-908`
- concern: The `UPDATE` joins temp table to `prow_pull_requests` on `sha` alone, while the
  `DELETE` a few lines later joins `pull_request_comments` on the full `(org, repo, number)`
  tuple. SHA collisions across repos are practically impossible so this isn't a correctness bug
  (and mirrors the original code's SHA-only matching), but the asymmetry within the same function
  is worth a comment or aligning both on the full tuple for clarity.
- excerpt: |
    UPDATE prow_pull_requests p
    SET merged_at = t.merged_at, updated_at = NOW()
    FROM tmp_merged_prs t
    WHERE p.sha = t.sha AND p.merged_at IS NULL
    ...
    DELETE FROM pull_request_comments c
    USING (SELECT DISTINCT org, repo, number FROM tmp_merged_prs) t
    WHERE c.org = t.org AND c.repo = t.repo AND c.pull_number = t.number
        AND c.comment_type = $1

### [question] Was the bulk backfill validated against prod-like data volume?
- where: `pkg/dataloader/prowloader/prow.go:810-910`
- concern: First run after merge will attempt to set `merged_at` for the ~82% of existing rows
  described in the PR body. The test plan in the PR description covers unit tests and lint but
  doesn't mention a dry run or row-count check against a prod-like copy of `prow_pull_requests`.
- excerpt: |
    (no excerpt — process question, not code)

## Checked
- Core fix: `ListRecentlyMergedPRs` returns `Head.SHA`, not `MergeCommitSHA` — confirmed in code and
  explicitly asserted in `TestClient_ListRecentlyMergedPRs`.
- `ClearPendingRecord`/`QueryPRPendingComments` (replaced by the bulk DELETE) only touch the DB, not
  the GitHub API — bulk DELETE is behaviorally equivalent, not a functional regression.
- `db.CopyToTempTable` + `stdlib.AcquireConn`/`ReleaseConn` usage matches existing idiom used
  elsewhere in the same file (`findNewJobRunIDs`) and in `bugloader.go`/`releasesync.go`.
- `gofmt -l` clean and `go build ./...` passes on all three changed files.
- Removed `gorm` import is correctly unused elsewhere in the file (`gorm.io/gorm/clause` is a
  separate import and still used).
- No BigQuery equivalent of `syncPRStatus`/`prow_pull_requests` exists, so the provider-parity rule
  does not apply here.
- SQL is parameterized (`$1` for `comment_type`); `CopyToTempTable` validates identifiers via
  `identifierRe` before building DDL.
- New test (`TestClient_ListRecentlyMergedPRs`) covers not-merged PRs, missing `Head`, and cache
  reuse on second call.

## Open questions
- Was this backfill run/dry-run against a prod-like snapshot of `prow_pull_requests` to confirm the
  expected ~82% of rows get `merged_at` set and nothing unexpected happens at that row count?
- Is the discarded-partial-progress-on-rate-limit behavior acceptable, or should collected
  `allMerged` data be flushed before returning the rate-limit error?
