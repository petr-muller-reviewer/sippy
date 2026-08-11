---
pr: openshift/sippy#3894
title: "Add GCD (Google Dedicated Cloud) Variant"
head_sha: 37b8bf28261a5f1dd4eaf13d8e52d081c363a166
base: main
reviewed_at: 2026-08-11T08:42:45Z
verdict: request-changes
---

## Summary

One-line diff in `pkg/variantregistry/ocp.go`: adds `{"-gcd-", "gcd"}` to the ordered `platformPatterns` table in `setPlatform`, so job names containing `-gcd-` are classified with platform variant `gcd` (Google Dedicated Cloud, Google's sovereign cloud offering). No other files changed.

## Findings

### [should-fix] pattern requires trailing dash unlike sibling entries
- where: `pkg/variantregistry/ocp.go:1145`
- concern: Every other entry in `platformPatterns` (`-gcp`, `-aws`, `-azure`, `-osd-ccs-gcp`, etc.) matches via `strings.Contains(jobNameLower, entry.substring)` (ocp.go:1163) against a bare leading-dash substring, so they match regardless of what follows. The new entry is `-gcd-` (both leading and trailing dash), so a job name that ends with `...-gcd` (no further suffix) or is followed by a non-dash separator will not match, silently falling through to "unable to determine platform" instead of being classified as `gcd`.
- excerpt: |
    {"-osd-ccs-gcp", "osd-gcp"},
    {"-gcd-", "gcd"},
    {"-gcp", "gcp"},

### [should-fix] no test coverage for the new platform pattern
- where: `pkg/variantregistry/ocp_test.go`
- concern: `TestVariantSyncer`'s table has a case for every other platform pattern in the table (e.g. `osd-ccs-gcp` at line 279), but none for `gcd`. Without one, the trailing-dash matching issue above (or any future reordering of the pattern table) would go undetected.
- excerpt: |
    job: "periodic-ci-openshift-release-master-nightly-4.19-e2e-osd-ccs-gcp",

## Checked
- Placement in `platformPatterns` (ocp.go:1145): correctly ordered before the generic `-gcp` entry, so GCD jobs won't be misclassified as plain GCP first.
- Matching mechanism confirmed via `strings.Contains(jobNameLower, entry.substring)` at ocp.go:1163.
- No other files in the diff (base fda66a324 vs head 37b8bf282 for `pkg/variantregistry/`) — this is a true one-line change.

## Open questions
- What do real GCD prow job names look like? If they never appear as the final path segment (always followed by a region/suffix after `-gcd-`), the trailing-dash requirement may be harmless in practice — but it's inconsistent with every sibling pattern and worth confirming intentionally rather than by luck.
