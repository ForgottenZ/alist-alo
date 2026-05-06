package handles

import (
	"strconv"

	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/notification"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/gin-gonic/gin"
)

type NotificationTestReq struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type NotificationPublicInfo struct {
	ID      uint   `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Enabled bool   `json:"enabled"`
	Remark  string `json:"remark"`
}

func ListEnabledNotifications(c *gin.Context) {
	items, _, err := op.GetNotifications(1, model.MaxInt)
	if err != nil {
		common.ErrorResp(c, err, 500, true)
		return
	}
	res := make([]NotificationPublicInfo, 0, len(items))
	for _, item := range items {
		if !item.Enabled {
			continue
		}
		res = append(res, NotificationPublicInfo{
			ID:      item.ID,
			Name:    item.Name,
			Type:    item.Type,
			Enabled: item.Enabled,
			Remark:  item.Remark,
		})
	}
	common.SuccessResp(c, res)
}

func ListNotifications(c *gin.Context) {
	var req model.PageReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	req.Validate()
	items, total, err := op.GetNotifications(req.Page, req.PerPage)
	if err != nil {
		common.ErrorResp(c, err, 500, true)
		return
	}
	common.SuccessResp(c, common.PageResp{
		Content: items,
		Total:   total,
	})
}

func GetNotification(c *gin.Context) {
	id, err := strconv.Atoi(c.Query("id"))
	if err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	item, err := op.GetNotificationById(uint(id))
	if err != nil {
		common.ErrorResp(c, err, 500, true)
		return
	}
	common.SuccessResp(c, item)
}

func CreateNotification(c *gin.Context) {
	var req model.Notification
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	if err := op.CreateNotification(&req); err != nil {
		common.ErrorResp(c, err, 500, true)
		return
	}
	common.SuccessResp(c)
}

func UpdateNotification(c *gin.Context) {
	var req model.Notification
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	if err := op.UpdateNotification(&req); err != nil {
		common.ErrorResp(c, err, 500, true)
		return
	}
	common.SuccessResp(c)
}

func DeleteNotification(c *gin.Context) {
	id, err := strconv.Atoi(c.Query("id"))
	if err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	if err := op.DeleteNotificationById(uint(id)); err != nil {
		common.ErrorResp(c, err, 500, true)
		return
	}
	common.SuccessResp(c)
}

func TestNotification(c *gin.Context) {
	id, err := strconv.Atoi(c.Query("id"))
	if err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	var req NotificationTestReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	if req.Title == "" {
		req.Title = "AList notification test"
	}
	if req.Body == "" {
		req.Body = "This is a test notification from AList."
	}
	if err := notification.SendByID(c, uint(id), req.Title, req.Body); err != nil {
		common.ErrorResp(c, err, 500, true)
		return
	}
	common.SuccessResp(c)
}
