package handles

import (
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/fs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

type FsSetPinnedReq struct {
	Path     string `json:"path" binding:"required"`
	Password string `json:"password"`
	Pinned   bool   `json:"pinned"`
}

func FsSetPinned(c *gin.Context) {
	var req FsSetPinnedReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	user := c.MustGet("user").(*model.User)
	if !user.CanPin() {
		common.ErrorResp(c, errs.PermissionDenied, 403)
		return
	}
	reqPath, err := user.JoinPath(req.Path)
	if err != nil {
		common.ErrorResp(c, err, 403)
		return
	}
	meta, err := op.GetNearestMeta(reqPath)
	if err != nil && !errors.Is(errors.Cause(err), errs.MetaNotFound) {
		common.ErrorResp(c, err, 500, true)
		return
	}
	c.Set("meta", meta)
	if !common.CanAccess(user, meta, reqPath, req.Password) {
		common.ErrorStrResp(c, "password is incorrect or you have no permission", 403)
		return
	}
	if _, err = fs.Get(c, reqPath, &fs.GetArgs{}); err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	if err = op.SetPinnedPath(reqPath, req.Pinned); err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	common.SuccessResp(c)
}
