---
pr: openshift/sippy#3556
title: "WIP: junit clustering"
head_sha: 5d3941ab6fce1b3983714b6096988f96627585c0
base: main
reviewed_at: 2026-05-26T23:59:33Z
verdict: approve
---

## Summary

Documentation-only PR adding a proposal for clustering the `ci_analysis_us.junit` BigQuery table by a new `release` column. Goal: reduce BigQuery scan costs from ~$36K/month to ~$9-14K/month via block-level pruning on release-filtered queries. Single new file in `docs/plans/`. No code changes.

## Findings

### [should-fix] Ingestion fallback to `branch` perpetuates mislabeling
- where: `docs/plans/bigquery-junit-clustering-proposal.md:289`
- concern: The proposed ingestion code falls back to the regex-derived `branch` value for jobs not in the variant registry. This silently writes a potentially wrong `release` value into the clustered column. Using `NULL` instead would honestly mark unknown jobs and avoid clustering them into the wrong release block.
- excerpt: |
    release=get_release_for_job(self.prowjob_name) or self.branch,  # new field

### [should-fix] Ingestion gap during CTAS rebuild not mitigated
- where: `docs/plans/bigquery-junit-clustering-proposal.md:198-207`
- concern: The proposal acknowledges 10-30 minutes of permanently lost junit data during the CTAS rebuild but doesn't explore narrowing the window. For a $264K+/year savings initiative, the runbook should describe whether the Cloud Function trigger can be paused/resumed around the rename, or whether a two-phase swap (CTAS to `junit_v2`, atomic reader switch) could reduce data loss.
- excerpt: |
    Any junit data from Prow jobs that complete during this window will be permanently lost —
    the ingestion pipeline does not replay missed data.

### [question] Multi-release query savings estimate
- where: `docs/plans/bigquery-junit-clustering-proposal.md:155-167`
- concern: Component Readiness compares base vs sample release. The querygenerators.go code uses separate CTEs for each, so clustering savings likely apply independently to each scan. The proposal should confirm this explicitly, since an `IN (@BaseRelease, @SampleRelease)` pattern would yield different savings than two independent scans.

### [nit] Cache staleness window not in risk table
- where: `docs/plans/bigquery-junit-clustering-proposal.md:319-327`
- concern: The 6-hour cache TTL means newly registered jobs produce rows with fallback `release` values for up to 6 hours. Worth a line in the Risk Assessment table for completeness.

### [nit] update_junit_release.py referenced but not listed as deliverable
- where: `docs/plans/bigquery-junit-clustering-proposal.md:329-350`
- concern: The correction script is described in detail but not listed in the Implementation Sequence as an explicit deliverable.

## Checked
- Proposal's claim about release filtering via JOIN to `job_variants` matches querygenerators.go (lines 79, 151, 388-389, 553-554)
- `@BaseRelease` and `@SampleRelease` parameters exist and are used as described
- `branch` column exists in junit data structures and is regex-derived
- `docs/plans/` directory already exists with other planning documents
- No code changes, no risk to existing functionality from merging this PR

## Open questions
- For jobs not in the variant registry, should `release` be `NULL` rather than falling back to the regex-derived `branch`? This avoids silently mislabeling rows in the clustered column.
- Has anyone estimated the actual ingestion gap during the CTAS rebuild window? Could the Cloud Function trigger be paused/resumed to bound it?
- Do Component Readiness queries scan the junit table once with both releases or twice independently? This affects the savings estimate.
