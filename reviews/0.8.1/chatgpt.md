# Kula — Code audit (quality · performance · security)

**Scope:** full repository review of `kula` (server monitor) — code layout, major modules (collection, storage, web, auth, sandbox, CLI), performance characteristics, and security posture. I inspected the public repository and key source files (main CLI, config, storage, tier format, web server, auth manager, collector, sandbox).
**Repo / owner:** c0m4r on GitHub. ([GitHub][1])

---

# Executive summary (tl;dr)

Kula is a compact, single-binary Linux monitoring agent that: samples `/proc` and `/sys` every second, persists samples into a multi-tier ring-buffer on disk, and exposes a REST + WebSocket UI. The implementation is pragmatic and readable, with many thoughtful design choices (tiered storage, memory cache for latest sample, aggregation, Landlock sandbox, Argon2 password hashing). The main risks I found are **storage durability / crash-atomicity tradeoffs**, **HTTP/CSP/static resource considerations**, **cookie/session deployment pitfalls (proxy/TLS assumptions)**, and a few performance/robustness edges (header write cadence, disk I/O patterns, and restart aggregation corner cases). Overall code quality is good for a compact Go project; security is consciously considered but needs a few configuration/runtime hardening steps. ([GitHub][2])

---

# How I inspected (methodology)

* Read top-level project README and key modules: `cmd/kula/main.go`, `internal/config`, `internal/storage` (store + tier implementation), `internal/web` (server + auth), `internal/collector`, and `internal/sandbox`. ([GitHub][1])
* Focused on correctness, concurrency, error handling, worst-case behavior (disk full, process restart), and web/auth security surface.

---

# Repository & architecture notes

* Single-binary Go app with an embedded SPA (static files embedded via `//go:embed static`). CLI supports `serve`, `tui`, `hash-password`, `inspect`. Startup enforces a **Landlock** sandbox (best-effort). Main collection loop samples once per `cfg.Collection.Interval` and writes to storage then broadcasts via WebSocket. ([GitHub][2])

---

# Detailed findings and recommendations

## 1) Storage engine — correctness, performance, durability

**What it is:** tiered ring-buffer binary files. Each tier holds a header (64 bytes) + data region with length-prefixed records. Tier writes append with wrap-around and write a zero-length sentinel when wrapping. The `Tier` code optimizes reads via `io.SectionReader` + buffered reader and uses a periodic header update (every 10 writes). ([GitHub][3])

**Strengths**

* Compact file format with metadata header (magic, version, maxData, offsets, timestamps).
* Read path optimizations (timestamp pre-extraction, SectionReader + large buffer).
* Warm-start logic in `storage.Store` that rebuilds in-memory aggregation state from tails of tiers to avoid losing partially-aggregated intervals. ([GitHub][4])

**Concerns / Risks**

1. **Crash/Power-loss & atomicity:** writes to record data are not protected by fsync; header is only written every 10 writes. If the process or machine crashes, the file may be left in an inconsistent state (partial record written, header stale). The code does write a zero-length sentinel before wrapping, but that does not protect against mid-record truncation or OS caching.
   **Mitigation:** provide an option to `fsync` periodically or on shutdown, or flush header on every write if durability is required. Make the transaction model explicit in docs: expected data loss window vs. durability cost.

2. **Concurrent process access:** `Tier` uses in-process mutexes but no inter-process locking (flock). If multiple instances point to the same storage directory, data corruption will follow. Documentically restrict to a single instance or add file locking.
   **Mitigation:** check and fail if another instance is running (pidfile) or implement OS file locks (advisable for environments with possible accidental duplicate runs).

3. **Header size and offsets assumptions:** header layout uses 64-bit little-endian integers; cross-platform OK, but if file gets truncated/corrupted the reader resets header — good. Add a stronger CRC or header checksum to detect stealth corruption.
   **Mitigation:** optional header checksum/versioning to detect accidental truncation vs. intended wrap.

4. **Large record protection:** the code checks `recordLen > maxData` and rejects; good.

**Performance notes**

* Using large buffered readers for range scans is good. The header-write cadence (every 10 writes) is an explicit tradeoff: lower disk I/O vs. potential for losing up to ~9 records + header mismatch window. Make this tradeoff configurable (e.g., headerFlushPeriod).

---

## 2) Aggregation & memory buffers (Store)

* Aggregation logic aggregates `tier0` -> `tier1` (ratio derived from resolutions) and `tier1` -> `tier2`. On startup, `reconstructAggregationState()` restores in-memory buffers by reading the tails of lower tiers. This is thoughtful and minimizes drop of incomplete intervals. ([GitHub][4])

**Edge cases**

* If `Collection.Interval` is altered in config between runs, the computed ratios might mismatch historical data; `reconstructAggregationState()` uses timestamps to decide which samples are pending which is right — but document that changing tier resolutions on an existing data directory may lead to unexpected aggregation/resampling behavior.
* If sample timestamps jump backward (clock changes), aggregation buffers use `Timestamp` and `Duration` assumptions — there is fallback in `WriteSample` for non-positive durations, but be aware of clock-drift or NTP step events. Consider monotonic time or detect large clock jumps and handle specially.

---

## 3) Web server, API, and static UI

**Implementation notes**

* HTTP server builds an API (`/api/current`, `/api/history`, `/api/config`, `/api/login`, `/ws`) and a WebSocket hub for live streaming. Static files served with `http.FileServer` from embedded FS. Logging, gzip and security middleware are applied. ([GitHub][5])

**Security / hardening observations**

1. **CSP header generation:** `securityMiddleware` creates a CSP header with a generated `nonce` (random per-request) and sets `script-src 'self' 'nonce-<nonce>'`. However, the server **does not** inject that nonce into the served HTML files (because those are static embedded files) unless the frontend template expects it. If the SPA contains inline `<script>` tags that require the nonce, those scripts will be blocked. Conversely, if the frontend relies only on self-hosted script files (external `<script src="...">`), the `nonce` is unnecessary.
   **Action:** verify that the embedded SPA consumes the per-request CSP nonce (i.e., HTML injects `nonce` into inline script tags) or change CSP to match the actual front-end (prefer `script-src 'self'` and avoid `unsafe-inline`). Also avoid allowing `fonts.googleapis.com` in CSP unless you intentionally load fonts from Google (this leaks a request to an external provider). ([GitHub][5])

2. **TrustProxy handling & client IP:** `getClientIP` supports `TrustProxy`. If `trust_proxy` is enabled without a secure reverse proxy, the app may trust spoofed `X-Forwarded-For`. The server logs warnings when TrustProxy is set — good. Make sure to set `trust_proxy` only when a trusted reverse proxy is in front and it strips unknown headers. ([GitHub][5])

3. **CORS & CSRF:** There's no CORS setup (expected for same-origin SPA). Cookies use `SameSite=Strict` which reduces CSRF risk for the cookie-based session. But if you allow cross-origin use or external dashboards, consider adding CSRF tokens to state-modifying endpoints. Login and logout are POSTs — good.

4. **Content leakage / external resources:** CSP allows Google fonts (fonts.googleapis.com / fonts.gstatic.com). That will cause client browsers to fetch external resources exposing server/client to third-party trackers. Consider bundling fonts or offering an option to disable external resources.

5. **Error messages & JSON responses:** Some handlers return `fmt.Sprintf` with `%s` directly inside JSON error responses (e.g., `http.Error(w, fmt.Sprintf(\`{"error":"%s"}`, err), http.StatusInternalServerError)`). Ensure `err` is sanitized to avoid leaking internal details to unauthenticated clients (or at least only in verbose / debug mode).

**Performance**

* `/api/history` loads data via `store.QueryRangeWithMeta`. The server measures and logs load time when `perf` logging is enabled — good for diagnosing slow queries. Consider paginating large ranges or adding a maximum `points` value/configurable cap to avoid heavy queries from the UI. ([GitHub][5])

---

## 4) Authentication & session management

**What it does**

* Auth is optional. When enabled, username/password is validated using Argon2id with stored salt & hash. Sessions are random tokens (32 bytes hex), but the server stores **sha256(token)** as keys in memory and on-disk. Session cookie `kula_session` set with `HttpOnly`, `MaxAge`, `SameSite=Strict`, and `Secure` if TLS or TrustProxy indicates HTTPS. Sessions are bound to IP + User-Agent; sliding expiration is implemented. Sessions persist to `sessions.json` with file perms `0600`. ([GitHub][6])

**Strengths**

* Use of Argon2id for password hashing with configurable parameters.
* Tokens are random and stored only as hash on disk (good).
* Session cookie flags are set (HttpOnly, SameSite Strict), and SaveSessions uses 0600 — good.

**Concerns / suggestions**

1. **IP binding of sessions:** sessions require the IP to match. This improves security but can break legit users behind dynamic IPs or clients whose IPs change (mobile networks, some proxies). Document this behavior and allow disabling IP binding by configuration if necessary.

2. **Rate limiting granularity:** rate limiter is per-IP and uses an in-memory map; this can be evaded by distributed attackers. For hardened environments, consider adding more robust rate limiting / fail2ban integration / throttle by username attempt as well.

3. **Session persistence and replay:** sessions saved are keyed by hashed token, so if someone obtains the `sessions.json` file they still have hashed tokens saved; but because the cookie holds the raw token (only raw token proves auth), an attacker who can read the file cannot directly impersonate unless they also can present the raw token. However leakage of `sessions.json` still reveals usernames, IPs, and user agents. Ensure storage directory permissions and consider encrypting session blobs at rest (optional).

4. **Cookie Secure flag & TrustProxy:** `Secure` cookie is set based on `r.TLS != nil` OR `(trustProxy && X-Forwarded-Proto == "https")`. This is pragmatic, but it *requires* correct proxy configuration. If a reverse proxy is misconfigured (or `trust_proxy` set incorrectly) cookies might be set insecurely. Document deployment requirements.

---

## 5) Sandbox (Landlock) usage

* The app attempts to enforce Landlock filesystem and bind restrictions via a BestEffort V5 call. This is an excellent defense-in-depth approach and will gracefully degrade on older kernels. Note: kernel support varies (Landlock v5 requires very recent kernels; BestEffort handles fallback). Ensure deployment kernels support the required features or accept the loss of sandboxing. Also, Landlock typically does not require CAPs, but kernel config must enable it. ([GitHub][7])

**Suggestion:** add explicit startup logging that reports whether sandbox restrictions were applied (the code logs a warning on failure already — good). For high-security deploys, consider additional process isolation (systemd sandboxing, containers with explicit seccomp profiles, SELinux).

---

## 6) CLI and operational concerns

* `kula hash-password` helps generate Argon2 params & salt. `config` default directory fallback to `$HOME/.kula` if `/var/lib/kula` not writable — good UX. `runInspectTier` prints tier metadata. ([GitHub][2])

**Operational suggestions**

* Provide a `--fsync` or `--durability` flag to allow administrators to choose stronger durability at the cost of I/O.
* Add a PID file or file lock on startup to prevent multiple instances using same storage.
* Add a `--dry-run` config validation mode (checks storage, permissions, Argon2 settings) if not present.

---

## 7) Code quality, structure, and maintainability

**Positives**

* Clear package separation: `collector`, `storage`, `web`, `config`, `sandbox`, `tui`. Functions are generally small and focused.
* Good use of `sync.RWMutex` for concurrency; careful about protecting shared state (`latestCache`, `sessions`, `tiers`).
* Documentation and README are comprehensive; defaults are sensible. ([GitHub][1])

**Improvements**

* Add more comments on invariants (e.g., how `writeOff`, `count`, `wrapped` relate).
* Increase test coverage on storage corruption recovery and partial-write scenarios (unit tests for `Tier` behavior on partial writes / truncated files).
* Consider adding benchmarks and CI checks that simulate crash/restart patterns (there are `addons/benchmark.sh` scripts mentioned; extend to include crash scenarios). ([GitHub][1])

---

# Concrete action items (priority ordered)

### High priority (should address before production)

1. **Document and optionally expose a durability mode** — either `--fsync` or a configurable header-flush frequency. Explain the risk: header updated every 10 writes by default (possible loss window). (Storage: `internal/storage/tier.go`.) ([GitHub][3])
2. **Add file locking or single-instance guard** to prevent multiple processes from writing the same tier files. (Storage: `NewStore` / `OpenTier`.) ([GitHub][4])
3. **Verify CSP / static UI behavior** — ensure the per-request CSP nonce is actually used by the embedded static HTML (or simplify CSP to match the front-end). Remove unnecessary external font origins unless intended. (Web: `securityMiddleware`). ([GitHub][5])

### Medium priority

4. **Make IP-binding of sessions configurable** (or provide a compatibility mode) and document `trust_proxy` requirements. (Auth / server). ([GitHub][6])
5. **Sanitize error messages** returned in JSON responses so internal errors are not exposed unless in a debug mode. (Web handlers.) ([GitHub][5])

### Low priority / nice-to-have

6. Add header checksum on tier files to detect partial corruption. (Storage.) ([GitHub][3])
7. Add optional encryption of `sessions.json` or recommend filesystem-level encryption for sensitive environments. (Auth.) ([GitHub][6])
8. Add extended tests simulating truncated writes, crash recovery, and cold-start aggregation corner cases. (Storage & Store.)

---

# Example small code suggestions

**1) Make header flush configurable (pseudocode)** — reduce `t.count%10` magic:

```go
// in Tier struct:
headerFlushEvery uint64 // set from config

// in Write:
if t.count % t.headerFlushEvery == 0 {
    return t.writeHeader()
}
```

Document default (10), allow admin to increase/decrease.

**2) Add simple PID lock at NewStore start**

```go
pidFile := filepath.Join(absDir, "kula.pid")
if _, err := os.Stat(pidFile); err == nil {
    // Optionally read PID and check process exists, else fail
}
os.WriteFile(pidFile, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0644)
// On Close(), remove pid file
```

(This is a simple mitigation; better to use file locking.)

---

# Final risk summary

* **Data durability:** configurable tradeoff; default saves header every 10 writes → possible recent-sample loss after crash. (Medium risk for monitoring; high if you rely on long-term guarantees.) ([GitHub][3])
* **Auth/session robustness:** good defaults (Argon2, HttpOnly cookie, SameSite), but IP binding, TrustProxy and session persistence require careful deployment. (Medium risk if misconfigured). ([GitHub][6])
* **Web UI CSP and external resources:** potential leakage via fonts.googleapis.com and possible mismatch between server-generated nonce and embedded HTML (affects XSS protection). (Medium risk). ([GitHub][5])
* **Sandboxing:** Landlock usage is a strong positive; verify kernel support on target hosts. (Low risk if kernel doesn't support — sandbox will simply not be enforced). ([GitHub][7])

---

# Wrap-up

Kula is well-designed for its purpose: compact, pragmatic, and with many security-minded features. The codebase is readable and modular. Addressing the few operational and durability items above will harden the project further and reduce surprises in production. If you want, I can:

* produce a targeted patchset (example PR) for: (a) configurable header flush; (b) simple PID file or file lock; (c) safer CSP configuration & documentation; or
* run a deeper audit of the front-end static files to verify CSP nonce usage and any XSS risks.

Which of those would you like first? (I can start with the storage durability patch and show a concrete diff.) ⚙️

[1]: https://github.com/c0m4r/kula "GitHub - c0m4r/kula: Lightweight, self-contained Linux® server monitoring tool · GitHub"
[2]: https://raw.githubusercontent.com/c0m4r/kula/main/cmd/kula/main.go "raw.githubusercontent.com"
[3]: https://raw.githubusercontent.com/c0m4r/kula/main/internal/storage/tier.go "raw.githubusercontent.com"
[4]: https://raw.githubusercontent.com/c0m4r/kula/main/internal/storage/store.go "raw.githubusercontent.com"
[5]: https://raw.githubusercontent.com/c0m4r/kula/main/internal/web/server.go "raw.githubusercontent.com"
[6]: https://raw.githubusercontent.com/c0m4r/kula/main/internal/web/auth.go "raw.githubusercontent.com"
[7]: https://raw.githubusercontent.com/c0m4r/kula/main/internal/sandbox/sandbox.go "raw.githubusercontent.com"
