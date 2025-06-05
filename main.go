package main

import (
	"fmt"
	"log"
	"os"

	"generic-database-service/config"
	"generic-database-service/database"
	"generic-database-service/handlers"
	"generic-database-service/logger"
	"generic-database-service/middleware"

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

	gormStdLogger := log.New(os.Stdout, "[GORM] ", log.LstdFlags)
	db, err := database.InitDB(cfg, gormStdLogger)
	if err != nil {
		appLogger.Fatalf("Failed to initialize database: %v", err)
	}
	appLogger.Infof("Database connection established: %s", cfg.DatabaseType)

	apiHandler := handlers.NewAPIHandler(db, &cfg)

	router := gin.New()

	// Register global middleware
	// 1. Request/Response Logger
	router.Use(middleware.RequestResponseLogger(appLogger))
	// 2. Panic Recovery
	router.Use(gin.Recovery())
	// 3. API Key Authentication (must be after logger to see its effect, before routes)
	router.Use(middleware.APIKeyAuth(cfg.APISecretKey, appLogger))


	v1 := router.Group("/api/v1")
	{
		// Collection routes
		v1.GET("/:tableName", apiHandler.HandleDynamicRequest)
		v1.POST("/:tableName", apiHandler.HandleDynamicRequest)

		// Specific record routes
		v1.GET("/:tableName/:id", apiHandler.HandleDynamicRequestWithID)
		v1.PUT("/:tableName/:id", apiHandler.HandleDynamicRequestWithID)
		v1.PATCH("/:tableName/:id", apiHandler.HandleDynamicRequestWithID)
		v1.DELETE("/:tableName/:id", apiHandler.HandleDynamicRequestWithID)
	}

	listenAddr := fmt.Sprintf(":%s", cfg.ServicePort)
	appLogger.Infof("Starting server on %s", listenAddr)
	if cfg.APISecretKey == "" {
		appLogger.Warn("API Secret Key is not configured. API authentication is disabled.")
	} else {
		appLogger.Info("API Secret Key authentication is enabled.")
	}

	if err := router.Run(listenAddr); err != nil {
		appLogger.Fatalf("Failed to start server: %v", err)
	}
}
