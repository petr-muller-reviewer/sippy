---
pr: openshift/sippy#3790
title: "Add analyze-db.sh script for post-restore database warmup"
head_sha: 3b8b69dbf0e434ce416d112b949a53842cab69d3
base: main
reviewed_at: 2026-07-26T11:48:15Z
verdict: needs-discussion
refresh_log:
  - from: 12f9de7d69562af7757e3edfb64de4b9f694f0c6
    to: 3b8b69dbf0e434ce416d112b949a53842cab69d3
    summary: >
      Genuine forward commit (OLD_SHA is an ancestor), ~81 inserted/9 deleted lines
      in scripts/analyze-db.sh plus a README update. Adds a third warmup step:
      a dynamic PL/pgSQL block that runs SELECT count(*) on every public-schema
      table (scoped to the last 30 days when a date/timestamp column exists) to
      pull heap pages off S3-backed storage. Pod command construction changed from
      manual triple-escaped bash/JSON to piping a heredoc through
      `python3 -c 'json.dumps(...)'` — this resolves the prior "fragile escaping"
      nit but introduces a new, undocumented local dependency on `python3`. PR
      description/test plan now cover ANALYZE+REINDEX+cache-warming, but the
      cache-warming verification checkbox is unchecked. No human review comments;
      CI green, `@coderabbitai review` requested by the author but produced no
      inline findings (incremental review, nothing new to flag). Judged this as
      update-in-place: single file, same script/oc-run domain as prior reviews, no
      near-total rewrite of unrelated sections.
  - from: 2391d59e475825cb4b1c4a07cf47f6ca63c24846
    to: 42b6154a5e6bee3521a255abf16c09d9f486d291
    summary: >
      Author dropped [WIP] title/label, so treating as ready for review. Script now
      runs the pod detached by default (no `--rm`/`-i`) and adds `--wait` to poll for
      completion, print logs, and clean up. Prior findings on image pinning, dry-run
      verbosity, pod-name collision, and swallowed delete errors are all still
      unresolved in the new code. backfill-summaries.sh also picked up an unrelated
      upstream change (dropping `daily-summaries` as a valid --table value) via
      rebase, not authored by this PR.
  - from: 42b6154a5e6bee3521a255abf16c09d9f486d291
    to: 12f9de7d69562af7757e3edfb64de4b9f694f0c6
    summary: >
      Force-pushed (rebase, OLD_SHA not an ancestor of NEW_SHA), but the file-level
      diff is small and targeted: adds a REINDEX DATABASE CONCURRENTLY step after
      ANALYZE VERBOSE (hardcodes the target database name `sippy_openshift`),
      switches the pod command to `sh -c` with an escaped multi-statement shell
      string, bumps `--wait` timeout from 30m to 120m, and updates README/comments
      to describe the "warmup" (ANALYZE + REINDEX) rationale (stale planner stats
      plus lazy-loaded EBS storage). PR now has `approved` label (author
      self-approved) and green CI; PR description/test plan still describe only the
      original ANALYZE-only behavior. All prior findings remain unresolved.
---

## Summary

Adds `scripts/analyze-db.sh` (runs a post-restore three-step "warmup" — `ANALYZE VERBOSE`, `REINDEX DATABASE CONCURRENTLY`, and a dynamic cache-warming pass — via an `oc run` pod), hardens `scripts/backfill-summaries.sh` flag parsing to reject missing flag values, and documents the new script in `README.md`. Previously `[WIP]`; author has since dropped the WIP title prefix and label, self-approved, and CI is green.

Since previous review:
- Added a third warmup step: a `DO $$ ... $$` PL/pgSQL block that iterates every non-partition table in `public`, runs `SELECT count(*)` (scoped to the last 30 days via `now() - interval '30 days'` when a date/timestamp column exists, full-table otherwise) to pull hot heap pages off S3-backed storage, and logs row counts via `RAISE NOTICE`.
- Pod command construction changed: the multi-statement `sh -c` script is now built as a bash heredoc and JSON-encoded by shelling out to `python3 -c 'json.dumps(...)'`, instead of manual triple-escaping. This resolves the earlier "fragile triple-escaping" nit but adds a new, undocumented dependency on `python3` being present on the operator's machine.
- README and header comment updated to describe all three steps.
- PR description/test plan now cover ANALYZE + REINDEX + cache-warming, but the cache-warming verification checkbox is unchecked (`- [ ] Run ./scripts/analyze-db.sh against staging to verify cache warming logs per-table row counts`).
- No human review comments since the last review. Author requested `@coderabbitai review`; CodeRabbit's incremental-review system reported nothing new to flag. CI green.

## Findings

### [should-fix] Hardcoded `:latest` image tag with no override
- where: `scripts/analyze-db.sh:18`
- status: unresolved
- concern: `IMAGE="registry.redhat.io/rhel9/postgresql-16:latest"` is still hardcoded with no `--image` override, unlike `backfill-summaries.sh` which auto-detects the image from the `sippy` DC or accepts `--image`. Not reproducible across runs, and no escape hatch if the environment's PG major version or registry access differs.
- excerpt: |
    IMAGE="registry.redhat.io/rhel9/postgresql-16:latest"

    oc -n "$NAMESPACE" run "$POD_NAME" --restart=Never \
        --image="$IMAGE" \

### [should-fix] `--dry-run` output is uninformative
- where: `scripts/analyze-db.sh:39-42`
- status: unresolved
- concern: Dry-run now prints `"Would create pod $POD_NAME to run ANALYZE VERBOSE and REINDEX"` (text updated for the new step) but still omits namespace, secret, and image, so it still doesn't let the operator verify the actual invocation before running for real — more important now that the pod also runs a REINDEX.
- excerpt: |
    if [[ "$DRY_RUN" == "true" ]]; then
        echo "Would create pod $POD_NAME to run ANALYZE VERBOSE and REINDEX"
        exit 0
    fi

### [should-fix] Hardcoded database name in REINDEX, unlike the parameterized DSN
- where: `scripts/analyze-db.sh:108`
- status: unresolved
- concern: `REINDEX DATABASE CONCURRENTLY sippy_openshift;` still hardcodes the database name, while every other part of the script (namespace, secret, DSN) is parameterized or sourced from the secret. Postgres requires `REINDEX DATABASE` to name the currently-connected database, so if `--db-secret` ever points at a DSN whose database isn't literally named `sippy_openshift`, this statement fails outright while `ANALYZE VERBOSE` and the cache-warming step (neither of which need a literal name) succeed — a partial, confusing failure.
- excerpt: |
    psql "$SIPPY_DATABASE_DSN" -c "REINDEX DATABASE CONCURRENTLY sippy_openshift;"

### [should-fix] New undocumented dependency on `python3` on the operator's machine
- where: `scripts/analyze-db.sh:125`
- status: new
- concern: Building the pod's `args` now shells out to `python3 -c "import sys,json; print(json.dumps(sys.stdin.read()))"` on the machine running the script, not inside the pod. This isn't mentioned in the header comment, `--help`-equivalent usage block, or README, and unlike `oc`, `python3` availability isn't obviously implied. If `python3` is missing, `set -euo pipefail` won't necessarily catch it cleanly since the failing command substitution sits inside a larger `--overrides="..."` argument rather than a standalone assignment — the likely failure mode is a confusing `oc run` error about malformed JSON (empty array element) rather than a clear "python3 not found."
- excerpt: |
    \"args\": [$(echo "$WARMUP_SCRIPT" | python3 -c "import sys,json; print(json.dumps(sys.stdin.read()))")],

### [question] `--wait` timeout unchanged despite a third, potentially expensive step
- where: `scripts/analyze-db.sh:136`
- status: new
- concern: The cache-warming step runs `SELECT count(*)` against every public-schema table (full scan for tables without a date/timestamp column). This is additive cost on top of ANALYZE + REINDEX, but the `--wait` timeout stayed at 120m (only raised once, for the REINDEX addition in the prior commit). Worth confirming 120m is still comfortably sufficient with three steps instead of two, especially for large undated tables.
- excerpt: |
    oc -n "$NAMESPACE" wait --for=jsonpath='{.status.phase}'=Succeeded --timeout=120m "pod/$POD_NAME" 2>/dev/null || {

### [question] Recency column chosen alphabetically, not by relevance
- where: `scripts/analyze-db.sh:75-81`
- status: new
- concern: When a table has multiple date/timestamp-typed columns, `date_col` is chosen via `ORDER BY c.column_name LIMIT 1` — i.e. whichever column name sorts first alphabetically (e.g. `archived_at` would beat `created_at`). For the stated goal (warm heap pages likely to be queried, i.e. "recent" rows), this heuristic could pick a column that doesn't reflect actual query recency patterns. Likely harmless (worst case: warms slightly the wrong 30-day window), but worth confirming this is intentional rather than an oversight.
- excerpt: |
    SELECT c.column_name INTO date_col
    FROM information_schema.columns c
    WHERE c.table_schema = 'public'
      AND c.table_name = tbl.tablename
      AND c.data_type IN ('date', 'timestamp with time zone', 'timestamp without time zone')
    ORDER BY c.column_name
    LIMIT 1;

### [question] Cache-warming step not yet manually verified by the author
- where: PR description test plan
- status: new
- concern: The PR's own test plan lists `- [ ] Run ./scripts/analyze-db.sh against staging to verify cache warming logs per-table row counts` as unchecked, while the ANALYZE/REINDEX and backfill-summaries items are checked. The new PL/pgSQL block (dynamic `EXECUTE` over every public table) hasn't been confirmed to run cleanly against real staging data as of this review.
- excerpt: |
    - [ ] Run `./scripts/analyze-db.sh` against staging to verify cache warming logs per-table row counts

### [nit] Pod no longer self-removes; litter accumulates without `--wait`
- where: `scripts/analyze-db.sh:39,117`
- status: unresolved
- concern: `--rm` was dropped, so a run without `--wait` leaves the pod (running, then completed) in the namespace indefinitely — only cleaned up by the next invocation's delete-before-create step, or manually. This is presumably intentional (README explains detached mode is so the local machine doesn't need to stay connected for long warmup runs), but there's no automatic GC and no mention of this tradeoff in the dry-run/help text.
- excerpt: |
    POD_NAME="sippy-analyze-db"
    IMAGE="registry.redhat.io/rhel9/postgresql-16:latest"
    ...
    oc -n "$NAMESPACE" run "$POD_NAME" --restart=Never \

### [nit] Static pod name can still race under concurrent use
- where: `scripts/analyze-db.sh:39,47`
- status: unresolved
- concern: `POD_NAME="sippy-analyze-db"` is still static. Now that the pod isn't `--rm`'d, and the warmup takes even longer with three steps, a re-run while a prior detached run is still in progress collides more visibly: the delete-before-create step kills the in-flight warmup from the previous invocation.
- excerpt: |
    POD_NAME="sippy-analyze-db"
    ...
    oc -n "$NAMESPACE" delete pod "$POD_NAME" --ignore-not-found --wait >/dev/null 2>&1 || true

### [nit] Delete step still swallows all errors, not just not-found
- where: `scripts/analyze-db.sh:47`
- status: unresolved
- concern: `--ignore-not-found` already makes a missing pod a non-error; the trailing `|| true` still hides real failures (e.g. expired `oc` auth), letting the script proceed silently into `oc run` where the failure resurfaces less clearly.
- excerpt: |
    oc -n "$NAMESPACE" delete pod "$POD_NAME" --ignore-not-found --wait >/dev/null 2>&1 || true

### [nit] `oc wait` failure path suppresses its own stderr
- where: `scripts/analyze-db.sh:136-141`
- status: unresolved
- concern: `oc ... wait ... 2>/dev/null || { ... }` discards the wait command's own error output (e.g. "pod not found" if creation is still propagating, or an RBAC error), relying entirely on the fallback `oc get pod` status check to explain what happened. In most failure modes the fallback covers it, but a wait-specific error (e.g. malformed jsonpath, permission denied on watch) would be silently dropped.
- excerpt: |
    oc -n "$NAMESPACE" wait --for=jsonpath='{.status.phase}'=Succeeded --timeout=120m "pod/$POD_NAME" 2>/dev/null || {
        STATUS=$(oc -n "$NAMESPACE" get pod "$POD_NAME" -o jsonpath='{.status.phase}' 2>/dev/null || echo "Unknown")
        echo "Pod finished with status: $STATUS" >&2
        oc -n "$NAMESPACE" logs "$POD_NAME" --tail=20 2>/dev/null || true
        exit 1
    }

### [question] Is `postgres-aws` the right default secret, and should it be required?
- where: `scripts/analyze-db.sh:25`
- status: unresolved
- concern: `backfill-summaries.sh` requires `--db-secret` (errors if unset) with no default; `analyze-db.sh` still silently defaults to `postgres-aws`. Confirm this default is correct for the intended target environment rather than an unintentional divergence from the sibling script's pattern.
- excerpt: |
    DB_SECRET="postgres-aws"

## Checked
- Verified (by manual bash reproduction) that `$(echo "$WARMUP_SCRIPT" | python3 -c "...")` works correctly even though it contains unescaped double quotes inside the outer `--overrides="..."` bash string — command substitution `$(...)` is its own quoting context, so this is not a bug. The resulting JSON is valid and the DSN reference (`$SIPPY_DATABASE_DSN`, unexpanded at capture time via the quoted `'SCRIPT_EOF'` heredoc delimiter) is correctly deferred to container runtime.
- The dynamic SQL in the `DO $$` block builds table/column names via `format(..., %I, ...)` (identifier-quoting), and the values come from `pg_tables`/`information_schema.columns` (server catalog), not external input — no SQL injection surface.
- `REINDEX DATABASE CONCURRENTLY` is valid Postgres 12+ syntax and avoids the exclusive lock a plain `REINDEX DATABASE` would take; consistent with wanting this safe to run against a live-ish staging DB.
- The partition-exclusion filter (`NOT EXISTS (... pg_inherits ... WHERE c.relname = t.tablename)`) correctly skips inheritance/declarative-partition child tables so they aren't redundantly warmed alongside their parent.
- Credential handling uses `secretKeyRef` into pod env, not baked into the manifest — consistent with `restore_prodlike_db.sh`. Unchanged.
- `backfill-summaries.sh` flag-parsing fix (`[[ $# -ge 2 ]]` guards) is consistent with the file's existing style and error-message convention; unchanged by this refresh.
- README and header comment updates are clear, match existing house style, and now describe all three warmup steps accurately.
- No unresolved human review threads; only CI/bot comments since last review (author requested `@coderabbitai review`, CodeRabbit reported nothing new via its incremental system, CI passed on 2026-07-25).

## Open questions
- `REINDEX DATABASE CONCURRENTLY sippy_openshift` hardcodes the database name — will this script ever be pointed at a DSN/secret whose database isn't named exactly `sippy_openshift`? If not, this is fine; if so, it needs to derive the name from the DSN or a flag.
- Is `python3` guaranteed to be available wherever this script runs? Should the script check for it up front with a clear error message, or avoid the dependency (e.g. `jq -Rs .` instead of `python3 -c json.dumps`)?
- Is the 120-minute `--wait` timeout still comfortable now that a third step (full-table counts across all public tables) has been added?
- Is the alphabetical `ORDER BY c.column_name LIMIT 1` choice of recency column intentional, or should it prefer a specific column name/pattern (e.g. `created_at`)?
- Has the cache-warming step actually been run against staging yet? The PR's own test plan checkbox for it is unchecked.
- Should `--image` be added to `analyze-db.sh` for parity with `backfill-summaries.sh`'s auto-detect/override pattern, and should the tag be pinned instead of `:latest`?
- Should `--dry-run` print the full resolved `oc run` invocation (namespace, secret, image, and the warmup script) instead of just a one-line summary?
- Is the `postgres-aws` default secret name confirmed correct, and should `--db-secret` be required like in `backfill-summaries.sh`?
- Now that the pod isn't `--rm`'d, is leftover-pod accumulation (when `--wait` isn't used) acceptable, or should the script warn about it / offer a cleanup flag?
- Is the static `POD_NAME` collision risk (now more consequential since detached runs can be killed by a subsequent invocation, and runs are longer with three steps) acceptable given expected usage patterns?
