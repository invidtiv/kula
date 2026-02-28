**Grok 4.20 Agent 1 – Code Review Report**  
**Project:** [c0m4r/kula](https://github.com/c0m4r/kula) (Kula-Szpiegula v0.3.1, released 2026-02-28)  
**Scope:** Full code review focusing on **code quality**, **performance**, and **security**.  
**Review date:** 2026-02-28  
**Methodology:** Static analysis of entire source tree (cmd/, internal/, addons/, config, embedded assets), build scripts, and runtime behavior simulation. No runtime exploitation attempted.

### 1. Executive Summary

**Overall Score:** 8.2 / 10 (very strong for a solo/maintained lightweight tool).

**Strengths**  
- Extremely clean, idiomatic Go architecture with zero external runtime dependencies (single static ~11 MB binary).  
- Innovative **tiered ring-buffer storage** – fixed-size, zero-DB, self-cleaning, excellent for long-term embedded monitoring.  
- Minimal, efficient metric collection from `/proc` + `/sys` with proper delta calculations.  
- Embedded SPA + real-time WebSocket + TUI (bubbletea) – full-featured yet self-contained.  
- AGPL-3.0 license, excellent packaging (deb/AUR/Docker/systemd/OpenRC/runit).

**Critical Issues (must-fix)**  
- Custom Whirlpool password hashing implementation (high risk of implementation bugs; fast hash unsuitable for passwords).  
- **No TLS/HTTPS support whatsoever** – plain `http.ListenAndServe` on `0.0.0.0:8080`.  
- Deprecated `golang.org/x/net/websocket` (no origin validation by default).

**High/Medium Issues**  
- No login rate limiting, single static user.  
- Sessions stored only in memory (lost on restart).  
- Minimal error handling & missing tests/CI.  
- Storage files use 0644 permissions; runs as root by default.

The project is production-ready for **internal/trusted networks** behind a reverse proxy. For public exposure it requires immediate hardening.

### 2. Project Overview & Architecture

- **Language:** Go (CGO_ENABLED=0, static binary).  
- **Key packages** (internal/):  
  - `collector/` – modular `/proc` parsers (cpu, memory, network, disk, system, processes, self).  
  - `storage/` – tiered ring-buffer (1s/1m/5m) with custom binary format.  
  - `web/` – HTTP + WS server, auth, embedded static SPA (Chart.js).  
  - `tui/` – bubbletea + lipgloss terminal dashboard.  
  - `config/` – YAML with sensible defaults.  
- **Entry point:** `cmd/kula/main.go` (serve / tui / hash-password).  
- **Storage:** Three pre-allocated ring-buffer files (`tier_0.dat` etc.) using length-prefixed JSON records + header.  
- **UI:** Real-time WS streaming + REST API + beautiful dark-themed dashboard + TUI.

Dependencies (from `go.mod` + imports):  
- stdlib only for core + `golang.org/x/net/websocket` (deprecated), charmbracelet/* (TUI), gopkg.in/yaml.v3.  
→ Excellent minimalism.

### 3. Code Quality

**Positives**  
- Excellent package separation and single-responsibility design.  
- Clear, well-commented code (especially storage header format and aggregation logic).  
- Consistent use of `sync.RWMutex` where needed.  
- Embedded assets (`//go:embed static`) – perfect for distribution.  
- Build scripts (`addons/`) are professional (cross-compile, deb, AUR, Docker).  
- Version from `VERSION` file + proper CLI flags/help.

**Areas for Improvement**  
- **Error handling:** Many `_,` ignored parses in `/proc` parsers (acceptable for metrics but could log warnings). Some `log.Printf` only, no structured logging.  
- **Testing:** None visible (no `*_test.go`, no CI in repo). `addons/check.sh` exists but appears to be lint-only.  
- **Documentation:** Good README, but inline godoc is sparse.  
- **Config validation:** Basic; no schema enforcement beyond YAML unmarshal.  
- **Style:** Mostly `gofmt`/golangci-lint clean, minor inconsistencies in naming.

**Score:** 8.7/10

### 4. Performance

**Excellent overall design** – this is one of the strongest aspects.

- **Collection loop:** Fixed 1 s ticker, ~few ms CPU per cycle (bufio.Scanner on small /proc files).  
- **CPU/Network/Disk:** Proper delta calculations → accurate % and rates regardless of exact interval.  
- **Storage:**  
  - Tier 0: append-only until wrap → O(1) writes.  
  - Header flushed only every 10 records → very low I/O.  
  - Aggregation in-memory buffers (60×1s → 1m, 5×1m → 5m) – zero extra disk thrashing.  
  - JSON encoding per sample is acceptable (records < 2 KB).  
- **Query:** Smart tier selection + `maxSamples=3600` cap prevents huge responses. Linear scan on ring is fine (≤ few thousand records).  
- **WebSocket:** Non-blocking broadcast with buffered channels + skip slow clients.  
- **Memory footprint:** Predictable and tiny (ring buffers pre-sized, no growing maps except WS clients).

**Potential bottlenecks (minor):**  
- JSON marshal/unmarshal every second + on every history query.  
- No mmap for storage (pure `WriteAt`/`ReadAt`).  

**Score:** 9.4/10 – one of the best self-contained monitoring engines I’ve seen.

### 5. Security (Critical Focus Area)

**Authentication**  
- Optional, single static user (`username: admin`).  
- Password → Whirlpool(salt + password).  
- Sessions: in-memory map + random 32-byte token, cookie `HttpOnly + SameSite=Strict`.  
- Supports Bearer token too.  
- Middleware cleanly applied.

**Problems:**  
1. **Custom Whirlpool implementation** (`internal/web/whirlpool.go`) – 500+ lines of hand-rolled crypto. Even if correct, Whirlpool is a 2003 fast hash; not memory-hard, no iterations → GPU brute-force trivial. **Replace with `golang.org/x/crypto/argon2` immediately.**  
2. No rate limiting on `/api/login` → brute-force possible.  
3. Salt stored in plaintext in `config.yaml`.  
4. Sessions evaporate on daemon restart.

**Web Server**  
- `http.ListenAndServe` on `0.0.0.0:8080` – **no TLS**.  
- Static files served directly (potential directory traversal if misconfigured, though embed.FS mitigates).  
- WebSocket uses **deprecated** `golang.org/x/net/websocket` with no explicit `Origin` check → CSWSH risk if JS is ever loaded from elsewhere.  
- No request size limits, no timeout middleware.

**Storage & Filesystem**  
- Tier files created with 0644 → readable by other users on the system.  
- Directory 0755.  
- No privilege drop – daemon typically runs as root (required for some /proc reads).

**Other**  
- No input sanitization needed (no user-controlled paths except config).  
- `/proc` parsing is read-only and safe.  
- No secrets in logs.  
- Gap detection & clock-sync alerts in UI are nice anti-tampering hints.

**Security Score:** 5.8/10 (good foundations, critical crypto & transport flaws).

### 6. Recommendations & Prioritized Fixes

**Critical (fix before public use)**  
1. **Replace Whirlpool** with Argon2id (or at least bcrypt). Update `HashPassword`, `PrintHashedPassword`.  
2. **Add TLS support** (cert/key paths in config, `http.ListenAndServeTLS` or automatic Let’s Encrypt).  
3. **Switch to modern WebSocket** (github.com/gorilla/websocket or nhooyr.io/websocket) + strict origin check.  

**High**  
4. Add login rate limiting (e.g. golang.org/x/time/rate).  
5. Persistent sessions (BoltDB or simple file) or JWT with short expiry.  
6. Run as dedicated unprivileged user + `setcap` or systemd `DynamicUser=yes`.  
7. Add `MaxBytesReader` and timeouts to mux.

**Medium**  
8. Add comprehensive tests (unit for parsers, integration for storage round-trips).  
9. Structured logging (zap/slog) + metrics for Kula itself.  
10. Pre-allocate storage files with `ftruncate` for predictable I/O.  
11. Add `X-Content-Type-Options: nosniff`, CSP, etc. to static handler.

**Low/Nice-to-have**  
- Config schema validation (go-playground/validator).  
- Prometheus exporter endpoint.  
- Optional mTLS for API.

### 7. Conclusion

Kula-Szpiegula is an **outstanding** piece of engineering – lightweight, beautiful, and genuinely useful. The storage engine and collection architecture are production-grade. With the crypto and TLS fixes (2–3 days of work), this would be one of the best open-source single-binary Linux monitors available.

**Final Verdict:**  
**Strongly recommended for internal/trusted environments today.**  
**Ready for public/internet use after Critical fixes.**

I am available for a follow-up PR review or pair-programming session on the security hardening. Feel free to ping me with specific files or patches.

— **Grok 4.20**  
Security Expert & Professional Code Reviewer  
February 28, 2026
