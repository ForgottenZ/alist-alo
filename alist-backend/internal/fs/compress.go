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
	"github.com/pkg/errors"
	"github.com/xhofe/tache"
	"github.com/yeka/zip"
)

type compressEntry struct {
	ActualPath string
	Relative   string
}

type CompressTask struct {
	task.TaskExtension
	Status       string
	Password     string
	OutputName   string
	DstDirMount  string
	entries      []compressEntry
	storage      driver.Driver
	storageMount string
	totalSize    int64
}

func (t *CompressTask) GetName() string {
	names := make([]string, 0, len(t.entries))
	for _, e := range t.entries {
		names = append(names, e.Relative)
	}
	return fmt.Sprintf("压缩 %s -> [%s](%s)", strings.Join(names, ", "), t.storageMount, t.DstDirMount)
}

func (t *CompressTask) GetStatus() string {
	return t.Status
}

func (t *CompressTask) Run() error {
	t.ReinitCtx()
	t.ClearEndTime()
	t.SetStartTime(time.Now())
	defer func() { t.SetEndTime(time.Now()) }()
	if len(t.entries) == 0 {
		return errors.New("没有可压缩的内容")
	}
	if t.storage == nil {
		var err error
		t.storage, err = op.GetStorageByMountPath(t.storageMount)
		if err != nil {
			return err
		}
	}
	tmpFile, err := os.CreateTemp(conf.Conf.TempDir, "compress-*.zip")
	if err != nil {
		return err
	}
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
	}()
	t.Status = "计算体积"
	total, err := t.computeTotalSize(t.entries)
	if err != nil {
		return err
	}
	t.totalSize = total
	t.SetTotalBytes(total)
	writer := zip.NewWriter(tmpFile)
	progress := &compressProgress{task: t, total: total}
	for _, entry := range t.entries {
		if err := t.writeEntry(entry.ActualPath, entry.Relative, writer, progress); err != nil {
			_ = writer.Close()
			return err
		}
	}
	if err := writer.Close(); err != nil {
		return err
	}
	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		return err
	}
	info, err := tmpFile.Stat()
	if err != nil {
		return err
	}
	obj := &model.Object{
		Name:     t.OutputName,
		Size:     info.Size(),
		Modified: time.Now(),
	}
	fileStream := &stream.FileStream{Ctx: t.Ctx(), Obj: obj}
	fileStream.SetTmpFile(tmpFile)
	t.Status = "上传压缩包"
	err = PutDirectly(context.WithValue(t.Ctx(), conf.NoTaskKey, struct{}{}), t.DstDirMount, fileStream)
	if err != nil {
		return err
	}
	t.SetProgress(100)
	return nil
}

func (t *CompressTask) computeTotalSize(entries []compressEntry) (int64, error) {
	var total int64
	for _, entry := range entries {
		obj, err := op.Get(t.Ctx(), t.storage, entry.ActualPath)
		if err != nil {
			return 0, err
		}
		if obj.IsDir() {
			children, err := op.List(t.Ctx(), t.storage, entry.ActualPath, model.ListArgs{})
			if err != nil {
				return 0, err
			}
			next := make([]compressEntry, 0, len(children))
			for _, child := range children {
				next = append(next, compressEntry{
					ActualPath: stdpath.Join(entry.ActualPath, child.GetName()),
					Relative:   stdpath.Join(entry.Relative, child.GetName()),
				})
			}
			size, err := t.computeTotalSize(next)
			if err != nil {
				return 0, err
			}
			total += size
		} else {
			total += obj.GetSize()
		}
	}
	return total, nil
}

func (t *CompressTask) writeEntry(actual, relative string, writer *zip.Writer, progress *compressProgress) error {
	obj, err := op.Get(t.Ctx(), t.storage, actual)
	if err != nil {
		return err
	}
	if obj.IsDir() {
		header := &zip.FileHeader{Name: strings.TrimSuffix(relative, "/") + "/"}
		header.Method = zip.Store
		header.SetModTime(obj.ModTime())
		if t.Password != "" {
			header.SetPassword(t.Password)
			header.SetEncryptionMethod(zip.AES256Encryption)
		}
		if _, err := writer.CreateHeader(header); err != nil {
			return err
		}
		children, err := op.List(t.Ctx(), t.storage, actual, model.ListArgs{})
		if err != nil {
			return err
		}
		for _, child := range children {
			if err := t.writeEntry(stdpath.Join(actual, child.GetName()), stdpath.Join(relative, child.GetName()), writer, progress); err != nil {
				return err
			}
		}
		return nil
	}
	header := &zip.FileHeader{Name: relative, Method: zip.Deflate}
	header.SetModTime(obj.ModTime())
	header.UncompressedSize64 = uint64(obj.GetSize())
	if t.Password != "" {
		header.SetPassword(t.Password)
		header.SetEncryptionMethod(zip.AES256Encryption)
	}
	fileWriter, err := writer.CreateHeader(header)
	if err != nil {
		return err
	}
	link, _, err := op.Link(t.Ctx(), t.storage, actual, model.LinkArgs{Header: http.Header{}})
	if err != nil {
		return err
	}
	fs := stream.FileStream{Obj: obj, Ctx: t.Ctx()}
	seekable, err := stream.NewSeekableStream(fs, link)
	if err != nil {
		return err
	}
	defer seekable.Close()
	reader := io.TeeReader(seekable, progress)
	if _, err := io.Copy(fileWriter, reader); err != nil {
		return err
	}
	return nil
}

type compressProgress struct {
	task    *CompressTask
	total   int64
	written int64
}

func (p *compressProgress) Write(b []byte) (int, error) {
	n := len(b)
	p.written += int64(n)
	if p.total > 0 {
		p.task.SetProgress(float64(p.written) / float64(p.total) * 100)
	}
	return n, nil
}

var CompressTaskManager *tache.Manager[*CompressTask]

func ArchiveCompress(ctx context.Context, srcPaths []string, dstDir, name, password string) (task.TaskExtensionInfo, error) {
	if len(srcPaths) == 0 {
		return nil, errors.New("没有可压缩的内容")
	}
	if name == "" {
		name = "archive.zip"
	}
	if !strings.Contains(name, ".") {
		name += ".zip"
	}
	dstStorage, _, err := op.GetStorageAndActualPath(dstDir)
	if err != nil {
		return nil, errors.WithMessage(err, "目标路径无效")
	}
	entries := make([]compressEntry, 0, len(srcPaths))
	var baseStorage driver.Driver
	var mountPath string
	for _, src := range srcPaths {
		storage, actual, err := op.GetStorageAndActualPath(src)
		if err != nil {
			return nil, errors.WithMessagef(err, "无法读取[%s]", src)
		}
		if baseStorage == nil {
			baseStorage = storage
			mountPath = storage.GetStorage().MountPath
		} else if baseStorage.GetStorage() != storage.GetStorage() {
			return nil, errors.New("暂不支持跨存储压缩")
		}
		entries = append(entries, compressEntry{
			ActualPath: actual,
			Relative:   stdpath.Base(actual),
		})
	}
	if baseStorage.GetStorage() != dstStorage.GetStorage() {
		return nil, errors.New("源文件与目标目录必须在同一存储内")
	}
	taskCreator, _ := ctx.Value("user").(*model.User)
	task := &CompressTask{
		TaskExtension: task.TaskExtension{Creator: taskCreator},
		Password:      password,
		OutputName:    name,
		DstDirMount:   dstDir,
		entries:       entries,
		storage:       baseStorage,
		storageMount:  mountPath,
	}
	if ctx.Value(conf.NoTaskKey) != nil {
		if err := task.Run(); err != nil {
			return nil, err
		}
		return nil, nil
	}
	CompressTaskManager.Add(task)
	return task, nil
}
