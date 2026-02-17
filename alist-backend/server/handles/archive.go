package handles

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/url"
	"os"
	"os/exec"
	stdpath "path"
	"path/filepath"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/internal/stream"
	"github.com/alist-org/alist/v3/internal/task"

	"github.com/alist-org/alist/v3/internal/archive/tool"
	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/fs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/internal/setting"
	"github.com/alist-org/alist/v3/internal/sign"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type ArchiveMetaReq struct {
	Path        string `json:"path" form:"path"`
	Password    string `json:"password" form:"password"`
	Refresh     bool   `json:"refresh" form:"refresh"`
	ArchivePass string `json:"archive_pass" form:"archive_pass"`
}

type ArchiveMetaResp struct {
	Comment     string               `json:"comment"`
	IsEncrypted bool                 `json:"encrypted"`
	Content     []ArchiveContentResp `json:"content"`
	Sort        *model.Sort          `json:"sort,omitempty"`
	RawURL      string               `json:"raw_url"`
	Sign        string               `json:"sign"`
}

type ArchiveContentResp struct {
	ObjResp
	Children []ArchiveContentResp `json:"children"`
}

func toObjsRespWithoutSignAndThumb(obj model.Obj) ObjResp {
	return ObjResp{
		Name:        obj.GetName(),
		Size:        obj.GetSize(),
		IsDir:       obj.IsDir(),
		Modified:    obj.ModTime(),
		Created:     obj.CreateTime(),
		HashInfoStr: obj.GetHash().String(),
		HashInfo:    obj.GetHash().Export(),
		Sign:        "",
		Thumb:       "",
		Type:        utils.GetObjType(obj.GetName(), obj.IsDir()),
	}
}

func toContentResp(objs []model.ObjTree) []ArchiveContentResp {
	if objs == nil {
		return nil
	}
	ret, _ := utils.SliceConvert(objs, func(src model.ObjTree) (ArchiveContentResp, error) {
		return ArchiveContentResp{
			ObjResp:  toObjsRespWithoutSignAndThumb(src),
			Children: toContentResp(src.GetChildren()),
		}, nil
	})
	return ret
}

func FsArchiveMeta(c *gin.Context) {
	var req ArchiveMetaReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	user := c.MustGet("user").(*model.User)
	if !user.CanReadArchives() {
		common.ErrorResp(c, errs.PermissionDenied, 403)
		return
	}
	reqPath, err := user.JoinPath(req.Path)
	if err != nil {
		common.ErrorResp(c, err, 403)
		return
	}
	meta, err := op.GetNearestMeta(reqPath)
	if err != nil {
		if !errors.Is(errors.Cause(err), errs.MetaNotFound) {
			common.ErrorResp(c, err, 500, true)
			return
		}
	}
	c.Set("meta", meta)
	if !common.CanAccess(user, meta, reqPath, req.Password) {
		common.ErrorStrResp(c, "password is incorrect or you have no permission", 403)
		return
	}
	archiveArgs := model.ArchiveArgs{
		LinkArgs: model.LinkArgs{
			Header:  c.Request.Header,
			Type:    c.Query("type"),
			HttpReq: c.Request,
		},
		Password: req.ArchivePass,
	}
	ret, err := fs.ArchiveMeta(c, reqPath, model.ArchiveMetaArgs{
		ArchiveArgs: archiveArgs,
		Refresh:     req.Refresh,
	})
	if err != nil {
		if errors.Is(err, errs.WrongArchivePassword) {
			common.ErrorResp(c, err, 202)
		} else {
			common.ErrorResp(c, err, 500)
		}
		return
	}
	s := ""
	if isEncrypt(meta, reqPath) || setting.GetBool(conf.SignAll) {
		s = sign.SignArchive(reqPath)
	}
	api := "/ae"
	if ret.DriverProviding {
		api = "/ad"
	}
	common.SuccessResp(c, ArchiveMetaResp{
		Comment:     ret.GetComment(),
		IsEncrypted: ret.IsEncrypted(),
		Content:     toContentResp(ret.GetTree()),
		Sort:        ret.Sort,
		RawURL:      fmt.Sprintf("%s%s%s", common.GetApiUrl(c.Request), api, utils.EncodePath(reqPath, true)),
		Sign:        s,
	})
}

type ArchiveListReq struct {
	ArchiveMetaReq
	model.PageReq
	InnerPath string `json:"inner_path" form:"inner_path"`
}

type ArchiveListResp struct {
	Content []ObjResp `json:"content"`
	Total   int64     `json:"total"`
}

func FsArchiveList(c *gin.Context) {
	var req ArchiveListReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	req.Validate()
	user := c.MustGet("user").(*model.User)
	if !user.CanReadArchives() {
		common.ErrorResp(c, errs.PermissionDenied, 403)
		return
	}
	reqPath, err := user.JoinPath(req.Path)
	if err != nil {
		common.ErrorResp(c, err, 403)
		return
	}
	meta, err := op.GetNearestMeta(reqPath)
	if err != nil {
		if !errors.Is(errors.Cause(err), errs.MetaNotFound) {
			common.ErrorResp(c, err, 500, true)
			return
		}
	}
	c.Set("meta", meta)
	if !common.CanAccess(user, meta, reqPath, req.Password) {
		common.ErrorStrResp(c, "password is incorrect or you have no permission", 403)
		return
	}
	objs, err := fs.ArchiveList(c, reqPath, model.ArchiveListArgs{
		ArchiveInnerArgs: model.ArchiveInnerArgs{
			ArchiveArgs: model.ArchiveArgs{
				LinkArgs: model.LinkArgs{
					Header:  c.Request.Header,
					Type:    c.Query("type"),
					HttpReq: c.Request,
				},
				Password: req.ArchivePass,
			},
			InnerPath: utils.FixAndCleanPath(req.InnerPath),
		},
		Refresh: req.Refresh,
	})
	if err != nil {
		if errors.Is(err, errs.WrongArchivePassword) {
			common.ErrorResp(c, err, 202)
		} else {
			common.ErrorResp(c, err, 500)
		}
		return
	}
	total, objs := pagination(objs, &req.PageReq)
	ret, _ := utils.SliceConvert(objs, func(src model.Obj) (ObjResp, error) {
		return toObjsRespWithoutSignAndThumb(src), nil
	})
	common.SuccessResp(c, ArchiveListResp{
		Content: ret,
		Total:   int64(total),
	})
}

type StringOrArray []string

func (s *StringOrArray) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err == nil {
		*s = []string{value}
		return nil
	}
	var sliceValue []string
	if err := json.Unmarshal(data, &sliceValue); err != nil {
		return err
	}
	*s = sliceValue
	return nil
}

type ArchiveDecompressReq struct {
	SrcDir        string        `json:"src_dir" form:"src_dir"`
	DstDir        string        `json:"dst_dir" form:"dst_dir"`
	Name          StringOrArray `json:"name" form:"name"`
	ArchivePass   string        `json:"archive_pass" form:"archive_pass"`
	InnerPath     string        `json:"inner_path" form:"inner_path"`
	CacheFull     bool          `json:"cache_full" form:"cache_full"`
	PutIntoNewDir bool          `json:"put_into_new_dir" form:"put_into_new_dir"`
}

func FsArchiveDecompress(c *gin.Context) {
	var req ArchiveDecompressReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	user := c.MustGet("user").(*model.User)
	if !user.CanDecompress() {
		common.ErrorResp(c, errs.PermissionDenied, 403)
		return
	}
	srcPaths := make([]string, 0, len(req.Name))
	for _, name := range req.Name {
		srcPath, err := user.JoinPath(stdpath.Join(req.SrcDir, name))
		if err != nil {
			common.ErrorResp(c, err, 403)
			return
		}
		srcPaths = append(srcPaths, srcPath)
	}
	dstDir, err := user.JoinPath(req.DstDir)
	if err != nil {
		common.ErrorResp(c, err, 403)
		return
	}
	tasks := make([]task.TaskExtensionInfo, 0, len(srcPaths))
	for _, srcPath := range srcPaths {
		t, e := fs.ArchiveDecompress(c, srcPath, dstDir, model.ArchiveDecompressArgs{
			ArchiveInnerArgs: model.ArchiveInnerArgs{
				ArchiveArgs: model.ArchiveArgs{
					LinkArgs: model.LinkArgs{
						Header:  c.Request.Header,
						Type:    c.Query("type"),
						HttpReq: c.Request,
					},
					Password: req.ArchivePass,
				},
				InnerPath: utils.FixAndCleanPath(req.InnerPath),
			},
			CacheFull:     req.CacheFull,
			PutIntoNewDir: req.PutIntoNewDir,
		})
		if e != nil {
			if errors.Is(e, errs.WrongArchivePassword) {
				common.ErrorResp(c, e, 202)
			} else {
				common.ErrorResp(c, e, 500)
			}
			return
		}
		if t != nil {
			tasks = append(tasks, t)
		}
	}
	common.SuccessResp(c, gin.H{
		"task": getTaskInfos(tasks),
	})
}

type ArchiveCompressReq struct {
	SrcDir   string        `json:"src_dir" form:"src_dir"`
	DstDir   string        `json:"dst_dir" form:"dst_dir"`
	Name     StringOrArray `json:"name" form:"name"`
	Format   string        `json:"format" form:"format"`
	Password string        `json:"password" form:"password"`
	DstName  string        `json:"dst_name" form:"dst_name"`
}

func FsArchiveCompress(c *gin.Context) {
	var req ArchiveCompressReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	user := c.MustGet("user").(*model.User)
	if !user.CanCompress() {
		common.ErrorResp(c, errs.PermissionDenied, 403)
		return
	}
	if len(req.Name) == 0 {
		common.ErrorStrResp(c, "name can not be empty", 400)
		return
	}
	archiveFormat := strings.ToLower(req.Format)
	if archiveFormat != "zip" && archiveFormat != "7z" {
		common.ErrorStrResp(c, "format must be zip or 7z", 400)
		return
	}
	archiveName := strings.TrimSpace(req.DstName)
	if archiveName == "" {
		common.ErrorStrResp(c, "dst_name can not be empty", 400)
		return
	}
	if !strings.HasSuffix(strings.ToLower(archiveName), "."+archiveFormat) {
		archiveName += "." + archiveFormat
	}
	srcDir, err := user.JoinPath(req.SrcDir)
	if err != nil {
		common.ErrorResp(c, err, 403)
		return
	}
	dstDir, err := user.JoinPath(req.DstDir)
	if err != nil {
		common.ErrorResp(c, err, 403)
		return
	}
	workDir, err := os.MkdirTemp(conf.Conf.TempDir, "compress-*")
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	defer os.RemoveAll(workDir)
	inputDir := filepath.Join(workDir, "input")
	if err = os.MkdirAll(inputDir, 0o755); err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	for _, name := range req.Name {
		srcPath := stdpath.Join(srcDir, name)
		if err = copyMountPathToLocal(c, srcPath, filepath.Join(inputDir, filepath.Base(name))); err != nil {
			common.ErrorResp(c, err, 500)
			return
		}
	}
	binary := "7z"
	if _, err = exec.LookPath(binary); err != nil {
		binary = "7zz"
	}
	if _, err = exec.LookPath(binary); err != nil {
		common.ErrorStrResp(c, "7z or 7zz is required on server", 500)
		return
	}
	archivePath := filepath.Join(workDir, archiveName)
	args := []string{"a", "-bd", "-bso0", "-bsp0", "-mmt=1", "-mx=1", "-t" + archiveFormat, archivePath}
	if req.Password != "" {
		args = append(args, "-p"+req.Password)
		if archiveFormat == "7z" {
			args = append(args, "-mhe=on")
		}
	}
	entries, err := os.ReadDir(inputDir)
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	for _, e := range entries {
		args = append(args, e.Name())
	}
	cmd := exec.CommandContext(c, binary, args...)
	cmd.Dir = inputDir
	if output, err := cmd.CombinedOutput(); err != nil {
		common.ErrorStrResp(c, fmt.Sprintf("compress failed: %s", string(output)), 500)
		return
	}
	archiveFile, err := os.Open(archivePath)
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	info, err := archiveFile.Stat()
	if err != nil {
		_ = archiveFile.Close()
		common.ErrorResp(c, err, 500)
		return
	}
	fileStream := &stream.FileStream{
		Obj:      &model.Object{Name: archiveName, Size: info.Size(), Modified: time.Now()},
		Reader:   archiveFile,
		Mimetype: mime.TypeByExtension(filepath.Ext(archiveName)),
	}
	fileStream.Closers.Add(archiveFile)
	if err = fs.PutDirectly(c, dstDir, fileStream, true); err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	common.SuccessResp(c)
}

func copyMountPathToLocal(ctx *gin.Context, srcPath, localPath string) error {
	obj, err := fs.Get(ctx, srcPath, &fs.GetArgs{})
	if err != nil {
		return err
	}
	if obj.IsDir() {
		if err = os.MkdirAll(localPath, 0o755); err != nil {
			return err
		}
		children, err := fs.List(ctx, srcPath, &fs.ListArgs{})
		if err != nil {
			return err
		}
		for _, child := range children {
			if err = copyMountPathToLocal(ctx, stdpath.Join(srcPath, child.GetName()), filepath.Join(localPath, child.GetName())); err != nil {
				return err
			}
		}
		return nil
	}
	if err = os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return err
	}
	link, file, err := fs.Link(ctx, srcPath, model.LinkArgs{
		Header:  ctx.Request.Header,
		Type:    ctx.Query("type"),
		HttpReq: ctx.Request,
	})
	if err != nil {
		return err
	}
	ss, err := stream.NewSeekableStream(stream.FileStream{
		Ctx: ctx,
		Obj: file,
	}, link)
	if err != nil {
		return err
	}
	defer ss.Close()
	out, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, ss)
	return err
}

func ArchiveDown(c *gin.Context) {
	archiveRawPath := c.MustGet("path").(string)
	innerPath := utils.FixAndCleanPath(c.Query("inner"))
	password := c.Query("pass")
	filename := stdpath.Base(innerPath)
	storage, err := fs.GetStorage(archiveRawPath, &fs.GetStoragesArgs{})
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	if common.ShouldProxy(storage, filename) {
		ArchiveProxy(c)
		return
	} else {
		link, _, err := fs.ArchiveDriverExtract(c, archiveRawPath, model.ArchiveInnerArgs{
			ArchiveArgs: model.ArchiveArgs{
				LinkArgs: model.LinkArgs{
					IP:       c.ClientIP(),
					Header:   c.Request.Header,
					Type:     c.Query("type"),
					HttpReq:  c.Request,
					Redirect: true,
				},
				Password: password,
			},
			InnerPath: innerPath,
		})
		if err != nil {
			common.ErrorResp(c, err, 500)
			return
		}
		down(c, link)
	}
}

func ArchiveProxy(c *gin.Context) {
	archiveRawPath := c.MustGet("path").(string)
	innerPath := utils.FixAndCleanPath(c.Query("inner"))
	password := c.Query("pass")
	filename := stdpath.Base(innerPath)
	storage, err := fs.GetStorage(archiveRawPath, &fs.GetStoragesArgs{})
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	if canProxy(storage, filename) {
		// TODO: Support external download proxy URL
		link, file, err := fs.ArchiveDriverExtract(c, archiveRawPath, model.ArchiveInnerArgs{
			ArchiveArgs: model.ArchiveArgs{
				LinkArgs: model.LinkArgs{
					Header:  c.Request.Header,
					Type:    c.Query("type"),
					HttpReq: c.Request,
				},
				Password: password,
			},
			InnerPath: innerPath,
		})
		if err != nil {
			common.ErrorResp(c, err, 500)
			return
		}
		localProxy(c, link, file, storage.GetStorage().ProxyRange)
	} else {
		common.ErrorStrResp(c, "proxy not allowed", 403)
		return
	}
}

func ArchiveInternalExtract(c *gin.Context) {
	archiveRawPath := c.MustGet("path").(string)
	innerPath := utils.FixAndCleanPath(c.Query("inner"))
	password := c.Query("pass")
	rc, size, err := fs.ArchiveInternalExtract(c, archiveRawPath, model.ArchiveInnerArgs{
		ArchiveArgs: model.ArchiveArgs{
			LinkArgs: model.LinkArgs{
				Header:  c.Request.Header,
				Type:    c.Query("type"),
				HttpReq: c.Request,
			},
			Password: password,
		},
		InnerPath: innerPath,
	})
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	defer func() {
		if err := rc.Close(); err != nil {
			log.Errorf("failed to close file streamer, %v", err)
		}
	}()
	headers := map[string]string{
		"Referrer-Policy": "no-referrer",
		"Cache-Control":   "max-age=0, no-cache, no-store, must-revalidate",
	}
	filename := stdpath.Base(innerPath)
	headers["Content-Disposition"] = fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, filename, url.PathEscape(filename))
	contentType := c.Request.Header.Get("Content-Type")
	if contentType == "" {
		contentType = utils.GetMimeType(filename)
	}
	c.DataFromReader(200, size, contentType, rc, headers)
}

func ArchiveExtensions(c *gin.Context) {
	var ext []string
	for key := range tool.Tools {
		ext = append(ext, key)
	}
	common.SuccessResp(c, ext)
}
