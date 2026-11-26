package httpauth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
)

const SessionCookieName = "nano_studio_session"

// Config configures HTTP/WebSocket/MCP Streamable HTTP security controls.
type Config struct {
	BindHost         string
	AuthToken        string
	AllowRemoteAuth  bool
	TLSEnabled       bool
	AllowedOrigins   []string
	PublicGetPaths   map[string]bool // e.g., "/" HTML bootstrap page (never /api/*)
}

// GenerateBootToken generates a 256-bit cryptographically random hex bearer token.
func GenerateBootToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("crypto/rand failed: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// ResolveAuthToken uses NANO_AUTH_TOKEN if non-empty, otherwise generates a CSPRNG boot token.
func ResolveAuthToken() (string, error) {
	if env := strings.TrimSpace(os.Getenv("NANO_AUTH_TOKEN")); env != "" {
		if len(env) < 24 {
			return "", fmt.Errorf("NANO_AUTH_TOKEN must be at least 24 characters")
		}
		return env, nil
	}
	return GenerateBootToken()
}

// IsLoopbackHost returns true only for explicit local loopback addresses.
func IsLoopbackHost(host string) bool {
	h := strings.ToLower(strings.Trim(strings.TrimSpace(host), "[]"))
	return h == "" || h == "127.0.0.1" || h == "localhost" || h == "::1"
}

// ValidateBindConfig fails closed if binding outside loopback without remote auth + TLS.
func ValidateBindConfig(cfg Config) error {
	if strings.TrimSpace(cfg.AuthToken) == "" {
		return fmt.Errorf("auth token must never be empty")
	}
	if !IsLoopbackHost(cfg.BindHost) {
		if !cfg.AllowRemoteAuth || !cfg.TLSEnabled {
			return fmt.Errorf("refusing non-loopback bind %q without explicit --allow-remote-auth and TLS", cfg.BindHost)
		}
	}
	return nil
}

// Middleware enforces DNS-rebinding Host validation, strict Origin CSRF checks,
// security headers, and constant-time bearer/session-cookie authentication on API routes.
func Middleware(cfg Config, next http.Handler) http.Handler {
	isLoopback := IsLoopbackHost(cfg.BindHost)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")

		// 1. DNS Rebinding defense: verify Host header.
		hostOnly := r.Host
		if h, _, err := net.SplitHostPort(r.Host); err == nil {
			hostOnly = h
		}
		if isLoopback && !IsLoopbackHost(hostOnly) {
			http.Error(w, "forbidden host header", http.StatusForbidden)
			return
		}

		// 2. Origin / CSRF defense on state-mutating requests, WS upgrades, and Streamable HTTP.
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		isUpgrade := strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
		if origin != "" {
			if !isAllowedOrigin(origin, isLoopback, cfg.AllowedOrigins) {
				http.Error(w, "cross-origin request blocked", http.StatusForbidden)
				return
			}
		} else if isUpgrade {
			// WebSocket upgrades from browsers always carry Origin; require it if browser-like or reject unknown.
			http.Error(w, "missing origin on websocket upgrade", http.StatusForbidden)
			return
		}

		// 3. Public bootstrap HTML shell ("/") mints HttpOnly SameSite=Strict cookie when visited with ?token= or localhost browser bootstrap.
		if r.Method == http.MethodGet && cfg.PublicGetPaths[r.URL.Path] {
			if qTok := strings.TrimSpace(r.URL.Query().Get("token")); qTok != "" {
				if !tokensEqual(qTok, cfg.AuthToken) {
					http.Error(w, "invalid bootstrap token", http.StatusUnauthorized)
					return
				}
				http.SetCookie(w, &http.Cookie{
					Name:     SessionCookieName,
					Value:    cfg.AuthToken,
					Path:     "/",
					HttpOnly: true,
					SameSite: http.SameSiteStrictMode,
					Secure:   cfg.TLSEnabled,
				})
			}
			next.ServeHTTP(w, r)
			return
		}

		// 4. All API and action endpoints require Bearer token or matching HttpOnly session cookie.
		if !isAuthorized(r, cfg.AuthToken) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="nano-android"`)
			http.Error(w, "unauthorized: valid Bearer token or session cookie required", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func isAllowedOrigin(origin string, isLoopback bool, allowlist []string) bool {
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	for _, allowed := range allowlist {
		if strings.EqualFold(strings.TrimRight(origin, "/"), strings.TrimRight(allowed, "/")) {
			return true
		}
	}
	if isLoopback {
		return IsLoopbackHost(u.Hostname())
	}
	return false
}

func isAuthorized(r *http.Request, expectedToken string) bool {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(auth) > 7 && strings.EqualFold(auth[:7], "Bearer ") {
		if tokensEqual(strings.TrimSpace(auth[7:]), expectedToken) {
			return true
		}
	}
	if c, err := r.Cookie(SessionCookieName); err == nil && c != nil {
		if tokensEqual(c.Value, expectedToken) {
			return true
		}
	}
	return false
}

func tokensEqual(a, b string) bool {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
