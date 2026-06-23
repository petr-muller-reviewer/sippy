---
pr: openshift/sippy#3666
title: "ROSAENG-326: Add operator e2e jobs and separate rosa-integration from rosa-stage"
head_sha: 9581923514845fc26cc10eedfb4d33cd019c6608
base: main
reviewed_at: 2026-06-23T16:30:13Z
verdict: request-changes
refresh_log:
  - from_sha: 9581923514845fc26cc10eedfb4d33cd019c6608
    to_sha: 9581923514845fc26cc10eedfb4d33cd019c6608
    at: 2026-06-23T16:30:13Z
    summary: "No code changes; smg247 LGTM'd at 16:19Z, bot marked APPROVED at 16:21Z; open findings unaddressed"
---

## Summary

Replaces catch-all `^periodic-ci-openshift-online-rosa-e2e-main-.*` in rosa-stage with specific patterns, and populates the previously-empty rosa-integration view with two regexp patterns. Single file change in `config/openshift-customizations.yaml`.

## Findings

### [should-fix] ocm-fvt production job dropped from all views
- where: `config/openshift-customizations.yaml:72`
- concern: The job `periodic-ci-openshift-online-rosa-e2e-main-ocm-fvt-rosa-hcp-production-ocm-fvt-periodic-cs-rosa-hcp-ad-production-main` exists in `config/openshift.yaml` and was previously matched by the old catch-all. New patterns cover `*-staging-*` (rosa-stage) and `*-integration-*` (rosa-integration) but nothing for `*-production-*`. This job will now fall into no view.
- excerpt: |
    - "^periodic-ci-openshift-online-rosa-e2e-main-ocm-fvt-.*-staging-.*"

### [question] daily CI status bot mentioned but not covered
- where: `config/openshift-customizations.yaml:68`
- concern: The PR description lists "Daily CI status bot" as a category that should remain in rosa-stage, but none of the three new `rosa-e2e-main-*` patterns (`periodics-*`, `upgrade-*`, `ocm-fvt-*-staging-*`) would cover a daily/status-type job. If such a job exists under that prefix, it is now untracked. Needs confirmation from author whether this job exists and whether it was already covered by a separate pattern.

## Checked
- `-master-` branch literal in operator e2e patterns: verified against `config/openshift.yaml` — the actual job is `periodic-ci-openshift-route-monitor-operator-master-rosa-sts-e2e-promotion-stage`, so `-master-` is correct
- Removing `jobs: {}` from rosa-integration: Go nil-map reads/ranges are safe; all callers in `pkg/variantregistry/` handle nil maps correctly
- rosa-integration database registration: `rosa-integration` was introduced as a registered placeholder in a prior commit; no new TRT ticket needed for this PR
- Pattern structure and conventions: consistent with established `aro-integration`/`aro-stage` pairing pattern; regexp-only sections (no `jobs:` key) match `Presubmits` and `ocp-hypershift` precedent

## Open questions
- Does the `ocm-fvt-*-production-*` job belong in rosa-stage or rosa-production? If rosa-production, add a regexp there; if rosa-stage, add a `*-production-*` pattern.
- Does the "Daily CI status bot" job actually exist under `periodic-ci-openshift-online-rosa-e2e-main-*`? If so, which suffix does it use?

## Activity since initial review
- 2026-06-23T16:19Z: smg247 posted `/lgtm`
- 2026-06-23T16:21Z: openshift-ci[bot] marked PR **APPROVED** (author self-approved + smg247 LGTM)
- No code changes; open findings not addressed
