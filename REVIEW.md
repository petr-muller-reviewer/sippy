---
pr: openshift/sippy#3665
title: "Add 4.22-qe-auto-release view"
head_sha: 60bbbb0fe0fb4f48a641a4b678b759db4c046499
base: main
reviewed_at: 2026-06-23T20:43:04Z
verdict: approve
---

## Summary

Adds `4.22-qe-auto-release` component readiness view to `config/qe-views.yaml`. Config-only, no Go changes. Follows the established pattern for prior `qe-auto-release` views (4.14–4.21, 5.0).

## Findings

### [nit] 4.22 entries appear after 5.0 entries in file
- where: `config/qe-views.yaml:983`
- concern: All prior releases (4.14–4.21) are ordered chronologically with lower release numbers before higher ones. The new 4.22 entries are appended after `5.0-qe-main` (line 873) and `5.0-qe-auto-release` (line 927), breaking that convention. No runtime impact.
- excerpt: |
    5.0-qe-auto-release  (line 927)
    4.22-qe-auto-release (line 983)  ← appended here instead of before 5.0-*
    4.22-qe-main         (line 1039)

## Checked
- `base_release` uses `ga-30d`/`ga` for same release — correct, matches 4.14–4.21 pattern
- `sample_release` uses `now-7d`/`now` — correct for auto-release views
- `Upgrade` in `db_group_by` but absent from `include_variants` — intentional; absence means no filter on that dimension; consistent with all other auto-release views
- `Procedure: automated-release` is a recognized variant value in `pkg/variantregistry/ocp.go`
- `ignore_disruption: true` — standard for all qe-auto-release views
- No Go code has hardcoded view name lists; file is consumed via CLI flag at runtime
- `validateViews` only checks `variant_cross_compare` vs `db_group_by`; no `variant_cross_compare` in this entry, passes cleanly
- All platforms, installers, networks, topologies match the 4.21 auto-release view exactly

## Open questions
- Should the 4.22 entries be inserted before the 5.0 entries to restore chronological ordering?
