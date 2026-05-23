package handles

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	stdpath "path"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/fs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
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

var chunkUploadLocks sync.Map

func getChunkUploadLock(uploadID string) *sync.Mutex {
	lock, _ := chunkUploadLocks.LoadOrStore(uploadID, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

func cleanChunkUploadLock(uploadID string) {
	chunkUploadLocks.Delete(uploadID)
}

func getChunkUploadTempPath(tempDir, uploadID string) string {
	sum := sha256.Sum256([]byte(uploadID))
	shortName := hex.EncodeToString(sum[:16])
	return filepath.Join(tempDir, shortName+".part")
}

type chunkUploadProgress struct {
	UploadID    string    `json:"upload_id"`
	LastChunk   int       `json:"last_chunk"`
	TotalChunks int       `json:"total_chunks"`
	TotalSize   int64     `json:"total_size"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func getChunkUploadProgressPath(tempPath string) string {
	return tempPath + ".progress.json"
}

func readChunkUploadProgress(progressPath string) (chunkUploadProgress, bool, error) {
	progress := chunkUploadProgress{LastChunk: -1}
	data, err := os.ReadFile(progressPath)
	if err != nil {
		if os.IsNotExist(err) {
			return progress, false, nil
		}
		return progress, false, err
	}
	if err = json.Unmarshal(data, &progress); err != nil {
		return chunkUploadProgress{LastChunk: -1}, false, err
	}
	return progress, true, nil
}

func writeChunkUploadProgress(progressPath string, progress chunkUploadProgress) error {
	progress.UpdatedAt = time.Now()
	data, err := json.Marshal(progress)
	if err != nil {
		return err
	}
	return os.WriteFile(progressPath, data, 0o644)
}

func chunkProgressResp(progress chunkUploadProgress) gin.H {
	nextChunk := progress.LastChunk + 1
	if progress.TotalChunks > 0 && nextChunk > progress.TotalChunks {
		nextChunk = progress.TotalChunks
	}
	return gin.H{
		"last_chunk":   progress.LastChunk,
		"next_chunk":   nextChunk,
		"total_chunks": progress.TotalChunks,
		"total_size":   progress.TotalSize,
	}
}

func getChunkUploadPaths(c *gin.Context, path, uploadID string, tempInTarget bool) (string, string, error) {
	tempDir := filepath.Join(conf.Conf.TempDir, "chunk_upload")
	if tempInTarget {
		dir, _ := stdpath.Split(path)
		storage, actualDirPath, err := op.GetStorageAndActualPath(dir)
		if err != nil {
			return "", "", err
		}
		if !storage.Config().OnlyLocal {
			return "", "", errors.New("chunk temp in target directory only supports Local storage")
		}
		if err := fs.MakeDir(c, dir, true); err != nil {
			return "", "", err
		}
		dirObj, err := op.GetUnwrap(c, storage, actualDirPath)
		if err != nil {
			return "", "", err
		}
		if !dirObj.IsDir() {
			return "", "", errors.New("target path is not a directory")
		}
		tempDir = dirObj.GetPath()
	}
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return "", "", err
	}
	tempPath := getChunkUploadTempPath(tempDir, uploadID)
	return tempPath, getChunkUploadProgressPath(tempPath), nil
}

func parseChunkPartSize(c *gin.Context) int64 {
	partSize, _ := strconv.ParseInt(c.GetHeader("Chunk-Size"), 10, 64)
	if partSize < 0 {
		return 0
	}
	return partSize
}

func FsChunkStatus(c *gin.Context) {
	path := c.GetHeader("File-Path")
	path, err := url.PathUnescape(path)
	if err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	uploadID := c.GetHeader("Upload-Id")
	if uploadID == "" {
		common.ErrorStrResp(c, "missing upload id", 400)
		return
	}
	user := c.MustGet("user").(*model.User)
	path, err = user.JoinPath(path)
	if err != nil {
		common.ErrorResp(c, err, 403)
		return
	}
	_, progressPath, err := getChunkUploadPaths(c, path, uploadID, c.GetHeader("Chunk-Temp-In-Target") == "true")
	if err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	lock := getChunkUploadLock(uploadID)
	lock.Lock()
	defer lock.Unlock()
	progress, ok, err := readChunkUploadProgress(progressPath)
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	if !ok {
		progress = chunkUploadProgress{UploadID: uploadID, LastChunk: -1}
	}
	common.SuccessResp(c, chunkProgressResp(progress))
}

func handleChunkUpload(c *gin.Context, path string, asTask bool, overwrite bool) {
	chunkIndex, err := strconv.Atoi(c.GetHeader("Chunk-Index"))
	if err != nil || chunkIndex < 0 {
		common.ErrorStrResp(c, "invalid chunk index", 400)
		return
	}
	totalChunks, err := strconv.Atoi(c.GetHeader("Total-Chunks"))
	if err != nil || totalChunks <= 0 {
		common.ErrorStrResp(c, "invalid total chunks", 400)
		return
	}
	totalSize, err := strconv.ParseInt(c.GetHeader("Total-Size"), 10, 64)
	if err != nil || totalSize < 0 {
		common.ErrorStrResp(c, "invalid total size", 400)
		return
	}
	uploadID := c.GetHeader("Upload-Id")
	if uploadID == "" {
		common.ErrorStrResp(c, "missing upload id", 400)
		return
	}

	user := c.MustGet("user").(*model.User)
	path, err = user.JoinPath(path)
	if err != nil {
		common.ErrorResp(c, err, 403)
		return
	}
	if chunkIndex == 0 && !overwrite {
		if res, _ := fs.Get(c, path, &fs.GetArgs{NoLog: true}); res != nil {
			_, _ = utils.CopyWithBuffer(io.Discard, c.Request.Body)
			common.ErrorStrResp(c, "file exists", 403)
			return
		}
	}

	tempPath, progressPath, err := getChunkUploadPaths(c, path, uploadID, c.GetHeader("Chunk-Temp-In-Target") == "true")
	if err != nil {
		common.ErrorResp(c, err, 400)
		return
	}

	lock := getChunkUploadLock(uploadID)
	lock.Lock()
	defer lock.Unlock()

	progress, hasProgress, err := readChunkUploadProgress(progressPath)
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	resume := c.GetHeader("Resume-Upload") == "true"
	if !resume || !hasProgress || progress.TotalChunks != totalChunks || progress.TotalSize != totalSize {
		progress = chunkUploadProgress{
			UploadID:    uploadID,
			LastChunk:   -1,
			TotalChunks: totalChunks,
			TotalSize:   totalSize,
		}
	}
	if chunkIndex == 0 && progress.LastChunk < 0 {
		_ = os.Remove(tempPath)
		_ = os.Remove(progressPath)
	}
	alreadyUploaded := resume && progress.LastChunk >= chunkIndex
	if alreadyUploaded {
		_, _ = utils.CopyWithBuffer(io.Discard, c.Request.Body)
		if chunkIndex < totalChunks-1 {
			common.SuccessResp(c, chunkProgressResp(progress))
			return
		}
	} else if resume && chunkIndex != progress.LastChunk+1 {
		_, _ = utils.CopyWithBuffer(io.Discard, c.Request.Body)
		common.ErrorWithDataResp(c, errors.New("chunk out of order"), 409, chunkProgressResp(progress))
		return
	} else {
		f, err := os.OpenFile(tempPath, os.O_CREATE|os.O_RDWR, 0o644)
		if err != nil {
			common.ErrorResp(c, err, 500)
			return
		}
		partSize := parseChunkPartSize(c)
		if partSize > 0 {
			if _, err = f.Seek(int64(chunkIndex)*partSize, io.SeekStart); err != nil {
				_ = f.Close()
				common.ErrorResp(c, err, 500)
				return
			}
		} else if _, err = f.Seek(0, io.SeekEnd); err != nil {
			_ = f.Close()
			common.ErrorResp(c, err, 500)
			return
		}
		written, err := io.Copy(f, c.Request.Body)
		if err != nil {
			_ = f.Close()
			common.ErrorResp(c, err, 500)
			return
		}
		if partSize > 0 && chunkIndex < totalChunks-1 && written != partSize {
			_ = f.Close()
			common.ErrorStrResp(c, "chunk size mismatch", 400)
			return
		}
		_ = f.Close()

		progress.LastChunk = chunkIndex
		progress.TotalChunks = totalChunks
		progress.TotalSize = totalSize
		if err = writeChunkUploadProgress(progressPath, progress); err != nil {
			common.ErrorResp(c, err, 500)
			return
		}
		if chunkIndex < totalChunks-1 {
			common.SuccessResp(c, chunkProgressResp(progress))
			return
		}
	}

	f, err := os.Open(tempPath)
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	if stat, err := f.Stat(); err == nil && stat.Size() != totalSize {
		_ = f.Close()
		common.ErrorStrResp(c, "chunk upload size mismatch", 400)
		return
	}
	finalized := false
	taskQueued := false
	defer func() {
		if !taskQueued {
			_ = f.Close()
		}
		if finalized && !taskQueued {
			_ = os.Remove(tempPath)
		}
		if finalized {
			_ = os.Remove(progressPath)
		}
		cleanChunkUploadLock(uploadID)
	}()

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
	s := &stream.FileStream{
		Obj: &model.Object{
			Name:     name,
			Size:     totalSize,
			Modified: getLastModified(c),
			HashInfo: utils.NewHashInfoByMap(h),
		},
		Reader:       f,
		Mimetype:     c.GetHeader("Content-Type"),
		WebPutAsTask: asTask,
	}
	if asTask {
		s.SetTmpFile(f)
	}
	if s.Mimetype == "" {
		s.Mimetype = utils.GetMimeType(name)
	}
	var t task.TaskExtensionInfo
	if asTask {
		t, err = fs.PutAsTask(c, dir, s)
	} else {
		err = fs.PutDirectly(c, dir, s, true)
	}
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	finalized = true
	taskQueued = asTask && t != nil
	if t == nil {
		common.SuccessResp(c)
		return
	}
	common.SuccessResp(c, gin.H{"task": getTaskInfo(t)})
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
	if c.GetHeader("Chunk-Index") != "" {
		handleChunkUpload(c, path, asTask, overwrite)
		return
	}
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
	dir, name := stdpath.Split(path)
	sizeStr := c.GetHeader("Content-Length")
	size, err := strconv.ParseInt(sizeStr, 10, 64)
	if err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
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
	mimetype := c.GetHeader("Content-Type")
	if len(mimetype) == 0 {
		mimetype = utils.GetMimeType(name)
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
