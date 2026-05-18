---
pr: openshift/sippy#3533
title: "TRT-1989: add benchmarking db query tests"
head_sha: fe42ba08360d9a2bb2c959421c562010a5e1b943
base: master
reviewed_at: 2026-05-18T15:16:19Z
verdict: approve
refresh_log:
  - from: 7eaee42187506fff3fa2ba91962d966a7ddc0cb9
    to: fe42ba08360d9a2bb2c959421c562010a5e1b943
    summary: "3 commits, pkg/flags/postgres_benchmarking_test.go only (+254/-25); added 7 new benchmark cases, benchmarkResult struct, printSummaryTable, runBenchmarkCase helper; iterations bumped to 3; new Test_BenchmarkFindTestsByRelease and Test_BenchmarkCombined tests"
---

## Since previous review (7eaee42 → fe42ba08)

- Added `benchmarkResult` struct with min/max/avg/total tracking, `runBenchmarkCase` helper, and `printSummaryTable` — eliminates duplicate timing boilerplate; partially addresses the "custom timing framework" finding.
- `iterations` bumped from 1 to 3 in `Test_BenchmarkIndividual` and new `Test_BenchmarkCombined`.
- Added 7 new benchmark cases: `TestAnalysisOverall`, `TestAnalysisByJob`, `TestAnalysisByJobWithVariantFilter`, `TestCountsByLookback14/9`, `TestCountsByLookback14/9ForRelease`, `TestAnalysisPassRate`.
- `FindTestsByRelease` extracted to `getIndividualBenchmarkCases()` returning a map; `Test_BenchmarkFindTestsByRelease` exercises it alone.
- `benchmarkJobName` constant added (job name previously inline in `JobDetails` case).

## Findings

### [resolved] Custom timing framework — sufficiently improved
- where: `pkg/flags/postgres_benchmarking_test.go`
- concern: Original finding: duplicated timing with `fmt.Printf`, iterations hardcoded to 1. New code adds `benchmarkResult` struct (min/max/avg/total), `runBenchmarkCase` helper, `printSummaryTable`, and bumps iterations to 3. Still not `testing.B` / `benchstat`, but the custom framework is now genuinely useful.

### [should-fix] Hardcoded release and test name constants will rot
- where: `pkg/flags/postgres_benchmarking_test.go:15-17`
- concern: `benchmarkRelease = "4.22"`, `benchmarkTestName`, and `benchmarkJobName` are hardcoded. These will become stale as releases advance. Make them configurable via env var (like `db_benchmarking_dsn`) or document that they need periodic updates.
- excerpt: |
    const benchmarkRelease = "4.22"
    const benchmarkTestName = "[Monitor:legacy-test-framework-invariants-pathological][sig-arch] events should not repeat pathologically for ns/kube-system"
    const benchmarkJobName = "periodic-ci-openshift-release-main-ci-4.22-e2e-aws-ovn"

### [nit] DB client never closed — leaked connections
- where: `pkg/flags/postgres_benchmarking_test.go` — `getBenchmarkDBClient`
- concern: CodeRabbit flagged this: the helper opens a DB connection but registers no `t.Cleanup` to close it. Unlikely to matter in practice for a manual benchmark tool, but good hygiene.
- excerpt: |
    dbc, err := dbFlags.GetDBClient()
    if err != nil {
        t.Fatalf("couldn't get DB client: %v", err)
    }
    return dbc

### [nit] Direct map index into getIndividualBenchmarkCases() can panic
- where: `pkg/flags/postgres_benchmarking_test.go` — `Test_BenchmarkFindTestsByRelease`
- concern: `bc := getIndividualBenchmarkCases()["FindTestsByRelease"]` — if the key drifts (rename, typo), `bc` is a zero-value `benchmarkCase` and `bc.fn` is nil; calling it panics with no useful message. Use a comma-ok check or `t.Fatal` on miss.
- excerpt: |
    bc := getIndividualBenchmarkCases()["FindTestsByRelease"]
    r := runBenchmarkCase(t, dbc, bc, iterations)

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
- concern: All other tests were bumped to 3 iterations; `Test_BenchmarkGroup` keeps `iterations := 1`. Likely intentional (group runs are expensive), but worth a comment if so.

### [nit] Test_BenchmarkCombined runs FindTestsByRelease twice
- where: `pkg/flags/postgres_benchmarking_test.go` — `Test_BenchmarkCombined`
- concern: It iterates both `getBenchmarkCases()` and `getIndividualBenchmarkCases()`, so `FindTestsByRelease` is included twice in the combined results table. The table's sort-by-avg will show two entries, which is confusing.

### [nit] Test file placement in pkg/flags
- where: `pkg/flags/postgres_benchmarking_test.go`
- concern: Tests queries from `pkg/api` and `pkg/db/query` but lives in `pkg/flags`. Only `pkg/flags` dependency is `PostgresFlags.GetDBClient()`. Precedent exists (`component_readiness_test.go`), so not blocking.

## Checked
- `JobDetailsReport` extraction is mechanically correct — same query, same field ordering, caller delegates properly
- Error handling preserved: `PrintJobDetailsReportFromDB` logs and returns the error from `JobDetailsReport`
- Removed comment about magic number 12 is pre-existing, not introduced here
- Tests gated behind `db_benchmarking_dsn` env var — skip in CI and normal `make test`
- No changes to any API contract or response format
- `runBenchmarkCase` properly tracks min correctly (resets on i==0)
- `printSummaryTable` sorts by avg descending — useful for identifying slowest queries

## Open questions
- Is the intent to run these manually against a prod-like DB before and after optimization? If so, `benchstat` on `testing.B` output would give statistically meaningful comparisons.
- Should the raw SQL benchmark cases be extracted into production query functions for consistency?
