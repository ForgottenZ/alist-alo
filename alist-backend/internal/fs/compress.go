package fs

import (
	"context"
	"fmt"
	"io"
	"mime"
	"os"
	"os/exec"
	stdpath "path"
	"path/filepath"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/internal/conf"
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

	SrcDirPath string   `json:"src_dir"`
	DstDirPath string   `json:"dst_dir"`
	SrcNames   []string `json:"src_names"`
	Format     string   `json:"format"`
	Password   string   `json:"password"`
	DstName    string   `json:"dst_name"`

	SrcStorageMp string `json:"src_storage_mp"`
	DstStorageMp string `json:"dst_storage_mp"`
}

func (t *ArchiveCompressTask) GetName() string {
	return fmt.Sprintf("compress %v from [%s](%s) to [%s](%s) as %s", t.SrcNames, t.SrcStorageMp, t.SrcDirPath, t.DstStorageMp, t.DstDirPath, t.DstName)
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

	for _, name := range t.SrcNames {
		cleanName := strings.TrimPrefix(stdpath.Clean("/"+strings.TrimSpace(name)), "/")
		if cleanName == "" {
			continue
		}
		srcPath := stdpath.Join(t.SrcDirPath, cleanName)
		if err = copyMountPathToLocal(t.Ctx(), srcPath, filepath.Join(inputDir, filepath.Base(cleanName))); err != nil {
			return errors.WithMessagef(err, "failed to prepare source %s", srcPath)
		}
	}

	t.Status = "compressing"
	binary := "7zz"
	if _, err = exec.LookPath(binary); err != nil {
		binary = "7z"
	}
	if _, err = exec.LookPath(binary); err != nil {
		return errors.New("7zz or 7z is required on server")
	}
	archivePath := filepath.Join(workDir, t.DstName)
	args := []string{"a", "-bd", "-bso0", "-bsp0", "-mmt=1", "-mx=1", "-t" + t.Format, archivePath}
	if t.Password != "" {
		args = append(args, "-p"+t.Password)
		if t.Format == "7z" {
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
	if _, err = Get(t.Ctx(), t.DstDirPath, &GetArgs{NoLog: true}); err != nil {
		if err = MakeDir(t.Ctx(), t.DstDirPath, true); err != nil {
			return errors.WithMessage(err, "failed to prepare destination dir")
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
		Obj:      &model.Object{Name: t.DstName, Size: info.Size(), Modified: time.Now()},
		Reader:   archiveFile,
		Mimetype: mime.TypeByExtension(filepath.Ext(t.DstName)),
	}
	return PutDirectly(t.Ctx(), t.DstDirPath, fileStream, true)
}

func copyMountPathToLocal(ctx context.Context, srcPath, localPath string) error {
	obj, err := Get(ctx, srcPath, &GetArgs{NoLog: true})
	if err != nil {
		return err
	}
	if obj.IsDir() {
		if err = os.MkdirAll(localPath, 0o755); err != nil {
			return err
		}
		children, err := List(ctx, srcPath, &ListArgs{NoLog: true})
		if err != nil {
			return err
		}
		for _, child := range children {
			childSrc := stdpath.Join(srcPath, child.GetName())
			childLocal := filepath.Join(localPath, child.GetName())
			if err = copyMountPathToLocal(ctx, childSrc, childLocal); err != nil {
				return err
			}
		}
		return nil
	}
	if err = os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return err
	}
	link, file, err := Link(ctx, srcPath, model.LinkArgs{})
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
	srcStorage, _, err := op.GetStorageAndActualPath(srcDirPath)
	if err != nil {
		return nil, errors.WithMessage(err, "failed get src storage")
	}
	dstStorage, _, err := op.GetStorageAndActualPath(dstDirPath)
	if err != nil {
		return nil, errors.WithMessage(err, "failed get dst storage")
	}
	taskCreator, _ := ctx.Value("user").(*model.User)
	t := &ArchiveCompressTask{
		TaskExtension: task.TaskExtension{Creator: taskCreator},
		SrcDirPath:    srcDirPath,
		DstDirPath:    dstDirPath,
		SrcNames:      args.Names,
		Format:        args.Format,
		Password:      args.Password,
		DstName:       args.DstName,
		SrcStorageMp:  srcStorage.GetStorage().MountPath,
		DstStorageMp:  dstStorage.GetStorage().MountPath,
	}
	ArchiveCompressTaskManager.Add(t)
	return t, nil
}
