# Add Plain HTTP Forward Proxy Support

The cloister proxy currently only handles CONNECT (TLS tunneling). Plain HTTP
requests (GET, POST, etc. with absolute URIs) get 405. This means `curl
http://example.com` through the proxy fails, even if the domain is allowlisted.
Add forward proxy support for all standard HTTP methods so non-TLS connections
work with the same auth, policy, and domain approval flow.

## Testing Philosophy

- **Test-first**: Each behavioral change starts with a failing test that
  documents the expected behavior, confirmed failing, then the fix
- **Unit tests only**: All proxy tests use `httptest`, raw TCP, and mock
  interfaces — no Docker, no real guardian
- **Reuse existing harness**: `proxyTestHarness`, `sendHTTPViaProxy`,
  `mockTunnelHandler`, `newTestProxyPolicyEngine` are already in
  `proxy_test.go` and `proxy_approval_test.go`
- **Go `testing` package**: Table-driven tests where multiple methods or
  domains vary; standalone tests for distinct behavioral assertions
- **Hop-by-hop header handling**: Dedicated tests for header stripping,
  not just happy-path forwarding

## Verification Checklist

Before marking a phase complete and committing it:

1. `make test` passes
2. `make lint` passes
3. `make fmt` produces no changes
4. `make build` succeeds
5. All new tests confirmed failing before the corresponding fix is applied
6. No dead code left behind (unused types, functions, imports)

When verification of a phase or subphase is complete, commit all
relevant newly-created and modified files.

## Dependencies Between Phases

```
Phase 1 (Failing tests: routing + auth)
       │
       ▼
Phase 2 (Route non-CONNECT into new handler, pass auth/domain tests)
       │
       ▼
Phase 3 (Failing tests: HTTP forwarding + hop-by-hop)
       │
       ▼
Phase 4 (Implement forwardHTTP, pass forwarding tests)
       │
       ▼
Phase 5 (Failing tests: error handling + edge cases)
       │
       ▼
Phase 6 (Error handling implementation, pass edge case tests)
       │
       ▼
Phase 7 (Update existing 405 test + e2e coverage)
```

---

## Phase 1: Failing Tests — Routing, Auth, and Domain Policy

Write tests that assert the desired behavior for non-CONNECT requests.
All tests should fail against the current code (which returns 405 for
everything non-CONNECT).

### 1.1 Basic routing and auth tests

These use raw TCP or `sendHTTPViaProxy` to send plain HTTP through the proxy.

- [x] **Test**: `TestProxyServer_PlainHTTP_AllowedDomain` — send
  `GET http://allowed.example.com/ HTTP/1.1` with valid token through
  proxy; expect 200 or 502 (upstream unreachable), NOT 405
- [x] **Test**: `TestProxyServer_PlainHTTP_DeniedDomain` — send GET to a
  denied domain with valid token; expect 403, NOT 405
- [x] **Test**: `TestProxyServer_PlainHTTP_NoAuth` — send GET without
  `Proxy-Authorization`; expect 407, NOT 405
- [x] **Test**: `TestProxyServer_PlainHTTP_InvalidToken` — send GET with
  bad token; expect 407, NOT 405
- [x] **Test**: `TestProxyServer_PlainHTTP_Methods` — table-driven test
  sending GET, POST, PUT, DELETE, PATCH, HEAD through proxy to an
  allowed domain; none should return 405
- [x] Confirm all tests fail with current code (405 for all)

### 1.2 Domain policy tests for plain HTTP

- [x] **Test**: `TestProxyServer_PlainHTTP_DomainApproval` — send GET to
  unlisted domain; expect domain approval flow to be invoked (mock
  `DomainApprover` records the call)
- [x] **Test**: `TestProxyServer_PlainHTTP_PolicyCheck` — send GET with
  `mockPolicyChecker`; verify `Check()` is called with correct token,
  project, and domain extracted from the absolute URI
- [x] Confirm all tests fail

---

## Phase 2: Route Non-CONNECT Requests and Apply Auth/Policy

Modify `handleRequest` to dispatch plain HTTP requests to a new handler
that applies the same auth and domain policy as CONNECT. The handler
does not yet forward to upstream — it can return 502 after policy checks
pass. This is enough to make Phase 1 tests pass.

### 2.1 Add `handlePlainHTTP` skeleton

- [x] In `handleRequest`, replace the blanket 405 with a branch: if
  `r.URL.Scheme == "http"` and `r.URL.Host != ""` (absolute-URI form),
  call `handlePlainHTTP(w, r)`; otherwise keep 405 for malformed
  requests
- [x] `handlePlainHTTP` calls `authenticate` (reused), `resolveRequest`
  (reused), extracts domain from `r.URL.Host` via `stripPort`,
  calls `checkDomainAccess` (reused), then returns 502 as a stub
- [x] Phase 1 tests pass (auth → 407, denied → 403, allowed → 502,
  approval flow invoked)
- [x] Existing CONNECT tests still pass
- [x] `TestProxyApproval_NonCONNECTReturns405` now fails (expected — we
  handle the request instead of returning 405); update it in Phase 7

---

## Phase 3: Failing Tests — HTTP Forwarding

Write tests that verify actual upstream forwarding behavior. These need
a mock HTTP upstream (via `httptest.NewServer`) that the proxy forwards
to.

### 3.1 Forwarding round-trip tests

- [x] **Test**: `TestProxyServer_PlainHTTP_ForwardGET` — start
  `httptest.NewServer` echoing request details; send
  `GET http://<upstream>/path?q=1` through proxy; assert response
  status, body, and that `Host` header reaches upstream correctly
- [x] **Test**: `TestProxyServer_PlainHTTP_ForwardPOST` — send POST
  with body through proxy; assert body arrives at upstream intact
- [x] **Test**: `TestProxyServer_PlainHTTP_PreservesHeaders` — send
  request with custom headers (`X-Custom: foo`); verify they reach
  upstream
- [x] Confirm tests fail (Phase 2 stub returns 502)

### 3.2 Hop-by-hop header tests

- [x] **Test**: `TestProxyServer_PlainHTTP_StripsHopByHop` — send
  request with `Proxy-Authorization`, `Proxy-Connection`,
  `Connection: keep-alive`, `TE`, `Transfer-Encoding: chunked`
  headers; verify upstream does NOT receive `Proxy-Authorization`,
  `Proxy-Connection`; verify `Connection` is handled correctly
- [x] **Test**: `TestProxyServer_PlainHTTP_NoProxyAuthToUpstream` —
  explicitly verify `Proxy-Authorization` is never forwarded (security
  critical — leaking the cloister token to upstream is a vulnerability)
- [x] Confirm tests fail

---

## Phase 4: Implement HTTP Forwarding

Replace the 502 stub in `handlePlainHTTP` with actual forwarding logic.

### 4.1 Implement `forwardHTTP`

- [x] Create `forwardHTTP(w http.ResponseWriter, r *http.Request)` that:
  - Clones the inbound request for the outbound call
  - Sets `r.URL` from the absolute URI (already parsed by Go)
  - Strips hop-by-hop headers: `Proxy-Authorization`,
    `Proxy-Connection`, and headers listed in the `Connection` header
  - Uses `http.Transport` (with `dialTimeout` and configured timeouts)
    to execute the request against the upstream
  - Copies response status, headers, and body back to `w`
  - Does NOT follow redirects (the client handles those)
  - Does NOT set `X-Forwarded-For` — omit to avoid leaking internal
    container IPs to upstream (this is a security sandbox, not a
    transparent corporate proxy)
- [x] Add `*http.Transport` as a field on `ProxyServer` (not package-level)
  to allow connection reuse and to respect proxy-configured timeouts
- [x] Phase 3 forwarding tests pass
- [x] Phase 3 hop-by-hop tests pass
- [x] Phase 1 tests still pass (allowed domain now gets 200 from mock
  upstream instead of 502)

---

## Phase 5: Failing Tests — Error Handling and Edge Cases

### 5.1 Upstream failure tests

- [x] **Test**: `TestProxyServer_PlainHTTP_UpstreamRefused` — proxy
  forwards to a closed port; expect 502 Bad Gateway
- [x] **Test**: `TestProxyServer_PlainHTTP_UpstreamTimeout` — proxy
  forwards to a server that never responds; expect 504 Gateway Timeout
- [x] Confirm tests fail or document the current behavior matches

### 5.2 Edge case tests

**Note:** Go's `net/http` server may not populate `r.URL.Scheme`/`r.URL.Host`
for non-absolute-URI requests (relative URI, HTTPS scheme). These tests likely
need raw TCP via `sendHTTPViaProxy` to send the exact request line, since Go's
HTTP client won't generate these malformed proxy requests.

- [x] **Test**: `TestProxyServer_PlainHTTP_RelativeURI` — send
  `GET /path HTTP/1.1` (not absolute URI) via raw TCP through proxy;
  expect 400 Bad Request (not a valid forward proxy request)
- [x] **Test**: `TestProxyServer_PlainHTTP_HTTPSScheme` — send
  `GET https://example.com/ HTTP/1.1` via raw TCP through proxy (wrong
  — HTTPS should use CONNECT); expect 400 Bad Request
- [x] **Test**: `TestProxyServer_PlainHTTP_EmptyHost` — absolute URI
  with empty host; expect 400 Bad Request
- [x] Confirm tests fail

---

## Phase 6: Error Handling Implementation

### 6.1 Upstream error mapping

- [x] In `forwardHTTP`, map `net.Error` timeout to 504, connection
  refused to 502, other dial errors to 502 (matching CONNECT behavior
  in `dialAndTunnel`)
- [x] Phase 5.1 tests pass

### 6.2 Request validation

- [x] In `handlePlainHTTP`, validate **after** auth (consistent with
  CONNECT path — auth failures should always return 407 regardless of
  request form, and validating before auth leaks information about what
  the proxy accepts to unauthenticated clients): `r.URL.Host` is
  non-empty, `r.URL.Scheme` is `"http"` (not `"https"`), and the
  request is in absolute-URI form; return 400 for violations
- [x] Phase 5.2 tests pass

---

## Phase 7: Update Existing Tests and E2E

### 7.1 Update the existing 405 test

- [x] Rename `TestProxyApproval_NonCONNECTReturns405` to
  `TestProxyApproval_NonCONNECTForwardsPlainHTTP` (or similar)
- [x] Update assertion: the test currently sends
  `HEAD http://some-domain.example.com/` through the proxy and expects
  405; update to expect the correct new behavior (403 if domain is
  unlisted, or 200 if allowed, depending on harness config)
- [x] Verify no tunnel handler calls (plain HTTP should never invoke
  `TunnelHandler`)

### 7.2 E2E test for plain HTTP proxy

Use a local `httptest.NewServer` reachable from the container network
(bind to the Docker bridge IP or use host networking) rather than
hitting real external domains — avoids DNS flakiness and external
service dependencies.

- [ ] **Test**: Add e2e test in `test/e2e/` that sends a plain HTTP
  request through the proxy to an allowlisted domain and verifies it
  succeeds
- [ ] **Test**: Add e2e test verifying denied domain returns 403 for
  plain HTTP (not just CONNECT)

---

## Future Phases (Deferred)

### WebSocket Support
- Plain HTTP upgrade to WebSocket through the proxy
- Requires special handling of `Connection: Upgrade` + `Upgrade: websocket`

### HTTPS-in-HTTP (CONNECT with policy inspection)
- MitM-style TLS inspection for CONNECT requests (out of scope, security implications)

### Request/Response Logging
- Audit logging for plain HTTP requests (method, URL, status, latency)
- Currently only CONNECT tunnels are logged; forwarded requests should match

### Rate Limiting
- Per-token or per-domain rate limiting for forwarded requests
- CONNECT tunnels are long-lived so rate limiting is less meaningful there
