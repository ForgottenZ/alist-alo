package fs

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	stdpath "path"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/internal/stream"
	"github.com/alist-org/alist/v3/internal/task"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/pkg/errors"
	"github.com/xhofe/tache"
	"github.com/yeka/zip"
)

type compressSource struct {
	StorageMp  string `json:"storage_mp"`
	ActualPath string `json:"actual_path"`
	Name       string `json:"name"`
}

type CompressTask struct {
	task.TaskExtension
	Status       string           `json:"-"`
	Sources      []compressSource `json:"sources"`
	DstDirPath   string           `json:"dst_dir_path"`
	DstStorageMp string           `json:"dst_storage_mp"`
	ArchiveName  string           `json:"archive_name"`
	Password     string           `json:"password"`

	dstStorage driver.Driver `json:"-"`
}

func (t *CompressTask) GetName() string {
	return fmt.Sprintf("压缩%d个对象到[%s](%s)/%s", len(t.Sources), t.DstStorageMp, t.DstDirPath, t.ArchiveName)
}

func (t *CompressTask) GetStatus() string {
	return t.Status
}

func (t *CompressTask) Run() error {
	t.ReinitCtx()
	t.ClearEndTime()
	t.SetStartTime(time.Now())
	defer func() { t.SetEndTime(time.Now()) }()
	if t.ArchiveName == "" {
		return errors.New("压缩文件名不能为空")
	}
	var err error
	if t.dstStorage == nil {
		t.dstStorage, err = op.GetStorageByMountPath(t.DstStorageMp)
		if err != nil {
			return errors.WithMessage(err, "获取目标存储失败")
		}
	}
	total, err := t.calculateTotalSize()
	if err != nil {
		return err
	}
	if total > 0 {
		t.SetTotalBytes(total)
	}
	archiveFile, err := os.CreateTemp(conf.Conf.TempDir, "compress-*.zip")
	if err != nil {
		return err
	}
	defer func() {
		_ = archiveFile.Close()
		_ = os.Remove(archiveFile.Name())
	}()
	zipWriter := zip.NewWriter(archiveFile)
	processed := int64(0)
	for _, source := range t.Sources {
		storage, e := op.GetStorageByMountPath(source.StorageMp)
		if e != nil {
			zipWriter.Close()
			return errors.WithMessagef(e, "加载存储[%s]失败", source.StorageMp)
		}
		entryName := source.Name
		if entryName == "" {
			entryName = stdpath.Base(source.ActualPath)
		}
		if err = t.writeEntry(storage, source.ActualPath, entryName, zipWriter, &processed, total); err != nil {
			zipWriter.Close()
			return err
		}
	}
	if err = zipWriter.Close(); err != nil {
		return err
	}
	if _, err = archiveFile.Seek(0, io.SeekStart); err != nil {
		return err
	}
	info, err := archiveFile.Stat()
	if err != nil {
		return err
	}
	obj := &model.Object{
		Name:     t.ArchiveName,
		Size:     info.Size(),
		Modified: time.Now(),
		Ctime:    time.Now(),
	}
	fileStream := &stream.FileStream{
		Obj: obj,
		Ctx: t.Ctx(),
	}
	fileStream.SetTmpFile(archiveFile)
	if err = op.Put(t.Ctx(), t.dstStorage, t.DstDirPath, fileStream, t.SetProgress, true); err != nil {
		return err
	}
	if total == 0 {
		t.SetProgress(100)
	}
	t.Status = "压缩完成"
	return nil
}

var CompressTaskManager *tache.Manager[*CompressTask]

func ArchiveCompress(ctx context.Context, srcPaths []string, dstDirPath, archiveName, password string) (task.TaskExtensionInfo, error) {
	if len(srcPaths) == 0 {
		return nil, errors.New("没有可压缩的对象")
	}
	archiveName = strings.TrimSpace(archiveName)
	if archiveName == "" {
		return nil, errors.New("压缩文件名不能为空")
	}
	dstStorage, dstActual, err := op.GetStorageAndActualPath(dstDirPath)
	if err != nil {
		return nil, errors.WithMessage(err, "获取目标存储失败")
	}
	sources := make([]compressSource, 0, len(srcPaths))
	for _, path := range srcPaths {
		storage, actual, e := op.GetStorageAndActualPath(path)
		if e != nil {
			return nil, errors.WithMessagef(e, "读取[%s]失败", path)
		}
		sources = append(sources, compressSource{
			StorageMp:  storage.GetStorage().MountPath,
			ActualPath: actual,
			Name:       stdpath.Base(actual),
		})
	}
	taskCreator, _ := ctx.Value("user").(*model.User)
	t := &CompressTask{
		TaskExtension: task.TaskExtension{Creator: taskCreator},
		Sources:       sources,
		DstDirPath:    dstActual,
		DstStorageMp:  dstStorage.GetStorage().MountPath,
		ArchiveName:   archiveName,
		Password:      password,
		dstStorage:    dstStorage,
	}
	CompressTaskManager.Add(t)
	return t, nil
}

func (t *CompressTask) calculateTotalSize() (int64, error) {
	total := int64(0)
	for _, source := range t.Sources {
		storage, err := op.GetStorageByMountPath(source.StorageMp)
		if err != nil {
			return 0, err
		}
		size, err := t.entrySize(storage, source.ActualPath)
		if err != nil {
			return 0, err
		}
		total += size
	}
	return total, nil
}

func (t *CompressTask) entrySize(storage driver.Driver, actualPath string) (int64, error) {
	obj, err := op.Get(t.Ctx(), storage, actualPath)
	if err != nil {
		return 0, err
	}
	if !obj.IsDir() {
		return obj.GetSize(), nil
	}
	children, err := op.List(t.Ctx(), storage, actualPath, model.ListArgs{Refresh: true})
	if err != nil {
		return 0, err
	}
	total := int64(0)
	for _, child := range children {
		size, err := t.entrySize(storage, stdpath.Join(actualPath, child.GetName()))
		if err != nil {
			return 0, err
		}
		total += size
	}
	return total, nil
}

func (t *CompressTask) writeEntry(storage driver.Driver, actualPath, entryName string, zw *zip.Writer, processed *int64, total int64) error {
	if utils.IsCanceled(t.Ctx()) {
		return context.Canceled
	}
	obj, err := op.Get(t.Ctx(), storage, actualPath)
	if err != nil {
		return err
	}
	if obj.IsDir() {
		if entryName != "" {
			t.Status = fmt.Sprintf("扫描目录 %s", entryName)
			if !strings.HasSuffix(entryName, "/") {
				if _, err := zw.Create(entryName + "/"); err != nil {
					return err
				}
			}
		}
		children, err := op.List(t.Ctx(), storage, actualPath, model.ListArgs{Refresh: true})
		if err != nil {
			return err
		}
		for _, child := range children {
			childActual := stdpath.Join(actualPath, child.GetName())
			childEntry := child.GetName()
			if entryName != "" {
				childEntry = stdpath.Join(entryName, child.GetName())
			}
			if err := t.writeEntry(storage, childActual, childEntry, zw, processed, total); err != nil {
				return err
			}
		}
		return nil
	}
	t.Status = fmt.Sprintf("压缩 %s", entryName)
	link, _, err := op.Link(t.Ctx(), storage, actualPath, model.LinkArgs{Header: http.Header{}})
	if err != nil {
		return err
	}
	fs := stream.FileStream{
		Obj: obj,
		Ctx: t.Ctx(),
	}
	ss, err := stream.NewSeekableStream(fs, link)
	if err != nil {
		return err
	}
	defer ss.Close()
	var writer io.Writer
	if t.Password != "" {
		writer, err = zw.Encrypt(entryName, t.Password, zip.AES256Encryption)
	} else {
		writer, err = zw.Create(entryName)
	}
	if err != nil {
		return err
	}
	if _, err = io.Copy(writer, ss); err != nil {
		return err
	}
	*processed += obj.GetSize()
	if total > 0 {
		t.SetProgress(float64(*processed) / float64(total) * 100)
	}
	return nil
}
