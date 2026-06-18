package executor

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"gobench/internal/model"
	"gobench/internal/queue"
	"gobench/internal/repository"
	pb "gobench/pb/executor"
	"gobench/pkg/event"
)

// ExecutorServer 实现 pb.ExecutorServiceServer 接口
type ExecutorServer struct {
	pb.UnimplementedExecutorServiceServer
	logRepo repository.TaskLogRepository
	queue   *queue.Queue
	bus     *event.Bus
}

func NewExecutorServer(logRepo repository.TaskLogRepository, q *queue.Queue, bus *event.Bus) *ExecutorServer {
	return &ExecutorServer{
		logRepo: logRepo,
		queue:   q,
		bus:     bus,
	}
}

// TriggerTask 创建 pending log，推入 Redis queue
// api-service 已提前验证 task 存在，executor 直接信任传入的 task_id
func (s *ExecutorServer) TriggerTask(ctx context.Context, req *pb.TriggerTaskRequest) (*pb.TriggerTaskResponse, error) {
	taskLog := &model.TaskLog{
		TaskID: uint(req.TaskId),
		Status: "pending",
	}
	if err := s.logRepo.Create(taskLog); err != nil {
		return nil, status.Errorf(codes.Internal, "create log: %v", err)
	}

	msg := queue.TaskMessage{
		TaskID: uint(req.TaskId),
		LogID:  taskLog.ID,
	}
	if err := s.queue.Push(ctx, msg); err != nil {
		// 补偿：入队失败时标记为 failed，避免永久 pending 的孤儿记录
		now := time.Now()
		_ = s.logRepo.UpdateStatus(taskLog.ID, "", 0, "failed", "",
			"failed to enqueue: "+err.Error(), &now, &now, 0)
		return nil, status.Errorf(codes.Internal, "enqueue: %v", err)
	}

	return &pb.TriggerTaskResponse{
		LogId:  uint32(taskLog.ID),
		Status: "pending",
	}, nil
}

// ScheduleTask 创建 pending log，推入延迟队列（ZSet）
func (s *ExecutorServer) ScheduleTask(ctx context.Context, req *pb.ScheduleTaskRequest) (*pb.ScheduleTaskResponse, error) {
	taskLog := &model.TaskLog{
		TaskID: uint(req.TaskId),
		Status: "pending",
	}
	if err := s.logRepo.Create(taskLog); err != nil {
		return nil, status.Errorf(codes.Internal, "create log: %v", err)
	}

	msg := queue.TaskMessage{
		TaskID: uint(req.TaskId),
		LogID:  taskLog.ID,
	}
	runAt := time.Now().Add(time.Duration(req.DelaySeconds) * time.Second)
	if err := s.queue.PushDelayed(ctx, msg, runAt); err != nil {
		return nil, status.Errorf(codes.Internal, "schedule: %v", err)
	}

	return &pb.ScheduleTaskResponse{
		LogId:  uint32(taskLog.ID),
		Status: "pending",
	}, nil
}

// StreamTaskEvents 从本地 event.Bus 订阅，用 gRPC server-side stream 推送给 api-service
// 每个 WebSocket 客户端对应一个 gRPC stream 连接；ws 断开时 stream.Context() 取消，goroutine 自动退出
func (s *ExecutorServer) StreamTaskEvents(req *pb.StreamRequest, stream pb.ExecutorService_StreamTaskEventsServer) error {
	taskID := uint(req.TaskId)
	ch := s.bus.Subscribe(taskID, 16)
	defer s.bus.Unsubscribe(taskID, ch)

	for {
		select {
		case <-stream.Context().Done():
			return nil
		case e, ok := <-ch:
			if !ok {
				return nil
			}
			if err := stream.Send(&pb.TaskEvent{
				Type:       e.Type,
				TaskId:     uint32(e.TaskID),
				LogId:      uint32(e.LogID),
				Status:     e.Status,
				WorkerId:   e.WorkerID,
				Output:     e.Output,
				ErrorMsg:   e.ErrorMsg,
				DurationMs: e.DurationMs,
			}); err != nil {
				return err
			}
		}
	}
}
