---
pr: openshift/sippy#3596
title: "Trt 1989 post migration"
head_sha: ba9cdde564f31280f552a65e2e3426e364d61045
base: main
reviewed_at: 2026-06-09T19:17:43Z
verdict: approve
---

## Summary

- Reverts load timeout from 14h (temporary migration bump) to standard 4h.
- Partition creation failure now aborts the load instead of warn-and-continue into DEFAULT partition.
- Comment clarification in `pkg/db/db.go` for migration ordering.

## Findings

### [question] Asymmetry between partition creation and cleanup error handling
- where: `pkg/dataloader/prowloader/prow.go:239-249`
- concern: Partition creation failure now aborts (correct), but cleanup failure at lines 245-249 still warn-and-continues. Likely intentional since stale partitions don't affect correctness, but worth confirming not an oversight.
- excerpt: |
    if err := pl.ensurePartitions(); err != nil {
        pl.errors = append(pl.errors, errors.Wrap(err, "failed to ensure partitions"))
        return
    }
    // vs cleanup:
    if detached, dropped, err := pl.dbc.CleanupPartitions(false); err != nil {
        log.WithError(err).Warning("failed to cleanup old partitions, continuing with load")
    }

## Checked
- Error appended to `pl.errors` matches pattern used by other fatal errors in `Load()` (lines 228, 233).
- 4h timeout matches pre-migration value; no code depends on extended 14h.
- Comment rewrite in `db.go` is correct and clearer.
- No test changes needed: configuration/control-flow only.

## Open questions
- Is the creation-fatal / cleanup-nonfatal asymmetry intentional, or should cleanup also abort?
