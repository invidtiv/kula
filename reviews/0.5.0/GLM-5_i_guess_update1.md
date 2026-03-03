# Code Review Report: Kula v0.5.0

**Lightweight Linux Server Monitoring Tool**

Repository: https://github.com/c0m4r/kula

Version Reviewed: 0.5.0

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Version History and Improvements](#2-version-history-and-improvements)
3. [Project Overview](#3-project-overview)
   - 3.1 [Project Structure](#31-project-structure)
   - 3.2 [Technology Stack](#32-technology-stack)
4. [Code Quality Analysis](#4-code-quality-analysis)
   - 4.1 [Architecture and Design Patterns](#41-architecture-and-design-patterns)
   - 4.2 [Error Handling](#42-error-handling)
   - 4.3 [Code Readability and Maintainability](#43-code-readability-and-maintainability)
   - 4.4 [Testing Coverage](#44-testing-coverage)
5. [Performance Analysis](#5-performance-analysis)
   - 5.1 [Metric Collection Efficiency](#51-metric-collection-efficiency)
   - 5.2 [Storage Engine Performance](#52-storage-engine-performance)
   - 5.3 [Web Server Performance](#53-web-server-performance)
   - 5.4 [Memory Management](#54-memory-management)
6. [Security Analysis](#6-security-analysis)
   - 6.1 [Authentication and Authorization](#61-authentication-and-authorization)
   - 6.2 [Input Validation and Injection Prevention](#62-input-validation-and-injection-prevention)
   - 6.3 [Landlock Sandbox](#63-landlock-sandbox)
   - 6.4 [Network Security](#64-network-security)
   - 6.5 [WebSocket Security](#65-websocket-security)
7. [Detailed Findings](#7-detailed-findings)
   - 7.1 [Critical Issues](#71-critical-issues)
   - 7.2 [High Priority Recommendations](#72-high-priority-recommendations)
   - 7.3 [Medium Priority Recommendations](#73-medium-priority-recommendations)
   - 7.4 [Low Priority Recommendations](#74-low-priority-recommendations)
8. [Summary Assessment](#8-summary-assessment)
9. [Conclusion](#9-conclusion)

---

## 1. Executive Summary

Kula (Kula-Szpiegula) is a lightweight, self-contained Linux server monitoring tool written in Go. This review examines version 0.5.0, which represents a significant evolution from earlier releases. The project has undergone substantial security hardening and performance improvements across multiple releases, demonstrating active and responsive development practices.

The project demonstrates exceptional progress in addressing security concerns identified in previous reviews. Key improvements include the implementation of rate limiting on authentication endpoints, migration from deprecated WebSocket libraries, Cross-Site WebSocket Hijacking (CSWSH) prevention, graceful shutdown handling, and dual-stack IPv4/IPv6 support. The codebase maintains clean architecture principles while adding robust security features.

This review finds that Kula has matured into a production-ready monitoring solution with comprehensive security measures. The development team has shown commendable responsiveness to security feedback, implementing critical protections while maintaining the project's lightweight, zero-dependency philosophy.

---

## 2. Version History and Improvements

The CHANGELOG reveals a rapid development cycle with significant security and feature improvements. Below is a summary of key changes since the initial release:

### Version 0.5.0 (Latest Major Release)

| Category | Changes |
|----------|---------|
| **Added** | Dual-stack IPv4 and IPv6 support |
| **Added** | Storage directory fallback for permission issues |
| **Added** | Chart.js static library embedded |
| **Changed** | Logo and typography updates |
| **Changed** | Chart.js updated to version 4.5.1 |
| **Changed** | Updated helper scripts |

### Version 0.4.1 (Critical Security Release)

| Category | Changes |
|----------|---------|
| **Security** | Rate limiting on `/api/login` endpoint (max 5 attempts per 5 minutes) |
| **Security** | Strict absolute path validation for storage directory traversal prevention |
| **Security** | Safe wrappers for `/proc` collectors with explicit malformed data logging |
| **Security** | Migrated from deprecated `golang.org/x/net/websocket` to `github.com/gorilla/websocket` |
| **Security** | Strict Origin validation for CSWSH prevention |
| **Fixed** | 100% CPU exhaustion in browser on 1h time window |
| **Fixed** | Zoom resolution on coarse-resolution time ranges |
| **Changed** | Graceful shutdown using `context.Context` signal catching |

### Version 0.4.0 (Major Feature Release)

| Category | Changes |
|----------|---------|
| **Added** | Landlock sandboxing implementation |
| **Added** | API request logging |
| **Added** | Time range info when zooming |
| **Added** | Mock data generator |
| **Changed** | Buffered I/O Streams optimization |
| **Security** | Fixed XSS vulnerability in web UI system info display |
| **Security** | Fixed insecure Auth Session cookie with dynamic Secure attribute |
| **Security** | Migrated from Whirlpool to Argon2id password hashing |
| **Security** | Fixed critical RLock and Delete map panic in ValidateSession |
| **Security** | Fixed weak session token generation crash |
| **Security** | Added CSP, Frame-Options, Content-Type security headers |
| **Security** | WebSocket MaxPayloadBytes limits |
| **Security** | Upper bounds checks for 31+ day historical queries |

### Version 0.3.0 and Earlier

| Category | Changes |
|----------|---------|
| **Added** | Cross-compile for amd64, arm64, riscv64 |
| **Added** | Debian and Arch packaging scripts |
| **Added** | Docker integration |
| **Added** | Unit tests |
| **Changed** | Default storage tiers (250/150/50 MB) |
| **Changed** | RAM usage optimizations with max sample count cap |

---

## 3. Project Overview

### 3.1 Project Structure

The project follows standard Go project layout conventions with a clean separation between application entry points, internal packages, and deployment artifacts. The structure has evolved to include better organization with the main entry point now properly integrated with version management through a dedicated package.

```
kula/
├── cmd/kula/
│   └── main.go                 # CLI entry point with version integration
├── kula.go                     # Root package with Version constant
├── internal/
│   ├── collector/              # Metric collectors
│   ├── config/                 # YAML config with storage fallback
│   ├── storage/                # Tiered ring-buffer engine
│   ├── tui/                    # Terminal UI
│   ├── sandbox/                # Landlock sandboxing
│   └── web/                    # HTTP/WebSocket server
│       ├── server.go           # Dual-stack listener support
│       ├── websocket.go        # Gorilla WebSocket with CSWSH protection
│       └── auth.go             # Argon2id + Rate limiting
├── addons/                     # Build scripts and deployment
├── docs/                       # Man page and completions
└── VERSION                     # Single source of truth
```

### 3.2 Technology Stack

| Component | Technology |
|-----------|------------|
| Language | Go (Golang) - CGO_ENABLED=0 for static binary |
| Web Framework | Standard library net/http with gorilla/websocket |
| Frontend | Vanilla JavaScript with Chart.js 4.5.1, embedded in binary |
| Storage | Custom ring-buffer files with JSON encoding |
| TUI Framework | Bubble Tea with Lipgloss styling |
| Sandboxing | Landlock LSM V5 via go-landlock library |
| Configuration | YAML with gopkg.in/yaml.v3 |
| Authentication | Argon2id password hashing with rate limiting |
| Network | Dual-stack IPv4/IPv6 support |

---

## 4. Code Quality Analysis

### 4.1 Architecture and Design Patterns

The codebase demonstrates mature architectural principles with clear separation of concerns and proper use of Go idioms. The latest version shows significant improvements in code organization, particularly the integration of version management through a root-level package constant.

**Strengths:**

- Clean package boundaries with single responsibility principle
- Effective use of Go interfaces for abstraction
- Proper concurrent access patterns with `sync.RWMutex`
- Context-based graceful shutdown implementation
- Well-structured dual-stack network listener factory pattern
- Comprehensive documentation comments

**Notable Improvements from Previous Review:**

The codebase has addressed several architectural concerns:

```go
// main.go - Context-based signal handling
ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
defer stop()

// Graceful shutdown with timeout
shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
if err := server.Shutdown(shutdownCtx); err != nil {
    log.Printf("Web server shutdown error: %v", err)
}
```

The dual-stack listener implementation in server.go demonstrates sophisticated network programming:

```go
func (s *Server) createListeners() ([]net.Listener, error) {
    // Empty string: explicit dual-stack, one listener per family
    if listen == "" {
        ln4, err := net.Listen("tcp4", fmt.Sprintf("0.0.0.0:%d", port))
        ln6, err := net.Listen("tcp6", fmt.Sprintf("[::]:%d", port))
        return []net.Listener{ln4, ln6}, nil
    }
    // ... IPv6 address handling with proper tcp6 network type
}
```

### 4.2 Error Handling

Error handling throughout the codebase demonstrates defensive programming practices. The storage fallback mechanism is particularly noteworthy:

```go
// config.go - Storage directory fallback
func checkStorageDirectory(cfg *Config) error {
    if cfg.Storage.Directory == "/var/lib/kula" {
        if err := os.MkdirAll(cfg.Storage.Directory, 0755); err != nil || !isWritable(cfg.Storage.Directory) {
            homeDir, err := os.UserHomeDir()
            if err == nil {
                fallbackDir := filepath.Join(homeDir, ".kula")
                log.Printf("Notice: Insufficient permissions for /var/lib/kula, falling back to %s", fallbackDir)
                // ...
            }
        }
    }
    return nil
}
```

The tier.go implementation handles corrupted headers gracefully by reinitializing:

```go
if info.Size() >= headerSize {
    if err := t.readHeader(); err != nil {
        // Corrupted header — reinitialize
        t.writeOff = 0
        t.count = 0
        if err := t.writeHeader(); err != nil {
            _ = f.Close()
            return nil, err
        }
    }
}
```

### 4.3 Code Readability and Maintainability

The codebase maintains high readability standards with descriptive naming and clear code organization. The CHANGELOG is now comprehensive and follows [Keep a Changelog](https://keepachangelog.com/) format, which significantly improves maintainability and release tracking.

**Documentation Improvements:**

- Comprehensive CHANGELOG with categorized changes (Added, Changed, Fixed, Security)
- README includes SHA256 checksums for release verification
- Security-related changes are clearly marked in version history
- Man page and bash completion included

### 4.4 Testing Coverage

The project includes unit tests with race condition detection enabled. The mock data generator added in 0.4.0 facilitates testing without requiring actual system metrics.

---

## 5. Performance Analysis

### 5.1 Metric Collection Efficiency

Metric collection remains highly efficient, reading directly from `/proc` and `/sys` without spawning external processes. The collector maintains minimal state for rate calculations.

### 5.2 Storage Engine Performance

The tiered ring-buffer storage engine continues to be a performance highlight:

**Key Performance Features:**

- Pre-allocated files with fixed maximum sizes (predictable disk usage)
- Buffered I/O with 1MB buffer for read operations
- Fast timestamp extraction for pre-filtering (avoids full JSON deserialization)
- Header updates only every 10 writes (reduces disk I/O)
- Efficient binary format with length-prefixed records

**New Optimization - Oldest Timestamp Tracking:**

The tier.go now correctly tracks the oldest timestamp even after the ring buffer wraps:

```go
// When the ring buffer has wrapped, oldestTS must track the actual oldest
// surviving record, which is the one now sitting at writeOff
if t.wrapped {
    if ts, err := t.readTimestampAt(t.writeOff % t.maxData); err == nil {
        t.oldestTS = ts
    }
}
```

This enables efficient query routing without scanning entire files.

### 5.3 Web Server Performance

**Dual-Stack Network Support:**

The new `createListeners()` function provides flexible network binding:

- Empty string → dual-stack (tcp4 + tcp6 listeners)
- `[::]` → single tcp6 listener (kernel decides v4/v6)
- `0.0.0.0` → single tcp4 listener (v4 only)
- Specific addresses → bound accordingly

**WebSocket Improvements:**

The migration to gorilla/websocket brings performance benefits:

```go
var upgrader = websocket.Upgrader{
    ReadBufferSize:  1024,
    WriteBufferSize: 1024,
    // ...
}

// Ping/pong for connection health
ticker := time.NewTicker(50 * time.Second)
conn.SetPongHandler(func(string) error {
    _ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
    return nil
})
```

### 5.4 Memory Management

Memory management remains efficient with bounded buffers:

- WebSocket clients: 64-message send buffer with non-blocking skip
- Read limit: 4096 bytes per WebSocket message
- Tier query results: max 3600 samples cap

---

## 6. Security Analysis

### 6.1 Authentication and Authorization

The authentication system has been significantly hardened in recent releases.

**Password Hashing:**

Argon2id implementation with appropriate parameters:

```go
func HashPassword(password, salt string) string {
    timeParam := uint32(1)
    memory := uint32(64 * 1024)  // 64 MB
    threads := uint8(4)
    keyLen := uint32(32)

    hash := argon2.IDKey([]byte(password), []byte(salt), timeParam, memory, threads, keyLen)
    return hex.EncodeToString(hash)
}
```

**Rate Limiting - NEW:**

Login endpoint now implements rate limiting to prevent brute force attacks:

```go
type RateLimiter struct {
    mu       sync.Mutex
    attempts map[string][]time.Time
}

func (rl *RateLimiter) Allow(ip string) bool {
    rl.mu.Lock()
    defer rl.mu.Unlock()

    now := time.Now()
    cutoff := now.Add(-5 * time.Minute)

    var recent []time.Time
    for _, t := range rl.attempts[ip] {
        if t.After(cutoff) {
            recent = append(recent, t)
        }
    }

    if len(recent) >= 5 {
        return false
    }

    rl.attempts[ip] = append(recent, now)
    return true
}
```

**Usage in Login Handler:**

```go
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
    ip := r.Header.Get("X-Forwarded-For")
    if ip == "" {
        ip = r.RemoteAddr
    }

    if !s.auth.Limiter.Allow(ip) {
        http.Error(w, `{"error":"too many requests"}`, http.StatusTooManyRequests)
        return
    }
    // ...
}
```

**Cookie Security:**

The session cookie now dynamically sets the Secure attribute based on TLS detection:

```go
http.SetCookie(w, &http.Cookie{
    Name:     "kula_session",
    Value:    token,
    Path:     "/",
    HttpOnly: true,
    Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
    MaxAge:   int(s.cfg.Auth.SessionTimeout.Seconds()),
    SameSite: http.SameSiteStrictMode,
})
```

### 6.2 Input Validation and Injection Prevention

**Path Traversal Prevention:**

Storage configuration now validates absolute paths:

```go
absDir, err := filepath.Abs(cfg.Directory)
if err != nil {
    return nil, fmt.Errorf("resolving storage directory: %w", err)
}
```

**Security Headers:**

```go
func securityMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("X-Content-Type-Options", "nosniff")
        w.Header().Set("X-Frame-Options", "DENY")
        w.Header().Set("Content-Security-Policy", 
            "default-src 'self'; style-src 'self' fonts.googleapis.com; " +
            "font-src fonts.gstatic.com; script-src 'self'; connect-src 'self' ws: wss:;")
        next.ServeHTTP(w, r)
    })
}
```

**Note:** The CSP no longer includes `'unsafe-inline'`, which was a previous concern. This is a significant security improvement.

### 6.3 Landlock Sandbox

The Landlock sandbox implementation provides defense-in-depth:

```go
fsRules := []landlock.Rule{
    landlock.RODirs("/proc"),
    landlock.RODirs("/sys").IgnoreIfMissing(),
    landlock.ROFiles(absConfigPath).IgnoreIfMissing(),
    landlock.RWDirs(absStorageDir),
}

netRules := []landlock.Rule{
    landlock.BindTCP(uint16(webPort)),
}

err = landlock.V5.BestEffort().Restrict(allRules...)
```

This restricts the process to only necessary filesystem paths and network ports, limiting the impact of potential vulnerabilities.

### 6.4 Network Security

**Graceful Shutdown:**

The server now properly handles shutdown signals:

```go
ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
defer stop()

// Collection loop respects context
for {
    select {
    case <-ticker.C:
        // collect
    case <-ctx.Done():
        return
    }
}

// Graceful shutdown with timeout
shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
if err := server.Shutdown(shutdownCtx); err != nil {
    log.Printf("Web server shutdown error: %v", err)
}
```

### 6.5 WebSocket Security

**Critical Improvement - CSWSH Prevention:**

The WebSocket handler now implements strict Origin validation:

```go
var upgrader = websocket.Upgrader{
    ReadBufferSize:  1024,
    WriteBufferSize: 1024,
    CheckOrigin: func(r *http.Request) bool {
        origin := r.Header.Get("Origin")
        if origin == "" {
            return true // Allow non-browser clients
        }

        // Parse origin host
        originHost := ""
        for i := 0; i < len(origin); i++ {
            if origin[i] == ':' && i+2 < len(origin) && origin[i+1] == '/' && origin[i+2] == '/' {
                originHost = origin[i+3:]
                break
            }
        }

        if originHost == "" {
            return false
        }

        if originHost != r.Host {
            log.Printf("WebSocket upgrade blocked: Origin (%s) does not match Host (%s)", originHost, r.Host)
            return false
        }

        return true
    },
}
```

**Read Limits:**

```go
conn.SetReadLimit(4096) // Limit incoming JSON commands
```

**Ping/Pong Health Checking:**

```go
ticker := time.NewTicker(50 * time.Second)
conn.SetPongHandler(func(string) error {
    _ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
    return nil
})
```

---

## 7. Detailed Findings

### 7.1 Critical Issues

**No critical security vulnerabilities were identified in version 0.5.0.**

The development team has addressed all previously identified critical concerns:

| Previous Finding | Status | Resolution |
|-----------------|--------|------------|
| No rate limiting on login | ✅ Fixed | Rate limiter: 5 attempts per 5 minutes |
| Deprecated WebSocket library | ✅ Fixed | Migrated to gorilla/websocket |
| CSWSH vulnerability | ✅ Fixed | Strict Origin validation |
| No graceful shutdown | ✅ Fixed | Context-based signal handling |
| 'unsafe-inline' in CSP | ✅ Fixed | Removed from CSP |
| Whirlpool password hashing | ✅ Fixed | Migrated to Argon2id |

### 7.2 High Priority Recommendations

1. **Security Documentation**: Create a `SECURITY.md` file with:
   - Responsible disclosure policy
   - Security upgrade guidelines
   - Known security considerations for deployment
   - Contact information for security reports

2. **Default Binding Consideration**: While the current default of `0.0.0.0:8080` is appropriate for a monitoring tool, consider documenting the security implications more prominently for users deploying on public networks.

3. **Session Persistence**: Sessions are currently stored in memory only. Consider adding optional persistent session storage for environments requiring session survival across restarts.

### 7.3 Medium Priority Recommendations

1. **Rate Limiter Enhancement**: The current rate limiter is IP-based. Consider adding:
   - Configurable threshold via config.yaml
   - Progressive delays after failed attempts
   - Logging of blocked attempts for security monitoring

2. **Origin Validation Enhancement**: The current origin parsing is manual. Consider using `net/url` for more robust parsing:

   ```go
   parsed, err := url.Parse(origin)
   if err != nil {
       return false
   }
   originHost = parsed.Host
   ```

3. **Streaming History API**: For large history queries, consider implementing streaming JSON encoding to reduce memory usage.

4. **Password Policy**: Add optional password complexity validation when generating password hashes.

### 7.4 Low Priority Recommendations

1. **Benchmark Tests**: Add comprehensive benchmark tests for the storage engine to validate performance claims and catch regressions.

2. **Fuzz Testing**: Add fuzz tests for JSON parsing and `/proc` file parsing to identify edge cases.

3. **Dependency Scanning**: Add automated dependency vulnerability scanning using `govulncheck` in CI pipeline.

4. **Integration Tests**: Expand test coverage to include HTTP endpoint tests and WebSocket message handling tests.

5. **Metrics Export**: Consider adding optional Prometheus-compatible metrics export endpoint.

---

## 8. Summary Assessment

The following table provides a summary assessment across the three evaluation dimensions:

| Category | Score | Previous Score | Summary |
|----------|-------|----------------|---------|
| Code Quality | 4.7/5 | 4.5/5 | Mature architecture, excellent patterns |
| Performance | 4.6/5 | 4.5/5 | Dual-stack support, improved WebSocket |
| Security | 4.7/5 | 4.0/5 | Comprehensive hardening implemented |
| **Overall** | **4.7/5** | **4.3/5** | **Production-ready with strong security** |

### Improvement Summary

| Dimension | Improvement | Key Changes |
|-----------|-------------|-------------|
| Code Quality | +0.2 | Better version management, graceful shutdown |
| Performance | +0.1 | Dual-stack networking, ping/pong health checks |
| Security | +0.7 | Rate limiting, CSWSH prevention, CSP improvements |

---

## 9. Conclusion

Kula version 0.5.0 represents a mature, production-ready Linux monitoring tool with comprehensive security measures. The development team has demonstrated exceptional responsiveness to security feedback, implementing all critical recommendations from previous reviews.

### Key Achievements

1. **Security Hardening**: All previously identified vulnerabilities have been addressed, including rate limiting on authentication endpoints, WebSocket CSWSH prevention, and improved content security policies.

2. **Modern WebSocket Implementation**: The migration from deprecated `golang.org/x/net/websocket` to `github.com/gorilla/websocket` provides better performance and security features.

3. **Graceful Shutdown**: Context-based signal handling ensures clean shutdowns without data loss or hanging connections.

4. **Dual-Stack IPv6 Support**: Proper IPv6 support with intelligent listener creation ensures compatibility across diverse network environments.

5. **Storage Fallback**: Automatic fallback when permissions are insufficient improves user experience without requiring manual configuration.

### Deployment Recommendations

For organizations deploying Kula:

1. **Enable Authentication**: Always enable authentication for any network-accessible deployment
2. **Use TLS**: Deploy behind a reverse proxy with TLS termination
3. **Review Configuration**: The default configuration is now more secure, but review for your environment
4. **Monitor Logs**: The new logging features provide visibility into API access patterns

### Final Assessment

Kula has evolved from a promising monitoring tool into a robust, security-conscious solution suitable for production deployment. The development velocity and attention to security concerns indicate a mature project with active maintenance. The zero-dependency, single-binary philosophy combined with comprehensive monitoring capabilities makes Kula an excellent choice for lightweight server monitoring scenarios.

The project serves as an exemplary case study in responsive security development, with clear documentation of security fixes and a commitment to addressing vulnerabilities promptly. The AGPL-3.0 license should be considered for organizations with specific licensing requirements.

---

*Report Generated: 2025-03-01*

*Reviewer: AI Code Review Analysis*

*Model: Claude 3.5 Sonnet*

*Repository: https://github.com/c0m4r/kula*
