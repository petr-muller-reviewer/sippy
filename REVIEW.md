---
pr: openshift/sippy#3909
title: "Trt 2879 5.0 branching views"
head_sha: 70ccae0eaed3a990d4a9160bd5dac86b841b67a2
base: main
reviewed_at: 2026-08-18T19:58:04Z
verdict: approve
---

## What this PR does
- Adds new component-readiness view stanzas for `4.23-*` and `5.1-*` releases in `config/views.yaml`, mirroring the existing `5.0-*`/`4.22-*` templates.
- Reclassifies 339 `JobTier` entries in `pkg/variantregistry/snapshot.yaml` (mostly `standard`/`informing` → `candidate`).
- Purely generated/data config, no Go code changes.

## Findings

(none)

## Checked
- Diffed each new `4.23-*`/`5.1-*` view against `5.0-*` template field-by-field: only difference is `Platform: gcd` missing from `4.23-main`/`5.1-main`, correct since the sole `gcd` job is pinned to `release-5.0`.
- Verified the one anomalous tier change (`blocking` -> `candidate` for `periodic-ci-openshift-release-main-ci-5.1-e2e-aws-upgrade-ovn-single-node`) has no observable effect: its `Topology: single` already excluded it from every `blocking`-including view before the change.
- Confirmed `ga` vs `now` `base_release` convention per `.apm/prompts/sippy-generate-release-views.prompt.md`: `4.23-main` uses `ga` (4.22 already GA'd), `5.1-main` uses `now` (5.0 not GA'd yet). Both correct.
- Cross-checked all 337/339 recovered job records in old vs new `snapshot.yaml` against every view's `include_variants` filters via script: no job matched under old `JobTier` becomes unmatched under all views with new `JobTier` — no jobs silently drop out of monitoring.
- Ran `go test -run TestProductionViewsConfiguration ./pkg/flags/...`: passes.

## Open questions
(none)
