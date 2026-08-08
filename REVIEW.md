---
pr: openshift/sippy#3860
title: "Add OSD GCP operator e2e and HyperShift CI to ROSA releases"
head_sha: 20313de85b942456d280438cd47ab023ddeb4add
base: main
reviewed_at: 2026-08-08T11:37:27Z
verdict: approve
---

## Summary

Single-file config change to `config/openshift-customizations.yaml`. Widens the `rosa-stage`/`rosa-integration` SRE operator e2e regexp from `-master-rosa-sts-e2e-promotion-(stage|int)$` to `-(master|main)-(rosa-sts|osd-gcp)-e2e-promotion-(stage|int)$` (picks up 5 new OSD GCP operator jobs + fixes AVO, which branches off `main`). Also adds a new regexp line for HyperShift CI sector tests (staging + integration).

## Findings

### [question] no rosa-production equivalent added
- where: `config/openshift-customizations.yaml:101-110`
- concern: Neither the OSD GCP operator regex nor the HyperShift CI regex has a `rosa-production` counterpart. This matches pre-existing behavior (no `-prod` variant existed before this PR either), so it's not a regression, but worth confirming whether GCP-operator/HyperShift production jobs exist and are intentionally out of scope.
- excerpt: |
    rosa-production:
      synthetic: true
      ...
      regexp:
        # OCM FVT production
        - "^periodic-ci-openshift-online-rosa-e2e-main-ocm-fvt-.*-production-.*"

## Checked
- Anchoring (`^`/`$`) preserved correctly on the modified SRE operator regexp lines.
- Comment updates (`STS + GCP`) accurately reflect the widened match scope.
- New HyperShift regexp's unanchored trailing `.*` is consistent with existing style in the same block (e.g. the `ocm-fvt-.*-staging-.*` line).
- Widening `master` → `(master|main)` is scoped tightly enough (combined with `rosa-sts`/`osd-gcp` + `e2e-promotion-(int|stage)`) that accidental overbroad matches are unlikely.
- No test coverage exists or is expected for this file; it's runtime-loaded YAML with no schema/regex tests in the repo, consistent with prior commits touching only this file.
- Independent second-pass review (line-by-line diff, cross-file trace into `pkg/releaseoverride/override.go` and `pkg/variantregistry/synthetic.go`) confirmed no correctness or convention issues.
- `SyntheticReleaseOverrides.Lookup` iterates the `releases` map in nondeterministic order, so a job matching regexes in two different synthetic releases could resolve inconsistently; not triggered here since the new patterns use disjoint literal substrings (`-staging-` vs `-integration-`, `-promotion-stage$` vs `-promotion-int$`) across releases. Pre-existing structural property of the override mechanism, not introduced by this PR.

## Open questions
- Are there production-environment equivalents of the OSD GCP operator or HyperShift CI jobs that should eventually be added to `rosa-production`, or are those environments not applicable here?
