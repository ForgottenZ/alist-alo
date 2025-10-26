package handles

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/internal/automation"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/gin-gonic/gin"
)

const automationTimeLayout = "2006/01/02 15:04"

type AutomationStepPayload struct {
	ID        uint              `json:"id"`
	SortOrder int               `json:"sort_order"`
	Action    string            `json:"action"`
	Source    string            `json:"source"`
	Target    string            `json:"target"`
	Options   map[string]string `json:"options"`
}

type AutomationTaskPayload struct {
	ID            uint                    `json:"id"`
	Name          string                  `json:"name" binding:"required"`
	Enabled       bool                    `json:"enabled"`
	ScheduleType  string                  `json:"schedule_type" binding:"required"`
	IntervalValue int                     `json:"interval_value"`
	IntervalUnit  string                  `json:"interval_unit"`
	Weekdays      []int                   `json:"weekdays"`
	TimeOfDay     string                  `json:"time_of_day"`
	SpecificTime  string                  `json:"specific_time"`
	Steps         []AutomationStepPayload `json:"steps"`
}

type AutomationTaskResp struct {
	ID            uint                    `json:"id"`
	Name          string                  `json:"name"`
	Enabled       bool                    `json:"enabled"`
	ScheduleType  string                  `json:"schedule_type"`
	IntervalValue int                     `json:"interval_value"`
	IntervalUnit  string                  `json:"interval_unit"`
	Weekdays      []int                   `json:"weekdays"`
	TimeOfDay     string                  `json:"time_of_day"`
	SpecificTime  string                  `json:"specific_time"`
	LastRunAt     *time.Time              `json:"last_run_at"`
	NextRunAt     *time.Time              `json:"next_run_at"`
	Status        string                  `json:"status"`
	LastMessage   string                  `json:"last_message"`
	Steps         []AutomationStepPayload `json:"steps"`
}

type AutomationHistoryResp struct {
	ID         uint   `json:"id"`
	Status     string `json:"status"`
	Message    string `json:"message"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
}

type AutomationIDReq struct {
	ID uint `json:"id" binding:"required"`
}

type AutomationToggleReq struct {
	ID      uint `json:"id" binding:"required"`
	Enabled bool `json:"enabled"`
}

func toAutomationTaskResp(task *model.AutomationTask) AutomationTaskResp {
	steps := make([]AutomationStepPayload, 0, len(task.Steps))
	for _, step := range task.Steps {
		options := step.Options
		if options == nil {
			options = map[string]string{}
		}
		steps = append(steps, AutomationStepPayload{
			ID:        step.ID,
			SortOrder: step.SortOrder,
			Action:    step.Action,
			Source:    step.Source,
			Target:    step.Target,
			Options:   options,
		})
	}
	specific := ""
	if task.SpecificTime != nil {
		specific = task.SpecificTime.In(time.Local).Format(automationTimeLayout)
	}
	return AutomationTaskResp{
		ID:            task.ID,
		Name:          task.Name,
		Enabled:       task.Enabled,
		ScheduleType:  task.ScheduleType,
		IntervalValue: task.IntervalValue,
		IntervalUnit:  task.IntervalUnit,
		Weekdays:      task.GetWeekdays(),
		TimeOfDay:     task.TimeOfDay,
		SpecificTime:  specific,
		LastRunAt:     task.LastRunAt,
		NextRunAt:     task.NextRunAt,
		Status:        task.Status,
		LastMessage:   task.LastMessage,
		Steps:         steps,
	}
}

func parseTimeOfDay(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	layouts := []string{"15:04", "15:04:05"}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, value); err == nil {
			if layout == "15:04:05" {
				return t.Format("15:04:05"), nil
			}
			return t.Format("15:04"), nil
		}
	}
	return "", fmt.Errorf("时间格式无效")
}

func bindTaskPayload(c *gin.Context, req *AutomationTaskPayload) bool {
	if err := c.ShouldBind(req); err != nil {
		common.ErrorResp(c, err, 400)
		return false
	}
	if len(req.Steps) == 0 {
		common.ErrorStrResp(c, "至少需要一个操作步骤", 400)
		return false
	}
	schedule := strings.ToLower(req.ScheduleType)
	switch schedule {
	case "interval":
		if req.IntervalValue <= 0 {
			common.ErrorStrResp(c, "间隔任务需要正整数间隔", 400)
			return false
		}
		if req.IntervalUnit == "" {
			req.IntervalUnit = "hour"
		}
	case "weekly":
		if len(req.Weekdays) == 0 {
			common.ErrorStrResp(c, "请选择至少一个执行日", 400)
			return false
		}
		normalized, err := parseTimeOfDay(req.TimeOfDay)
		if err != nil {
			common.ErrorResp(c, err, 400)
			return false
		}
		req.TimeOfDay = normalized
	case "once":
		if strings.TrimSpace(req.SpecificTime) == "" {
			common.ErrorStrResp(c, "请填写执行时间", 400)
			return false
		}
	default:
		common.ErrorStrResp(c, "未知的调度类型", 400)
		return false
	}
	req.ScheduleType = schedule
	return true
}

func fillTaskFromPayload(req *AutomationTaskPayload, task *model.AutomationTask, creator *model.User) error {
	task.Name = req.Name
	task.Enabled = req.Enabled
	task.ScheduleType = req.ScheduleType
	task.IntervalValue = req.IntervalValue
	task.IntervalUnit = strings.ToLower(req.IntervalUnit)
	task.TimeOfDay = req.TimeOfDay
	task.CreatorID = creator.ID
	task.SetWeekdays(req.Weekdays)
	if req.ScheduleType == "once" {
		loc := time.Local
		t, err := time.ParseInLocation(automationTimeLayout, req.SpecificTime, loc)
		if err != nil {
			return err
		}
		task.SpecificTime = &t
	} else {
		task.SpecificTime = nil
	}
	return nil
}

func buildSteps(reqSteps []AutomationStepPayload) []model.AutomationStep {
	steps := make([]model.AutomationStep, 0, len(reqSteps))
	for idx, payload := range reqSteps {
		order := payload.SortOrder
		if order == 0 {
			order = idx + 1
		}
		opts := payload.Options
		if opts == nil {
			opts = map[string]string{}
		}
		steps = append(steps, model.AutomationStep{
			ID:        payload.ID,
			SortOrder: order,
			Action:    strings.ToLower(payload.Action),
			Source:    payload.Source,
			Target:    payload.Target,
			Options:   opts,
		})
	}
	return steps
}

func ensureAutomationPermission(c *gin.Context) (*model.User, bool) {
	user := c.MustGet("user").(*model.User)
	if !user.CanAutomation() {
		common.ErrorResp(c, errs.PermissionDenied, 403)
		return nil, false
	}
	return user, true
}

func ListAutomationTasks(c *gin.Context) {
	if _, ok := ensureAutomationPermission(c); !ok {
		return
	}
	tasks, err := automation.ListTasks()
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	resp := make([]AutomationTaskResp, 0, len(tasks))
	for i := range tasks {
		resp = append(resp, toAutomationTaskResp(&tasks[i]))
	}
	common.SuccessResp(c, resp)
}

func CreateAutomationTask(c *gin.Context) {
	var req AutomationTaskPayload
	if !bindTaskPayload(c, &req) {
		return
	}
	user, ok := ensureAutomationPermission(c)
	if !ok {
		return
	}
	task := &model.AutomationTask{}
	if err := fillTaskFromPayload(&req, task, user); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	task.LastMessage = ""
	steps := buildSteps(req.Steps)
	created, err := automation.CreateTask(task, steps)
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	common.SuccessResp(c, toAutomationTaskResp(created))
}

func UpdateAutomationTask(c *gin.Context) {
	var req AutomationTaskPayload
	if !bindTaskPayload(c, &req) {
		return
	}
	if req.ID == 0 {
		common.ErrorStrResp(c, "缺少任务编号", 400)
		return
	}
	user, ok := ensureAutomationPermission(c)
	if !ok {
		return
	}
	task := &model.AutomationTask{ID: req.ID}
	if err := fillTaskFromPayload(&req, task, user); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	task.LastMessage = ""
	steps := buildSteps(req.Steps)
	updated, err := automation.UpdateTask(task, steps)
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	common.SuccessResp(c, toAutomationTaskResp(updated))
}

func DeleteAutomationTask(c *gin.Context) {
	if _, ok := ensureAutomationPermission(c); !ok {
		return
	}
	var req AutomationIDReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	if err := automation.DeleteTask(req.ID); err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	common.SuccessResp(c)
}

func ToggleAutomationTask(c *gin.Context) {
	if _, ok := ensureAutomationPermission(c); !ok {
		return
	}
	var req AutomationToggleReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	task, err := automation.ToggleTask(req.ID, req.Enabled)
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	common.SuccessResp(c, toAutomationTaskResp(task))
}

func RunAutomationTask(c *gin.Context) {
	if _, ok := ensureAutomationPermission(c); !ok {
		return
	}
	var req AutomationIDReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	if err := automation.RunTaskNow(req.ID); err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	common.SuccessResp(c)
}

func AutomationHistory(c *gin.Context) {
	if _, ok := ensureAutomationPermission(c); !ok {
		return
	}
	idStr := c.Query("id")
	if idStr == "" {
		common.ErrorStrResp(c, "缺少任务编号", 400)
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	history, err := automation.ListHistory(uint(id))
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	resp := make([]AutomationHistoryResp, 0, len(history))
	for _, item := range history {
		resp = append(resp, AutomationHistoryResp{
			ID:         item.ID,
			Status:     item.Status,
			Message:    item.Message,
			StartedAt:  item.StartedAt.In(time.Local).Format("2006-01-02 15:04:05"),
			FinishedAt: item.FinishedAt.In(time.Local).Format("2006-01-02 15:04:05"),
		})
	}
	common.SuccessResp(c, resp)
}
