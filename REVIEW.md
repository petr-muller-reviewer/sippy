---
pr: openshift/sippy#3830
title: "INTEROP-9255: Register ACM+Virt interop jobs in Sippy"
head_sha: 80787a500e216179f813c2c8894e0f1072c56f6c
base: main
reviewed_at: 2026-07-26T11:24:39Z
verdict: approve
---

## Summary
Adds two substring patterns (`-acm-cnv-`, `-acm-virt-`) to `layeredProductPatterns` in `pkg/variantregistry/ocp.go` so ACM+Virt interop jobs classify as `lp-interop--acm-virt` instead of falling into generic `virt`. Adds a new CR dashboard view `4.22-LP-Interop--lpMainline` in `config/views.yaml`. Regenerates `pkg/variantregistry/snapshot.yaml` (one job reclassified).

## Findings

### [should-fix] no dedicated test case for new patterns
- where: `pkg/variantregistry/ocp_test.go`
- concern: No `TestExtractVariants` case exercises a job name matching `-acm-cnv-` or `-acm-virt-`. Precedent is mixed (the `-coo-` pattern also lacks one, but `-virt`/`-cnv` generic and `-lpga-lp-ocp-compat-cr--cnv-` do have dedicated cases), and project conventions favor table-driven coverage for new classification logic. Would guard against future pattern-ordering regressions.
- excerpt: |
    {"-coo-", "lp-interop-coo"},
    {"-acm-cnv-", "lp-interop--acm-virt"},
    {"-acm-virt-", "lp-interop--acm-virt"},
    {"-virt", "virt"},
    {"-cnv", "virt"},

### [question] `-acm-cnv-` job not yet present in snapshot.yaml
- where: `pkg/variantregistry/snapshot.yaml`
- concern: Only the live-migration job (`...-acm-virt-ocp4.22-p2p-cclm-liv-mig-lp-interop-aws`) was reclassified; the P2P upgrade job (`...-acm-cnv-ocp-4.22-p2p-lp-interop-aws` per PR description) doesn't appear in the snapshot at all. Likely just means that job hasn't run/synced yet, but worth confirming the actual job name matches the pattern once it exists.
- excerpt: |
    periodic-ci-RedHatQE-interop-testing-master-acm-virt-ocp4.22-p2p-cclm-liv-mig-lp-interop-aws:
        LayeredProduct: lp-interop--acm-virt

### [question] `LayeredProduct: []` (match-any) filter in new view
- where: `config/views.yaml:1645-1689` (`4.22-LP-Interop--lpMainline`)
- concern: Sibling views (e.g. `4.22-LP-OCP-Compat--lpMainline`) filter `include_variants.LayeredProduct` to a specific product value; this new view uses `[]` (any), scoped down only by `Owner: mpiit` + `Network: ovn`. Intentional per PR description (aggregate LP-Interop view), but will silently pull in any future `mpiit`-owned lp-interop product added under `ovn`, not just ACM+Virt.
- excerpt: |
    include_variants:
      LayeredProduct: []
      Network:
      - ovn
      Owner:
      - mpiit

### [nit] PR carries `do-not-merge/work-in-progress` label
- where: PR metadata
- concern: Flag before merge; the label suggests this isn't ready yet, and the PR description references two companion PRs (openshift/release#82108, ci-test-mapping#786) whose merge state should be confirmed to sync with this change.
- excerpt: |
    labels: do-not-merge/work-in-progress, jira/valid-reference

## Checked
- Pattern ordering: `-acm-cnv-`/`-acm-virt-` precede generic `-virt`/`-cnv` entries in the ordered slice, and `setLayeredProduct` returns on first match, so no risk of falling back to generic `virt`.
- Both target job names (from PR description) contain the new substrings and also contain `-lp-interop-`, so `setOwner` still correctly assigns `Owner: mpiit`.
- New view's YAML structure (base/sample release windows, `advanced_options`, `regression_tracking: enabled: true`, `prime_cache: enabled: false`) matches sibling `4.22-LP-*` views.
- No gofmt/lint issues visible in the diff.

## Open questions
- Has the second job (`-acm-cnv-` / P2P upgrade) actually started reporting yet, or is its absence from `snapshot.yaml` expected?
- Is the `LayeredProduct: []` match-any filter on the new view intentional as a durable "catch all mpiit lp-interop" view, or should it be scoped to `lp-interop--acm-virt` (and `lp-interop-coo`) explicitly?
- Are openshift/release#82108 and ci-test-mapping#786 merged/ready, since this PR's classification is inert until those land?
