---
pr: openshift/sippy#3536
title: "Fix WebSocket chat proxy broken by status-capturing middleware"
head_sha: 793f256a502f3e39efd1a1d8fea1630b901ff836
base: main
reviewed_at: 2026-05-18T14:26:22Z
verdict: approve
---

## Findings

### [nit] Logged status is 200 for hijacked WebSocket connections
- where: `pkg/sippyserver/server.go:2963-2969`
- concern: After a successful `Hijack()`, `WriteHeader(101)` is never called on the wrapper — `sw.status` stays at the default `http.StatusOK`. The log line at :2969 will report `status: 200` for successful WebSocket upgrades. Cosmetically misleading in logs but functionally harmless.
- excerpt: |
    sw := &statusCapturingResponseWriter{ResponseWriter: w, status: http.StatusOK}
    start := time.Now()
    h.ServeHTTP(sw, r)
    log.WithFields(log.Fields{
        "status": sw.status,
    }).Info("responded to request")

### [nit] Other optional ResponseWriter interfaces not delegated
- where: `pkg/sippyserver/server.go:2935-2951`
- concern: The wrapper also doesn't implement `http.Flusher` (needed for SSE/chunked responses) or `http.Pusher`. Only `Hijacker` is needed right now for the WebSocket path, so this is correct scope. If SSE endpoints are added later, `Flush()` delegation would be needed too. No action required.

## Checked
- `Hijack()` implementation correctly type-asserts the underlying writer and returns a clear error if it doesn't implement `http.Hijacker`
- Comment on the `Hijack()` method explains why (the specific endpoint), not just what
- Test uses `httptest.NewServer` with `logRequestHandler` wrapping a real gorilla upgrader, verifies full round-trip including 101 status
- Import additions (`bufio`, `net`, `gorilla/websocket`) are all necessary and minimal
- The root cause (commit `160d54da` introduced `statusCapturingResponseWriter` without `Hijacker` support) is correct

## Open questions
- Would it be worth logging `status: 101` explicitly when a hijack occurs, so WebSocket upgrades are visible in logs? (Minor, not blocking.)
