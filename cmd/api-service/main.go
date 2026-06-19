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
	"github.com/joho/godotenv"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.uber.org/zap"

	"gobench/internal/handler"
	"gobench/internal/middleware"
	"gobench/internal/repository"
	"gobench/internal/scheduler"
	"gobench/internal/service"
	"gobench/pkg/config"
	"gobench/pkg/database"
	"gobench/pkg/grpcclient"
	"gobench/pkg/logger"
	"gobench/pkg/redis"
	"gobench/pkg/tracing"
)

func main() {
	// 开发环境加载 .env 文件，生产环境忽略此步骤（文件不存在时自动跳过）
	_ = godotenv.Load()

	// 1. Init logger
	if err := logger.Init(); err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Log.Info("Starting api-service...")

	// 2. Init config
	if err := config.Init("config/config.yaml"); err != nil {
		logger.Log.Fatal("Failed to load config", zap.Error(err))
	}

	// 3. Init database
	if err := database.Init(); err != nil {
		logger.Log.Fatal("Failed to connect to database", zap.Error(err))
	}
	logger.Log.Info("Connected to MySQL database successfully")

	// 4. Init Redis
	if err := redis.Init(); err != nil {
		logger.Log.Fatal("Failed to connect to Redis", zap.Error(err))
	}
	logger.Log.Info("Connected to Redis successfully")

	// 5. Init tracing（可选，Jaeger 未启动时只打 warn 日志，不影响主服务）
	if config.AppConfig.Tracing.Enabled {
		shutdown, err := tracing.Init(
			context.Background(),
			config.AppConfig.Tracing.ServiceName,
			config.AppConfig.Tracing.OTLPEndpoint,
		)
		if err != nil {
			logger.Log.Warn("Failed to init tracing, continuing without it", zap.Error(err))
		} else {
			defer shutdown(context.Background())
			logger.Log.Info("Tracing initialized")
		}
	}

	// 6. 连接 executor-service（gRPC 客户端）
	executorClient, err := grpcclient.NewExecutorClient(config.AppConfig.Executor.Addr)
	if err != nil {
		logger.Log.Fatal("Failed to connect to executor-service", zap.Error(err))
	}
	defer executorClient.Close()
	logger.Log.Info("Connected to executor-service", zap.String("addr", config.AppConfig.Executor.Addr))

	// 7. Dependency Injection
	userRepo := repository.NewUserRepository()
	taskRepo := repository.NewTaskRepository()
	logRepo := repository.NewTaskLogRepository()

	// 8. TaskService（通过 gRPC 调用 executor-service）
	taskSvc := service.NewTaskService(taskRepo, logRepo, executorClient)

	// 9. Cron 调度器
	sched := scheduler.NewScheduler(taskSvc)

	// 10. 启动时加载所有 active 任务到调度器
	activeTasks, _, _ := taskSvc.ListTasks(1, 1000)
	sched.LoadActiveTasks(activeTasks)
	sched.Start()
	defer sched.Stop()

	// 11. Handlers
	authSvc := service.NewAuthService(userRepo)
	authHdl := handler.NewAuthHandler(authSvc)
	taskHdl := handler.NewTaskHandler(taskSvc, sched)
	wsHdl := handler.NewWSHandler(executorClient)

	// 12. Init Gin router & Routes
	router := gin.Default()
	router.Use(otelgin.Middleware("api-service"))
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

		// WebSocket 不走限流中间件，走独立认证
		apiV1.GET("/ws/tasks/:id/logs", wsHdl.ServeTaskLogs)

		// 统计接口
		apiV1.GET("/stats", middleware.AuthMiddleware(), taskHdl.GetStats)

		taskGroup := apiV1.Group("/tasks")
		taskGroup.Use(middleware.AuthMiddleware())
		taskGroup.Use(middleware.RateLimitMiddleware(10, time.Minute))
		{
			taskGroup.POST("", taskHdl.Create)
			taskGroup.GET("/:id", taskHdl.Get)
			taskGroup.GET("", taskHdl.List)
			taskGroup.PUT("/:id", taskHdl.Update)
			taskGroup.DELETE("/:id", taskHdl.Delete)
			taskGroup.POST("/:id/trigger", taskHdl.Trigger)
			taskGroup.GET("/:id/logs", taskHdl.ListLogs)
			taskGroup.POST("/:id/schedule", taskHdl.Schedule)
			taskGroup.GET("/:id/stats", taskHdl.GetTaskStats)
		}
	}

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", config.AppConfig.Server.Port),
		Handler: router,
	}

	// 13. Start HTTP server
	go func() {
		logger.Log.Info("Listening and serving HTTP", zap.Int("port", config.AppConfig.Server.Port))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Log.Fatal("Listen error", zap.Error(err))
		}
	}()

	// 14. Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Log.Info("Shutting down api-service...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Log.Fatal("Server forced to shutdown:", zap.Error(err))
	}

	logger.Log.Info("api-service exited")
}
