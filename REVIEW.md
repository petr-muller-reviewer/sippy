---
pr: openshift/sippy#3819
title: "SPLAT-2814: Set Nutanix upgrade job to candidate tier"
head_sha: 48049c693053e3dbc7ad6a4d11bf002472ecbdea
base: main
reviewed_at: 2026-07-29T11:51:49Z
verdict: approve
refresh_log:
  - old_sha: 0166dc2049c18bb10101dc4bb2da6c8fb3e32939
    new_sha: 48049c693053e3dbc7ad6a4d11bf002472ecbdea
    summary: "PR rebased onto main; PR's own code unchanged. Tests passed, /lgtm added by vr4manta, still awaiting approval."
  - old_sha: 48049c693053e3dbc7ad6a4d11bf002472ecbdea
    new_sha: 48049c693053e3dbc7ad6a4d11bf002472ecbdea
    summary: "No code change. Author assigned dgoodwin for approval."
---

## Summary

Adds one entry to the `jobTierPatterns` table in `setJobTier` (pkg/variantregistry/ocp.go:855), mapping jobs whose name contains `-e2e-nutanix-upgrade` to `candidate` tier. Comment cites CSI operator conformance failures as the instability reason. 3 lines added, 1 file changed.

## Findings

### [question] confirm exact substring matches real job name
- where: `pkg/variantregistry/ocp.go:855`
- concern: `strings.Contains` match on `-e2e-nutanix-upgrade` (lowercased). No job name containing this exact substring was found in `pkg/variantregistry/snapshot.yaml` (test fixture) — only unrelated `nutanix` platform jobs (agent-based QE jobs, perfscale jobs). A silent no-op typo here would not fail CI, just quietly not classify the intended job.
- excerpt: |
    // Nutanix upgrade job not yet stable due to CSI operator conformance failures
    {[]string{"-e2e-nutanix-upgrade"}, "candidate"},

### [question] no dedicated test for this entry
- where: `pkg/variantregistry/ocp.go:855`
- concern: No unit test asserts a job containing `-e2e-nutanix-upgrade` resolves to `candidate`. Consistent with sibling entries in this table (e.g. `-hybrid-env`, `-vsphere-host-groups` also untested individually), so likely not a PR-specific gap, just worth a quick confirmation from the author that the real job name was checked against a live snapshot/config rather than assumed.

## Since previous review (2026-07-29T00:44:12Z)
- No code change (head still `48049c693`).
- Author (nischawl) assigned dgoodwin for approval (2026-07-29T08:05:19Z).
- PR still OPEN, NOT APPROVED.

## Since previous review (2026-07-25)
- PR rebased onto main (new SHA `48049c693`); the PR's own change to `pkg/variantregistry/ocp.go` is identical.
- CI tests passed (2026-07-27).
- `/lgtm` added by vr4manta (2026-07-27). PR is NOT APPROVED yet (needs an approver from OWNERS).
- No inline review comments or formal reviews submitted.

## Checked
- Placement and style consistent with existing `jobTierPatterns` entries (comment + single substring + tier).
- No BigQuery/Postgres parity concern — `setJobTier` runs uniformly regardless of data backend.
- Change is additive/narrow; a non-matching substring would be a silent no-op, not a regression of other jobs.
- PR already carries `lgtm` and `ready-for-human-review` labels.

## Open questions
- Can you confirm `-e2e-nutanix-upgrade` is the literal substring present in the actual Prow job name (not e.g. `-nutanix-e2e-upgrade-` or similar)?
