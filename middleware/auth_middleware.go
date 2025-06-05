package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap" // For logging within the middleware if needed
	// It's good practice to have access to the app logger if decisions need to be logged
	// However, this middleware is simple, so direct logging might not be extensive.
	// Let's assume logger is passed or use a global one if available and necessary.
	// For this version, focus on core auth logic.
)

const (
	// APIKeyHeader is the default header name to check for the API key.
	APIKeyHeader = "X-API-KEY"
)

// APIKeyAuth creates a Gin middleware for API key authentication.
// It checks for a key in the X-API-KEY header.
// If expectedAPIKey is empty, authentication is considered disabled and all requests are allowed.
func APIKeyAuth(expectedAPIKey string, logger *zap.SugaredLogger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// If no API key is configured in config.json, authentication is disabled.
		if expectedAPIKey == "" {
			logger.Debug("API key authentication is disabled (no key configured). Allowing request.")
			c.Next()
			return
		}

		providedKey := c.GetHeader(APIKeyHeader)

		if providedKey == "" {
			logger.Warnw("API key missing from request.",
				"client_ip", c.ClientIP(),
				"path", c.Request.URL.Path,
				"header_checked", APIKeyHeader,
			)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: API key required."})
			return
		}

		if providedKey != expectedAPIKey {
			logger.Warnw("Invalid API key provided.",
				"client_ip", c.ClientIP(),
				"path", c.Request.URL.Path,
			)
			// Do not log the providedKey itself unless in very verbose debug mode and handled carefully,
			// as it might be sensitive if someone is trying to guess keys.
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: Invalid API key."})
			return
		}

		// If keys match, proceed to the next handler.
		logger.Debug("API key authentication successful. Allowing request.")
		c.Next()
	}
}
