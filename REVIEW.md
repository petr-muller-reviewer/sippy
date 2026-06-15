---
pr: openshift/sippy#3617
title: "Reclassify spot-check jobs from 'rare' to 'spotcheck-30d' tier"
head_sha: dd2dd3172e071f6b1ce295baa93d95d92b4c88f5
base: main
reviewed_at: 2026-06-15T10:04:25Z
verdict: approve
---

## Findings

### [nit] Stale comment on VariantJobTier constant
- where: `pkg/variantregistry/ocp.go:465`
- concern: Inline comment still says `// specifies rare, blocking, informing, standard jobs`. The `rare` tier no longer exists and `spotcheck-30d` was added. Should match the `setJobTier` godoc which was correctly updated.
- excerpt: |
    VariantJobTier             = "JobTier"      // specifies rare, blocking, informing, standard jobs

### [nit] No direct unit test for validateSpotCheckVariants error path
- where: `pkg/variantregistry/ocp.go:279-286`
- concern: The validation function is tested indirectly through `TestVariantSyncer` (happy path), but the negative case (spotcheck tier present without component/capability) has no direct test. Since this is a safety net for future classification errors, a table-driven test covering the error path would add confidence.

### [nit] spotCheckPatterns substrings slice always has one element
- where: `pkg/variantregistry/ocp.go:767-774`
- concern: Each entry in `spotCheckPatterns` uses `substrings []string` but only ever contains a single substring. Consistent with `jobTierPatterns` elsewhere so fine for pattern consistency, but could be simplified to a single string if multi-match is never needed.

## Checked
- All snapshot.yaml entries consistently updated: `rare` -> `spotcheck-30d`, correct component/capability per job type across all releases (4.12-5.0) and platforms including shiftstack
- `adjustJobTierBasedOnView` correctly handles `spotcheck-30d`: tier is not in view include lists, so the `!tierIncluded` early return preserves it without downgrade
- `setSpotCheckClassification` runs before `setJobTier` in the setter chain (line 507 before line 508), so the component variant is set before `setJobTier` checks for it
- No remaining `"rare"` references in config YAML/JSON files
- Validation added to both code paths: DB-driven `Identify()` in `ocp.go` and config-driven `Identify()` in `snapshot.go`
- View configs in `config/views.yaml` intentionally do not include `spotcheck-30d` (views opt in per PR description)

## Open questions
- Is there a plan to remove the `rare` tier entirely from the codebase, or will it remain as a valid tier for other future jobs? The `setJobTier` godoc no longer lists it, but `ocp.go:882` still references it in a comment about `cert-rotation-shutdown`.
