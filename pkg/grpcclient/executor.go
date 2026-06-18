package grpcclient

import (
	"context"
	"fmt"

	pb "gobench/pb/executor"
	"gobench/pkg/event"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type ExecutorClient struct {
	conn   *grpc.ClientConn
	client pb.ExecutorServiceClient
}

func NewExecutorClient(addr string) (*ExecutorClient, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("connect executor-service %s: %w", addr, err)
	}
	return &ExecutorClient{conn: conn, client: pb.NewExecutorServiceClient(conn)}, nil
}

func (c *ExecutorClient) Close() {
	_ = c.conn.Close()
}

// TriggerTask 触发立即执行，返回 log_id
func (c *ExecutorClient) TriggerTask(taskID uint) (uint, error) {
	resp, err := c.client.TriggerTask(context.Background(), &pb.TriggerTaskRequest{
		TaskId: uint32(taskID),
	})
	if err != nil {
		return 0, fmt.Errorf("TriggerTask RPC: %w", err)
	}
	return uint(resp.LogId), nil
}

// ScheduleTask 延迟调度，返回 log_id
func (c *ExecutorClient) ScheduleTask(taskID uint, delaySeconds int) (uint, error) {
	resp, err := c.client.ScheduleTask(context.Background(), &pb.ScheduleTaskRequest{
		TaskId:       uint32(taskID),
		DelaySeconds: int32(delaySeconds),
	})
	if err != nil {
		return 0, fmt.Errorf("ScheduleTask RPC: %w", err)
	}
	return uint(resp.LogId), nil
}

// SubscribeTaskEvents 订阅任务事件，返回 event.Event channel。
// 调用方负责在不需要时 cancel 传入的 ctx，goroutine 会自动退出并关闭 channel。
// pb 类型转换在这里完成，调用方（ws.go）无需导入 pb 包。
func (c *ExecutorClient) SubscribeTaskEvents(ctx context.Context, taskID uint) (<-chan event.Event, error) {
	stream, err := c.client.StreamTaskEvents(ctx, &pb.StreamRequest{
		TaskId: uint32(taskID),
	})
	if err != nil {
		return nil, fmt.Errorf("StreamTaskEvents RPC: %w", err)
	}

	ch := make(chan event.Event, 16)
	go func() {
		defer close(ch)
		for {
			e, err := stream.Recv()
			if err != nil {
				return // ctx cancelled or stream ended
			}
			select {
			case ch <- event.Event{
				Type:       e.Type,
				TaskID:     uint(e.TaskId),
				LogID:      uint(e.LogId),
				Status:     e.Status,
				WorkerID:   e.WorkerId,
				Output:     e.Output,
				ErrorMsg:   e.ErrorMsg,
				DurationMs: e.DurationMs,
			}:
			default: // 慢消费者丢弃，与 bus.go 行为一致
			}
		}
	}()
	return ch, nil
}
