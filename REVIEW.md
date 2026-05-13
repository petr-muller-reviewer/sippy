---
pr: openshift/sippy#3528
title: "ROSAENG-1083: remove unrelated jobs from 4.22 section"
head_sha: 05ace06cf612a72a9b7c289c28d76fae04820e91
base: main
reviewed_at: 2026-05-13T22:14:22+02:00
verdict: approve
---

## Findings

No blocking, should-fix, or nit-level findings.

### [question] Were these jobs ever intentionally in 4.22?
- where: `config/openshift.yaml:12583-12614` (pre-patch)
- concern: The removed nightly jobs span versions 4.19 through 5.0, not just 4.22. Were these added intentionally at some point (e.g. to get ROSA visibility within 4.22 dashboards) or was it always a mistake? Matters only for historical context.

## Checked
- All 31 removed jobs match the `rosa-stage` regexp `^periodic-ci-openshift-online-rosa-e2e-main-.*` (line 17163), so none lose tracking.
- The `rosa-stage` section (lines 17159-17165) has no version-specific filter that would exclude any of these jobs.
- No other sections in the config reference these exact job names (no orphaned cross-references).
- The three job categories (OCM FVT staging, ROSA nightly, daily status) are all ROSA-specific, not OCP 4.22 release tests.
- Surrounding jobs in the 4.22 section (OLS load generator, openshift-tests-private) are unaffected.

## Open questions
- Were these jobs producing any 4.22-specific dashboard widgets that will now disappear from the 4.22 view? If so, is that the desired outcome?
