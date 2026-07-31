---
pr: openshift/sippy#3850
title: "Add ODF lp-interop variant and CR view for OCP 4.22"
head_sha: cdfb7b8729ff23a6026e80b01f4b3cb365b6725b
base: main
reviewed_at: 2026-07-31T10:30:42Z
verdict: request-changes
---

## What this PR does

- Adds `{"-lpga-lp-interop-cr--", "lp-interop--odf--lpGA"}` to `layeredProductPatterns` in `pkg/variantregistry/ocp.go` (`setLayeredProduct`), classifying ODF lp-interop CR jobs.
- Adds a `TestVariantSyncer` case in `pkg/variantregistry/ocp_test.go` for job `periodic-ci-red-hat-storage-ocs-ci-master-odf-ocp-4.22-lpGA-lp-interop-cr--aws`.
- Adds a new Component Readiness view `4.22-LP-Interop--lpGA` in `config/views.yaml`.
- Follow-up commit (`cdfb7b872`) since the last review round narrowed the match pattern from `-lpga-lp-interop-cr--odf--` to `-lpga-lp-interop-cr--` and updated the test job name to match, tracking a renamed upstream CR job (companion `openshift/release#82653`).

## Findings

### [blocking] New pattern matches any lp-interop CR job but hardcodes the ODF label
- where: `pkg/variantregistry/ocp.go:1371`
- concern: the pattern was changed from `-lpga-lp-interop-cr--odf--` to `-lpga-lp-interop-cr--`, dropping the product discriminator entirely, while the mapped value is still hardcoded to `lp-interop--odf--lpGA`. Any current or future `-lpga-lp-interop-cr--<other-product>...` job will be misclassified as ODF. Compare the OCP-Compat family in the same list, where every `-lpga-lp-ocp-compat-cr--<product>-` pattern explicitly encodes the product it maps to (e.g. `-lpga-lp-ocp-compat-cr--odf-`, `-lpga-lp-ocp-compat-cr--quay-`). This looks like an overcorrection of the previous review's trailing-dash nit — the fix should restore a product-scoped substring (e.g. `-lpga-lp-interop-cr--odf-`) rather than remove the product entirely, unless the actual renamed upstream job name genuinely dropped its product segment (worth confirming against `openshift/release#82653`).
- excerpt: |
    {"-lpga-lp-ocp-compat-cr--odf-", "lp-ocp-compat--odf--lpGA"},   // product-scoped
    {"-lpga-lp-interop-cr--", "lp-interop--odf--lpGA"},             // not scoped, but hardcoded to ODF

### [blocking] New view's include_variants won't scope to the new LayeredProduct value, and will likely return zero rows
- where: `config/views.yaml` (new `4.22-LP-Interop--lpGA` view), compare `4.22-LP-OCP-Compat--lpGA` in the same file
- concern: `4.22-LP-OCP-Compat--lpGA` explicitly enumerates its LP-GA layered products under `include_variants.LayeredProduct`. The new `4.22-LP-Interop--lpGA` view instead has `include_variants.LayeredProduct: []` (empty) and filters only on `Owner: [mpiit]` + `Network: [ovn]` — identical to the pre-existing `4.22-LP-Interop--lpMainline` view apart from the release window. An empty `include_variants` entry is not a no-op filter: `BuildComponentReportQuery` (`pkg/api/componentreadiness/dataprovider/bigquery/querygenerators.go:474-479`) emits `AND (jv_LayeredProduct.variant_value in UNNEST(@variantGroup_LayeredProduct))` bound to an empty array, always false in BigQuery. The Postgres provider has the same gap: `matchesIncludeVariants` (`pkg/api/componentreadiness/dataprovider/postgres/provider.go:85-95`) does `slices.Contains(allowed, val)` with no `len(allowed)==0` guard. This exact pattern exists in `4.20-LP-Interop`, `4.21-LP-Interop`, and `4.22-LP-Interop--lpMainline` already, so may be latent/pre-existing, but this PR adds a 4th instance and its own unchecked test-plan item "View renders correctly in Sippy CR dashboard" is exactly the check that would surface it.
- excerpt: |
    include_variants:
      LayeredProduct: []
      Network:
      - ovn
      Owner:
      - mpiit
- suggested fix: populate `include_variants.LayeredProduct` with the actual GA value, e.g. `["lp-interop--odf--lpGA"]`.

## Checked
- `TestVariantSyncer` new case: expected variant values traced through `setLayeredProduct`, `setJobTier` (defaults to candidate), `Owner` (`-lp-interop-` substring match) — consistent with existing logic, and consistent with the renamed test job name after the follow-up commit.
- Pattern placement in `layeredProductPatterns` list does not collide with earlier/later entries for the given test job name.
- No README/API doc updates required — `config/views.yaml` isn't documented in a README; not an API or CLI flag change.
- `gofmt` formatting looks consistent with surrounding code.

## Open questions
- Is the renamed CR job name (`-lp-interop-cr--` with no product segment) actually shared across multiple layered products in `openshift/release#82653`, or was `odf` simply dropped by mistake when fixing the trailing-dash nit?
- Have you manually loaded `4.22-LP-Interop--lpGA` in a running Sippy instance to confirm it renders data? Given the `include_variants.LayeredProduct: []` analysis above, I'd expect it to render empty.
