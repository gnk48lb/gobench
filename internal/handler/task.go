package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gobench/internal/model"
	"gobench/internal/scheduler"
	"gobench/internal/service"
	"gobench/pkg/apperrors"
	"gobench/pkg/response"
)

type TaskHandler struct {
	taskService service.TaskService
	scheduler   *scheduler.Scheduler
}

func NewTaskHandler(taskService service.TaskService, sched *scheduler.Scheduler) *TaskHandler {
	return &TaskHandler{taskService: taskService, scheduler: sched}
}

type CreateTaskRequest struct {
	Name       string `json:"name" binding:"required,min=1,max=100"`
	TaskType   string `json:"task_type" binding:"required,oneof=http shell function"`
	CronExpr   string `json:"cron_expr"`
	Payload    string `json:"payload"`
	RetryCount int    `json:"retry_count" binding:"min=0,max=10"`
	Timeout    int    `json:"timeout" binding:"min=1,max=3600"`
	Status     string `json:"status"`
}

type UpdateTaskRequest struct {
	Name       string `json:"name" binding:"required,min=1,max=100"`
	TaskType   string `json:"task_type" binding:"required,oneof=http shell function"`
	CronExpr   string `json:"cron_expr"`
	Payload    string `json:"payload"`
	RetryCount int    `json:"retry_count" binding:"min=0,max=10"`
	Timeout    int    `json:"timeout" binding:"min=1,max=3600"`
	Status     string `json:"status" binding:"omitempty,oneof=active paused deleted"`
}

func (h *TaskHandler) Create(c *gin.Context) {
	var req CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}

	task := model.Task{
		Name:       req.Name,
		TaskType:   req.TaskType,
		CronExpr:   req.CronExpr,
		Payload:    req.Payload,
		RetryCount: req.RetryCount,
		Timeout:    req.Timeout,
		Status:     "active",
	}

	// Get user ID from context (set by AuthMiddleware)
	if id, ok := c.Get("userID"); ok {
		if uid, valid := id.(uint); valid {
			task.CreatorID = uid
		}
	}

	if err := h.taskService.CreateTask(&task); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	h.scheduler.RegisterTask(&task)

	response.Success(c, task)
}

func (h *TaskHandler) Get(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid task ID")
		return
	}

	task, err := h.taskService.GetTask(uint(id))
	if err != nil {
		if errors.Is(err, apperrors.ErrTaskNotFound) {
			response.Error(c, http.StatusNotFound, err.Error())
			return
		}
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(c, task)
}

type ListTasksResponse struct {
	Total int64         `json:"total"`
	Items []*model.Task `json:"items"`
}

func (h *TaskHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	tasks, total, err := h.taskService.ListTasks(page, pageSize)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(c, ListTasksResponse{
		Total: total,
		Items: tasks,
	})
}

func (h *TaskHandler) Update(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid task ID")
		return
	}

	var req UpdateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}

	task := model.Task{
		Name:       req.Name,
		TaskType:   req.TaskType,
		CronExpr:   req.CronExpr,
		Payload:    req.Payload,
		RetryCount: req.RetryCount,
		Timeout:    req.Timeout,
		Status:     req.Status,
	}
	task.ID = uint(id)

	if err := h.taskService.UpdateTask(&task); err != nil {
		if errors.Is(err, apperrors.ErrTaskNotFound) {
			response.Error(c, http.StatusNotFound, err.Error())
			return
		}
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	if updated, err := h.taskService.GetTask(task.ID); err == nil {
		h.scheduler.UpdateTask(updated)
	}

	response.Success(c, nil)
}

func (h *TaskHandler) Delete(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid task ID")
		return
	}

	if err := h.taskService.DeleteTask(uint(id)); err != nil {
		if errors.Is(err, apperrors.ErrTaskNotFound) {
			response.Error(c, http.StatusNotFound, err.Error())
			return
		}
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	h.scheduler.UnregisterTask(uint(id))

	response.Success(c, nil)
}

func (h *TaskHandler) Trigger(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid task ID")
		return
	}

	taskLog, err := h.taskService.TriggerTask(uint(id))
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(c, taskLog)
}

type ListTaskLogsResponse struct {
	Total int64            `json:"total"`
	Items []*model.TaskLog `json:"items"`
}

func (h *TaskHandler) ListLogs(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid task ID")
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	logs, total, err := h.taskService.GetTaskLogs(uint(id), page, pageSize)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(c, ListTaskLogsResponse{
		Total: total,
		Items: logs,
	})
}

type ScheduleRequest struct {
	DelaySeconds int `json:"delay_seconds" binding:"required,min=1"`
}

func (h *TaskHandler) Schedule(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid task ID")
		return
	}

	var req ScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid request body")
		return
	}

	taskLog, err := h.taskService.ScheduleTask(uint(id), req.DelaySeconds)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(c, taskLog)
}

func (h *TaskHandler) GetStats(c *gin.Context) {
	since := time.Now().Add(-24 * time.Hour)
	stats, err := h.taskService.GetOverallStats(since)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, stats)
}

func (h *TaskHandler) GetTaskStats(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid task ID")
		return
	}

	stats, err := h.taskService.GetTaskStats(uint(id))
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, stats)
}
