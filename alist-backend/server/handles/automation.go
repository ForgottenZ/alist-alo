package handles

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/internal/automation"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/gin-gonic/gin"
)

type automationOperationReq struct {
	Type          string `json:"type"`
	Source        string `json:"source"`
	Destination   string `json:"destination"`
	NewName       string `json:"new_name"`
	InnerPath     string `json:"inner_path"`
	Password      string `json:"password"`
	CacheFull     bool   `json:"cache_full"`
	PutIntoNewDir bool   `json:"put_into_new_dir"`
}

type automationTaskReq struct {
	ID            uint                     `json:"id"`
	Name          string                   `json:"name"`
	Description   string                   `json:"description"`
	Enabled       bool                     `json:"enabled"`
	ScheduleType  string                   `json:"schedule_type"`
	IntervalValue int                      `json:"interval_value"`
	IntervalUnit  string                   `json:"interval_unit"`
	TimeOfDay     string                   `json:"time_of_day"`
	Weekdays      []int                    `json:"weekdays"`
	OnceAt        string                   `json:"once_at"`
	Operations    []automationOperationReq `json:"operations"`
}

type automationToggleReq struct {
	ID      uint `json:"id"`
	Enabled bool `json:"enabled"`
}

func SetupAutomationRoute(g *gin.RouterGroup) {
	g.GET("/list", func(c *gin.Context) {
		user := c.MustGet("user").(*model.User)
		tasks, err := automation.ListTasks(user)
		if err != nil {
			common.ErrorResp(c, err, 500)
			return
		}
		resp := make([]gin.H, 0, len(tasks))
		for _, task := range tasks {
			resp = append(resp, convertAutomationTask(task))
		}
		common.SuccessResp(c, resp)
	})
	g.GET("/detail", func(c *gin.Context) {
		user := c.MustGet("user").(*model.User)
		idStr := c.Query("id")
		if idStr == "" {
			common.ErrorStrResp(c, "缺少任务ID", 400)
			return
		}
		idUint, err := strconv.ParseUint(idStr, 10, 64)
		if err != nil {
			common.ErrorResp(c, err, 400)
			return
		}
		task, ops, histories, err := automation.GetTaskDetail(user, uint(idUint))
		if err != nil {
			common.ErrorResp(c, err, 500)
			return
		}
		common.SuccessResp(c, gin.H{
			"task":      convertAutomationTask(*task, ops),
			"histories": histories,
		})
	})
	g.POST("/create", func(c *gin.Context) {
		user := c.MustGet("user").(*model.User)
		var req automationTaskReq
		if err := c.ShouldBindJSON(&req); err != nil {
			common.ErrorResp(c, err, 400)
			return
		}
		payload := toAutomationPayload(&req)
		task, err := automation.CreateTask(user, payload)
		if err != nil {
			common.ErrorResp(c, err, 500)
			return
		}
		common.SuccessResp(c, convertAutomationTask(*task))
	})
	g.POST("/update", func(c *gin.Context) {
		user := c.MustGet("user").(*model.User)
		var req automationTaskReq
		if err := c.ShouldBindJSON(&req); err != nil {
			common.ErrorResp(c, err, 400)
			return
		}
		payload := toAutomationPayload(&req)
		task, err := automation.UpdateTask(user, payload)
		if err != nil {
			common.ErrorResp(c, err, 500)
			return
		}
		common.SuccessResp(c, convertAutomationTask(*task))
	})
	g.POST("/delete", func(c *gin.Context) {
		user := c.MustGet("user").(*model.User)
		var req struct {
			ID uint `json:"id"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			common.ErrorResp(c, err, 400)
			return
		}
		if err := automation.DeleteTask(user, req.ID); err != nil {
			common.ErrorResp(c, err, 500)
			return
		}
		common.SuccessResp(c)
	})
	g.POST("/toggle", func(c *gin.Context) {
		user := c.MustGet("user").(*model.User)
		var req automationToggleReq
		if err := c.ShouldBindJSON(&req); err != nil {
			common.ErrorResp(c, err, 400)
			return
		}
		task, err := automation.ToggleTask(user, req.ID, req.Enabled)
		if err != nil {
			common.ErrorResp(c, err, 500)
			return
		}
		common.SuccessResp(c, convertAutomationTask(*task))
	})
	g.POST("/run", func(c *gin.Context) {
		user := c.MustGet("user").(*model.User)
		var req struct {
			ID uint `json:"id"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			common.ErrorResp(c, err, 400)
			return
		}
		if err := automation.RunNow(user, req.ID); err != nil {
			common.ErrorResp(c, err, 500)
			return
		}
		common.SuccessResp(c)
	})
}

func toAutomationPayload(req *automationTaskReq) *automation.TaskPayload {
	payload := &automation.TaskPayload{
		ID:            req.ID,
		Name:          req.Name,
		Description:   req.Description,
		Enabled:       req.Enabled,
		ScheduleType:  req.ScheduleType,
		IntervalValue: req.IntervalValue,
		IntervalUnit:  req.IntervalUnit,
		TimeOfDay:     req.TimeOfDay,
		Weekdays:      req.Weekdays,
	}
	if strings.TrimSpace(req.OnceAt) != "" {
		if parsed, err := time.Parse(time.RFC3339, req.OnceAt); err == nil {
			payload.OnceAt = &parsed
		}
	}
	ops := make([]automation.Operation, 0, len(req.Operations))
	for _, op := range req.Operations {
		ops = append(ops, automation.Operation{
			Type:          op.Type,
			Source:        op.Source,
			Destination:   op.Destination,
			NewName:       op.NewName,
			InnerPath:     op.InnerPath,
			Password:      op.Password,
			CacheFull:     op.CacheFull,
			PutIntoNewDir: op.PutIntoNewDir,
		})
	}
	payload.Operations = ops
	return payload
}

func convertAutomationTask(task model.AutomationTask, preset ...[]automation.Operation) gin.H {
	var weekdays []int
	if strings.TrimSpace(task.WeekdaysJSON) != "" {
		_ = json.Unmarshal([]byte(task.WeekdaysJSON), &weekdays)
	}
	var ops []automation.Operation
	if len(preset) > 0 && preset[0] != nil {
		ops = preset[0]
	} else if strings.TrimSpace(task.OperationsJSON) != "" {
		_ = json.Unmarshal([]byte(task.OperationsJSON), &ops)
	}
	return gin.H{
		"id":             task.ID,
		"name":           task.Name,
		"description":    task.Description,
		"enabled":        task.Enabled,
		"schedule_type":  task.ScheduleType,
		"interval_value": task.IntervalValue,
		"interval_unit":  task.IntervalUnit,
		"time_of_day":    task.TimeOfDay,
		"weekdays":       weekdays,
		"once_at":        task.OnceAt,
		"next_run":       task.NextRun,
		"last_run":       task.LastRun,
		"last_status":    task.LastStatus,
		"last_error":     task.LastError,
		"operations":     ops,
	}
}
