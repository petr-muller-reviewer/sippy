---
pr: openshift/sippy#3560
title: "NO-JIRA: Move synthetic release flag from BigQuery to config"
head_sha: a05ac8eca5d92f003ac13e392ea91999e4212e8b
base: main
reviewed_at: 2026-05-28T23:51:37Z
verdict: approve
---

## Summary

Moves the `Synthetic` boolean that identifies non-OCP-version releases (rosa-stage, aro-*, etc.) from the BigQuery `Releases` table into `v1.ReleaseConfig.Synthetic` in YAML config. Removes the GCP credentials requirement from `make update-variants`. Drops `Synthetic` from `sippyv1.Release` and `sippyv1.ReleaseRow`, simplifies `BuildSyntheticReleaseJobOverrides` signature by eliminating the `releaseConfigs []sippyv1.Release` parameter, and regenerates `openshift.yaml` (preview of ci-tools companion PR #5210 changes).

## Findings

### [nit] Renamed test case name is misleading
- where: `pkg/variantregistry/synthetic_test.go:156`
- concern: Case renamed from "release in releaseConfigs but not in config is ignored" to "release marked synthetic but not in config is fine" — but the test body has a `"4.22"` entry without `Synthetic: true`, not an entry marked synthetic. The new name describes a scenario the test doesn't exercise.
- excerpt: |
    name: "release marked synthetic but not in config is fine",
    releases: map[string]v1.ReleaseConfig{
        "4.22": {
            Jobs: map[string]bool{"job-a": true},
        },
    },

## Checked

- `Synthetic` field removal from `sippyv1.Release` is safe: frontend (sippy-ng) has zero references to it; the only API mapping was `transformRelease()` in `pkg/api/releases.go:481`.
- `BuildSyntheticReleaseJobOverrides` call sites updated consistently: `cmd/sippy/load.go`, `cmd/sippy/variants_generate.go`, `cmd/sippy/variants_snapshot.go`, and both test files.
- All 12 synthetic releases declared in the PR description are marked `synthetic: true` in `openshift-customizations.yaml` and present in the regenerated `openshift.yaml`.
- `aro-classic-*` periodic jobs in `snapshot.yaml` correctly gain a `Release:` field and change `JobTier: candidate → standard`, which is the functional fix these jobs needed.
- `TestVariantsSnapshot` reads the committed `openshift.yaml` on disk; since that file has been regenerated with `synthetic: true` entries, the test will pass and validate the correct merged state.
- `syntheticReleaseNames()` helper deleted with no remaining callers — no dead code left behind.
- BigQuery/GCP flags removed from `VariantSnapshotFlags` and `Makefile` comment updated — DX improvement is real.
- `openshift.yaml` deliberate preview of ci-tools PR #5210 is documented in the PR description; this is normal operating procedure for this repo.

## Open questions

- The `TestVariantsSnapshot` test now depends on `synthetic: true` being present in the committed `openshift.yaml`. If someone regenerates that file from the generator before the ci-tools companion PR ships (and the generator doesn't emit `synthetic:`), the test would still pass but with different semantics. Is there a guard or CI check that would catch this? (Not a blocker given the generator run will emit the right content once both PRs are merged.)
