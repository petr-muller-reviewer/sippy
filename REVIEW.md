---
pr: openshift/sippy#3823
title: "wip: add support for parsing JUnit testcase lifecycle"
head_sha: 3633b44cea75c06d09ad89c213e56571eed1c141
base: main
reviewed_at: 2026-07-25T13:51:40Z
verdict: needs-discussion
---

## Summary

Adds a `Lifecycle` XML attribute to `junit.TestCase` and plumbs it into `prowloader`'s `TestCaseEntry` during `extractTestCases`. Two new table-driven test cases cover pass/fail lifecycle passthrough. PR carries `do-not-merge/work-in-progress` label; author's own title marks it `wip:`.

## Findings

### [should-fix] Lifecycle dropped in flake-merge branch
- where: `pkg/dataloader/prowloader/prow.go:1721-1726`
- concern: When a test appears twice with differing pass/fail status (the `existing` branch), only `Output` is backfilled onto the existing entry; `Lifecycle` is never merged/updated. If the first-seen run lacked lifecycle data (or had different lifecycle data) than a later run, information is silently lost.
- excerpt: |
    } else if (existing.Status == int(sippyprocessingv1.TestStatusFailure) && status == sippyprocessingv1.TestStatusSuccess) ||
        (existing.Status == int(sippyprocessingv1.TestStatusSuccess) && status == sippyprocessingv1.TestStatusFailure) {
        existing.Status = int(sippyprocessingv1.TestStatusFlake)
        if existing.Output == nil {
            existing.Output = output
        }
    }

### [should-fix] Lifecycle captured but never consumed downstream
- where: `pkg/dataloader/prowloader/testconversion/testconversion.go`, `pkg/dataloader/prowloader/prow.go` (results-building loop, ~line 1680)
- concern: `TestCaseEntry.Lifecycle` is set but not read by `testsToRawJobRunResult`, `ConvertProwJobRunToSyntheticTests`, or the loop that builds the final `RawJobRunResult`. As it stands the field has no observable effect on Sippy output — consistent with the WIP label, but worth confirming this is tracked before the label is removed.
- excerpt: |
    (no excerpt — absence of a read site is the finding)

### [question] BigQuery parity
- where: n/a (dataloader/postgres path only)
- concern: Project convention requires parity between BigQuery and PostgreSQL/dataloader providers for query-logic/data changes. If `Lifecycle` is meant to surface in query results or the frontend, does the BigQuery loading path need the same field? May be intentionally deferred given WIP status.
- excerpt: |
    n/a

### [question] Suite-level lifecycle attribute?
- where: `pkg/apis/junit/types.go:58-73`
- concern: Only `TestCase` gets the new attribute. Confirm whether `lifecycle` can also appear on `<testsuite>` elements in the openshift-tests-private JUnit schema this models, or if it is testcase-only by design.
- excerpt: |
    n/a

## Checked
- `TestCase.Lifecycle` addition is a purely additive struct field; XML unmarshal is tolerant of the missing attribute on older reports (zero-value empty string), no behavior change for existing consumers.
- New test cases in `extract_test_cases_test.go` correctly exercise pass and fail-with-output paths with lifecycle set.
- No security concerns; no user input or injection surface touched.

## Open questions
- Is preserving/merging `Lifecycle` across the flake-merge branch planned in a follow-up commit within this same PR?
- Where will `Lifecycle` ultimately be surfaced (API response, frontend, BigQuery), and is that tracked separately or expected here before the WIP label comes off?
- Does the JUnit schema being modeled ever place `lifecycle` on the suite element rather than (or in addition to) the testcase element?
