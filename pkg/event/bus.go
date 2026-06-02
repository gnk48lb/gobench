package event

import (
	"sync"
)

// 事件类型常量
const (
	TypeLogUpdate = "log_update"
)

// Event 是在总线上传递的事件结构
type Event struct {
	Type       string `json:"type"`
	TaskID     uint   `json:"task_id"`
	LogID      uint   `json:"log_id"`
	Status     string `json:"status"` // pending/running/success/failed
	WorkerID   string `json:"worker_id"`
	Output     string `json:"output,omitempty"`
	ErrorMsg   string `json:"error_msg,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
}

// Bus 是线程安全的进程内事件总线，按 taskID 分组订阅
type Bus struct {
	mu          sync.RWMutex
	subscribers map[uint][]chan Event
}

func NewBus() *Bus {
	return &Bus{
		subscribers: make(map[uint][]chan Event),
	}
}

// Subscribe 订阅某个任务的事件，返回一个只读 channel
// bufSize 建议传 16，防止慢消费者阻塞 Publish
func (b *Bus) Subscribe(taskID uint, bufSize int) chan Event {
	ch := make(chan Event, bufSize)
	b.mu.Lock()
	b.subscribers[taskID] = append(b.subscribers[taskID], ch)
	b.mu.Unlock()
	return ch
}

// Unsubscribe 取消订阅，关闭并移除 channel
func (b *Bus) Unsubscribe(taskID uint, ch chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	subs := b.subscribers[taskID]
	for i, sub := range subs {
		if sub == ch {
			close(ch)
			b.subscribers[taskID] = append(subs[:i], subs[i+1:]...)
			break
		}
	}
	if len(b.subscribers[taskID]) == 0 {
		delete(b.subscribers, taskID)
	}
}

// Publish 向所有订阅了该任务的 channel 发送事件
// 使用非阻塞发送，慢消费者会丢失事件（可接受，WebSocket 是最终一致显示）
func (b *Bus) Publish(e Event) {
	b.mu.RLock()
	subs := make([]chan Event, len(b.subscribers[e.TaskID]))
	copy(subs, b.subscribers[e.TaskID])
	b.mu.RUnlock()

	for _, ch := range subs {
		select {
		case ch <- e:
		default:
			// 消费者太慢，跳过此事件
		}
	}
}
