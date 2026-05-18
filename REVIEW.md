---
pr: openshift/sippy#3535
title: "MPIIT: Update Owned Views"
head_sha: 05b2ed468471013c05a7258879d52f02a2b2e614
base: master
reviewed_at: 2026-05-18T14:49:25Z
verdict: approve
---

## Findings

### [nit] Name reuse may confuse git history
- where: `config/views.yaml:617`
- concern: The new `5.0-LP-Interop` reuses the name the old view had before rename to `5.0-LP-OCP-Compat`. Git history searches for this name will conflate the two distinct views. A distinct name like `5.0-LP-Interop-Onboarding` would be cleaner.

### [question] Identical new views
- where: `config/views.yaml:617-708`
- concern: `5.0-LP-Interop` and `5.0-LP-Chaos` are byte-for-byte identical (same releases, variant options, advanced options, empty product list). Expected to diverge soon with different `LayeredProduct` entries?

## Checked
- Rename consistent across all four releases (5.0, 4.22, 4.21, 4.20)
- No stale `LP-Interop` references in Go, JS/TS, JSON, or docs
- Renamed `5.0-LP-OCP-Compat` retains original `LayeredProduct` list
- New views match existing structural pattern
- `regression_tracking: enabled: true` set on all six affected views
- New views are 5.0-only, no 4.x counterparts
- Adjacent views (`5.0-lp-Interop-coo`, etc.) unaffected

## Open questions
- Is the `5.0-LP-Interop` name reuse intentional or would a distinct name be better?
- Will `5.0-LP-Interop` and `5.0-LP-Chaos` receive different `LayeredProduct` entries soon?
