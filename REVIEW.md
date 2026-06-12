---
pr: openshift/sippy#3610
title: "add feature docs for symptoms"
head_sha: 3c9e831e63d858f7ba40f43f4b5860f294066a21
base: main
reviewed_at: 2026-06-12T15:14:20Z
verdict: approve
---

## Findings

### [question] GEMINI.md not regenerated
- where: repo root
- concern: `.apm/instructions/docs.instructions.md` changed and both `CLAUDE.md` and `AGENTS.md` were regenerated, but `GEMINI.md` is absent from the diff. If it derives from the same APM source, `make verify-apm` may fail in CI.

### [nit] CodeRabbit brace-expansion glob may not match
- where: `.coderabbit.yaml:59`
- concern: The path `pkg/**/jobrun{scan,annotator}/**` uses shell-style brace expansion. CodeRabbit uses minimatch/micromatch-style globs which do support braces, so this should work, but it's worth confirming — if not supported, this silently matches nothing and the two paths need separate entries.
- excerpt: |
    - path: "pkg/**/jobrun{scan,annotator}/**"

### [nit] Parenthetical about JobRunAnnotator may go stale
- where: `docs/features/job-analysis-symptoms.md:128`
- concern: The doc says JobRunAnnotator "can add labels but doesn't (yet) know about symptoms." This is accurate today but if the annotator gains symptom awareness via the cloud function integration, this parenthetical will become misleading. Consider removing the "(yet)" qualifier or phrasing it as current-state without implying future intent.
- excerpt: |
    | `pkg/componentreadiness/jobrunannotator/jobrunannotator.go` | `JobRunAnnotator` - the `annotate-job-runs` tool which can add labels but doesn't (yet) know about symptoms. |

## Checked
- All 15 code paths in the feature doc exist at the claimed locations.
- All named symbols (`GatherLabelsFromBQ`, `SyncTriageSymptoms`, `JobRunAnnotator`, `WriteHTMLSummaryToBucket`, `ContentMatcher`) verified at documented files.
- Generated `CLAUDE.md` and `AGENTS.md` diffs are build-ID + propagated instruction only, consistent with the APM source change.
- Feature doc data flow description (definition -> cloud function -> BQ -> prow loader -> postgres -> UI) matches the actual code structure.
- CodeRabbit "Feature Documentation" pre-merge check is correctly placed and uses `warning` mode (not `error`).

## Open questions
- Was `GEMINI.md` intentionally left out, or does it need regeneration from the same APM source?
