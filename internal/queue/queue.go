package queue

import (
	"context"
	"encoding/json"
	"time"
	"github.com/redis/go-redis/v9"
)

type TaskMessage struct {
	TaskID uint `json:"task_id"`
	LogID  uint `json:"log_id"`
}

type Queue struct {
	client   *redis.Client
	queueKey string
}

func NewQueue(client *redis.Client, queueKey string) *Queue {
	return &Queue{client: client, queueKey: queueKey}
}

// Push serializes message and LPUSH to Redis
func (q *Queue) Push(ctx context.Context, msg TaskMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return q.client.LPush(ctx, q.queueKey, data).Err()
}

// Pop blocking BRPOP gets message from Redis
func (q *Queue) Pop(ctx context.Context, timeout time.Duration) (*TaskMessage, error) {
	result, err := q.client.BRPop(ctx, timeout, q.queueKey).Result()
	if err != nil {
		return nil, err
	}
	
	// result is [key, value]
	if len(result) < 2 {
		return nil, redis.Nil
	}
	
	var msg TaskMessage
	if err := json.Unmarshal([]byte(result[1]), &msg); err != nil {
		return nil, err
	}
	
	return &msg, nil
}
