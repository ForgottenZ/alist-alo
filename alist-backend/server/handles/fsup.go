package handles

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	stdpath "path"
	"path/filepath"
	"strconv"
	"time"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/fs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/stream"
	"github.com/alist-org/alist/v3/internal/task"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/gin-gonic/gin"
)

func getLastModified(c *gin.Context) time.Time {
	now := time.Now()
	lastModifiedStr := c.GetHeader("Last-Modified")
	lastModifiedMillisecond, err := strconv.ParseInt(lastModifiedStr, 10, 64)
	if err != nil {
		return now
	}
	lastModified := time.UnixMilli(lastModifiedMillisecond)
	return lastModified
}

func chunkTmpPath(uid uint, filePath string) string {
	h := sha1.Sum([]byte(fmt.Sprintf("%d:%s", uid, filePath)))
	fileName := hex.EncodeToString(h[:]) + ".part"
	return filepath.Join(conf.Conf.TempDir, "chunk_upload", fileName)
}

func appendChunk(path string, body io.Reader, chunkIndex int) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if chunkIndex == 0 {
		_ = os.Remove(path)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, body)
	return err
}

func parseChunkHeader(c *gin.Context) (bool, int, int, int64, error) {
	if c.GetHeader("X-Chunk-Upload") != "true" {
		return false, 0, 0, 0, nil
	}
	idx, err := strconv.Atoi(c.GetHeader("X-Chunk-Index"))
	if err != nil {
		return true, 0, 0, 0, err
	}
	total, err := strconv.Atoi(c.GetHeader("X-Chunk-Total"))
	if err != nil {
		return true, 0, 0, 0, err
	}
	fileSize, err := strconv.ParseInt(c.GetHeader("X-File-Size"), 10, 64)
	if err != nil {
		return true, 0, 0, 0, err
	}
	if idx < 0 || total <= 0 || idx >= total || fileSize < 0 {
		return true, 0, 0, 0, fmt.Errorf("invalid chunk metadata")
	}
	return true, idx, total, fileSize, nil
}

func FsStream(c *gin.Context) {
	path := c.GetHeader("File-Path")
	path, err := url.PathUnescape(path)
	if err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	asTask := c.GetHeader("As-Task") == "true"
	overwrite := c.GetHeader("Overwrite") != "false"
	user := c.MustGet("user").(*model.User)
	path, err = user.JoinPath(path)
	if err != nil {
		common.ErrorResp(c, err, 403)
		return
	}
	chunkMode, chunkIndex, chunkTotal, fileSize, err := parseChunkHeader(c)
	if err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	if chunkMode && asTask {
		common.ErrorStrResp(c, "chunk upload does not support task mode", 400)
		return
	}
	if !overwrite && (!chunkMode || chunkIndex == 0) {
		if res, _ := fs.Get(c, path, &fs.GetArgs{NoLog: true}); res != nil {
			_, _ = utils.CopyWithBuffer(io.Discard, c.Request.Body)
			common.ErrorStrResp(c, "file exists", 403)
			return
		}
	}
	dir, name := stdpath.Split(path)
	h := make(map[*utils.HashType]string)
	if md5 := c.GetHeader("X-File-Md5"); md5 != "" {
		h[utils.MD5] = md5
	}
	if sha1v := c.GetHeader("X-File-Sha1"); sha1v != "" {
		h[utils.SHA1] = sha1v
	}
	if sha256 := c.GetHeader("X-File-Sha256"); sha256 != "" {
		h[utils.SHA256] = sha256
	}
	mimetype := c.GetHeader("Content-Type")
	if len(mimetype) == 0 {
		mimetype = utils.GetMimeType(name)
	}

	if chunkMode {
		tmpPath := chunkTmpPath(user.ID, path)
		defer c.Request.Body.Close()
		if err := appendChunk(tmpPath, c.Request.Body, chunkIndex); err != nil {
			common.ErrorResp(c, err, 500)
			return
		}
		if chunkIndex < chunkTotal-1 {
			common.SuccessResp(c)
			return
		}
		f, err := os.Open(tmpPath)
		if err != nil {
			common.ErrorResp(c, err, 500)
			return
		}
		defer func() {
			_ = f.Close()
			_ = os.Remove(tmpPath)
		}()
		s := &stream.FileStream{
			Obj: &model.Object{
				Name:     name,
				Size:     fileSize,
				Modified: getLastModified(c),
				HashInfo: utils.NewHashInfoByMap(h),
			},
			Reader:       f,
			Mimetype:     mimetype,
			WebPutAsTask: false,
		}
		if err := fs.PutDirectly(c, dir, s, true); err != nil {
			common.ErrorResp(c, err, 500)
			return
		}
		common.SuccessResp(c)
		return
	}

	sizeStr := c.GetHeader("Content-Length")
	size, err := strconv.ParseInt(sizeStr, 10, 64)
	if err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	s := &stream.FileStream{
		Obj: &model.Object{
			Name:     name,
			Size:     size,
			Modified: getLastModified(c),
			HashInfo: utils.NewHashInfoByMap(h),
		},
		Reader:       c.Request.Body,
		Mimetype:     mimetype,
		WebPutAsTask: asTask,
	}
	var t task.TaskExtensionInfo
	if asTask {
		t, err = fs.PutAsTask(c, dir, s)
	} else {
		err = fs.PutDirectly(c, dir, s, true)
	}
	defer c.Request.Body.Close()
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	if t == nil {
		if n, _ := io.ReadFull(c.Request.Body, []byte{0}); n == 1 {
			_, _ = utils.CopyWithBuffer(io.Discard, c.Request.Body)
		}
		common.SuccessResp(c)
		return
	}
	common.SuccessResp(c, gin.H{
		"task": getTaskInfo(t),
	})
}

func FsForm(c *gin.Context) {
	path := c.GetHeader("File-Path")
	path, err := url.PathUnescape(path)
	if err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	asTask := c.GetHeader("As-Task") == "true"
	overwrite := c.GetHeader("Overwrite") != "false"
	user := c.MustGet("user").(*model.User)
	path, err = user.JoinPath(path)
	if err != nil {
		common.ErrorResp(c, err, 403)
		return
	}
	if !overwrite {
		if res, _ := fs.Get(c, path, &fs.GetArgs{NoLog: true}); res != nil {
			_, _ = utils.CopyWithBuffer(io.Discard, c.Request.Body)
			common.ErrorStrResp(c, "file exists", 403)
			return
		}
	}
	storage, err := fs.GetStorage(path, &fs.GetStoragesArgs{})
	if err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	if storage.Config().NoUpload {
		common.ErrorStrResp(c, "Current storage doesn't support upload", 405)
		return
	}
	file, err := c.FormFile("file")
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	f, err := file.Open()
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	defer f.Close()
	dir, name := stdpath.Split(path)
	h := make(map[*utils.HashType]string)
	if md5 := c.GetHeader("X-File-Md5"); md5 != "" {
		h[utils.MD5] = md5
	}
	if sha1 := c.GetHeader("X-File-Sha1"); sha1 != "" {
		h[utils.SHA1] = sha1
	}
	if sha256 := c.GetHeader("X-File-Sha256"); sha256 != "" {
		h[utils.SHA256] = sha256
	}
	mimetype := file.Header.Get("Content-Type")
	if len(mimetype) == 0 {
		mimetype = utils.GetMimeType(name)
	}
	s := stream.FileStream{
		Obj: &model.Object{
			Name:     name,
			Size:     file.Size,
			Modified: getLastModified(c),
			HashInfo: utils.NewHashInfoByMap(h),
		},
		Reader:       f,
		Mimetype:     mimetype,
		WebPutAsTask: asTask,
	}
	var t task.TaskExtensionInfo
	if asTask {
		s.Reader = struct {
			io.Reader
		}{f}
		t, err = fs.PutAsTask(c, dir, &s)
	} else {
		err = fs.PutDirectly(c, dir, &s, true)
	}
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	if t == nil {
		common.SuccessResp(c)
		return
	}
	common.SuccessResp(c, gin.H{
		"task": getTaskInfo(t),
	})
}
