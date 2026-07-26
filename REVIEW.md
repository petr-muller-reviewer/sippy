---
pr: openshift/sippy#3790
title: "Add analyze-db.sh script for post-restore database warmup"
head_sha: 12f9de7d69562af7757e3edfb64de4b9f694f0c6
base: main
reviewed_at: 2026-07-25T16:40:18Z
verdict: needs-discussion
refresh_log:
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

Adds `scripts/analyze-db.sh` (runs a post-restore "warmup" — `ANALYZE VERBOSE` plus `REINDEX DATABASE CONCURRENTLY` — via an `oc run` pod), hardens `scripts/backfill-summaries.sh` flag parsing to reject missing flag values, and documents the new script in `README.md`. Previously `[WIP]`; author has since dropped the WIP title prefix and label, self-approved, and CI is green.

Since previous review:
- Script scope expanded from ANALYZE-only to a two-step "warmup": `ANALYZE VERBOSE` followed by `REINDEX DATABASE CONCURRENTLY sippy_openshift`, justified in the README/header comment as fixing both stale planner statistics and lazy-loaded EBS storage pages.
- Pod command switched from `["psql", "$(DSN)", "-c", "ANALYZE VERBOSE;"]` to `["sh", "-c", "<escaped multi-statement string>"]` to sequence the two `psql` invocations with `&&` and interstitial `echo` progress messages.
- `--wait` timeout raised from 30m to 120m, consistent with REINDEX taking materially longer than ANALYZE alone.
- No human review comments or requested changes since the last review; CI bot activity only (approval bot self-approve notice, test scheduling, all tests passed on 2026-07-25). PR description/test plan still describe only the original ANALYZE-only behavior — not updated for REINDEX or `--wait`.

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
- where: `scripts/analyze-db.sh:56`
- status: new
- concern: `REINDEX DATABASE CONCURRENTLY sippy_openshift;` hardcodes the database name, while every other part of the script (namespace, secret, DSN) is parameterized or sourced from the secret. Postgres requires `REINDEX DATABASE` to name the currently-connected database, so if `--db-secret` ever points at a DSN whose database isn't literally named `sippy_openshift` (a differently-named clone, a per-environment DB name, etc.), this statement fails outright while the preceding `ANALYZE VERBOSE` (which takes no name) succeeds — a partial, confusing failure.
- excerpt: |
    \"args\": [\"echo 'Starting ANALYZE VERBOSE...' && psql \\\"\$SIPPY_DATABASE_DSN\\\" -c 'ANALYZE VERBOSE;' && echo 'Starting REINDEX DATABASE...' && psql \\\"\$SIPPY_DATABASE_DSN\\\" -c 'REINDEX DATABASE CONCURRENTLY sippy_openshift;' && echo 'Warmup complete.'\"],

### [nit] Triple-nested string escaping for the pod command is fragile to review
- where: `scripts/analyze-db.sh:55-56`
- status: new
- concern: The `sh -c` argument is escaped through three layers (bash double-quoted string → JSON string → shell metacharacters) to sequence two `psql` calls with `&&`/`echo`. I traced the escaping by hand and it is currently correct (`\\\"\$SIPPY_DATABASE_DSN\\\"` decodes to a properly double-quoted `$SIPPY_DATABASE_DSN` inside the container's `sh -c` script), but this is exactly the kind of construct where a single missed backslash silently breaks quoting instead of erroring — worth a comment noting the escaping layers, or moving the script to a heredoc/ConfigMap to reduce the escaping depth.
- excerpt: |
    \"command\": [\"sh\", \"-c\"],
    \"args\": [\"echo 'Starting ANALYZE VERBOSE...' && psql \\\"\$SIPPY_DATABASE_DSN\\\" -c 'ANALYZE VERBOSE;' && ...\"],

### [nit] Pod no longer self-removes; litter accumulates without `--wait`
- where: `scripts/analyze-db.sh:36,44`
- status: unresolved
- concern: `--rm` was dropped, so a run without `--wait` leaves the pod (running, then completed) in the namespace indefinitely — only cleaned up by the next invocation's delete-before-create step, or manually. This is presumably intentional (README explains detached mode is so the local machine doesn't need to stay connected for long ANALYZE runs), but there's no automatic GC and no mention of this tradeoff in the dry-run/help text.
- excerpt: |
    POD_NAME="sippy-analyze-db"
    IMAGE="registry.redhat.io/rhel9/postgresql-16:latest"
    ...
    oc -n "$NAMESPACE" run "$POD_NAME" --restart=Never \

### [nit] Static pod name can still race under concurrent use
- where: `scripts/analyze-db.sh:36,44`
- status: unresolved
- concern: `POD_NAME="sippy-analyze-db"` is still static. Now that the pod isn't `--rm`'d, two concurrent invocations (or a re-run while a prior detached run is still in progress) collide more visibly: the delete-before-create step will kill an in-flight ANALYZE+REINDEX from a previous invocation.
- excerpt: |
    POD_NAME="sippy-analyze-db"
    ...
    oc -n "$NAMESPACE" delete pod "$POD_NAME" --ignore-not-found --wait >/dev/null 2>&1 || true

### [nit] Delete step still swallows all errors, not just not-found
- where: `scripts/analyze-db.sh:44`
- status: unresolved
- concern: `--ignore-not-found` already makes a missing pod a non-error; the trailing `|| true` still hides real failures (e.g. expired `oc` auth), letting the script proceed silently into `oc run` where the failure resurfaces less clearly.
- excerpt: |
    oc -n "$NAMESPACE" delete pod "$POD_NAME" --ignore-not-found --wait >/dev/null 2>&1 || true

### [nit] `oc wait` failure path suppresses its own stderr
- where: `scripts/analyze-db.sh:67-72`
- status: unresolved
- concern: `oc ... wait ... 2>/dev/null || { ... }` discards the wait command's own error output (e.g. "pod not found" if creation is still propagating, or an RBAC error), relying entirely on the fallback `oc get pod` status check to explain what happened. In most failure modes the fallback covers it, but a wait-specific error (e.g. malformed jsonpath, permission denied on watch) would be silently dropped. Timeout was raised 30m→120m in this update, unrelated to the stderr-suppression concern.
- excerpt: |
    oc -n "$NAMESPACE" wait --for=jsonpath='{.status.phase}'=Succeeded --timeout=120m "pod/$POD_NAME" 2>/dev/null || {
        STATUS=$(oc -n "$NAMESPACE" get pod "$POD_NAME" -o jsonpath='{.status.phase}' 2>/dev/null || echo "Unknown")
        echo "Pod finished with status: $STATUS" >&2
        oc -n "$NAMESPACE" logs "$POD_NAME" --tail=20 2>/dev/null || true
        exit 1
    }

### [question] Is `postgres-aws` the right default secret, and should it be required?
- where: `scripts/analyze-db.sh:22`
- status: unresolved
- concern: `backfill-summaries.sh` requires `--db-secret` (errors if unset) with no default; `analyze-db.sh` still silently defaults to `postgres-aws`. Confirm this default is correct for the intended target environment rather than an unintentional divergence from the sibling script's pattern.
- excerpt: |
    DB_SECRET="postgres-aws"

## Checked
- Hand-traced the new triple-escaped `sh -c` args string end to end (bash → JSON → container shell): it decodes correctly to `psql "$SIPPY_DATABASE_DSN" -c 'ANALYZE VERBOSE;' && ... psql "$SIPPY_DATABASE_DSN" -c 'REINDEX DATABASE CONCURRENTLY sippy_openshift;'`, with the DSN properly double-quoted inside the container's shell. No injection or quoting bug found (see nit on fragility above, though).
- `REINDEX DATABASE CONCURRENTLY` is valid Postgres 12+ syntax and avoids the exclusive lock a plain `REINDEX DATABASE` would take; consistent with wanting this safe to run against a live-ish staging DB.
- Credential handling uses `secretKeyRef` into pod env, not baked into the manifest — consistent with `restore_prodlike_db.sh`. Unchanged.
- `backfill-summaries.sh` flag-parsing fix (`[[ $# -ge 2 ]]` guards) is consistent with the file's existing style and error-message convention; unchanged by this refresh.
- `--wait` timeout bump 30m→120m is a sensible adjustment given REINDEX CONCURRENTLY on a full database can run substantially longer than ANALYZE alone.
- README addition/update is clear, matches existing house style (no headers-heavy formatting, no em dashes), and now explains the EBS-lazy-loading rationale for REINDEX.
- No unresolved human review threads; only CI/bot comments since last review (self-approval notice, test scheduling, all tests passed on 2026-07-25).
- PR description/test plan is stale (still describes ANALYZE-only, no mention of REINDEX or `--wait`) — noted as an open question below, not a code finding.

## Open questions
- `REINDEX DATABASE CONCURRENTLY sippy_openshift` hardcodes the database name — will this script ever be pointed at a DSN/secret whose database isn't named exactly `sippy_openshift`? If not, this is fine; if so, it needs to derive the name from the DSN or a flag.
- Should `--image` be added to `analyze-db.sh` for parity with `backfill-summaries.sh`'s auto-detect/override pattern, and should the tag be pinned instead of `:latest`?
- Should `--dry-run` print the full resolved `oc run` invocation (namespace, secret, image, and now the two-statement SQL) instead of just the pod name?
- Is the `postgres-aws` default secret name confirmed correct, and should `--db-secret` be required like in `backfill-summaries.sh`?
- Now that the pod isn't `--rm`'d, is leftover-pod accumulation (when `--wait` isn't used) acceptable, or should the script warn about it / offer a cleanup flag?
- Is the static `POD_NAME` collision risk (now more consequential since detached runs can be killed by a subsequent invocation, and runs are longer with REINDEX added) acceptable given expected usage patterns?
- Can the PR description/test plan be updated to reflect the REINDEX step and `--wait` flag before merge?
