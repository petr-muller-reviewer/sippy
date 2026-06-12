---
pr: openshift/sippy#3613
title: "NO-JIRA: Change hypershift azure v2 job to standard"
head_sha: fad7e33208712b3f93261c4c1abc633c8d699ae0
base: main
reviewed_at: 2026-06-12T10:42:07Z
verdict: approve
---

## Findings

No blocking, should-fix, nit, or question findings.

## Checked
- Pattern rule placement: new `standard` rule for `*-e2e-azure-v2-self-managed` is inserted before the Hypershift catch-all `candidate` rule, so it takes precedence correctly.
- Pattern matching: the two-element prefix/suffix pattern `{"periodic-ci-openshift-hypershift-", "-e2e-azure-v2-self-managed"}` matches the job names in the snapshot (`periodic-ci-openshift-hypershift-release-4.23-periodics-e2e-azure-v2-self-managed` and `periodic-ci-openshift-hypershift-release-5.0-periodics-e2e-azure-v2-self-managed`).
- Test case: new test in `ocp_test.go` validates the full variant map for the 5.0 job, including `VariantJobTier: "standard"`. All expected variant values are consistent with the existing Hypershift Azure test cases.
- Snapshot consistency: both 4.23 and 5.0 entries in `snapshot.yaml` updated from `candidate` to `standard`, matching the code change.
- No other files or logic affected.

## Open questions
- None.
