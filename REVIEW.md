---
pr: openshift/sippy#3865
title: "INTEROP-9317: Add OPP LP interop variant and CR view for OCP 4.22"
head_sha: a961047a0fc39e4ba96ce81f309fe4bd1c875529
base: main
reviewed_at: 2026-08-05T22:06:49Z
verdict: approve
---

## Summary

Adds `LayeredProduct` variant `lp-interop--OPP` (OPP multi-LP interop jobs: ACM + ODF + Quay), a matching component-readiness view `4.22-LP-Interop--OPP`, and unit test coverage for job-name classification. Structurally a clone of the existing `4.22-LP-Interop--lpMainline` pattern.

## Findings

### [nit] Inconsistent casing for the new LayeredProduct value
- where: `pkg/variantregistry/ocp.go:1374`
- concern: Every other entry in this pattern list is lowercase (`lp-interop-coo`, `lp-interop--acm-virt`, `lp-ocp-compat--odf--lpGA`), but this one uses `lp-interop--OPP`. Harmless functionally since matching runs on `strings.ToLower(jobName)` and the value is opaque, but it breaks the established lowercase convention for variant values propagated into `config/views.yaml` and `snapshot.yaml`.
- excerpt: |
    {"-coo-", "lp-interop-coo"},
    {"-acm-cnv-", "lp-interop--acm-virt"},
    {"-acm-virt-", "lp-interop--acm-virt"},
    {"-interop-opp-", "lp-interop--OPP"},

### [nit] Test job names don't match real job name shape
- where: `pkg/variantregistry/ocp_test.go:1802,1831`
- concern: Test fixtures use `...ocp4.22-lp-interop-opp-aws` / `...vsphere`, but the real jobs in `snapshot.yaml` are named `...ocp4.22-interop-opp-aws` (no `lp-` segment). Both match the `-interop-opp-` substring so the tests still exercise the correct code path, but the fixture doesn't mirror production job naming.
- excerpt: |
    job: "periodic-ci-stolostron-policy-collection-main-ocp4.22-lp-interop-opp-aws"

## Checked
- Pattern placement in `setLayeredProduct` (ocp.go:1371-1374): specific substring, no collision risk with other `lp-interop-*`/`-virt`/`-cnv` entries, correctly ordered before generic fallbacks.
- `snapshot.yaml` diff: exactly the 5 real affected jobs (4.22 and 5.0 releases, aws/vsphere/upgrade) flip `LayeredProduct: none → lp-interop--OPP`.
- `JobTier: informing → candidate` downgrade in snapshot is expected/automatic via `adjustJobTierBasedOnView` (ocp.go:379) since the new LP value isn't in the main release view's `include_variants` — consistent with prior LP additions, not a manual/separate change.
- `config/views.yaml` new view (`4.22-LP-Interop--OPP`) is a structural clone of `4.22-LP-Interop--lpMainline` (same column_group_by/db_group_by/advanced_options), only `include_variants.LayeredProduct` differs.
- No other files reference layered-product identifiers (`lp-interop-coo`, `lp-interop--acm-virt`, `lp-ocp-compat--acs`) outside the four changed files, so no missed touch points.
- Two new table-driven test cases (aws, vsphere) cover full expected variant maps including the new field.

## Open questions
- Was `lp-interop--OPP` casing intentional (branding/acronym), or should it be `lp-interop--opp` for consistency with sibling values?
