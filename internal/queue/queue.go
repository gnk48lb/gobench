package queue

import (
	"context"
	"encoding/json"
	"time"
	"github.com/redis/go-redis/v9"
)

type TaskMessage struct {
	TaskID   uint `json:"task_id"`
	LogID    uint `json:"log_id"`
	RetryNum int  `json:"retry_num"`
}

type Queue struct {
	client     *redis.Client
	queueKey   string
	delayedKey string
}

func NewQueue(client *redis.Client, queueKey string) *Queue {
	return &Queue{
		client:     client,
		queueKey:   queueKey,
		delayedKey: "gobench:task:delayed",
	}
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

// PushDelayed pushes a message to the delayed sorted set
func (q *Queue) PushDelayed(ctx context.Context, msg TaskMessage, runAt time.Time) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return q.client.ZAdd(ctx, q.delayedKey, redis.Z{
		Score:  float64(runAt.Unix()),
		Member: data,
	}).Err()
}

// MoveDelayedToQueue moves ready tasks from delayed set to the active queue
func (q *Queue) MoveDelayedToQueue(ctx context.Context) (int, error) {
	script := `
		local items = redis.call("ZRANGEBYSCORE", KEYS[1], "-inf", ARGV[1])
		if #items > 0 then
			redis.call("LPUSH", KEYS[2], unpack(items))
			redis.call("ZREMRANGEBYSCORE", KEYS[1], "-inf", ARGV[1])
		end
		return #items
	`
	now := time.Now().Unix()
	res, err := q.client.Eval(ctx, script, []string{q.delayedKey, q.queueKey}, now).Result()
	if err != nil && err != redis.Nil {
		return 0, err
	}
	
	count, _ := res.(int64)
	return int(count), nil
}
