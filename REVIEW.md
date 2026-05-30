---
pr: openshift/sippy#3563
title: "Use the newly clustered release junit column for ~70% cost savings"
head_sha: 586e93441d2fc4dc93251638698ddbd008fd457b
base: main
reviewed_at: 2026-05-30T13:11:41Z
verdict: approve
---

## Summary

Adds early `AND release = @ReleaseFilter` predicate to `deduped_testcases_with_rownum` CTE to leverage BigQuery clustering on the `junit.release` column. Existing `jv_Release.variant_value` filter remains. Base queries always pass `BaseRelease.Name`; sample queries pass `SampleRelease.Name` unless PR/payload options are set. Parameterized, no injection risk. Redundant filter fails safe (empty results, not wrong results on mismatch).

## Findings

### [should-fix] No test coverage for non-empty releaseFilter
- where: `pkg/api/componentreadiness/dataprovider/bigquery/querygenerators_test.go:90-98`
- concern: All four test call sites pass `""`. The clause generation and parameter appending for non-empty release filter is never tested. Add a test case passing e.g. `"4.18"` and assert the query contains `AND release = @ReleaseFilter` and params include `{Name: "ReleaseFilter", Value: "4.18"}`.
- excerpt: |
    BuildComponentReportQuery(
        mockClient,
        reqOptions,
        allJobVariants,
        includeVariants,
        DefaultJunitTable,
        false,
        "",
    )

### [nit] Duplicated PR/payload guard for sample release filter
- where: `pkg/api/componentreadiness/dataprovider/bigquery/querygenerators.go:143-146` and `949-952`
- concern: Identical three-line block in `sampleQueryGenerator.QueryTestStatus` and `sampleTestDetailsQueryGenerator.QueryTestStatus`. Consistent with existing duplication patterns in the file, but a small helper would prevent drift.
- excerpt: |
    sampleReleaseFilter := s.ReqOptions.SampleRelease.Name
    if s.ReqOptions.SampleRelease.PullRequestOptions != nil || s.ReqOptions.SampleRelease.PayloadOptions != nil {
        sampleReleaseFilter = ""
    }

### [question] Semantics of junit.release vs jv_Release.variant_value
- where: `pkg/api/componentreadiness/dataprovider/bigquery/querygenerators.go:311-312`
- concern: New filter uses `release` column on `junit` table; existing filter uses `jv_Release.variant_value` from LEFT JOIN to `job_variants`. If these ever disagree for a standard CI job, the query silently returns no results for that job's tests. Need confirmation they are always identical for non-PR/non-payload jobs.

## Checked

- SQL injection safety: `@ReleaseFilter` is a parameterized query parameter
- PR/payload exclusion logic mirrors existing `@SampleRelease` skip pattern
- Base query unconditionally passes release name (always has a known release)
- Filter placed inside CTE before deduplication: optimal for partition pruning
- Redundant filter (early + variant-based) fails safe on disagreement

## Open questions

- Are `junit.release` and `jv_Release.variant_value` guaranteed identical for all standard CI jobs? Could they diverge during backfills or schema migrations?
- Was the ~70% cost savings measured on a specific query or estimated across all component readiness queries?
