---
pr: openshift/sippy#3557
title: "Trt 1989 benchmarking"
head_sha: 9b3122d9ff70cc43fd5a69757f1221b65ba39fe1
base: main
reviewed_at: 2026-05-30T00:35:40Z
verdict: approve
refresh_log:
  - old_sha: 117d9e3f446ad0ce95560bd4b34f7765b2844858
    new_sha: 9b3122d9ff70cc43fd5a69757f1221b65ba39fe1
    summary: "1 commit (lint fix: 0600 → 0o600 octal literal). CI passed. No review comments."
  - old_sha: 9b3122d9ff70cc43fd5a69757f1221b65ba39fe1
    new_sha: 9b3122d9ff70cc43fd5a69757f1221b65ba39fe1
    summary: "No code change. Author dismissed localhost concern as intentional. G703 nosec confirmed correct (taint analysis rule, not unhandled error). smg247 /lgtm. PR approved."
---

## Summary

Extends `pkg/flags/postgres_benchmarking_test.go` with 8 new benchmark cases (VariantReports, JobReports, BuildClusterHealth, RecentTestFailures, PullRequestReport, RepositoryReport, JobsRunsReport, ProwJobHistoricalTestCounts). Adds optional file output of benchmark results via `benchmarking_file_path` env var. Adds `extractConnectionName` to parse hostname from DSN for use in output filenames. Passes `*testing.T` and `connName` through to `printSummaryTable`.

Test-only file. Zero production impact.

Since previous review (first refresh): 1 commit (`9b3122d`) — lint fix changing `0600` to `0o600` (modern Go octal literal syntax) on the `os.WriteFile` call. CI e2e tests passed.

Since previous review (second refresh): No code changes. Author (neisw) dismissed the `extractConnectionName` localhost concern as intentional — the DSNs in use are specific and this is not meant to be a robust solution; a proper benchmarking framework is future work. CodeRabbit confirmed `#nosec G703` is correct: G703 is a taint-analysis rule ("path traversal via taint analysis"), not "unhandled errors" — `os.Getenv()` taints `fullPath` and the annotation is the correct suppression. smg247 gave `/lgtm` (2026-05-29T14:45:06Z). PR approved by openshift-ci (2026-05-29T14:46:36Z).

## Findings

### [should-fix] ProwJobHistoricalTestCounts fatals on missing job
- where: `pkg/flags/postgres_benchmarking_test.go:388-391`
- concern: If `benchmarkJobName` doesn't exist in the target database, `dbc.DB.Where(...).First(&prowJob).Error` returns `gorm.ErrRecordNotFound`, which flows into `runBenchmarkCase` → `t.Fatalf`, killing the subtest. This blocks the result from appearing in the summary table with no clear message. A `t.Skipf` would be more appropriate since this is a data availability issue, not a code failure.
- excerpt: |
    var prowJob models.ProwJob
    if err := dbc.DB.Where("name = ? AND release = ?", benchmarkJobName, benchmarkRelease).First(&prowJob).Error; err != nil {
        return err
    }

### [nit] Missing t.Helper() in printSummaryTable
- where: `pkg/flags/postgres_benchmarking_test.go:52`
- concern: Function accepts `*testing.T` and calls `t.Logf` but omits `t.Helper()`. Inconsistent with `getBenchmarkDBClient` and `runBenchmarkCase` which both call it.
- excerpt: |
    func printSummaryTable(t *testing.T, results []benchmarkResult, connName string) {

### [nit] Timestamp format looks like a typo but isn't
- where: `pkg/flags/postgres_benchmarking_test.go:80`
- concern: `"2006-01-02T15-04-05"` is correct (dashes instead of colons for filename safety) but looks like a bug. A brief comment would prevent future "fix" attempts.
- excerpt: |
    ts := time.Now().UTC().Format("2006-01-02T15-04-05")

## Checked
- All 8 new benchmark cases follow the identical pattern as existing ones (call query, log row count, return error)
- `getBenchmarkDBClient` correctly extracts and returns `connName` alongside `dbc`
- All call sites of `printSummaryTable` updated to new signature
- File write uses `0o600` permissions (appropriate for a report file; updated to modern octal syntax in `9b3122d`)
- `filepath.Clean` applied after `filepath.Join` (mitigates simple path traversal from env var)
- Benchmarks only run when `db_benchmarking_dsn` is set — no CI impact
- New imports are all used

## Resolved

### [should-fix] Wrong nosec annotation — RETRACTED
- where: `pkg/flags/postgres_benchmarking_test.go:84`
- resolution: G703 is a legitimate gosec taint-analysis rule ("path traversal via taint analysis"), not "unhandled errors." The `os.Getenv("benchmarking_file_path")` value taints `fullPath`, and `#nosec G703` is the correct suppression. My original finding was wrong. Confirmed by CodeRabbit after investigation (2026-05-29T12:47:45Z).

### [should-fix] extractConnectionName silently returns "" for localhost — DISMISSED BY AUTHOR
- where: `pkg/flags/postgres_benchmarking_test.go:40-50`
- resolution: Author (neisw, 2026-05-28T20:47:19Z) stated this is intentional — the DSNs used for benchmarking are specific and known. Not intended to be a robust solution; a proper benchmarking framework is future work.

## Open questions
- None remaining. Previous questions about `extractConnectionName` localhost behavior answered by author.
