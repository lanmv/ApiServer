package main

import (
	"fmt"
	"log"
	"os"

	"generic-database-service/config"
	"generic-database-service/database"
	"generic-database-service/handlers"
	"generic-database-service/logger"

	"github.com/gin-gonic/gin"
)

func main() {
	cfg, err := config.LoadConfig(".")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	if err := logger.InitLogger(cfg.LogLevel, "logs"); err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	appLogger := logger.Get()
	appLogger.Info("Logger initialized successfully.")

	// Create a standard logger for GORM for now
	gormStdLogger := log.New(os.Stdout, "[GORM] ", log.LstdFlags)
	db, err := database.InitDB(cfg, gormStdLogger)
	if err != nil {
		appLogger.Fatalf("Failed to initialize database: %v", err)
	}
	appLogger.Infof("Database connection established: %s", cfg.DatabaseType)

	apiHandler := handlers.NewAPIHandler(db, &cfg)

	router := gin.Default()

	v1 := router.Group("/api/v1")
	{
		// Routes without an ID parameter
		v1.GET("/:tableName", apiHandler.HandleDynamicRequest)  // List/Query
		v1.POST("/:tableName", apiHandler.HandleDynamicRequest) // Create

		// Routes with an ID parameter
		v1.GET("/:tableName/:id", apiHandler.HandleDynamicRequestWithID)    // Get single record by ID
		v1.PUT("/:tableName/:id", apiHandler.HandleDynamicRequestWithID)    // Update record by ID
		v1.PATCH("/:tableName/:id", apiHandler.HandleDynamicRequestWithID)  // Partial update record by ID
		v1.DELETE("/:tableName/:id", apiHandler.HandleDynamicRequestWithID) // Delete record by ID
	}

	listenAddr := fmt.Sprintf(":%s", cfg.ServicePort)
	appLogger.Infof("Starting server on %s", listenAddr)
	if err := router.Run(listenAddr); err != nil {
		appLogger.Fatalf("Failed to start server: %v", err)
	}
}
