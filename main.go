package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"generic-database-service/config"
	"generic-database-service/database"
	"generic-database-service/handlers"
	"generic-database-service/logger"
	"generic-database-service/middleware"

	"github.com/gin-gonic/gin"
)

func healthCheckHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "UP",
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
	})
}

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

	router.Use(middleware.RequestResponseLogger(appLogger))
	router.Use(gin.Recovery())

	router.GET("/health", healthCheckHandler)

	v1 := router.Group("/api/v1")
	v1.Use(middleware.APIKeyAuth(cfg.APISecretKey, appLogger))
	{
		// New routes for getting table and view lists
		v1.GET("/getTables", apiHandler.HandleGetTablesList)
		v1.GET("/getViews", apiHandler.HandleGetViewsList)

		// Existing CRUD routes for dynamic table/view access
		// These should come after specific named routes like /getTables to avoid path conflicts
		// if a table was ever named "getTables". Gin matches routes in order of definition.
		v1.GET("/:tableName", apiHandler.HandleDynamicRequest)
		v1.POST("/:tableName", apiHandler.HandleDynamicRequest)

		v1.GET("/:tableName/:id", apiHandler.HandleDynamicRequestWithID)
		v1.PUT("/:tableName/:id", apiHandler.HandleDynamicRequestWithID)
		v1.PATCH("/:tableName/:id", apiHandler.HandleDynamicRequestWithID)
		v1.DELETE("/:tableName/:id", apiHandler.HandleDynamicRequestWithID)
	}

	listenAddr := fmt.Sprintf(":%s", cfg.ServicePort)
	appLogger.Infof("Starting server on %s", listenAddr)
	if cfg.APISecretKey == "" {
		appLogger.Warn("API Secret Key is not configured. API authentication for /api/v1 routes is disabled.")
	} else {
		appLogger.Info("API Secret Key authentication for /api/v1 routes is enabled.")
	}

	if err := router.Run(listenAddr); err != nil {
		appLogger.Fatalf("Failed to start server: %v", err)
	}
}
