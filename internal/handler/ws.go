package handler

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"gobench/pkg/grpcclient"
	"gobench/pkg/jwt"
	"gobench/pkg/response"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // 开发阶段允许所有来源，生产环境需要校验
	},
}

type WSHandler struct {
	executorClient *grpcclient.ExecutorClient
}

func NewWSHandler(client *grpcclient.ExecutorClient) *WSHandler {
	return &WSHandler{executorClient: client}
}

// ServeTaskLogs 升级为 WebSocket，实时推送指定任务的执行日志变更
// GET /api/v1/ws/tasks/:id/logs?token=xxx
func (h *WSHandler) ServeTaskLogs(c *gin.Context) {
	// 1. 从 query 参数验证 JWT（WebSocket 不支持 Authorization header）
	tokenStr := c.Query("token")
	if tokenStr == "" {
		response.Error(c, http.StatusUnauthorized, "token required")
		return
	}
	if _, err := jwt.ParseToken(tokenStr); err != nil {
		response.Error(c, http.StatusUnauthorized, "invalid token")
		return
	}

	// 2. 解析 task ID
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid task id")
		return
	}
	taskID := uint(id)

	// 3. 升级为 WebSocket
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// 4. 创建可取消的 context：WebSocket 断开时取消，gRPC stream 随之结束
	streamCtx, cancelStream := context.WithCancel(context.Background())
	defer cancelStream()

	// 5. 订阅 executor gRPC 事件流（pb 转换在 grpcclient 内部完成，这里得到的是 event.Event channel）
	eventCh, err := h.executorClient.SubscribeTaskEvents(streamCtx, taskID)
	if err != nil {
		return
	}

	// 6. 启动 read goroutine（处理客户端 ping/close）
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn.SetReadLimit(512)
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			return nil
		})
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	// 7. Ping ticker
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// 8. write loop：转发事件 + 心跳
	for {
		select {
		case <-done:
			cancelStream() // 通知 gRPC stream 退出
			return

		case e, ok := <-eventCh:
			if !ok {
				return // gRPC stream 结束（executor 关闭或 ctx 取消）
			}
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteJSON(e); err != nil {
				return
			}

		case <-ticker.C:
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
