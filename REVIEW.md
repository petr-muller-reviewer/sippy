---
pr: openshift/sippy#3853
title: "Remove prow_ga_test_statuses_matview reintroduced by bad merge"
head_sha: 61a720f38cd9a65dad8d58a5a84099a6206808e9
base: main
reviewed_at: 2026-07-30T17:14:13Z
verdict: approve
---

## Summary

PR #3838 (TRT-2814) was rebased onto `main` after `2a0a86e4` (TRT-2741) had
already removed the unused `prow_ga_test_statuses_matview`. The rebase/merge
was resolved incorrectly: it resurrected the `prow_ga_test_statuses_matview`
struct entry and `gaTestStatusMatView` SQL constant in `pkg/db/views.go`,
while dropping the actually-intended removal of
`payload_test_failures_14d_matview`. This PR deletes the reintroduced dead
code, restoring the state both prior PRs intended.

Verified against history:
- `git show 2a0a86e40` — removes the identical 6-line struct entry + SQL const.
- `git show e35dbaca6` (the bad-merge commit) — its diff on `pkg/db/views.go`
  re-adds `prow_ga_test_statuses_matview` in place of removing
  `payload_test_failures_14d_matview`, matching the PR description exactly.
- `git grep -n "gaTestStatusMatView\|prow_ga_test_statuses_matview"` — zero
  remaining references after this diff.

## Findings

### [question] Possible orphaned matview on already-deployed databases
- where: `pkg/db/views.go:71-119` (`syncPostgresMaterializedViews`)
- concern: The sync loop only creates/updates matviews present in
  `PostgresMatViews`; it has no path that drops a matview removed from the
  list. If the bad-merge state (`e35dbaca6`) was ever deployed to a live
  database, `prow_ga_test_statuses_matview` would now exist as a real
  Postgres object with nothing left in code to clean it up. Not introduced by
  this PR (same gap existed for the original `2a0a86e40` removal), but worth
  confirming whether the bad-merge state reached any real environment before
  merging this fix. If so, a manual `DROP MATERIALIZED VIEW IF EXISTS
  prow_ga_test_statuses_matview CASCADE` may be needed out of band.
- excerpt: |
    for _, pmv := range PostgresMatViews {
        ...
        dropSQL := fmt.Sprintf("DROP MATERIALIZED VIEW IF EXISTS %s CASCADE", pmv.Name)
        schema := fmt.Sprintf("CREATE MATERIALIZED VIEW %s AS %s WITH NO DATA", pmv.Name, viewDef)
        matViewUpdated, err := syncSchema(db, hashTypeMatView, pmv.Name, schema, dropSQL, false)

## Checked
- `gofmt -l pkg/db/views.go` — clean.
- `go build ./pkg/db/...` — succeeds.
- Diff is a pure deletion, byte-identical to what `2a0a86e40` removed; no
  unrelated changes.
- No other file in the repo references `gaTestStatusMatView` or
  `prow_ga_test_statuses_matview`.
- CR provider already reads `prow_ga_raw_test_data` directly (per
  `2a0a86e40`'s commit message), so removing the matview has no behavioral
  impact on any live code path.

## Open questions
- Did the bad-merge state (`e35dbaca6`) ever ship to a deployed database
  before this fix landed? If yes, is a manual `DROP MATERIALIZED VIEW`
  needed against that environment, since the code-level sync never drops
  matviews removed from `PostgresMatViews`?
