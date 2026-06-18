# Phase 4 Walkthrough: 单体拆分为 api-service + executor-service

## 变更总结

### 新建文件 (8 个)

| 文件 | 用途 |
|------|------|
| [executor.proto](file:///d:/code/VisualStudioCode/go/gobench/proto/executor.proto) | gRPC 服务定义：TriggerTask、ScheduleTask (Unary) + StreamTaskEvents (Server streaming) |
| [tracing.go](file:///d:/code/VisualStudioCode/go/gobench/pkg/tracing/tracing.go) | OpenTelemetry OTLP exporter 初始化 |
| [executor.go](file:///d:/code/VisualStudioCode/go/gobench/pkg/grpcclient/executor.go) | gRPC 客户端封装，内部完成 pb→Event 类型转换 |
| [server.go](file:///d:/code/VisualStudioCode/go/gobench/internal/executor/server.go) | gRPC 服务端实现（executor-service 核心） |
| [api-service/main.go](file:///d:/code/VisualStudioCode/go/gobench/cmd/api-service/main.go) | api-service 入口（HTTP + gRPC 客户端） |
| [executor-service/main.go](file:///d:/code/VisualStudioCode/go/gobench/cmd/executor-service/main.go) | executor-service 入口（gRPC server + Worker） |
| [Dockerfile](file:///d:/code/VisualStudioCode/go/gobench/Dockerfile) | 多阶段构建，通过 CMD_PATH 选择构建目标 |
| [docker-compose.yml](file:///d:/code/VisualStudioCode/go/gobench/docker-compose.yml) | 5 服务编排：mysql、redis、jaeger、executor、api |

### 修改文件 (4 个)

| 文件 | 变更 |
|------|------|
| [config.go](file:///d:/code/VisualStudioCode/go/gobench/pkg/config/config.go) | 新增 `ExecutorConfig` + `TracingConfig` struct 及 BindEnv |
| [config.yaml](file:///d:/code/VisualStudioCode/go/gobench/config/config.yaml) | 追加 `executor` 和 `tracing` 配置块 |
| [task.go](file:///d:/code/VisualStudioCode/go/gobench/internal/service/task.go) | `taskService` 依赖从 `*queue.Queue` 改为 `TaskExecutor` 接口 |
| [ws.go](file:///d:/code/VisualStudioCode/go/gobench/internal/handler/ws.go) | 事件源从本地 `event.Bus` 改为 gRPC streaming (`ExecutorClient`) |

### 删除文件 (1 个)

| 文件 | 说明 |
|------|------|
| `cmd/server/main.go` | 旧单体入口，功能已拆分到 api-service 和 executor-service |

### 辅助文件更新

- [.gitignore](file:///d:/code/VisualStudioCode/go/gobench/.gitignore) — 添加 `pb/` 目录
- [.env.example](file:///d:/code/VisualStudioCode/go/gobench/.env.example) — 添加 4 个新环境变量
- [.env](file:///d:/code/VisualStudioCode/go/gobench/.env) — 同步添加新环境变量

### 未修改的文件

以下文件**完全不变**：
`internal/handler/task.go`、`internal/handler/auth.go`、`internal/middleware/`、`internal/model/`、
`internal/repository/`、`internal/queue/`、`internal/worker/`、`pkg/event/bus.go`、
`internal/scheduler/scheduler.go`、所有 pkg 基础设施

---

## 你需要手动执行的步骤

### 1. 安装 Go 依赖

```bash
go get google.golang.org/grpc@latest
go get google.golang.org/protobuf@latest
go get go.opentelemetry.io/otel@latest
go get go.opentelemetry.io/otel/sdk@latest
go get go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc@latest
go get go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc@latest
go get go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin@latest
```

### 2. 安装 protoc 插件

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

> [!IMPORTANT]
> `protoc` 本体需从 [GitHub Releases](https://github.com/protocolbuffers/protobuf/releases) 单独下载安装。

### 3. 生成 pb stub

```bash
mkdir -p pb/executor
protoc --go_out=./pb --go-grpc_out=./pb \
  --go_opt=paths=source_relative \
  --go-grpc_opt=paths=source_relative \
  proto/executor.proto
```

生成文件：`pb/executor/executor.pb.go` 和 `pb/executor/executor_grpc.pb.go`

### 4. 编译检查

```bash
go build ./...
```

### 5. 本地双服务验证

```bash
# 终端 1
go run ./cmd/executor-service/

# 终端 2
go run ./cmd/api-service/
```

> [!TIP]
> 调试建议：第一次跑时 `TRACING_ENABLED=false`，等 gRPC 通信跑通后再开 OTel。

### 6. Docker 验证（可选）

```bash
docker-compose up --build
```

打开 http://localhost:16686 (Jaeger UI) 触发任务后查看 trace span。
