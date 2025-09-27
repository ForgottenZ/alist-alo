package handles

import (
	"github.com/alist-org/alist/v3/internal/automation"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/gin-gonic/gin"
)

func ListAutomationJobs(c *gin.Context) {
	jobs, err := automation.ListJobs()
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	common.SuccessResp(c, jobs)
}

func CreateAutomationJob(c *gin.Context) {
	var req automation.JobConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	user := c.MustGet("user").(*model.User)
	job, err := automation.CreateJob(req, user)
	if err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	common.SuccessResp(c, job)
}

func UpdateAutomationJob(c *gin.Context) {
	var req automation.JobConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	user := c.MustGet("user").(*model.User)
	job, err := automation.UpdateJob(req, user)
	if err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	common.SuccessResp(c, job)
}

func DeleteAutomationJob(c *gin.Context) {
	var req struct {
		ID uint `json:"id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ID == 0 {
		common.ErrorStrResp(c, "缺少任务ID", 400)
		return
	}
	if err := automation.DeleteJob(req.ID); err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	common.SuccessResp(c)
}

func ToggleAutomationJob(c *gin.Context) {
	var req struct {
		ID      uint `json:"id"`
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ID == 0 {
		common.ErrorStrResp(c, "缺少任务ID", 400)
		return
	}
	job, err := automation.ToggleJob(req.ID, req.Enabled)
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	common.SuccessResp(c, job)
}

func RunAutomationJob(c *gin.Context) {
	var req struct {
		ID uint `json:"id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ID == 0 {
		common.ErrorStrResp(c, "缺少任务ID", 400)
		return
	}
	if err := automation.RunJobNow(req.ID); err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	common.SuccessResp(c)
}

func AutomationJobHistory(c *gin.Context) {
	idStr := c.Query("job_id")
	if idStr == "" {
		common.ErrorStrResp(c, "缺少任务ID", 400)
		return
	}
	var req struct {
		JobID uint `form:"job_id" binding:"required"`
	}
	if err := c.ShouldBindQuery(&req); err != nil {
		common.ErrorStrResp(c, "缺少任务ID", 400)
		return
	}
	histories, err := automation.JobHistories(req.JobID)
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	common.SuccessResp(c, histories)
}
