package logger

import (
	"fmt"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var globalLogger *zap.SugaredLogger

// InitLogger initializes the global logger
func InitLogger(logLevel string, logPath string) error {
	level := zap.NewAtomicLevel()
	switch strings.ToLower(logLevel) {
	case "debug":
		level.SetLevel(zap.DebugLevel)
	case "info":
		level.SetLevel(zap.InfoLevel)
	case "warn":
		level.SetLevel(zap.WarnLevel)
	case "error":
		level.SetLevel(zap.ErrorLevel)
	default:
		level.SetLevel(zap.InfoLevel)
	}

	// Get current date for the log file name
	currentDate := time.Now().Format("2006-01-02")
	logFileName := fmt.Sprintf("%s/%s.log", strings.TrimRight(logPath, "/"), currentDate)

	// Ensure logs directory exists
	if err := os.MkdirAll(logPath, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Configure a file syncer
	fileWriter := zapcore.AddSync(&zapcore.BufferedWriteSyncer{
		WS: zapcore.AddSync(mustOpenFile(logFileName)),
		//BufferSize: 256 * 1024, // 256KB
		//FlushInterval: 30 * time.Second,
	})

	// For now, we will also log to console for easier debugging during development
	consoleDebugging := zapcore.Lock(os.Stdout)

	// High-priority output should also go to standard error, and low-priority
	// output should also go to standard out.
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder

	core := zapcore.NewTee(
		zapcore.NewCore(zapcore.NewJSONEncoder(encoderConfig), fileWriter, level),
		zapcore.NewCore(zapcore.NewConsoleEncoder(encoderConfig), consoleDebugging, level),
	)

	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	globalLogger = logger.Sugar()

	globalLogger.Infof("Logger initialized. Log level: %s. Log file: %s", logLevel, logFileName)
	return nil
}

// Get returns the global sugared logger instance
func Get() *zap.SugaredLogger {
	if globalLogger == nil {
		// Fallback to a default logger if not initialized, though InitLogger should always be called first.
		// This basic logger will print to stdout.
		fmt.Println("Warning: Global logger not initialized. Using default stdout logger.")
		logger, _ := zap.NewDevelopment()
		return logger.Sugar()
	}
	return globalLogger
}

// mustOpenFile opens a file, panicking on error.
// This is a helper for zapcore.AddSync.
func mustOpenFile(filename string) *os.File {
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		panic(fmt.Sprintf("failed to open log file %s: %v", filename, err))
	}
	return file
}
