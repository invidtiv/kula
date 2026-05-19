package web

import (
	"html"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kula/internal/config"
)

func TestTemplateInjection(t *testing.T) {
	s := NewServer(config.WebConfig{}, config.GlobalConfig{}, nil, nil, t.TempDir(), config.OllamaConfig{})
	
	// Create a recorder to capture the response
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	
	// Wrap with securityMiddleware to get the nonce
	handler := s.securityMiddleware(http.HandlerFunc(s.handleIndex))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rec.Code)
	}

	body := html.UnescapeString(rec.Body.String())

	// Verify nonce is in CSP header
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "nonce-") {
		t.Errorf("CSP header missing nonce: %s", csp)
	}

	// Extract nonce from CSP
	parts := strings.Split(csp, "'nonce-")
	if len(parts) < 2 {
		t.Fatalf("Could not parse nonce from CSP: %s", csp)
	}
	nonce := strings.Split(parts[1], "'")[0]

	// Verify nonce is injected into HTML
	if !strings.Contains(body, `nonce="`+nonce+`"`) {
		t.Errorf("HTML body missing injected nonce %s", nonce)
	}

	// Verify SRI is injected into HTML
	sri := s.sriHashes["js/app/main.js"]
	if sri == "" {
		t.Error("SRI hash for js/app/main.js is empty in server")
	}
	if !strings.Contains(body, `integrity="`+sri+`"`) {
		t.Errorf("HTML body missing injected SRI %s", sri)
	}
}

func TestGameTemplateInjection(t *testing.T) {
	s := NewServer(config.WebConfig{}, config.GlobalConfig{}, nil, nil, t.TempDir(), config.OllamaConfig{})
	
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/game.html", nil)
	
	handler := s.securityMiddleware(http.HandlerFunc(s.handleGame))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rec.Code)
	}

	body := html.UnescapeString(rec.Body.String())
	
	// Verify SRI for game.js
	sri := s.sriHashes["game.js"]
	if sri == "" {
		t.Error("SRI hash for game.js is empty in server")
	}
	if !strings.Contains(body, `integrity="`+sri+`"`) {
		t.Errorf("Game HTML body missing injected SRI %s", sri)
	}
}

func TestCreateUnixListener(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "kula.sock")

	ln, err := createUnixListener(sock, "0660")
	if err != nil {
		t.Fatalf("createUnixListener: %v", err)
	}
	defer func() { _ = ln.Close() }()

	if ln.Addr().Network() != "unix" {
		t.Fatalf("expected unix network, got %s", ln.Addr().Network())
	}

	info, err := os.Stat(sock)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		t.Fatalf("path is not a socket")
	}
	if perm := info.Mode().Perm(); perm != 0660 {
		t.Fatalf("expected mode 0660, got %#o", perm)
	}
}

func TestCreateUnixListenerInvalidMode(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "kula.sock")

	if _, err := createUnixListener(sock, "not-octal"); err == nil {
		t.Fatalf("expected error for invalid mode")
	}
	if _, err := os.Stat(sock); !os.IsNotExist(err) {
		t.Fatalf("socket file should not be left behind after mode error, stat err=%v", err)
	}
}

func TestCreateUnixListenerRequiresAbsolute(t *testing.T) {
	if _, err := createUnixListener("relative.sock", "0660"); err == nil {
		t.Fatalf("expected error for relative path")
	}
}

func TestCreateUnixListenerRemovesStale(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "kula.sock")

	// Create a stale socket file (no listener attached).
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("seed listen: %v", err)
	}
	_ = ln.Close()
	// Close removes the file on most platforms; recreate to simulate a stale leftover.
	if _, err := os.Stat(sock); os.IsNotExist(err) {
		f, err := os.Create(sock + ".tmp")
		if err != nil {
			t.Fatalf("create tmp: %v", err)
		}
		_ = f.Close()
		// Bind a fresh listener and immediately close it without removing.
		ln2, err := net.Listen("unix", sock)
		if err != nil {
			t.Fatalf("seed listen 2: %v", err)
		}
		// Disable unlink-on-close so the file persists as a stale socket.
		if ul, ok := ln2.(*net.UnixListener); ok {
			ul.SetUnlinkOnClose(false)
		}
		_ = ln2.Close()
	}

	ln3, err := createUnixListener(sock, "0660")
	if err != nil {
		t.Fatalf("createUnixListener over stale: %v", err)
	}
	_ = ln3.Close()
}

func TestCreateUnixListenerRefusesLive(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "kula.sock")

	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("seed listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	if _, err := createUnixListener(sock, "0660"); err == nil {
		t.Fatalf("expected error when another process is listening")
	}
}

func TestHandleHealth(t *testing.T) {
	s := NewServer(config.WebConfig{}, config.GlobalConfig{}, nil, nil, t.TempDir(), config.OllamaConfig{})

	for _, path := range []string{"/health", "/status"} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, path, nil)

			http.HandlerFunc(s.handleHealth).ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("Expected status 200 for %s, got %d", path, rec.Code)
			}
			if rec.Body.String() != "kula is healthy" {
				t.Fatalf("Expected body %q for %s, got %q", "kula is healthy", path, rec.Body.String())
			}
		})
	}
}
