package database

import (
	"fmt"
	"log" // Standard log, can be replaced by Zap later
	"time"

	"generic-database-service/config" // Assuming module name is generic-database-service
	// Corrected import for Oracle GORM dialect
	// gormoracle "github.com/CengSin/gorm-oracle" // Commented out due to fetch issues

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// DB is the global database connection instance
// It's generally better to pass the DB instance around rather than using a global.
// However, following the plan's implication for now.
var DB *gorm.DB

// InitDB initializes the database connection based on the provided configuration.
// Takes a standard library logger for now, can be adapted for Zap.
func InitDB(cfg config.Config, logger *log.Logger) (*gorm.DB, error) {
	var err error
	var dialect gorm.Dialector

	gormLogLevel := gormlogger.Warn // Default GORM log level
	// Example: Map your app's log level to GORM's log level
	// if cfg.LogLevel == "debug" { gormLogLevel = gormlogger.Info }

	// Using the passed logger for GORM output
	newGormLogger := gormlogger.New(
		logger,
		gormlogger.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  gormLogLevel,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false, // Usually false for file logs
		},
	)

	switch cfg.DatabaseType {
	case "mysql":
		dialect = mysql.Open(cfg.DatabaseConnectionString)
	case "postgres":
		dialect = postgres.Open(cfg.DatabaseConnectionString)
	case "sqlserver":
		dialect = sqlserver.Open(cfg.DatabaseConnectionString)
	case "oracle":
		// dialect = gormoracle.Open(cfg.DatabaseConnectionString) // Commented out due to fetch issues
		return nil, fmt.Errorf("oracle database type is temporarily unsupported pending dialect resolution")
	default:
		return nil, fmt.Errorf("unsupported database type: %s", cfg.DatabaseType)
	}

	db, err := gorm.Open(dialect, &gorm.Config{
		Logger: newGormLogger,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database (%s): %w", cfg.DatabaseType, err)
	}

	// Assign to global DB if that's the chosen pattern
	DB = db

	logger.Printf("Database connection established for type: %s", cfg.DatabaseType)
	return db, nil
}
