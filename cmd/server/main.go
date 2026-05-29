package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"gobench/internal/handler"
	"gobench/internal/middleware"
	"gobench/internal/repository"
	"gobench/internal/service"
	"gobench/pkg/config"
	"gobench/pkg/database"
	"gobench/pkg/logger"
)

func main() {
	// 1. Init logger
	if err := logger.Init(); err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Log.Info("Starting GoBench server...")

	// 2. Init config
	if err := config.Init("config/config.yaml"); err != nil {
		logger.Log.Fatal("Failed to load config", zap.Error(err))
	}

	// 3. Init database
	if err := database.Init(); err != nil {
		logger.Log.Fatal("Failed to connect to database", zap.Error(err))
	}
	logger.Log.Info("Connected to MySQL database successfully")

	// 4. Dependency Injection
	userRepo := repository.NewUserRepository()
	authSvc := service.NewAuthService(userRepo)
	authHdl := handler.NewAuthHandler(authSvc)

	taskRepo := repository.NewTaskRepository()
	taskSvc := service.NewTaskService(taskRepo)
	taskHdl := handler.NewTaskHandler(taskSvc)

	// 5. Init Gin router & Routes
	router := gin.Default()
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	apiV1 := router.Group("/api/v1")
	{
		authGroup := apiV1.Group("/auth")
		{
			authGroup.POST("/register", authHdl.Register)
			authGroup.POST("/login", authHdl.Login)
		}

		taskGroup := apiV1.Group("/tasks")
		taskGroup.Use(middleware.AuthMiddleware())
		{
			taskGroup.POST("", taskHdl.Create)
			taskGroup.GET("/:id", taskHdl.Get)
			taskGroup.GET("", taskHdl.List)
			taskGroup.PUT("/:id", taskHdl.Update)
			taskGroup.DELETE("/:id", taskHdl.Delete)
		}
	}

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", config.AppConfig.Server.Port),
		Handler: router,
	}

	// 5. Start HTTP server
	go func() {
		logger.Log.Info("Listening and serving HTTP", zap.Int("port", config.AppConfig.Server.Port))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Log.Fatal("Listen error", zap.Error(err))
		}
	}()

	// 6. Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Log.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Log.Fatal("Server forced to shutdown:", zap.Error(err))
	}

	logger.Log.Info("Server exiting")
}
