package fs

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"
	stdpath "path"
	"path/filepath"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/internal/stream"
	"github.com/alist-org/alist/v3/internal/task"
	"github.com/pkg/errors"
	"github.com/xhofe/tache"
)

type ArchiveCompressTask struct {
	task.TaskExtension
	Status string `json:"-"`

	srcStorage driver.Driver `json:"-"`
	dstStorage driver.Driver `json:"-"`

	srcDirActualPath string   `json:"src_dir"`
	dstDirActualPath string   `json:"dst_dir"`
	srcNames         []string `json:"src_names"`
	format           string   `json:"format"`
	password         string   `json:"password"`
	dstName          string   `json:"dst_name"`

	SrcStorageMp string `json:"src_storage_mp"`
	DstStorageMp string `json:"dst_storage_mp"`
}

func (t *ArchiveCompressTask) GetName() string {
	return fmt.Sprintf("compress %v from [%s](%s) to [%s](%s) as %s", t.srcNames, t.SrcStorageMp, t.srcDirActualPath, t.DstStorageMp, t.dstDirActualPath, t.dstName)
}

func (t *ArchiveCompressTask) GetStatus() string {
	return t.Status
}

func (t *ArchiveCompressTask) Run() error {
	t.ReinitCtx()
	t.ClearEndTime()
	t.SetStartTime(time.Now())
	defer func() { t.SetEndTime(time.Now()) }()

	t.Status = "preparing files"
	workDir, err := os.MkdirTemp(conf.Conf.TempDir, "compress-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workDir)

	inputDir := filepath.Join(workDir, "input")
	if err = os.MkdirAll(inputDir, 0o755); err != nil {
		return err
	}

	for _, name := range t.srcNames {
		srcActualPath := stdpath.Join(t.srcDirActualPath, name)
		if err = copyStoragePathToLocal(t.Ctx(), t.srcStorage, srcActualPath, filepath.Join(inputDir, filepath.Base(name))); err != nil {
			return errors.WithMessagef(err, "failed to prepare source %s", srcActualPath)
		}
	}

	t.Status = "compressing"
	binary := "7z"
	if _, err = exec.LookPath(binary); err != nil {
		binary = "7zz"
	}
	if _, err = exec.LookPath(binary); err != nil {
		return errors.New("7z or 7zz is required on server")
	}

	archivePath := filepath.Join(workDir, t.dstName)
	args := []string{"a", "-bd", "-bso0", "-bsp0", "-mmt=1", "-mx=1", "-t" + t.format, archivePath}
	if t.password != "" {
		args = append(args, "-p"+t.password)
		if t.format == "7z" {
			args = append(args, "-mhe=on")
		}
	}
	entries, err := os.ReadDir(inputDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		args = append(args, e.Name())
	}
	cmd := exec.CommandContext(t.Ctx(), binary, args...)
	cmd.Dir = inputDir
	if output, err := cmd.CombinedOutput(); err != nil {
		return errors.Errorf("compress failed: %s", strings.TrimSpace(string(output)))
	}

	t.Status = "uploading"
	if _, err = op.Get(t.Ctx(), t.dstStorage, t.dstDirActualPath); err != nil {
		if mkErr := op.MakeDir(t.Ctx(), t.dstStorage, t.dstDirActualPath, true); mkErr != nil {
			return errors.WithMessage(mkErr, "failed to prepare destination dir")
		}
	}

	archiveFile, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer archiveFile.Close()
	info, err := archiveFile.Stat()
	if err != nil {
		return err
	}
	t.SetTotalBytes(info.Size())
	fileStream := &stream.FileStream{
		Obj:      &model.Object{Name: t.dstName, Size: info.Size(), Modified: time.Now()},
		Reader:   archiveFile,
		Mimetype: mime.TypeByExtension(filepath.Ext(t.dstName)),
	}
	return op.Put(t.Ctx(), t.dstStorage, t.dstDirActualPath, fileStream, t.SetProgress, true)
}

func copyStoragePathToLocal(ctx context.Context, storage driver.Driver, srcPath, localPath string) error {
	obj, err := op.Get(ctx, storage, srcPath)
	if err != nil {
		return err
	}
	if obj.IsDir() {
		if err = os.MkdirAll(localPath, 0o755); err != nil {
			return err
		}
		children, err := op.List(ctx, storage, srcPath, model.ListArgs{})
		if err != nil {
			return err
		}
		for _, child := range children {
			if err = copyStoragePathToLocal(ctx, storage, stdpath.Join(srcPath, child.GetName()), filepath.Join(localPath, child.GetName())); err != nil {
				return err
			}
		}
		return nil
	}
	if err = os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return err
	}
	link, file, err := op.Link(ctx, storage, srcPath, model.LinkArgs{Header: http.Header{}})
	if err != nil {
		return err
	}
	ss, err := stream.NewSeekableStream(stream.FileStream{Ctx: ctx, Obj: file}, link)
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

var ArchiveCompressTaskManager *tache.Manager[*ArchiveCompressTask]

type ArchiveCompressArgs struct {
	Names    []string
	Format   string
	Password string
	DstName  string
}

func archiveCompress(ctx context.Context, srcDirPath, dstDirPath string, args ArchiveCompressArgs) (task.TaskExtensionInfo, error) {
	srcStorage, srcDirActualPath, err := op.GetStorageAndActualPath(srcDirPath)
	if err != nil {
		return nil, errors.WithMessage(err, "failed get src storage")
	}
	dstStorage, dstDirActualPath, err := op.GetStorageAndActualPath(dstDirPath)
	if err != nil {
		return nil, errors.WithMessage(err, "failed get dst storage")
	}
	taskCreator, _ := ctx.Value("user").(*model.User)
	t := &ArchiveCompressTask{
		TaskExtension:    task.TaskExtension{Creator: taskCreator},
		srcStorage:       srcStorage,
		dstStorage:       dstStorage,
		srcDirActualPath: srcDirActualPath,
		dstDirActualPath: dstDirActualPath,
		srcNames:         args.Names,
		format:           args.Format,
		password:         args.Password,
		dstName:          args.DstName,
		SrcStorageMp:     srcStorage.GetStorage().MountPath,
		DstStorageMp:     dstStorage.GetStorage().MountPath,
	}
	ArchiveCompressTaskManager.Add(t)
	return t, nil
}
