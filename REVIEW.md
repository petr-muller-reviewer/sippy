---
pr: openshift/sippy#3533
title: "TRT-1989: add benchmarking db query tests"
head_sha: 2408ed2f18d6bbd96232d50b6fe8e312a6de7601
base: master
reviewed_at: 2026-05-18T18:23:50Z
verdict: approve
refresh_log:
  - from: 7eaee42187506fff3fa2ba91962d966a7ddc0cb9
    to: fe42ba08360d9a2bb2c959421c562010a5e1b943
    summary: "3 commits, pkg/flags/postgres_benchmarking_test.go only (+254/-25); added 7 new benchmark cases, benchmarkResult struct, printSummaryTable, runBenchmarkCase helper; iterations bumped to 3; new Test_BenchmarkFindTestsByRelease and Test_BenchmarkCombined tests"
  - from: fe42ba08360d9a2bb2c959421c562010a5e1b943
    to: 2408ed2f18d6bbd96232d50b6fe8e312a6de7601
    summary: "1 commit (+24/-9); DB client cleanup via t.Cleanup; asOf timestamp parameter added to getBenchmarkCases; map lookup guarded in Test_BenchmarkFindTestsByRelease; petr-muller approved with /hold"
---

## Since previous review (7eaee42 → fe42ba08)

- Added `benchmarkResult` struct with min/max/avg/total tracking, `runBenchmarkCase` helper, and `printSummaryTable` — eliminates duplicate timing boilerplate; partially addresses the "custom timing framework" finding.
- `iterations` bumped from 1 to 3 in `Test_BenchmarkIndividual` and new `Test_BenchmarkCombined`.
- Added 7 new benchmark cases: `TestAnalysisOverall`, `TestAnalysisByJob`, `TestAnalysisByJobWithVariantFilter`, `TestCountsByLookback14/9`, `TestCountsByLookback14/9ForRelease`, `TestAnalysisPassRate`.
- `FindTestsByRelease` extracted to `getIndividualBenchmarkCases()` returning a map; `Test_BenchmarkFindTestsByRelease` exercises it alone.
- `benchmarkJobName` constant added (job name previously inline in `JobDetails` case).

## Since previous review (fe42ba08 → 2408ed2f)

- `getBenchmarkCases` now takes an `asOf time.Time` parameter; callers capture `time.Now().UTC()` once and pass it in — fixes `time.Now()` drift across iterations (CodeRabbit's "stabilize benchmark time windows" nit).
- `getBenchmarkDBClient` now registers `t.Cleanup` to close the DB connection — resolves "DB client never closed" finding.
- `Test_BenchmarkFindTestsByRelease` now uses comma-ok guard on map lookup — resolves "direct map index can panic" finding.
- petr-muller approved the PR and added `/hold` with a comment pointing to Go's built-in benchmarking.

## Findings

### [resolved] Custom timing framework — sufficiently improved
- where: `pkg/flags/postgres_benchmarking_test.go`
- concern: Original finding: duplicated timing with `fmt.Printf`, iterations hardcoded to 1. New code adds `benchmarkResult` with min/max/avg/total, `runBenchmarkCase` helper, `printSummaryTable`, and bumps iterations to 3. Still not `testing.B` / `benchstat`, but the custom framework is now genuinely useful.

### [resolved] DB client never closed
- where: `pkg/flags/postgres_benchmarking_test.go` — `getBenchmarkDBClient`
- concern: Fixed in 2408ed2f. `t.Cleanup` now closes the `sql.DB` handle.

### [resolved] Direct map index into getIndividualBenchmarkCases() can panic
- where: `pkg/flags/postgres_benchmarking_test.go` — `Test_BenchmarkFindTestsByRelease`
- concern: Fixed in 2408ed2f. Comma-ok check + `t.Fatal` on miss.

### [should-fix] Hardcoded release and test name constants will rot
- where: `pkg/flags/postgres_benchmarking_test.go:15-17`
- concern: `benchmarkRelease = "4.22"`, `benchmarkTestName`, and `benchmarkJobName` are hardcoded. These will become stale as releases advance. Make them configurable via env var (like `db_benchmarking_dsn`) or document that they need periodic updates.
- excerpt: |
    const benchmarkRelease = "4.22"
    const benchmarkTestName = "[Monitor:legacy-test-framework-invariants-pathological][sig-arch] events should not repeat pathologically for ns/kube-system"
    const benchmarkJobName = "periodic-ci-openshift-release-main-ci-4.22-e2e-aws-ovn"

### [nit] Multiple benchmark cases use raw SQL not present in production
- where: `pkg/flags/postgres_benchmarking_test.go` — `FindTestsByRelease`, `TestCountsByLookback14ForRelease`, `TestCountsByLookback9ForRelease`, `TestAnalysisPassRate`
- concern: Four cases embed raw SQL inline rather than calling production query functions. They benchmark queries that won't be optimized alongside production code. `TestAnalysisPassRate` uses `query.QueryTestAnalysis` (a real constant), which is better, but the call site still isn't the real production code path.

### [nit] Per-row logging included in benchmark timing
- where: `pkg/flags/postgres_benchmarking_test.go` — `FindTestsByRelease` fn body
- concern: The `log.Printf` per result row runs inside the timed section, adding I/O noise to the measured query duration.
- excerpt: |
    for _, r := range results {
        log.Printf("  [%d] %s", r.ID, r.Name)
    }

### [nit] Test_BenchmarkGroup still uses iterations := 1
- where: `pkg/flags/postgres_benchmarking_test.go` — `Test_BenchmarkGroup`
- concern: All other tests were bumped to 3 iterations; `Test_BenchmarkGroup` keeps 1. Likely intentional (group runs are expensive), but worth a comment if so.

### [nit] Test_BenchmarkCombined runs FindTestsByRelease twice
- where: `pkg/flags/postgres_benchmarking_test.go` — `Test_BenchmarkCombined`
- concern: It iterates both `getBenchmarkCases()` and `getIndividualBenchmarkCases()`, so `FindTestsByRelease` appears twice in the combined summary table — confusing when reading results.

### [nit] Test file placement in pkg/flags
- where: `pkg/flags/postgres_benchmarking_test.go`
- concern: Tests queries from `pkg/api` and `pkg/db/query` but lives in `pkg/flags`. The only `pkg/flags` dependency is `PostgresFlags.GetDBClient()`. Precedent exists (`component_readiness_test.go`), so not blocking.

## Checked
- `JobDetailsReport` extraction is mechanically correct — same query, same field ordering, caller delegates properly
- Error handling preserved: `PrintJobDetailsReportFromDB` logs and returns the error from `JobDetailsReport`
- Removed comment about magic number 12 is pre-existing, not introduced here
- Tests gated behind `db_benchmarking_dsn` env var — skip in CI and normal `make test`
- No changes to any API contract or response format
- `runBenchmarkCase` properly resets min on first iteration (`i == 0` check)
- `printSummaryTable` sorts by avg descending — useful for identifying slowest queries at a glance
- `asOf` captured once per test function and passed to `getBenchmarkCases` — time windows are now consistent across all iterations

## Open questions
- Is the intent to run these manually against a prod-like DB before and after optimization? If so, `benchstat` on `testing.B` output would give statistically meaningful comparisons.
- Should the raw SQL benchmark cases be extracted into production query functions for consistency with the other cases?
