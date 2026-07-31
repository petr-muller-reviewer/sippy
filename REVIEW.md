---
pr: openshift/sippy#3850
title: "Add ODF lp-interop variant and CR view for OCP 4.22"
head_sha: 8a8430eca697eb5c90619b860c1513cd3bb418a4
base: main
reviewed_at: 2026-07-30T17:44:06Z
verdict: request-changes
---

## What this PR does

- Adds `{"-lpga-lp-interop-cr--odf--", "lp-interop--odf--lpGA"}` to `layeredProductPatterns` in `pkg/variantregistry/ocp.go` (`setLayeredProduct`), classifying ODF lp-interop CR jobs.
- Adds a `TestVariantSyncer` case in `pkg/variantregistry/ocp_test.go` for job `periodic-ci-red-hat-storage-ocs-ci-master-odf-ocp-4.22-lpGA-lp-interop-cr--odf--aws`.
- Adds a new Component Readiness view `4.22-LP-Interop--lpGA` in `config/views.yaml`.

## Findings

### [blocking] New view's include_variants won't scope to the new LayeredProduct value, and will likely return zero rows
- where: `config/views.yaml:1792-1834` (new `4.22-LP-Interop--lpGA` view), compare `config/views.yaml:1736-1790` (`4.22-LP-OCP-Compat--lpGA`)
- concern: `4.22-LP-OCP-Compat--lpGA` explicitly enumerates its LP-GA layered products under `include_variants.LayeredProduct` (e.g. `lp-ocp-compat--odf--lpGA`, `lp-ocp-compat--virt--lpGA`, ...) plus `Owner: [lp]`. The new `4.22-LP-Interop--lpGA` view instead has `include_variants.LayeredProduct: []` (empty) and filters only on `Owner: [mpiit]` + `Network: [ovn]`. Since `Owner: mpiit` is set generically for any job matching `-lp-interop-` (`pkg/variantregistry/ocp.go:558`), this new view is byte-for-byte identical (apart from `name:`) to the pre-existing `4.22-LP-Interop--lpMainline` view — the new `lp-interop--odf--lpGA` variant value introduced by this PR is never used to scope it. Worse, an empty `include_variants` value is not a no-op filter in the query builder: `BuildComponentReportQuery` (`pkg/api/componentreadiness/dataprovider/bigquery/querygenerators.go:470-478`) emits `AND (jv_LayeredProduct.variant_value in UNNEST(@variantGroup_LayeredProduct))` bound to an empty array, which is always false in BigQuery — so the view would return zero rows. The Postgres provider has the same gap: `matchesIncludeVariants` (`pkg/api/componentreadiness/dataprovider/postgres/provider.go:85-95`) does `slices.Contains(allowed, val)` with no `len(allowed)==0` guard. Contrast with `adjustJobTierBasedOnView` (`pkg/variantregistry/ocp.go:398-401`), which explicitly treats an empty allowed-list as "no filter" — that intent exists elsewhere in the codebase but not in the query-building path. This exact `LayeredProduct: []` pattern already exists in `4.20-LP-Interop`, `4.21-LP-Interop`, and `4.22-LP-Interop--lpMainline`, so it may be a pre-existing/latent issue rather than new — but this PR adds a 4th instance and its own (unchecked) test-plan item "View renders correctly in Sippy CR dashboard" is exactly the check that would surface it.
- excerpt: |
    - name: 4.22-LP-Interop--lpGA
      ...
      include_variants:
        LayeredProduct: []
        Network:
        - ovn
        Owner:
        - mpiit
- suggested fix: populate `include_variants.LayeredProduct` with the actual GA value(s), e.g. `["lp-interop--odf--lpGA"]`, mirroring `4.22-LP-OCP-Compat--lpGA`.

### [nit] Trailing-dash inconsistency in new pattern
- where: `pkg/variantregistry/ocp.go:1371`
- concern: new pattern is `-lpga-lp-interop-cr--odf--` (double trailing dash) while the analogous compat pattern is `-lpga-lp-ocp-compat-cr--odf-` (single trailing dash, `pkg/variantregistry/ocp.go:1364`). Matches the given test job name (`...--odf--aws`), so not necessarily wrong, but worth confirming it won't miss ODF interop jobs whose name only has a single dash after `odf`.
- excerpt: |
    {"-lpga-lp-ocp-compat-cr--odf-", "lp-ocp-compat--odf--lpGA"},   // single trailing dash
    {"-lpga-lp-interop-cr--odf--", "lp-interop--odf--lpGA"},        // double trailing dash

### [question] No lpMainline counterpart pattern added
- where: `pkg/variantregistry/ocp.go:1371`, `config/views.yaml:1645` (existing `4.22-LP-Interop--lpMainline` view)
- concern: only a `-lpga-...` pattern is added for ODF interop; there's no `-lpmainline-lp-interop-cr--odf--` pattern, unlike the OCP-Compat family which has both lpMainline and lpGA ACS patterns. Presumably intentional since the companion `openshift/release#82653` PR only defines a GA job today, but flagging in case a Mainline ODF interop job is expected later.

## Checked
- `TestVariantSyncer` new case: all expected variant values traced through the relevant setter functions (`setLayeredProduct`, `setJobTier` default-to-candidate, `Owner` via `-lp-interop-` substring match) and are consistent with existing logic.
- Pattern placement in `layeredProductPatterns` list does not collide with earlier/later entries for the given test job name.
- No README/API doc updates required — `config/views.yaml` isn't documented in a README; not an API or CLI flag change.
- CI checks (build, lint, security, verify, yaml-lint) passing at time of review; unit/e2e pending.
- `gofmt` formatting looks consistent with surrounding code.

## Open questions
- Have you manually loaded this view in a running Sippy instance to confirm it renders data (per the unchecked "View renders correctly in Sippy CR dashboard" test-plan item)? Given the `include_variants.LayeredProduct: []` analysis above, I'd expect it to render empty.
- Is the double-trailing-dash in the new pattern (`--odf--`) deliberate, or should it match the single-dash convention used by the compat pattern?
