package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"gobench/internal/executor"
	"gobench/internal/queue"
	"gobench/internal/repository"
	"gobench/internal/worker"
	pb "gobench/pb/executor"
	"gobench/pkg/config"
	"gobench/pkg/database"
	"gobench/pkg/event"
	"gobench/pkg/logger"
	"gobench/pkg/redis"
	"gobench/pkg/tracing"
)

func main() {
	// 开发环境加载 .env 文件
	_ = godotenv.Load()

	// 1. Init logger
	if err := logger.Init(); err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Log.Info("Starting executor-service...")

	// 2. Init config
	if err := config.Init("config/config.yaml"); err != nil {
		logger.Log.Fatal("Failed to load config", zap.Error(err))
	}

	// 3. Init database（executor-service 也需要 DB 访问，用于创建/更新 TaskLog）
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

	// 6. 初始化 repository
	taskRepo := repository.NewTaskRepository()
	logRepo := repository.NewTaskLogRepository()

	// 7. 初始化 queue
	taskQueue := queue.NewQueue(redis.Client, config.AppConfig.Worker.QueueKey)

	// 8. 初始化本地 event bus（executor 内部的进程内 bus，worker.go 的 bus.Publish() 用这个）
	eventBus := event.NewBus()

	// 9. 初始化 Worker（传入 eventBus）
	taskWorker := worker.NewWorker(taskQueue, taskRepo, logRepo, eventBus)
	taskWorker.Start()
	defer taskWorker.Stop()

	// 10. 初始化 ExecutorServer
	executorSrv := executor.NewExecutorServer(logRepo, taskQueue, eventBus)

	// 11. 创建 gRPC server，带日志拦截器 + OTel tracing
	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(loggingUnaryInterceptor),
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
	)
	pb.RegisterExecutorServiceServer(grpcServer, executorSrv)

	// 12. 在 goroutine 里启动 gRPC server（监听 :9000）
	lis, err := net.Listen("tcp", ":9000")
	if err != nil {
		logger.Log.Fatal("Failed to listen on :9000", zap.Error(err))
	}
	go func() {
		logger.Log.Info("gRPC server listening on :9000")
		if err := grpcServer.Serve(lis); err != nil {
			logger.Log.Fatal("gRPC serve error", zap.Error(err))
		}
	}()

	// 13. 等待 SIGINT/SIGTERM 信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Log.Info("Shutting down executor-service...")

	// 14. 优雅关闭：grpcServer.GracefulStop()，Worker.Stop() 已在 defer 中
	grpcServer.GracefulStop()

	logger.Log.Info("executor-service exited")
}

// loggingUnaryInterceptor 记录每个 gRPC Unary 调用的方法名、耗时和错误
func loggingUnaryInterceptor(
	ctx context.Context,
	req any,
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (any, error) {
	start := time.Now()
	resp, err := handler(ctx, req)
	logger.Log.Info("gRPC call",
		zap.String("method", info.FullMethod),
		zap.Duration("duration", time.Since(start)),
		zap.Error(err),
	)
	return resp, err
}
