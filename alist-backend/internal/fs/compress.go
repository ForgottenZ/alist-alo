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
	"sort"
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
	CopyMode   string   `json:"copy_mode"`
	VolumeSize string   `json:"volume_size"`

	SrcStorageMp string `json:"src_storage_mp"`
	DstStorageMp string `json:"dst_storage_mp"`
}

const (
	ArchiveCompressCopyModeTemp    = "temp"
	ArchiveCompressCopyModeSrcTemp = "src_temp"
	ArchiveCompressCopyModeNone    = "none"
)

func NormalizeArchiveCompressCopyMode(mode string) string {
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode == "" {
		return ArchiveCompressCopyModeTemp
	}
	return mode
}

func IsArchiveCompressCopyModeValid(mode string) bool {
	switch mode {
	case ArchiveCompressCopyModeTemp, ArchiveCompressCopyModeSrcTemp, ArchiveCompressCopyModeNone:
		return true
	default:
		return false
	}
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

	t.CopyMode = NormalizeArchiveCompressCopyMode(t.CopyMode)
	if !IsArchiveCompressCopyModeValid(t.CopyMode) {
		return errors.Errorf("invalid copy mode: %s", t.CopyMode)
	}

	t.Status = "preparing files"
	cmdDir, sources, archivePath, cleanup, err := t.prepareCompressWorkspace()
	if err != nil {
		return err
	}
	defer cleanup()

	t.Status = "compressing"
	binary := "7zz"
	if _, err = exec.LookPath(binary); err != nil {
		binary = "7z"
	}
	if _, err = exec.LookPath(binary); err != nil {
		return errors.New("7zz or 7z is required on server")
	}
	args := []string{"a", "-bd", "-bso0", "-bsp0", "-mmt=1", "-mx=1", "-t" + t.Format, archivePath}
	if t.Password != "" {
		args = append(args, "-p"+t.Password)
		if t.Format == "7z" {
			args = append(args, "-mhe=on")
		}
	}
	if t.VolumeSize != "" {
		args = append(args, "-v"+t.VolumeSize)
	}
	args = append(args, sources...)
	cmd := exec.CommandContext(t.Ctx(), binary, args...)
	cmd.Dir = cmdDir
	if output, err := cmd.CombinedOutput(); err != nil {
		return errors.Errorf("compress failed: %s", strings.TrimSpace(string(output)))
	}

	t.Status = "uploading"
	if _, err = Get(t.Ctx(), t.DstDirPath, &GetArgs{NoLog: true}); err != nil {
		if err = MakeDir(t.Ctx(), t.DstDirPath, true); err != nil {
			return errors.WithMessage(err, "failed to prepare destination dir")
		}
	}
	archiveFiles, err := collectGeneratedArchiveFiles(archivePath)
	if err != nil {
		return err
	}
	var totalSize int64
	for _, archiveFilePath := range archiveFiles {
		info, statErr := os.Stat(archiveFilePath)
		if statErr != nil {
			return statErr
		}
		totalSize += info.Size()
	}
	t.SetTotalBytes(totalSize)
	for _, archiveFilePath := range archiveFiles {
		info, statErr := os.Stat(archiveFilePath)
		if statErr != nil {
			return statErr
		}
		archiveFile, openErr := os.Open(archiveFilePath)
		if openErr != nil {
			return openErr
		}
		fileName := t.getUploadArchiveName(filepath.Base(archiveFilePath), archivePath)
		fileStream := &stream.FileStream{
			Obj:      &model.Object{Name: fileName, Size: info.Size(), Modified: time.Now()},
			Reader:   archiveFile,
			Mimetype: mime.TypeByExtension(filepath.Ext(fileName)),
		}
		putErr := PutDirectly(t.Ctx(), t.DstDirPath, fileStream, true)
		closeErr := archiveFile.Close()
		if putErr != nil {
			return putErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func (t *ArchiveCompressTask) prepareCompressWorkspace() (cmdDir string, sources []string, archivePath string, cleanup func(), err error) {
	cleanup = func() {}
	switch t.CopyMode {
	case ArchiveCompressCopyModeTemp, ArchiveCompressCopyModeSrcTemp:
		workRoot := conf.Conf.TempDir
		excludeLocalPathPrefixes := make([]string, 0, 1)
		if t.CopyMode == ArchiveCompressCopyModeSrcTemp {
			srcLocalDir, e := getMountDirLocalPath(t.Ctx(), t.SrcDirPath)
			if e != nil {
				err = errors.WithMessage(e, "copy_mode=src_temp requires source directory on local filesystem")
				return
			}
			workRoot = filepath.Join(srcLocalDir, "temp")
			if e = os.MkdirAll(workRoot, 0o755); e != nil {
				err = e
				return
			}
		}
		workDir, e := os.MkdirTemp(workRoot, "compress-*")
		if e != nil {
			err = e
			return
		}
		cleanup = func() {
			_ = os.RemoveAll(workDir)
		}
		if t.CopyMode == ArchiveCompressCopyModeSrcTemp {
			excludeLocalPathPrefixes = append(excludeLocalPathPrefixes, workDir)
		}
		inputDir := filepath.Join(workDir, "input")
		if e = os.MkdirAll(inputDir, 0o755); e != nil {
			err = e
			return
		}
		for _, name := range t.SrcNames {
			cleanName := cleanCompressSourceName(name)
			if cleanName == "" {
				continue
			}
			srcPath := stdpath.Join(t.SrcDirPath, cleanName)
			if e = copyMountPathToLocal(t.Ctx(), srcPath, filepath.Join(inputDir, filepath.Base(cleanName)), excludeLocalPathPrefixes...); e != nil {
				err = errors.WithMessagef(e, "failed to prepare source %s", srcPath)
				return
			}
		}
		entries, e := os.ReadDir(inputDir)
		if e != nil {
			err = e
			return
		}
		sources = make([]string, 0, len(entries))
		for _, entry := range entries {
			sources = append(sources, entry.Name())
		}
		if len(sources) == 0 {
			err = errors.New("name can not be empty")
			return
		}
		cmdDir = inputDir
		archivePath = filepath.Join(workDir, t.DstName)
		return
	case ArchiveCompressCopyModeNone:
		srcLocalDir, e := getMountDirLocalPath(t.Ctx(), t.SrcDirPath)
		if e != nil {
			err = errors.WithMessage(e, "copy_mode=none requires source directory on local filesystem")
			return
		}
		sources = make([]string, 0, len(t.SrcNames))
		for _, name := range t.SrcNames {
			cleanName := cleanCompressSourceName(name)
			if cleanName == "" {
				continue
			}
			sources = append(sources, filepath.FromSlash(cleanName))
		}
		if len(sources) == 0 {
			err = errors.New("name can not be empty")
			return
		}
		cmdDir = srcLocalDir
		archivePath = filepath.Join(srcLocalDir, fmt.Sprintf(".alist-compress-%d-%s", time.Now().UnixNano(), t.DstName))
		cleanup = func() {
			archiveFiles, e := collectGeneratedArchiveFiles(archivePath)
			if e != nil {
				return
			}
			for _, archiveFile := range archiveFiles {
				_ = os.Remove(archiveFile)
			}
		}
		return
	default:
		err = errors.Errorf("invalid copy mode: %s", t.CopyMode)
		return
	}
}

func collectGeneratedArchiveFiles(archivePath string) ([]string, error) {
	dir := filepath.Dir(archivePath)
	targetName := filepath.Base(archivePath)
	targetStem := strings.TrimSuffix(targetName, filepath.Ext(targetName))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	archiveFiles := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == targetName || strings.HasPrefix(name, targetName+".") || strings.HasPrefix(name, targetStem+".") {
			archiveFiles = append(archiveFiles, filepath.Join(dir, name))
		}
	}
	if len(archiveFiles) == 0 {
		return nil, errors.Errorf("compress output not found: %s", archivePath)
	}
	sort.Strings(archiveFiles)
	return archiveFiles, nil
}

func (t *ArchiveCompressTask) getUploadArchiveName(localName, archivePath string) string {
	prefix := strings.TrimSuffix(filepath.Base(archivePath), t.DstName)
	if prefix != "" && strings.HasPrefix(localName, prefix) {
		return strings.TrimPrefix(localName, prefix)
	}
	return localName
}

func cleanCompressSourceName(name string) string {
	return strings.TrimPrefix(stdpath.Clean("/"+strings.TrimSpace(name)), "/")
}

func getMountDirLocalPath(ctx context.Context, mountPath string) (string, error) {
	srcDirObj, err := Get(ctx, mountPath, &GetArgs{NoLog: true})
	if err != nil {
		return "", err
	}
	if !srcDirObj.IsDir() {
		return "", errors.New("source is not a folder")
	}
	localPath := srcDirObj.GetPath()
	if localPath == "" {
		return "", errors.New("source local path is empty")
	}
	stat, err := os.Stat(localPath)
	if err != nil {
		return "", err
	}
	if !stat.IsDir() {
		return "", errors.New("source local path is not a folder")
	}
	return localPath, nil
}

func copyMountPathToLocal(ctx context.Context, srcPath, localPath string, excludeLocalPathPrefixes ...string) error {
	obj, err := Get(ctx, srcPath, &GetArgs{NoLog: true})
	if err != nil {
		return err
	}
	if isExcludedByLocalPath(obj.GetPath(), excludeLocalPathPrefixes) {
		return nil
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
			if err = copyMountPathToLocal(ctx, childSrc, childLocal, excludeLocalPathPrefixes...); err != nil {
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

func isExcludedByLocalPath(localPath string, excludeLocalPathPrefixes []string) bool {
	if localPath == "" || len(excludeLocalPathPrefixes) == 0 {
		return false
	}
	cleanLocal := strings.ToLower(filepath.Clean(localPath))
	for _, excludePrefix := range excludeLocalPathPrefixes {
		if excludePrefix == "" {
			continue
		}
		cleanPrefix := strings.ToLower(filepath.Clean(excludePrefix))
		if cleanLocal == cleanPrefix || strings.HasPrefix(cleanLocal, cleanPrefix+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

var ArchiveCompressTaskManager *tache.Manager[*ArchiveCompressTask]

type ArchiveCompressArgs struct {
	Names      []string
	Format     string
	Password   string
	DstName    string
	CopyMode   string
	VolumeSize string
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
	copyMode := NormalizeArchiveCompressCopyMode(args.CopyMode)
	if !IsArchiveCompressCopyModeValid(copyMode) {
		return nil, errors.Errorf("invalid copy mode: %s", args.CopyMode)
	}
	if copyMode != ArchiveCompressCopyModeTemp && !srcStorage.Config().OnlyLocal {
		return nil, errors.New("copy_mode src_temp or none requires source storage to be local")
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
		CopyMode:      copyMode,
		VolumeSize:    args.VolumeSize,
		SrcStorageMp:  srcStorage.GetStorage().MountPath,
		DstStorageMp:  dstStorage.GetStorage().MountPath,
	}
	ArchiveCompressTaskManager.Add(t)
	return t, nil
}
