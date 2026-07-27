---
pr: openshift/sippy#3831
title: "TRT-2799: Extract PR merge sync into standalone loader and fix SHA bug"
head_sha: 723b1633b4debcfcf81baa5ca461246722ee61d8
base: main
reviewed_at: 2026-07-27T11:16:11Z
verdict: approve
---

## Summary

Extracts `syncPRStatus` out of `ProwLoader.Load()` into a standalone `PRMergeSyncLoader` (`pkg/dataloader/prmergesyncloader/`), used by both the regular `pr-merge-sync` load step and a new `backfill-pr-status` CLI command. Fixes a SHA-comparison bug (compared `MergeCommitSHA` against PR head SHA, which almost never matched; now uses `Head.SHA`), fixes a GitHub pagination bug (page counter incremented by page length instead of following `resp.NextPage`), and wraps the merged_at UPDATE + stale-comment DELETE in one pgx transaction via a temp-table join instead of row-by-row GORM saves.

## Findings

### [should-fix] No automated tests for the new package
- where: `pkg/dataloader/prmergesyncloader/prmergesyncloader.go`
- concern: No `_test.go` added for `prmergesyncloader`. Repo convention discourages mocking DB/GitHub clients, but the batch-splitting arithmetic in `Backfill` (`totalBatches`, `start`/`end` slicing, `batchIdx*batchSize < len(prs)`) is pure logic that could be extracted and unit tested without I/O. This loader performs UPDATE/DELETE against production data, so some coverage beyond the manual staging run would add confidence.
- excerpt: |
    totalBatches := (len(prs) + batchSize - 1) / batchSize

    for batchIdx := 0; batchIdx*batchSize < len(prs); batchIdx++ {
        start := batchIdx * batchSize
        end := start + batchSize
        if end > len(prs) {
            end = len(prs)
        }
        batch := prs[start:end]

### [question] Sibling-row semantics: retest rows never get merged_at
- where: `pkg/dataloader/prmergesyncloader/prmergesyncloader.go:205-217,459-468`
- concern: `unmatchedPRs()` excludes a PR row once *any* sibling row (same org/repo/number) has `merged_at` set, while `flushMergedPRs`'s UPDATE only sets `merged_at` on the row whose `sha` exactly matches the merge SHA. So once the matching-SHA row is updated, other unmerged SHA rows for the same PR (earlier retests) are permanently skipped by future runs and keep `merged_at = NULL` forever. This matches the old per-row behavior (not a regression), but is easy to mistake for a bug later — worth a one-line comment confirming it's intentional.
- excerpt: |
    func (l *PRMergeSyncLoader) unmatchedPRs() *gorm.DB {
        return l.dbc.DB.
            Table("prow_pull_requests p").
            Where("p.merged_at IS NULL").
            Where(`NOT EXISTS (
                SELECT 1 FROM prow_pull_requests p2
                WHERE p2.org = p.org AND p2.repo = p.repo AND p2.number = p.number
                  AND p2.merged_at IS NOT NULL
            )`)
    }

### [question] Unbounded GetPREntry cache during large backfills
- where: `pkg/dataloader/prowloader/github/github.go:258-286`
- concern: `Client.cache` is never evicted and `Backfill` can process large volumes of distinct PRs (bounded only by `--limit`). For a short-lived CLI process this is probably fine, but worth confirming there's no repo/PR count where this becomes a real memory concern, since backfill is explicitly meant to run over much larger volumes than a normal load step.
- excerpt: |
    func (c *Client) GetPREntry(org, repo string, number int) (*PREntry, error) {
        c.cacheLock.Lock()
        defer c.cacheLock.Unlock()
        prl := prlocator{org: org, repo: repo, number: number}
        if val, ok := c.cache[prl]; ok {
            return val, nil
        }

### [nit] Backfill context cancellation only checked between batches
- where: `pkg/dataloader/prmergesyncloader/prmergesyncloader.go:403-409`
- concern: `l.ctx.Done()` is only checked in the pause `select` between batches, not while iterating `GetPREntry` calls within a batch. In-flight GitHub calls for the current batch will run to completion even if the context is cancelled mid-batch. Acceptable for a CLI tool, just not immediately interruptible.
- excerpt: |
    if end < len(prs) {
        select {
        case <-l.ctx.Done():
            return l.ctx.Err()
        case <-time.After(time.Duration(pause) * time.Second):
        }
    }

## Checked
- `go vet ./pkg/dataloader/... ./pkg/db/...` passes; `gofmt -l` clean on all changed files.
- No remaining references to removed `syncPRStatus`/`IsPrRecentlyMerged`/`closedCache`.
- SHA fix is correct: `ProwPullRequest.SHA` is documented as "the specific commit at HEAD," which matches GitHub's `Head.SHA`, not `MergeCommitSHA`.
- Pagination fix correctly follows `resp.NextPage` and stops on `pastWindow || resp.NextPage == 0`.
- `flushMergedPRs` transaction (temp table + UPDATE + DELETE + commit) is sound; rollback deferred and ignores `pgx.ErrTxClosed` after commit.
- `pr-merge-sync` correctly added to both the default `--loader` flag list (`cmd/sippy/load.go`) and `loaderOrder` (`loaderwithmetrics.go`), positioned before `prow`.
- `ga-test-status` addition to `loaderOrder` is a plausible pre-existing omission fix, unrelated to but bundled with this change; low risk.

## Open questions
- Is it intentional that retest rows (non-matching SHA siblings) never receive `merged_at` once a sibling row is marked merged? If so, worth a comment on `unmatchedPRs()`.
- Any expected upper bound on distinct PRs per `backfill-pr-status` run that would make the unbounded `GetPREntry` cache a concern?
