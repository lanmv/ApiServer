package middleware

import (
	"bytes"
	// "io" // io.ReadAll was removed, so this might not be needed directly
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	// "go.uber.org/zap/zapcore" // Not directly needed for this fix
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

		blw := &bodyLogWriter{body: bytes.NewBufferString(""), ResponseWriter: c.Writer}
		c.Writer = blw

		c.Next()

		latency := time.Since(startTime)
		statusCode := c.Writer.Status()

		fields := []zap.Field{
			zap.String("client_ip", c.ClientIP()),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.String("query", c.Request.URL.RawQuery),
			zap.Int("status_code", statusCode),
			zap.Duration("latency", latency),
			zap.String("user_agent", c.Request.UserAgent()),
			zap.Int("response_size_bytes", blw.body.Len()),
		}

		if reqID := c.GetHeader("X-Request-ID"); reqID != "" {
			fields = append(fields, zap.String("request_id", reqID))
		}

		responseBodyStr := blw.body.String()
		maxRespBodyLogSize := 512
		if len(responseBodyStr) > maxRespBodyLogSize {
			responseBodyStr = responseBodyStr[:maxRespBodyLogSize] + "..."
		}
        if statusCode >= 400 {
            fields = append(fields, zap.String("response_body_snippet", responseBodyStr))
        }

		// Get the core logger to use methods accepting []zap.Field
		coreLogger := logger.Desugar()

		if statusCode >= 500 {
			coreLogger.Error("Incoming request (Server Error)", fields...)
		} else if statusCode >= 400 {
			coreLogger.Warn("Incoming request (Client Error)", fields...)
		} else {
			coreLogger.Info("Incoming request", fields...)
		}
	}
}
