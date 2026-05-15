---
pr: 3515
title: "Fix AI instruction quality and bump apm-cli to 0.13.0"
head_sha: 487ddddcc652e1b3f9c8cb517eb4d5f50e506b65
base: master
reviewed_at: "2026-05-15T11:21:45Z"
verdict: approve-with-comments
refresh_log:
  - old_sha: 4766bb4e3
    new_sha: 487ddddcc652e1b3f9c8cb517eb4d5f50e506b65
    summary: "apm-cli bumped again 0.12.4→0.13.0; regenerated version strings in 8 AGENTS/CLAUDE.md files; CI passed"
---

# PR #3515: Fix AI instruction quality and bump apm-cli to 0.13.0

**Author:** not-stbenjam | **+617 / -84 (original) +16 / -16 (refresh)** | No code changes | **Verdict:** Approve with comments

## Overview

Since previous review (2026-05-15): apm-cli was bumped again from 0.12.4 to 0.13.0 in a single additional commit, regenerating version strings in `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, and their `mcp/`/`sippy-ng/` counterparts. No instruction content changed. CI passed.

Three categories of changes (original):

1. **Instruction quality fixes** from [skillsaw](https://github.com/stbenjam/skillsaw) linter: removes hedging language, repositions critical instructions to document boundaries
2. **apm-cli 0.11.0 → 0.12.4**: new config format (`target` → `targets`, explicit `dependencies` lists), recompiled all generated output
3. **New `.cursor/commands/`**: APM now deploys slash commands to Cursor (previously only Claude/OpenCode/Gemini)

~75% of the diff is recompiled generated output; ~15% is instruction edits in source prompts; ~10% is APM config/tooling.

## Change Map

| Category | Files | Nature |
|---|---|---|
| Instruction quality | `.apm/prompts/sippy-generate-release-views.prompt.md`, `.apm/prompts/sippy-update-ga-release-views.prompt.md`, `.apm/prompts/sippy-update-job-variant.prompt.md`, `.coderabbit.yaml` | Manual edits — source of truth |
| APM tooling | `apm.yml`, `Makefile` | Version bump + config format migration |
| Generated output | `.claude/commands/*`, `.cursor/commands/*` (new), `.opencode/commands/*`, `.gemini/commands/*`, `CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, `mcp/CLAUDE.md`, `mcp/AGENTS.md`, `sippy-ng/CLAUDE.md`, `sippy-ng/AGENTS.md`, `apm.lock.yaml` | Recompiled from source — `make verify-apm` |

## Instruction Changes — What Moved Where

### sippy-generate-release-views

The ga→now replacement rule was promoted to a blockquote at the top of the document. The two mid-document copies were softened (removed `IMPORTANT`/`CRITICAL` markers) to avoid redundancy.

### sippy-update-ga-release-views

YAML formatting instruction moved from mid-document (between step F and step 4) to the very end.

### sippy-update-job-variant

"Pattern Ordering is CRITICAL" section moved from mid-document (after "Important Notes") to end (after "Helper Commands"). Inline step 4 softened.

## Findings

### [question] .coderabbit.yaml — hedging removal may be too aggressive

Two changes turn soft guidelines into hard requirements that CodeRabbit will enforce on every PR:

- `"should ideally stay under 200 lines"` → `"must stay under 200 lines"` — Some query-building functions may legitimately need more than 200 lines. With "must", CodeRabbit will flag all of them as violations rather than suggestions.
- `"should include test coverage where possible"` → `"must include test coverage"` — The removed qualifier "where possible" was doing useful work — config-only changes, documentation PRs, and view YAML updates don't need tests. With "must", CodeRabbit will flag those PRs too.

Is the intent to make CodeRabbit noisier here, or was this an overzealous lint fix? Consider keeping "should" (without "ideally") as a middle ground.

### [nit] Missing trailing newlines

Multiple files end without a trailing newline (`\ No newline at end of file` in the diff). Affects both new `.cursor/commands/` files and modified files like `sippy-update-ga-release-views.md` and `sippy-update-job-variant.md`. Some are pre-existing; others are new.

### [nit] sippy-update-job-variant: softened inline may read as informational

The inline step 4 now says *"More specific patterns come before more generic patterns"* — this reads as a description of the current state rather than a requirement. The emphatic version is preserved in the end-of-file section, so an LLM will likely still follow it. But lowercase "must" in the inline version would be unambiguous without being noisy.

## What Looks Good

- **Instruction repositioning is well-reasoned and consistent.** Critical instructions are placed at document boundaries (top blockquote or bottom section) where LLMs attend most. Mid-document copies are softened to avoid redundancy rather than deleted.
- **Cross-editor consistency.** All changes propagate identically across `.claude/`, `.cursor/`, `.opencode/`, `.gemini/`, and `.apm/` source prompts. The new `.cursor/commands/` directory brings Cursor to parity.
- **APM version bump is clean.** Config format migration is straightforward. Lock file hashes updated. Build IDs are no longer `__BUILD_ID__` placeholders.
- **No code changes.** Zero risk of runtime regression.

## Verdict

**Approve with comments.** Clean PR with no code changes and a well-reasoned approach to instruction positioning. The `.coderabbit.yaml` hedging removal is the only item worth discussing — it will make CodeRabbit reviews noisier for legitimate cases. The other findings are minor nits.

| Finding | Severity | Action needed? |
|---|---|---|
| .coderabbit.yaml hedging removal | question | Discuss: intentional or overzealous? |
| Missing trailing newlines | nit | Optional, partially pre-existing |
| Softened inline pattern ordering | nit | Optional, end-of-file version compensates |
