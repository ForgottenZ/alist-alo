package handles

import (
        "net/http"
        "time"

        "github.com/alist-org/alist/v3/internal/automation"
        "github.com/alist-org/alist/v3/internal/model"
        "github.com/alist-org/alist/v3/pkg/utils"
        "github.com/alist-org/alist/v3/server/common"
        "github.com/gin-gonic/gin"
)

type automationTaskPayload struct {
        ID         string                 `json:"id"`
        Name       string                 `json:"name"`
        Enabled    bool                   `json:"enabled"`
        Schedule   automation.Schedule    `json:"schedule"`
        Operations []automation.Operation `json:"operations"`
}

func getAutomationManager(c *gin.Context) (*automation.Manager, bool) {
        mgr := automation.ManagerInstance()
        if mgr == nil {
                common.ErrorStrResp(c, "自动化服务未初始化", http.StatusInternalServerError)
                return nil, false
        }
        return mgr, true
}

func AutomationList(c *gin.Context) {
        mgr, ok := getAutomationManager(c)
        if !ok {
                return
        }
        common.SuccessResp(c, gin.H{"tasks": mgr.List()})
}

func prepareAutomationOperations(ops []automation.Operation) []automation.Operation {
        result := make([]automation.Operation, 0, len(ops))
        for _, op := range ops {
                if op.ID == "" {
                        op.ID = utils.RandStringRunes(8)
                }
                op.Source = utils.FixAndCleanPath(op.Source)
                if op.Destination != "" {
                        op.Destination = utils.FixAndCleanPath(op.Destination)
                }
                result = append(result, op)
        }
        return result
}

func parseAutomationTask(c *gin.Context) (*automation.Task, bool) {
        var payload automationTaskPayload
        if err := c.ShouldBindJSON(&payload); err != nil {
                common.ErrorResp(c, err, http.StatusBadRequest)
                return nil, false
        }
        user := c.MustGet("user").(*model.User)
        task := &automation.Task{
                ID:         payload.ID,
                Name:       payload.Name,
                Enabled:    payload.Enabled,
                Schedule:   payload.Schedule,
                Operations: prepareAutomationOperations(payload.Operations),
        }
        if user != nil {
                task.CreatorID = user.ID
                task.Creator = user.Username
                task.CreatorRole = user.Role
        }
        switch task.Schedule.Mode {
        case "", automation.ScheduleOnce:
                if task.Schedule.Mode == "" {
                        task.Schedule.Mode = automation.ScheduleOnce
                }
        case automation.ScheduleInterval:
                if task.Schedule.IntervalValue <= 0 {
                        common.ErrorStrResp(c, "间隔任务需要正整数的间隔值", http.StatusBadRequest)
                        return nil, false
                }
        case automation.ScheduleWeekly:
                if len(task.Schedule.Weekdays) == 0 {
                        common.ErrorStrResp(c, "每周任务需至少选择一天", http.StatusBadRequest)
                        return nil, false
                }
        default:
                common.ErrorStrResp(c, "未知的任务计划类型", http.StatusBadRequest)
                return nil, false
        }
        if task.Schedule.Mode != automation.ScheduleWeekly {
                task.Schedule.Weekdays = nil
        }
        if task.Schedule.Mode != automation.ScheduleInterval {
                task.Schedule.IntervalValue = 0
        }
        return task, true
}

func AutomationCreate(c *gin.Context) {
        mgr, ok := getAutomationManager(c)
        if !ok {
                return
        }
        task, ok := parseAutomationTask(c)
        if !ok {
                return
        }
        task.ID = ""
        task.CreatedAt = time.Now()
        task.UpdatedAt = time.Now()
        created, err := mgr.Create(task)
        if err != nil {
                common.ErrorResp(c, err, http.StatusBadRequest)
                return
        }
        common.SuccessResp(c, created)
}

func AutomationUpdate(c *gin.Context) {
        mgr, ok := getAutomationManager(c)
        if !ok {
                return
        }
        task, ok := parseAutomationTask(c)
        if !ok {
                return
        }
        if task.ID == "" {
                common.ErrorStrResp(c, "任务ID不能为空", http.StatusBadRequest)
                return
        }
        task.UpdatedAt = time.Now()
        updated, err := mgr.Update(task)
        if err != nil {
                common.ErrorResp(c, err, http.StatusBadRequest)
                return
        }
        common.SuccessResp(c, updated)
}

func AutomationDelete(c *gin.Context) {
        mgr, ok := getAutomationManager(c)
        if !ok {
                return
        }
        var req struct {
                ID string `json:"id"`
        }
        if err := c.ShouldBindJSON(&req); err != nil {
                common.ErrorResp(c, err, http.StatusBadRequest)
                return
        }
        mgr.Delete(req.ID)
        common.SuccessResp(c)
}

func AutomationToggle(c *gin.Context) {
        mgr, ok := getAutomationManager(c)
        if !ok {
                return
        }
        var req struct {
                ID      string `json:"id"`
                Enabled bool   `json:"enabled"`
        }
        if err := c.ShouldBindJSON(&req); err != nil {
                common.ErrorResp(c, err, http.StatusBadRequest)
                return
        }
        task, err := mgr.Toggle(req.ID, req.Enabled)
        if err != nil {
                common.ErrorResp(c, err, http.StatusBadRequest)
                return
        }
        common.SuccessResp(c, task)
}

func AutomationRun(c *gin.Context) {
        mgr, ok := getAutomationManager(c)
        if !ok {
                return
        }
        var req struct {
                ID string `json:"id"`
        }
        if err := c.ShouldBindJSON(&req); err != nil {
                common.ErrorResp(c, err, http.StatusBadRequest)
                return
        }
        task, err := mgr.RunNow(req.ID)
        if err != nil {
                common.ErrorResp(c, err, http.StatusBadRequest)
                return
        }
        common.SuccessResp(c, task)
}
