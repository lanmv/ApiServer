package middleware

import (
	"bytes"
	"io"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// bodyLogWriter is a custom response writer to capture response body.
type bodyLogWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w bodyLogWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

// RequestResponseLogger creates a Gin middleware for logging request and response details.
func RequestResponseLogger(logger *zap.SugaredLogger) gin.HandlerFunc {
	return func(c *gin.Context) {
		startTime := time.Now()

		// Capture request body (optional and with care)
		// For simplicity, we won't read and replace the request body here,
		// as it can be complex and have side effects (e.g., c.ShouldBindJSON can only be called once).
		// We will log query parameters and path.
		// Logging full request bodies should be an explicit opt-in feature due to size/sensitivity.

		// Create a response writer to capture the response body
		blw := &bodyLogWriter{body: bytes.NewBufferString(""), ResponseWriter: c.Writer}
		c.Writer = blw

		// Process request
		c.Next()

		// After request
		latency := time.Since(startTime)
		statusCode := c.Writer.Status()

		// Prepare log fields
		fields := []zap.Field{
			zap.String("client_ip", c.ClientIP()),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.String("query", c.Request.URL.RawQuery),
			zap.Int("status_code", statusCode),
			zap.Duration("latency", latency),
			zap.String("user_agent", c.Request.UserAgent()),
			zap.Int("response_size_bytes", blw.body.Len()), // Size of the captured response body
		}

		// Log request ID if present (e.g., set by a load balancer or another middleware)
		if reqID := c.GetHeader("X-Request-ID"); reqID != "" {
			fields = append(fields, zap.String("request_id", reqID))
		}

		// Conditionally log response body (be careful with large or sensitive responses)
		// For now, let's log a truncated version or based on status code.
		// A more advanced setup might allow configuring this.
		responseBodyStr := blw.body.String()
		maxRespBodyLogSize := 512 // Log up to 512 bytes of response body
		if len(responseBodyStr) > maxRespBodyLogSize {
			responseBodyStr = responseBodyStr[:maxRespBodyLogSize] + "..."
		}
        if statusCode >= 400 { // Log body for errors
            fields = append(fields, zap.String("response_body_snippet", responseBodyStr))
        }


		if statusCode >= 500 {
			// Log internal server errors with higher severity
			logger.With(fields...).Error("Incoming request (Server Error)")
		} else if statusCode >= 400 {
			// Log client errors as warnings
			logger.With(fields...).Warn("Incoming request (Client Error)")
		} else {
			// Log successful requests as info
			logger.With(fields...).Info("Incoming request")
		}
	}
}
