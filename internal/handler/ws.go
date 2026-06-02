package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"gobench/pkg/event"
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
	bus *event.Bus
}

func NewWSHandler(bus *event.Bus) *WSHandler {
	return &WSHandler{bus: bus}
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
	_, err := jwt.ParseToken(tokenStr)
	if err != nil {
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
		// Upgrade 失败时 gorilla 会自动写 HTTP 错误，不需要额外处理
		return
	}
	defer conn.Close()

	// 4. 订阅事件总线
	eventCh := h.bus.Subscribe(taskID, 16)
	defer h.bus.Unsubscribe(taskID, eventCh)

	// 5. 启动 read goroutine（处理客户端发来的 ping/关闭消息）
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
			_, _, err := conn.ReadMessage()
			if err != nil {
				return // 客户端断开或错误，退出
			}
		}
	}()

	// 6. Ping ticker，每 30s 发一次 ping 检测连接活跃
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// 7. write loop：转发事件 + 心跳
	for {
		select {
		case <-done:
			return // 客户端断开

		case e, ok := <-eventCh:
			if !ok {
				return // channel 被关闭
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
