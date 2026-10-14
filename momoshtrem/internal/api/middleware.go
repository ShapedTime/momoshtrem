package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// TokenHeader carries the shared secret between the web app and the backend.
const TokenHeader = "X-Momoshtrem-Token"

const (
	corsAllowMethods = "GET, POST, PUT, PATCH, DELETE, OPTIONS"
	corsAllowHeaders = "Content-Type, Authorization, " + TokenHeader
	corsMaxAge       = "600"
)

// ValidateOptions logs warnings for risky security settings, mirroring
// webdav.ValidateConfig.
func ValidateOptions(opts Options) {
	if opts.APIToken == "" {
		slog.Warn("BACKEND_API_TOKEN not set; REST API is unauthenticated")
	} else if len(opts.APIToken) < 16 {
		slog.Warn("BACKEND_API_TOKEN is shorter than 16 characters, consider `openssl rand -hex 32`")
	}

	if len(opts.CORSAllowedOrigins) == 0 {
		slog.Info("CORS disabled (no CORS_ALLOWED_ORIGINS); REST API is server-to-server only")
	} else {
		slog.Info("CORS enabled", "allowed_origins", opts.CORSAllowedOrigins)
	}
}

// corsMiddleware answers browsers from allowlisted origins only. With an empty
// allowlist it returns nil and no CORS headers are ever sent. Requests from
// other origins get no CORS headers and continue down the chain untouched, so
// the browser blocks them. Preflights from allowed origins are answered here,
// before the token check, because browsers never send custom headers on them.
func corsMiddleware(allowedOrigins []string) gin.HandlerFunc {
	if len(allowedOrigins) == 0 {
		return nil
	}

	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[o] = struct{}{}
	}

	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Add("Vary", "Origin")

		origin := c.GetHeader("Origin")
		if _, ok := allowed[origin]; origin == "" || !ok {
			c.Next()
			return
		}

		h.Set("Access-Control-Allow-Origin", origin)
		h.Set("Access-Control-Allow-Methods", corsAllowMethods)
		h.Set("Access-Control-Allow-Headers", corsAllowHeaders)

		if c.Request.Method == http.MethodOptions {
			h.Set("Access-Control-Max-Age", corsMaxAge)
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// tokenMiddleware requires the shared secret on every /api route. With an
// empty token it returns nil and the API is open (ValidateOptions warns).
// Both sides are hashed first so the constant-time comparison doesn't leak
// the token length.
func tokenMiddleware(token string) gin.HandlerFunc {
	if token == "" {
		return nil
	}

	want := sha256.Sum256([]byte(token))

	return func(c *gin.Context) {
		if !isAPIPath(c.Request.URL.Path) {
			c.Next()
			return
		}

		provided := c.GetHeader(TokenHeader)
		if provided == "" {
			rejectToken(c, "missing "+TokenHeader+" header")
			return
		}

		got := sha256.Sum256([]byte(provided))
		if subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
			rejectToken(c, "invalid "+TokenHeader+" header")
			return
		}

		c.Next()
	}
}

func rejectToken(c *gin.Context, reason string) {
	slog.Warn("REST API auth failed",
		"reason", reason,
		"method", c.Request.Method,
		"path", c.Request.URL.Path,
		"remote_addr", c.Request.RemoteAddr,
	)
	errorCodeResponse(c, http.StatusUnauthorized, CodeUnauthorized, reason)
	c.Abort()
}

func isAPIPath(path string) bool {
	return path == "/api" || strings.HasPrefix(path, "/api/")
}
