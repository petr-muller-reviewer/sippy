---
pr: openshift/sippy#3806
title: "TRT-2787: Migrate sippy-ng from Create React App to Vite"
head_sha: c4cbe7a7ab95992ccc8f2322c8ea1cd02904e2db
base: main
reviewed_at: 2026-07-22T14:55:15Z
verdict: request-changes
---

## Summary

CRA/webpack -> Vite, Jest -> Vitest for sippy-ng. 203 changed files but 182
are pure `.js`->`.jsx`/`.ts`->`.tsx` renames with no content change. Real
diff is small: vite.config.js, package.json, env files, .eslintrc.js,
index.html relocation, setupTests, README, process.env.REACT_APP_* ->
import.meta.env.VITE_* conversions. Verified locally on head_sha: `npm ci`,
`npm test` (32/32 pass), `npm run build` all succeed.

## Findings

### [blocking] Revert commit reintroduces open redirect, ReDoS/crash, and stored DOM XSS
- where: `sippy-ng/src/component_readiness/JobArtifactQuery.jsx`, `sippy-ng/src/prow_job_runs/IntervalsChart.jsx`, `sippy-ng/src/releases/ReleaseOverview.jsx` (all reverted in commit c4cbe7a7a)
- concern: Commit c4cbe7a7a ("Revert security code changes, rely on .snyk excludes") frames itself as removing unnecessary churn caused by stale `.snyk` paths, but it actually reverts three real fixes from 8b910a33a. A `.snyk` exclude only suppresses the scanner finding; it does not fix the underlying vulnerability. All three bugs are live on head_sha.
- excerpt: |
    // JobArtifactQuery.jsx:~708 (handleOpenLinks) - reverted to bare window.open;
    // openLaunderedLink is still used 2 lines away in the same file for row.url,
    // so the fix exists and is proven in place, just selectively unwired here.
    function handleOpenLinks(event) {
      artifacts.forEach((file) => {
        window.open(file.artifact_url, '_blank')
      })
    }

    // IntervalsChart.jsx:filterIntervals - filterText comes from a URL query
    // param (params.get('filterText')); no try/catch means an invalid regex
    // throws uncaught, and a catastrophic-backtracking pattern can hang the tab.
    let re = null
    if (filterText) {
      re = new RegExp(filterText)
    }

    // ReleaseOverview.jsx:~227 - warning is API response content (release
    // controller), rendered unsanitized.
    <div dangerouslySetInnerHTML={{ __html: warning }}></div>
- verdict: CONFIRMED (locally reproduced: openLaunderedLink still defined/used elsewhere in same file; filterText traced to params.get('filterText'); warning traced to release-controller API response)

### [nit] Single 7.5 MB JS bundle, no code splitting
- where: `sippy-ng/vite.config.js`
- concern: `npm run build` on head_sha emits one `index-*.js` chunk at 7.5 MB (2.27 MB gzip), triggering Rollup's >500 KB chunk-size warning. Not a regression this PR needs to fix, but Vite makes `build.rollupOptions.output.manualChunks` / route-level dynamic `import()` easy to add as a follow-up.

### [nit] esbuild surfaces pre-existing dead code during build
- where: `sippy-ng/src/helpers.jsx`, `sippy-ng/src/jobs/JobAnalysis.jsx`
- concern: Build warns `Comparison using the "===" operator here is always false` for `filter.items === []` (array-literal reference comparison, always false). Pre-existing bug, out of scope for this PR, but now visible for free via Vite's esbuild step where CRA's pipeline didn't surface it. Worth a follow-up ticket.

### [nit] README drops documented API-component test convention with no replacement
- where: `sippy-ng/README.md`
- concern: The rewrite removes the old convention text ("components hitting the API should have a snapshot + canary text + call-count test") along with the now-defunct PollyJS/snapshot workflow instructions. Justified given Vitest+RTL don't use that flow, but nothing replaces the underlying convention guidance.

## Checked
- No leftover `process.env.REACT_APP_*` or `process.env.NODE_ENV` references anywhere in `sippy-ng/src/` (grep clean).
- No leftover Enzyme/PollyJS/`jest.*` references in `sippy-ng/src/` (grep clean).
- `vite.config.js` `base: '/sippy-ng/'` matches old CRA `homepage` field; `outDir: 'build'` preserves existing embed/deploy path.
- `index.html` move to project root + `<script type="module" src="/src/index.jsx">` is correct Vite convention; all referenced static assets (favicon.ico, apple-touch-icon.png, site.webmanifest) exist under `public/`.
- `mcp/server.py` dev-server process detection (`react-scripts` -> `vite`) matches new invocation.
- `.snyk` exclude path updates (73c4eb88a) correctly point at the renamed `.jsx` files.
- `npm ci`, `npm test` (32/32 pass), `npm run build` all succeed on head_sha; `git status` clean afterward (no stray build artifacts tracked).

## Open questions
- Why was commit c4cbe7a7a framed as reverting "unnecessary" code changes when it removes real, working security fixes (DOMPurify sanitization, openLaunderedLink, regex try/catch)? Was this intentional or a misunderstanding of what `.snyk` excludes actually suppress?
- Is there a follow-up planned for the 7.5 MB single-bundle build output, or is this considered acceptable for now?
