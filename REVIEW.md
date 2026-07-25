---
pr: openshift/sippy#3827
title: "NO-JIRA: Fix chart imports"
head_sha: 4b883318950ca4e07f238d117a65ef82ad15d19a
base: main
reviewed_at: 2026-07-25T16:32:59Z
verdict: approve
---

## Summary

Fixes `Error: "arc" is not a registered element.` by adding `sippy-ng/src/chartSetup.jsx`, which calls `Chart.register(...)` for the Chart.js controllers/elements/scales/plugins the app needs, and imports it (side-effect only) at the top of `App.jsx`. Likely surfaced by the Vite migration, since Chart.js v3's tree-shakeable build requires explicit registration.

## Findings

(none)

## Checked
- Verified every chart type actually rendered in the app (`Line` in `BuildClusterHealthChart.jsx`, `JobStackedChart.jsx`, `JobAnalysis.jsx`, `TestStackedChart.jsx`, `TestPassRateCharts.jsx`, `TestDurationChart.jsx`; `Doughnut` in `SummaryCard.jsx`) has all required chart.js building blocks present in the registered list (`LineController`, `LineElement`, `PointElement`, `CategoryScale`, `LinearScale`, `TimeScale`, `Filler`, `Title`, `Legend`, `Tooltip`, `DoughnutController`, `ArcElement`).
- No other chart types (`Bar`, `Pie`, `Scatter`, `Radar`, etc.) exist anywhere in `sippy-ng/src` — registration list is not under-inclusive.
- `annotationPlugin` is genuinely used (`prow_job_runs/IntervalsChart.jsx`, `tests/FeatureGateDetail.jsx`), not dead registration.
- No duplicate `Chart.register` calls elsewhere in the codebase to conflict with.
- Import ordering in `App.jsx` preserved (`./chartSetup` sorts alphabetically before `./components/...`).
- Registration is idempotent and global; registering unused-but-plausible elements carries no real downside.

## Open questions
- `chartSetup.jsx` contains no JSX — was `.js` considered instead of `.jsx`, or is `.jsx` the deliberate house convention for all `src` files?
- Existing tests mock `react-chartjs-2` (`setupTests.jsx`), so this exact bug class (missing `Chart.register`) wouldn't be caught by the test suite. Worth a lightweight smoke test that mounts a real chart component through actual Chart.js, so a future added chart type without corresponding registration fails CI instead of production?
