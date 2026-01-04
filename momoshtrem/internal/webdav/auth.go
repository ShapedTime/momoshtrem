package webdav

import (
	"crypto/subtle"
	"log/slog"
	"net/http"

	"github.com/shapedtime/momoshtrem/internal/config"
)

const authRealm = "momoshtrem WebDAV"

// AuthMiddleware wraps an http.Handler with HTTP Basic Authentication
type AuthMiddleware struct {
	next     http.Handler
	username string
	password string
}

// NewAuthMiddleware creates authentication middleware for the WebDAV server.
// If auth is disabled, returns the original handler unwrapped.
func NewAuthMiddleware(next http.Handler, cfg config.WebDAVAuthConfig) http.Handler {
	if !cfg.Enabled {
		return next
	}

	return &AuthMiddleware{
		next:     next,
		username: cfg.Username,
		password: cfg.Password,
	}
}

// ServeHTTP implements http.Handler
func (m *AuthMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	username, password, ok := r.BasicAuth()

	if !ok {
		m.unauthorized(w, r, "missing credentials")
		return
	}

	if !m.validateCredentials(username, password) {
		m.unauthorized(w, r, "invalid credentials")
		return
	}

	// Credentials valid, proceed to handler
	slog.Debug("WebDAV auth successful",
		"method", r.Method,
		"path", r.URL.Path,
		"remote_addr", r.RemoteAddr,
	)

	m.next.ServeHTTP(w, r)
}

// validateCredentials performs constant-time comparison of credentials
// to prevent timing attacks
func (m *AuthMiddleware) validateCredentials(username, password string) bool {
	usernameMatch := subtle.ConstantTimeCompare([]byte(username), []byte(m.username)) == 1
	passwordMatch := subtle.ConstantTimeCompare([]byte(password), []byte(m.password)) == 1

	// Both must match
	return usernameMatch && passwordMatch
}

// unauthorized sends a 401 response with proper WWW-Authenticate header
func (m *AuthMiddleware) unauthorized(w http.ResponseWriter, r *http.Request, reason string) {
	slog.Warn("WebDAV auth failed",
		"reason", reason,
		"method", r.Method,
		"path", r.URL.Path,
		"remote_addr", r.RemoteAddr,
	)

	w.Header().Set("WWW-Authenticate", `Basic realm="`+authRealm+`"`)
	http.Error(w, "401 Unauthorized", http.StatusUnauthorized)
}

// ValidateConfig checks auth config and logs warnings for potential issues
func ValidateConfig(cfg config.WebDAVAuthConfig) {
	if !cfg.Enabled {
		slog.Info("WebDAV authentication is disabled")
		return
	}

	if cfg.Username == "" {
		slog.Warn("WebDAV auth enabled but username is empty")
	}

	if cfg.Password == "" {
		slog.Warn("WebDAV auth enabled but password is empty")
	} else if len(cfg.Password) < 8 {
		slog.Warn("WebDAV password is less than 8 characters, consider using a stronger password")
	}
}
