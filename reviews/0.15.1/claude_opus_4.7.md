# Kula v0.15.1 — Security, Code Quality & Performance Review

**Reviewer:** Claude Opus 4.7 (security researcher / code review pass)
**Scope:** Go backend (`cmd/`, `internal/`), configuration, embedded frontend, install/addon scripts.
**Target commit:** current working tree (`main`, VERSION = 0.15.1)
**Methodology:** manual source review of the web stack (server, auth, websocket, ollama proxy, Prometheus exporter), config loader, sandbox, storage engine, collectors (postgres, nginx, custom, gpu, disk, system), and the embedded UI JavaScript.

---

## 1. Overall Summary

Kula is a well-engineered, zero-dependency Linux monitoring daemon. The codebase is idiomatic Go, clearly structured, with reasonable test coverage and thoughtful hardening throughout. The security posture is notably stronger than most projects of comparable size:

- **Hashing:** Argon2id with per-user salts, constant-time comparison, SHA-256–hashed session tokens at rest.
- **Sandboxing:** Landlock (BestEffort, ABI v5) restricts filesystem and network access at runtime.
- **Supply chain:** few dependencies, SRI hashes for every bundled JS file, CSP with per-request nonce.
- **Input handling:** request-body limits via `MaxBytesReader`, scanner buffers capped, embedded static FS (no filesystem traversal).
- **SSRF defense:** the Ollama URL is whitelisted to loopback.
- **Abuse:** per-IP + per-username login rate limiting, per-IP websocket caps, CSRF (origin + synchroniser token) on state-changing requests.

No critical vulnerability was found. The issues below are mostly **medium, low, or informational**, and several reflect defense-in-depth opportunities rather than exploitable flaws. A handful of behaviours deserve tightening before a 1.0 milestone.

**Scorecard** (0 = failing, 10 = best in class):

| Dimension                          | Score |
|------------------------------------|:-----:|
| Authentication & Session Mgmt      | 8 / 10 |
| Authorization / Access Control     | 8 / 10 |
| Input Validation & Request Safety  | 8 / 10 |
| CSRF / XSRF Defense                | 7 / 10 |
| XSS / Content Security             | 8 / 10 |
| SSRF / Outbound Traffic Safety     | 7 / 10 |
| Cryptography                       | 9 / 10 |
| Secrets Handling                   | 6 / 10 |
| Dependency & Supply Chain          | 8 / 10 |
| Sandboxing / Privilege Reduction   | 9 / 10 |
| Logging / Observability Safety     | 8 / 10 |
| Error Handling & Fail-Closed       | 8 / 10 |
| Code Quality / Readability         | 9 / 10 |
| Performance / Efficiency           | 8 / 10 |
| Test Coverage (security-relevant)  | 7 / 10 |
| **Overall**                        | **7.9 / 10** |

Severity labels used below: `Critical` · `High` · `Medium` · `Low` · `Informational`.

---

## 2. Findings

### 2.1 Authentication, Session & Access Control

#### [M-1] Session has no absolute lifetime; sliding expiration is unbounded — *Medium*
Every call to `AuthManager.ValidateSession` extends `expiresAt` by `SessionTimeout` ([auth.go:200](internal/web/auth.go#L200)). A session that receives traffic every 23 h with a 24-h timeout lives forever. There is no "hard" maximum lifetime, no re-authentication, no IP or User-Agent binding (explicitly disclaimed in the README), and no forced rotation on privilege-relevant events.

**Impact:** Stolen tokens are valid until manual logout / server restart. Combined with the fact that session IPs are intentionally not pinned, there is no automatic containment.

**Recommendation:** track `createdAt` (already present), enforce an absolute maximum (e.g. 7 days) independent of sliding activity, and rotate the token on that boundary.

```go
type session struct {
    username  string
    csrfToken string
    createdAt time.Time
    expiresAt time.Time
}

// in ValidateSession, after lookup:
if time.Since(sess.createdAt) > a.cfg.AbsoluteMaxAge {
    delete(a.sessions, hashedToken)
    return false
}
```

---

#### [M-2] `CSRFMiddleware` only enforces the synchroniser token when auth is enabled — *Medium*
[auth.go:378](internal/web/auth.go#L378) skips the token check entirely if `!a.cfg.Enabled`. With the default install (auth off, but still exposed on `localhost`/LAN), state-changing endpoints fall back to the Origin/Referer check alone. Origin can be absent in some non-browser contexts; Kula does reject empty-Origin in `ValidateOrigin` so this is *partially* mitigated, but the synchroniser token is the stronger guarantee.

Most state-changing endpoints in the current code are low-impact (login/logout/chat), but the pattern is brittle: adding any future privileged write endpoint silently loses CSRF protection when auth is off.

**Recommendation:** require the synchroniser token even in no-auth mode (issue the token from a first GET, store it in a cookie + mirror header). At minimum, add a regression test that fails if a non-idempotent endpoint becomes reachable without a token in no-auth mode.

---

#### [M-3] WebSocket `CheckOrigin` accepts empty `Origin` — *Medium*
[websocket.go:26-30](internal/web/websocket.go#L26-L30) allows WebSocket upgrades with no Origin header "to support CLI clients". Browsers always send Origin on WS, but:

1. Some niche clients (Electron, `fetch`-based WS polyfills, hostile browser extensions) omit or strip Origin.
2. Combined with `M-2`, if auth is disabled the WebSocket feed is reachable from any origin that omits the header.

**Impact:** potential Cross-Site WebSocket Hijacking when auth is disabled. Low impact today because the WS only exposes read-only sample data and pause/resume commands, but the data is not supposed to be cross-origin accessible.

**Recommendation:** reject empty Origin by default; gate the permissive path behind an explicit config flag (`web.allow_unauthenticated_ws: true`) or document that unauthenticated deployments should bind only to loopback.

```go
if origin == "" {
    // Only allow if explicitly configured
    if !allowCLIClients {
        log.Printf("WS upgrade blocked: missing Origin")
        return false
    }
    return true
}
```

---

#### [L-1] `Authorization: Bearer` prefix compared case-sensitively — *Low*
[auth.go:242](internal/web/auth.go#L242): `authHeader[:7] == "Bearer "`. RFC 7235 §2.1 makes the scheme name case-insensitive. Clients sending `bearer ` will silently fail auth.

**Recommendation:** `strings.HasPrefix(strings.ToLower(authHeader), "bearer ")` or `strings.EqualFold(authHeader[:7], "Bearer ")` after a bounds check.

---

#### [L-2] Login rate limiter is memory-only and unbounded in degenerate cases — *Low*
`RateLimiter.attempts` is a `map[string][]time.Time` ([auth.go:38](internal/web/auth.go#L38)). Purge runs inside `CleanupSessions` every 5 minutes, but between purges an attacker cycling distinct IPs (IPv6 /64 tenants trivially have millions) can grow the map unboundedly. For each IP the inner slice is bounded (≤5 entries after `Allow` check), but the outer key set is not.

**Recommendation:** cap the total map size (e.g. 50k distinct keys) and fall back to a global token bucket; eagerly purge when inserting into a full map.

---

#### [I-1] Session JSON written only on clean shutdown — *Informational*
`AuthManager.SaveSessions` is called from `Server.Shutdown`. A crash or SIGKILL loses all in-memory sessions. Not a security weakness (safer default: fail closed), but users may see unexpected logouts.

---

### 2.2 CSRF / Origin

#### [L-3] Origin host comparison ignores port edge cases — *Low*
`ValidateOrigin` uses `strings.EqualFold(u.Host, r.Host)` ([auth.go:364](internal/web/auth.go#L364)). When a reverse proxy rewrites Host (Docker default → `localhost:27960`, browser sends `kula.example.com:443` in Origin), the check returns false. Not exploitable, but causes spurious 403s that users "fix" by disabling the middleware. Consider documenting the interaction with `TrustProxy`/`X-Forwarded-Host`, or adding a configurable allowlist of origin hostnames for reverse-proxied deployments.

---

### 2.3 Ollama / LLM proxy

#### [M-4] Ollama URL SSRF gate checks only the hostname, not resolved IPs — *Medium*
`validateOllamaURL` ([config.go:371](internal/config/config.go#L371)) allowlists hostname strings `localhost`, `127.0.0.1`, `::1`. A user-controlled hosts file or a mis-configured DNS resolver where `localhost` maps elsewhere, or a creative URL like `http://localhost@169.254.169.254/` (spec-compliant userinfo) would bypass — `u.Hostname()` on `http://localhost@169.254.169.254/` returns `169.254.169.254`, so the existing check **does** reject that particular payload (✅). But `http://127.1/api/chat` resolves but `Hostname()` returns `127.1` and is rejected (✅). The real gap is DNS: `localhost` isn't always `127.0.0.1`. On a machine where `/etc/hosts` sets `127.0.0.1 localhost some.internal.svc`, `localhost` is still accepted.

Also, the check is purely config-time — if the configured host's DNS entry later points somewhere else (DNS rebinding), outbound chat traffic can target arbitrary IPs.

**Impact:** the threat model for Ollama is "admin-controlled config pointing at admin-controlled local daemon", which substantially limits this. Still, the documented guarantee ("only targets loopback") is stronger than the implementation delivers.

**Recommendation:** resolve the hostname at load time and verify the resolved IPs are in `127.0.0.0/8` or `::1/128`; re-resolve (or pin) on each request to defeat rebinding. Alternatively, require IP literals.

```go
ips, err := net.LookupIP(host)
if err != nil { return fmt.Errorf(...) }
for _, ip := range ips {
    if !ip.IsLoopback() {
        return fmt.Errorf("ollama.url: host %q resolves to non-loopback %s", host, ip)
    }
}
```

---

#### [L-4] Nginx `status_url` has no URL safety check — *Low*
[config.go](internal/config/config.go) does not validate `applications.nginx.status_url` the way it validates `ollama.url`. An operator who copy-pastes `http://169.254.169.254/status` (AWS IMDS) can exfiltrate metadata through the monitoring daemon's logs/metrics (the sandbox will open that port, since sandbox.go builds an outbound `ConnectTCP(port)` rule directly from the URL).

**Impact:** purely config-side; still a useful guard-rail for shared-config deployments (Ansible/Puppet templating).

**Recommendation:** apply the same SSRF allowlist logic, or at minimum refuse RFC 3927 / 6890 link-local ranges.

---

#### [L-5] Ollama chat streaming logs the entire prompt/response at debug level — *Low*
[ollama.go:485](internal/web/ollama.go#L485) dumps the full model response to the log. Chat history may contain sensitive infrastructure details (IPs, hostnames, connection strings pasted by the user). This is gated on `level=debug`, which is acceptable, but should be called out in the CHANGELOG/docs so operators know.

---

#### [I-2] The `get_metrics` tool accepts arbitrary relative durations — *Informational*
`executeGetMetrics` ([ollama.go:904-918](internal/web/ollama.go#L904-L918)) accepts `-24000h` and similar extreme values silently; `QueryRangeWithMeta` then caps to available history. Functionally correct but produces a large (100-row) CSV irrespective of the requested window. Low priority.

---

### 2.4 Config / Secrets

#### [M-5] PostgreSQL password stored plaintext in `config.yaml` — *Medium*
[config.go:189-197](internal/config/config.go#L189-L197). A `KULA_POSTGRES_PASSWORD` override exists (good), but `config.yaml` on most installs is world-readable or group-readable in `/etc/kula/`. The file itself is not created with restrictive permissions by the installer; the binary reads from the cwd by default.

**Recommendation:**
- Document a `mode 0600` / `chown root:kula` expectation for the config file in both the AUR/DEB/RPM installers and the README.
- Optionally add startup-time refusal if `config.yaml` is world-readable AND contains `postgres.password`. Something like:

```go
if mode := info.Mode().Perm(); mode&0o077 != 0 && cfg.Applications.Postgres.Password != "" {
    return fmt.Errorf("config %s contains secrets but has insecure permissions %o", path, mode)
}
```

---

#### [L-6] Prometheus bearer token stored plaintext in `config.yaml` — *Low*
[config.go:79-82](internal/config/config.go#L79-L82). Same concern as above and the same remediation applies (env var support + permission check).

---

#### [L-7] PostgreSQL DSN builder does not escape non-password fields — *Low*
[postgres.go:46-62](internal/collector/postgres.go#L46-L62). Only `password` is quoted/escaped. `host`, `user`, `dbname`, `sslmode` are interpolated as-is. libpq's `key=value` parser requires quoting/escaping for spaces, single quotes, and backslashes in any value. Today these come only from YAML (admin-controlled), but:

1. A username like `kula monitor` silently yields a malformed DSN.
2. The pattern is a footgun for any future feature that forwards user-supplied values.

**Recommendation:** use a helper that quotes every k=v pair uniformly, or switch to `pq.ParseURL`/the URI form.

```go
func pqKV(k, v string) string {
    v = strings.ReplaceAll(v, `\`, `\\`)
    v = strings.ReplaceAll(v, `'`, `\'`)
    return k + "='" + v + "'"
}
```

---

#### [L-8] `appCfg.Postgres.Host` passed to Landlock `RWDirs` with no validation — *Low*
[sandbox.go:142](internal/sandbox/sandbox.go#L142). When port == 0 the sandbox opens the configured `Host` as `RWDirs`. A misconfiguration of `host: "/"` grants the process read/write to the entire filesystem (within Landlock's model) — i.e. the sandbox tightens nothing. Because `IgnoreIfMissing()` is used, a bogus path won't error.

**Recommendation:** refuse non-absolute or root-level paths at config-load time; log the effective rule so operators can eyeball it.

---

### 2.5 Web server / HTTP

#### [L-9] Prometheus endpoint is public by default when no token is set — *Low*
[server.go:283-290](internal/web/server.go#L283-L290). Metrics include hostname, kernel clock source, filesystems, mountpoints, container list — useful for an attacker doing internal reconnaissance. A warning is logged but nothing forces the token.

**Recommendation:** either (a) require a token when `enabled: true`, (b) default-bind `/metrics` to loopback-only, or (c) print a loud warning at startup (current: single `log.Printf` that scrolls past fast).

---

#### [L-10] `createListeners` dual-stack uses two independent `net.Listen` calls — *Low*
[server.go:355-365](internal/web/server.go#L355-L365). If the IPv4 listener binds but the IPv6 one fails mid-startup, the IPv4 listener is closed; fine. But on systems with `net.ipv6.bindv6only = 0`, binding `tcp6 [::]` already covers both families. The dual-listen path may double-accept on affected kernels. Minor behavioural surprise.

---

#### [L-11] No response-body write-deadline on `/api/history` large payloads — *Low*
`WriteTimeout: 60s` is a blunt ceiling ([server.go:319](internal/web/server.go#L319)). Slow-reader clients pulling a 5000-point history can be killed mid-response. Not a vuln, but worth mentioning: consider `rc.SetWriteDeadline` with a longer deadline specifically for history, paralleling the SSE handler.

---

#### [L-12] `handleStatic` MIME type inferred from suffix only — *Low*
[server.go:868-888](internal/web/server.go#L868-L888). This is fine because `staticFS` is an `embed.FS` (contents fixed at build time), but the `octet-stream` fallback is suboptimal: any future static asset with an unknown extension won't render. Consider `mime.TypeByExtension` for a richer default.

---

#### [I-3] `X-Forwarded-For` parsing takes the rightmost entry — *Informational*
[server.go:799-806](internal/web/server.go#L799-L806). Correct against client-spoofing *if* the proxy appends; some proxies (certain nginx configs) overwrite with the client IP. The comment documents the assumption, which is the right call. Worth cross-linking the config docs so operators configure `proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;` in nginx.

---

### 2.6 WebSocket

#### [L-13] Read limit applies only to the message body, not to the upgrade handshake — *Low*
`conn.SetReadLimit(4096)` caps per-message body but the HTTP upgrade request itself is governed by the server's `ReadTimeout` (30s) / `http.Server.MaxHeaderBytes` (default 1 MiB). Low impact.

---

#### [I-4] No backpressure metrics on the broadcast — *Informational*
[server.go:741-756](internal/web/server.go#L741-L756) drops samples when `client.sendCh` is full. That's correct for liveness, but there's no metric/log when drops happen. A slow client is invisible.

---

### 2.7 Storage engine

#### [L-14] `OpenTier` silently reinitialises a corrupted header — *Low*
[tier.go:74-83](internal/storage/tier.go#L74-L83). On any header-read error `writeOff` and `count` are zeroed and the file is re-headered. A transient `ReadAt` error (I/O) looks identical to real corruption, and historical data is effectively wiped. There is no backup copy.

**Recommendation:** distinguish I/O errors from magic/format mismatches; on unclear cases, refuse to open and require a `--repair` flag. At minimum, rename the old file to `tier_N.dat.bak` before reinitialising, so operators can recover.

---

#### [L-15] Ring-buffer decoder trusts the on-disk `dataLen` loosely — *Low*
[tier.go:295](internal/storage/tier.go#L295) validates `dataLen <= maxData`, which is the only real bound. A corrupted length field that sits just under `maxData` (e.g. 249 MB) causes a multi-MB allocation per record in `ReadRange` / `ReadLatest`. Since the file itself has a fixed max size this is bounded overall, but a single corrupted byte could still allocate hundreds of megabytes transiently.

**Recommendation:** bound `dataLen` to a realistic per-sample cap (e.g. 64 KB — well above the encoded fixed+variable block) and break the loop on violation instead of continuing.

---

#### [I-5] `codec.go` uses per-record `decodeSampleJSON` fallback — *Informational*
Legacy JSON support paths are still in the decoder. Worth marking for removal at a 1.0 boundary, together with a migration-only flag. Carrying two codecs indefinitely increases the attack surface for bugs in the JSON decoder (stdlib, but still).

---

### 2.8 Collectors

#### [L-16] Custom-metrics Unix socket path is string-concatenated — *Low*
[collector.go:96](internal/collector/collector.go#L96): `sockPath := storageDir + "/kula.sock"`. Cosmetic — should be `filepath.Join(storageDir, "kula.sock")`. Will break subtly if `storageDir` ever ends with a trailing separator on some platform.

---

#### [L-17] `customCollector` chmod 0660 relies on group trust — *Low*
[custom.go:52](internal/collector/custom.go#L52). Anyone in the daemon's group can push arbitrary metric values. Metrics are filtered against `configuredNames`, so arbitrary injection is limited to configured chart groups — but a rogue process with group access can fabricate entire charts. For multi-user hosts, tighten to `0600` and document an explicit "to grant push access, add the user to the kula group" workflow.

---

#### [I-6] `nvidia.log` permission check is advisory only — *Informational*
[gpu_nvidia.go:29-32](internal/collector/gpu_nvidia.go#L29-L32). A warning is logged but the file is still read. On a shared host, a local user could write crafted CSV that poisons GPU stats (low value, low impact).

---

### 2.9 Frontend / XSS

#### [L-18] `renderMarkdownLite` reconstructs HTML from escaped text — *Low*
[ollama.js:565-600](internal/web/static/js/app/ollama.js#L565-L600). The function first HTML-escapes, then applies regex replacements that reintroduce raw tags (`<div class="ai-think">`, `<pre><code>`, tables). The regex pipeline looks safe in isolation — inputs are already escaped so `&lt;think&gt;…&lt;/think&gt;` is matched literally. However:

1. The function is exercised on streamed LLM output, which is trivially attacker-influenced when the user asks the model about user-provided data.
2. There's no test that constructs a malicious Markdown payload (e.g. triple-backtick with injected `</code></pre><script>…`) — worth adding.
3. Relying on regex-based Markdown parsing is always a smell; this is small enough to review, but any future feature additions (lists, links) will likely open a gap.

The current implementation appears safe given `escapeHTML` covers `& < > " '` and all regex replacements operate on the escaped string. Treat this as a hotspot for future audits.

**Recommendation:** add explicit tests for injection attempts; consider swapping to a mature, vetted library (marked.js / DOMPurify) only if that's a free call given the "no dependencies" goal — otherwise keep the current minimalism but freeze the grammar.

---

#### [I-7] `innerHTML` with template literals in session picker — *Informational*
[ollama.js:48](internal/web/static/js/app/ollama.js#L48): `select.innerHTML = \`<option value="${ollamaModel}">${ollamaModel}</option>\`;`. `ollamaModel` comes from config (admin-controlled) so this is safe today, but a future feature that populates model names from an upstream API without escaping would leak. Use `Option` constructor or `textContent` where possible.

---

### 2.10 Miscellaneous / Quality

#### [I-8] `gzipMiddleware` does not validate `Accept-Encoding` q-values — *Informational*
[server.go:166-167](internal/web/server.go#L166-L167). `strings.Contains(ae, "gzip")` matches `gzip;q=0` (which means "do NOT gzip"). Low impact because nothing Kula serves is harmful when gzipped; just spec-imprecise.

---

#### [I-9] `parseSize` uses `fmt.Sscanf` — *Informational*
[config.go:484-503](internal/config/config.go#L484-L503). `Sscanf` is permissive (accepts `"5MBjunk"` — returns 5, `"MB"` then ignores trailing). Use an explicit `regexp.MustCompile` or tokenise, so malformed size strings fail loudly.

---

#### [I-10] `CleanupSessions` holds the auth lock while purging rate-limit maps — *Informational*
[auth.go:256-274](internal/web/auth.go#L256-L274). Under sustained login load, login requests contend on the auth mutex with the cleanup tick. Purge the rate-limit maps under their own locks only, not under `a.mu`.

---

#### [I-11] `main.go:readPasswordWithAsterisks` allows `Ctrl+D` to submit empty password — *Informational*
[main.go:222-230](cmd/kula/main.go#L222-L230). EOF breaks the loop and returns whatever has been typed so far. A stray Ctrl+D during interactive setup yields a hashed empty string. Consider rejecting zero-length passwords explicitly.

---

#### [I-12] `cmd/kula/main.go:readPasswordWithAsterisks` prints `*` for every byte, including multi-byte UTF-8 — *Informational*
Typing an accented character prints two or more `*`. Cosmetic.

---

### 2.11 Performance

1. **`AuthManager.ValidateSession` uses a write lock** ([auth.go:185-202](internal/web/auth.go#L185-L202)) because of sliding expiration. Under heavy authenticated traffic, every request serialises through this mutex. Consider either:
   - Reading under `RLock` and upgrading only when expiration needs updating (most calls update, though, so the win is limited).
   - Tracking `atomic.Int64` for `expiresAt` per session.
2. **`calculateSRIs` runs synchronously in `NewServer`** ([server.go:772-796](internal/web/server.go#L772-L796)). SHA-384 over every JS file at startup. With the current ~15 files it's <10ms, but this grows; consider precomputing at build time.
3. **`QueryRangeWithMeta` cache cleared on every `WriteSample`** (every 1 s). Under heavy history-fetch load, cache hit rate is near zero. Consider caching per tier and invalidating only for tier-0.
4. **`tier.go:ReadLatest` allocates `make([]byte, 4)` per record for the length prefix**. A single `[4]byte` on the stack is free. Same pattern in `ReadRange`. Low-impact, hot path.
5. **`wsHub.broadcast` takes `RLock` then calls `client.mu.Lock` per client**. Lock ordering is fine, but N² lock acquisitions during broadcast. With 100 clients * 1 Hz * 365 days that's tolerable, but consider sharding the hub by client count if you ever lift `MaxWebsocketConns`.

---

## 3. Defense-in-Depth Suggestions (non-issues)

- **Expose a structured `security.md` threat model** next to the code. The current `SECURITY.md` is short; audit reviewers (and users) benefit from an explicit list of trust boundaries: attacker-on-LAN, attacker-on-same-host, malicious collaborator with group membership, compromised Ollama upstream, compromised config file.
- **Consider a periodic `SaveSessions`** (every N minutes, guarded by a dirty flag) so sessions survive a SIGKILL.
- **Add `go vet`/`staticcheck`/`gosec` to CI** if not already there (I didn't see a workflow for it in `.github/`). `gosec` would have flagged `M-5`, `L-7`, `L-8`.
- **Cut a Supply-Chain attestation**: you already ship SHA256s next to releases; SLSA v1 provenance via `gh attest` is nearly free given the current release workflow.
- **Integration test that serves the binary and hits every endpoint** including 401/403 paths. Unit tests cover most helpers but the end-to-end wiring around CSRFMiddleware + AuthMiddleware is where most regressions land.
- **Fuzz the binary codec** (`storage/codec.go`) with `go test -fuzz`. Any panic in the ring-buffer decoder takes the daemon down.

---

## 4. Consolidated Recommendations (prioritised)

| # | Severity | Area | Recommendation |
|--:|:--------:|------|----------------|
| 1 | Medium   | Sessions | Add absolute max session lifetime on top of sliding expiration (M-1). |
| 2 | Medium   | CSRF     | Enforce synchroniser token in no-auth mode as well (M-2). |
| 3 | Medium   | WS       | Reject empty Origin by default; gate the permissive path behind config (M-3). |
| 4 | Medium   | SSRF     | Resolve Ollama host to IPs and refuse non-loopback; consider per-request pinning (M-4). |
| 5 | Medium   | Secrets  | Enforce secure permissions on config.yaml when it contains secrets (M-5, L-6). |
| 6 | Low      | DSN      | Uniformly escape all libpq key-value fields (L-7). |
| 7 | Low      | Sandbox  | Reject dangerous paths in `Postgres.Host` before building Landlock rules (L-8). |
| 8 | Low      | Metrics  | Require a token when Prometheus endpoint is enabled, or loopback-only by default (L-9). |
| 9 | Low      | Storage  | Back up corrupted tier files before re-init; tighten per-record `dataLen` bound (L-14, L-15). |
|10 | Low      | XSS      | Add regression tests for `renderMarkdownLite` against crafted streamed Markdown (L-18). |
|11 | Low      | URLs     | Apply SSRF hardening to `nginx.status_url` (L-4). |
|12 | Low      | Auth     | Case-insensitive Bearer prefix match (L-1). |

---

## 5. Appendix: Files Reviewed

- `cmd/kula/main.go`, `cmd/kula/system_info.go`
- `internal/config/config.go`
- `internal/sandbox/sandbox.go`
- `internal/web/server.go`, `auth.go`, `websocket.go`, `prometheus.go`, `ollama.go`
- `internal/storage/codec.go` (partial), `store.go` (partial), `tier.go` (partial)
- `internal/collector/collector.go`, `types.go`, `custom.go`, `postgres.go`, `nginx.go`, `gpu.go`, `gpu_nvidia.go`, `system.go`, `disk.go` (partial)
- `internal/web/static/index.html`, `js/app/ollama.js`, `js/app/alerts.js`, `js/app/header.js`, `js/app/state.js`
- `addons/install.sh` (scanned; no issues beyond the standard "pipe-to-bash" caveat that the README already flags)

No findings required running code; all issues are derived from static review of `main` @ VERSION 0.15.1.
