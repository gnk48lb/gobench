package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gobench/internal/model"
	"gobench/internal/service"
	"gobench/pkg/response"
)

type TaskHandler struct {
	taskService service.TaskService
}

func NewTaskHandler(taskService service.TaskService) *TaskHandler {
	return &TaskHandler{taskService: taskService}
}

func (h *TaskHandler) Create(c *gin.Context) {
	var task model.Task
	if err := c.ShouldBindJSON(&task); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid request body")
		return
	}

	// Get user ID from context (set by AuthMiddleware)
	userID, exists := c.Get("userID")
	if exists {
		task.CreatorID = userID.(uint)
	}

	if err := h.taskService.CreateTask(&task); err != nil {
		response.Error(c, 500, err.Error())
		return
	}

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
		response.Error(c, 404, err.Error())
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
		response.Error(c, 500, err.Error())
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

	var task model.Task
	if err := c.ShouldBindJSON(&task); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid request body")
		return
	}
	task.ID = uint(id)

	if err := h.taskService.UpdateTask(&task); err != nil {
		response.Error(c, 500, err.Error())
		return
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
		response.Error(c, 500, err.Error())
		return
	}

	response.Success(c, nil)
}
