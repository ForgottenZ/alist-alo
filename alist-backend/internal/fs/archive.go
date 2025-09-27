package fs

import (
"context"
stderrors "errors"
"fmt"
"io"
"math/rand"
"mime"
"net/http"
"os"
stdpath "path"
"path/filepath"
"strconv"
"strings"
"time"

"github.com/alist-org/alist/v3/internal/conf"
"github.com/alist-org/alist/v3/internal/driver"
"github.com/alist-org/alist/v3/internal/errs"
"github.com/alist-org/alist/v3/internal/model"
"github.com/alist-org/alist/v3/internal/op"
"github.com/alist-org/alist/v3/internal/stream"
"github.com/alist-org/alist/v3/internal/task"
"github.com/pkg/errors"
log "github.com/sirupsen/logrus"
"github.com/xhofe/tache"
yzip "github.com/yeka/zip"
)

type ArchiveDownloadTask struct {
	task.TaskExtension
	model.ArchiveDecompressArgs
	status       string
	SrcObjPath   string
	DstDirPath   string
	srcStorage   driver.Driver
	dstStorage   driver.Driver
	SrcStorageMp string
	DstStorageMp string
}

func (t *ArchiveDownloadTask) GetName() string {
	return fmt.Sprintf("decompress [%s](%s)[%s] to [%s](%s) with password <%s>", t.SrcStorageMp, t.SrcObjPath,
		t.InnerPath, t.DstStorageMp, t.DstDirPath, t.Password)
}

func (t *ArchiveDownloadTask) GetStatus() string {
	return t.status
}

func (t *ArchiveDownloadTask) Run() error {
	t.ReinitCtx()
	t.ClearEndTime()
	t.SetStartTime(time.Now())
	defer func() { t.SetEndTime(time.Now()) }()
	uploadTask, err := t.RunWithoutPushUploadTask()
	if err != nil {
		return err
	}
	ArchiveContentUploadTaskManager.Add(uploadTask)
	return nil
}

func (t *ArchiveDownloadTask) RunWithoutPushUploadTask() (*ArchiveContentUploadTask, error) {
	var err error
	if t.srcStorage == nil {
		t.srcStorage, err = op.GetStorageByMountPath(t.SrcStorageMp)
	}
	srcObj, tool, ss, err := op.GetArchiveToolAndStream(t.Ctx(), t.srcStorage, t.SrcObjPath, model.LinkArgs{
		Header: http.Header{},
	})
	if err != nil {
		return nil, err
	}
	defer func() {
		var e error
		for _, s := range ss {
			e = stderrors.Join(e, s.Close())
		}
		if e != nil {
			log.Errorf("failed to close file streamer, %v", e)
		}
	}()
	var decompressUp model.UpdateProgress
	if t.CacheFull {
		var total, cur int64 = 0, 0
		for _, s := range ss {
			total += s.GetSize()
		}
		t.SetTotalBytes(total)
		t.status = "getting src object"
		for _, s := range ss {
			if s.GetFile() == nil {
				_, err = stream.CacheFullInTempFileAndUpdateProgress(s, func(p float64) {
					t.SetProgress((float64(cur) + float64(s.GetSize())*p/100.0) / float64(total))
				})
			}
			cur += s.GetSize()
			if err != nil {
				return nil, err
			}
		}
		t.SetProgress(100.0)
		decompressUp = func(_ float64) {}
	} else {
		decompressUp = t.SetProgress
	}
	t.status = "walking and decompressing"
	dir, err := os.MkdirTemp(conf.Conf.TempDir, "dir-*")
	if err != nil {
		return nil, err
	}
	err = tool.Decompress(ss, dir, t.ArchiveInnerArgs, decompressUp)
	if err != nil {
		return nil, err
	}
	baseName := strings.TrimSuffix(srcObj.GetName(), stdpath.Ext(srcObj.GetName()))
	uploadTask := &ArchiveContentUploadTask{
		TaskExtension: task.TaskExtension{
			Creator: t.GetCreator(),
		},
		ObjName:      baseName,
		InPlace:      !t.PutIntoNewDir,
		FilePath:     dir,
		DstDirPath:   t.DstDirPath,
		dstStorage:   t.dstStorage,
		DstStorageMp: t.DstStorageMp,
	}
	return uploadTask, nil
}

var ArchiveDownloadTaskManager *tache.Manager[*ArchiveDownloadTask]

type ArchiveContentUploadTask struct {
	task.TaskExtension
	status       string
	ObjName      string
	InPlace      bool
	FilePath     string
	DstDirPath   string
	dstStorage   driver.Driver
	DstStorageMp string
	finalized    bool
}

func (t *ArchiveContentUploadTask) GetName() string {
	return fmt.Sprintf("upload %s to [%s](%s)", t.ObjName, t.DstStorageMp, t.DstDirPath)
}

func (t *ArchiveContentUploadTask) GetStatus() string {
	return t.status
}

func (t *ArchiveContentUploadTask) Run() error {
	t.ReinitCtx()
	t.ClearEndTime()
	t.SetStartTime(time.Now())
	defer func() { t.SetEndTime(time.Now()) }()
	return t.RunWithNextTaskCallback(func(nextTsk *ArchiveContentUploadTask) error {
		ArchiveContentUploadTaskManager.Add(nextTsk)
		return nil
	})
}

func (t *ArchiveContentUploadTask) RunWithNextTaskCallback(f func(nextTsk *ArchiveContentUploadTask) error) error {
	var err error
	if t.dstStorage == nil {
		t.dstStorage, err = op.GetStorageByMountPath(t.DstStorageMp)
	}
	info, err := os.Stat(t.FilePath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		t.status = "src object is dir, listing objs"
		nextDstPath := t.DstDirPath
		if !t.InPlace {
			nextDstPath = stdpath.Join(nextDstPath, t.ObjName)
			err = op.MakeDir(t.Ctx(), t.dstStorage, nextDstPath)
			if err != nil {
				return err
			}
		}
		entries, err := os.ReadDir(t.FilePath)
		if err != nil {
			return err
		}
		var es error
		for _, entry := range entries {
			var nextFilePath string
			if entry.IsDir() {
				nextFilePath, err = moveToTempPath(stdpath.Join(t.FilePath, entry.Name()), "dir-")
			} else {
				nextFilePath, err = moveToTempPath(stdpath.Join(t.FilePath, entry.Name()), "file-")
			}
			if err != nil {
				es = stderrors.Join(es, err)
				continue
			}
			err = f(&ArchiveContentUploadTask{
				TaskExtension: task.TaskExtension{
					Creator: t.GetCreator(),
				},
				ObjName:      entry.Name(),
				InPlace:      false,
				FilePath:     nextFilePath,
				DstDirPath:   nextDstPath,
				dstStorage:   t.dstStorage,
				DstStorageMp: t.DstStorageMp,
			})
			if err != nil {
				es = stderrors.Join(es, err)
			}
		}
		if es != nil {
			return es
		}
	} else {
		t.SetTotalBytes(info.Size())
		file, err := os.Open(t.FilePath)
		if err != nil {
			return err
		}
		fs := &stream.FileStream{
			Obj: &model.Object{
				Name:     t.ObjName,
				Size:     info.Size(),
				Modified: time.Now(),
			},
			Mimetype:     mime.TypeByExtension(filepath.Ext(t.ObjName)),
			WebPutAsTask: true,
			Reader:       file,
		}
		fs.Closers.Add(file)
		t.status = "uploading"
		err = op.Put(t.Ctx(), t.dstStorage, t.DstDirPath, fs, t.SetProgress, true)
		if err != nil {
			return err
		}
	}
	t.deleteSrcFile()
	return nil
}

func (t *ArchiveContentUploadTask) Cancel() {
	t.TaskExtension.Cancel()
	if !conf.Conf.Tasks.AllowRetryCanceled {
		t.deleteSrcFile()
	}
}

func (t *ArchiveContentUploadTask) deleteSrcFile() {
	if !t.finalized {
		_ = os.RemoveAll(t.FilePath)
		t.finalized = true
	}
}

func moveToTempPath(path, prefix string) (string, error) {
	newPath, err := genTempFileName(prefix)
	if err != nil {
		return "", err
	}
	err = os.Rename(path, newPath)
	if err != nil {
		return "", err
	}
	return newPath, nil
}

func genTempFileName(prefix string) (string, error) {
	retry := 0
	for retry < 10000 {
		newPath := stdpath.Join(conf.Conf.TempDir, prefix+strconv.FormatUint(uint64(rand.Uint32()), 10))
		if _, err := os.Stat(newPath); err != nil {
			if os.IsNotExist(err) {
				return newPath, nil
			} else {
				return "", err
			}
		}
		retry++
	}
	return "", errors.New("failed to generate temp-file name: too many retries")
}

type archiveContentUploadTaskManagerType struct {
        *tache.Manager[*ArchiveContentUploadTask]
}

func (m *archiveContentUploadTaskManagerType) Remove(id string) {
        if t, ok := m.GetByID(id); ok {
                t.deleteSrcFile()
                m.Manager.Remove(id)
        }
}

type ArchiveCompressTask struct {
        task.TaskExtension
        status       string
        SrcDirPath   string
        Names        []string
        DstDirPath   string
        ArchiveName  string
        Password     string
        srcStorage   driver.Driver
        dstStorage   driver.Driver
        SrcStorageMp string
        DstStorageMp string
}

func (t *ArchiveCompressTask) GetName() string {
        return fmt.Sprintf("compress %s to [%s](%s)/%s", strings.Join(t.Names, ","), t.DstStorageMp, t.DstDirPath, t.ArchiveName)
}

func (t *ArchiveCompressTask) GetStatus() string {
        return t.status
}

func (t *ArchiveCompressTask) Run() error {
        t.ReinitCtx()
        t.ClearEndTime()
        t.SetStartTime(time.Now())
        defer func() { t.SetEndTime(time.Now()) }()
        var err error
        if t.srcStorage == nil {
                t.srcStorage, err = op.GetStorageByMountPath(t.SrcStorageMp)
                if err != nil {
                        return err
                }
        }
        if t.dstStorage == nil {
                t.dstStorage, err = op.GetStorageByMountPath(t.DstStorageMp)
                if err != nil {
                        return err
                }
        }
        if len(t.Names) == 0 {
                return errors.New("no objects to compress")
        }
        total, err := t.countTargets()
        if err != nil {
                return err
        }
        if total <= 0 {
                total = int64(len(t.Names))
        }
        t.SetTotalBytes(total)
        processed := int64(0)
        tempFile, err := os.CreateTemp(conf.Conf.TempDir, "compress-*.zip")
        if err != nil {
                return err
        }
        defer func() {
                _ = tempFile.Close()
                _ = os.Remove(tempFile.Name())
        }()
        zipWriter := yzip.NewWriter(tempFile)
        for _, name := range t.Names {
                t.status = fmt.Sprintf("compressing %s", name)
                actualPath := stdpath.Join(t.SrcDirPath, name)
                if err = t.writeEntry(zipWriter, actualPath, name, &processed, total); err != nil {
                        _ = zipWriter.Close()
                        return err
                }
        }
        if err = zipWriter.Close(); err != nil {
                return err
        }
        if _, err = tempFile.Seek(0, io.SeekStart); err != nil {
                return err
        }
        info, err := tempFile.Stat()
        if err != nil {
                return err
        }
        fileReader := tempFile
        fs := &stream.FileStream{
                Obj: &model.Object{
                        Name:     t.ArchiveName,
                        Size:     info.Size(),
                        Modified: time.Now(),
                },
                Mimetype:     mime.TypeByExtension(filepath.Ext(t.ArchiveName)),
                WebPutAsTask: true,
                Reader:       fileReader,
        }
        fs.Closers.Add(fileReader)
        t.status = "uploading archive"
        if err = op.Put(t.Ctx(), t.dstStorage, t.DstDirPath, fs, t.SetProgress, true); err != nil {
                return err
        }
        t.status = "done"
        return nil
}

func (t *ArchiveCompressTask) countTargets() (int64, error) {
        total := int64(0)
        for _, name := range t.Names {
                actual := stdpath.Join(t.SrcDirPath, name)
                obj, err := op.Get(t.Ctx(), t.srcStorage, actual)
                if err != nil {
                        return 0, err
                }
                c, err := t.countSingle(actual, obj)
                if err != nil {
                        return 0, err
                }
                total += c
        }
        return total, nil
}

func (t *ArchiveCompressTask) countSingle(actual string, obj model.Obj) (int64, error) {
        if !obj.IsDir() {
                return 1, nil
        }
        children, err := op.List(t.Ctx(), t.srcStorage, actual, model.ListArgs{})
        if err != nil {
                return 1, err
        }
        count := int64(1)
        for _, child := range children {
                childActual := stdpath.Join(actual, child.GetName())
                c, err := t.countSingle(childActual, child)
                if err != nil {
                        return 0, err
                }
                count += c
        }
        return count, nil
}

func (t *ArchiveCompressTask) writeEntry(zipWriter *yzip.Writer, actualPath, relativePath string, processed *int64, total int64) error {
        obj, err := op.Get(t.Ctx(), t.srcStorage, actualPath)
        if err != nil {
                        return err
        }
        name := strings.TrimPrefix(relativePath, "/")
        if obj.IsDir() {
                if name != "" {
                        header := &yzip.FileHeader{
                                Name:   utils.PathAddSeparatorSuffix(name),
                                Method: yzip.Store,
                        }
                        header.SetModTime(obj.ModTime())
                        if _, err = zipWriter.CreateHeader(header); err != nil {
                                return err
                        }
                }
                children, err := op.List(t.Ctx(), t.srcStorage, actualPath, model.ListArgs{})
                if err != nil {
                        return err
                }
                for _, child := range children {
                        childActual := stdpath.Join(actualPath, child.GetName())
                        childRelative := child.GetName()
                        if name != "" {
                                childRelative = stdpath.Join(name, childRelative)
                        }
                        if err = t.writeEntry(zipWriter, childActual, childRelative, processed, total); err != nil {
                                return err
                        }
                }
                return nil
        }
        header := &yzip.FileHeader{
                Name:   name,
                Method: yzip.Deflate,
        }
        header.SetModTime(obj.ModTime())
        var writer io.Writer
        if t.Password != "" {
                header.SetEncryptionMethod(yzip.AES256Encryption)
                header.SetPassword(t.Password)
                writer, err = zipWriter.CreateHeader(header)
        } else {
                writer, err = zipWriter.CreateHeader(header)
        }
        if err != nil {
                return err
        }
        link, _, err := op.Link(t.Ctx(), t.srcStorage, actualPath, model.LinkArgs{})
        if err != nil {
                return err
        }
        ss, err := stream.NewSeekableStream(stream.FileStream{Ctx: t.Ctx(), Obj: obj}, link)
        if err != nil {
                return err
        }
        defer ss.Close()
        if _, err = io.Copy(writer, ss); err != nil {
                return err
        }
        *processed = *processed + 1
        if total > 0 {
                t.SetProgress(float64(*processed) / float64(total) * 100)
        }
        return nil
}

func (m *archiveContentUploadTaskManagerType) RemoveAll() {
	tasks := m.GetAll()
	for _, t := range tasks {
		m.Remove(t.GetID())
	}
}

func (m *archiveContentUploadTaskManagerType) RemoveByState(state ...tache.State) {
	tasks := m.GetByState(state...)
	for _, t := range tasks {
		m.Remove(t.GetID())
	}
}

func (m *archiveContentUploadTaskManagerType) RemoveByCondition(condition func(task *ArchiveContentUploadTask) bool) {
	tasks := m.GetByCondition(condition)
	for _, t := range tasks {
		m.Remove(t.GetID())
	}
}

var ArchiveContentUploadTaskManager = &archiveContentUploadTaskManagerType{
        Manager: nil,
}

var ArchiveCompressTaskManager *tache.Manager[*ArchiveCompressTask]

func archiveMeta(ctx context.Context, path string, args model.ArchiveMetaArgs) (*model.ArchiveMetaProvider, error) {
	storage, actualPath, err := op.GetStorageAndActualPath(path)
	if err != nil {
		return nil, errors.WithMessage(err, "failed get storage")
	}
	return op.GetArchiveMeta(ctx, storage, actualPath, args)
}

func archiveList(ctx context.Context, path string, args model.ArchiveListArgs) ([]model.Obj, error) {
	storage, actualPath, err := op.GetStorageAndActualPath(path)
	if err != nil {
		return nil, errors.WithMessage(err, "failed get storage")
	}
	return op.ListArchive(ctx, storage, actualPath, args)
}

func archiveDecompress(ctx context.Context, srcObjPath, dstDirPath string, args model.ArchiveDecompressArgs, lazyCache ...bool) (task.TaskExtensionInfo, error) {
        srcStorage, srcObjActualPath, err := op.GetStorageAndActualPath(srcObjPath)
        if err != nil {
                return nil, errors.WithMessage(err, "failed get src storage")
        }
	dstStorage, dstDirActualPath, err := op.GetStorageAndActualPath(dstDirPath)
	if err != nil {
		return nil, errors.WithMessage(err, "failed get dst storage")
	}
	if srcStorage.GetStorage() == dstStorage.GetStorage() {
		err = op.ArchiveDecompress(ctx, srcStorage, srcObjActualPath, dstDirActualPath, args, lazyCache...)
		if !errors.Is(err, errs.NotImplement) {
			return nil, err
		}
	}
	taskCreator, _ := ctx.Value("user").(*model.User)
	tsk := &ArchiveDownloadTask{
		TaskExtension: task.TaskExtension{
			Creator: taskCreator,
		},
		ArchiveDecompressArgs: args,
		srcStorage:            srcStorage,
		dstStorage:            dstStorage,
		SrcObjPath:            srcObjActualPath,
		DstDirPath:            dstDirActualPath,
		SrcStorageMp:          srcStorage.GetStorage().MountPath,
		DstStorageMp:          dstStorage.GetStorage().MountPath,
	}
	if ctx.Value(conf.NoTaskKey) != nil {
		uploadTask, err := tsk.RunWithoutPushUploadTask()
		if err != nil {
			return nil, errors.WithMessagef(err, "failed download [%s]", srcObjPath)
		}
		defer uploadTask.deleteSrcFile()
		var callback func(t *ArchiveContentUploadTask) error
		callback = func(t *ArchiveContentUploadTask) error {
			e := t.RunWithNextTaskCallback(callback)
			t.deleteSrcFile()
			return e
		}
		return nil, uploadTask.RunWithNextTaskCallback(callback)
	} else {
		ArchiveDownloadTaskManager.Add(tsk)
		return tsk, nil
	}
}

func archiveCompress(ctx context.Context, srcDirPath, dstDirPath, archiveName string, names []string, password string) (task.TaskExtensionInfo, error) {
        srcStorage, srcActualDirPath, err := op.GetStorageAndActualPath(srcDirPath)
        if err != nil {
                return nil, errors.WithMessage(err, "failed get src storage")
        }
        dstStorage, dstActualDirPath, err := op.GetStorageAndActualPath(dstDirPath)
        if err != nil {
                return nil, errors.WithMessage(err, "failed get dst storage")
        }
        sanitized := make([]string, 0, len(names))
        for _, name := range names {
                name = strings.TrimSpace(name)
                if name != "" {
                        sanitized = append(sanitized, name)
                }
        }
        if len(sanitized) == 0 {
                return nil, errors.New("no objects to compress")
        }
        if strings.TrimSpace(archiveName) == "" {
                archiveName = fmt.Sprintf("压缩包-%s.zip", time.Now().Format("20060102150405"))
        }
        if !strings.HasSuffix(strings.ToLower(archiveName), ".zip") {
                archiveName += ".zip"
        }
        taskCreator, _ := ctx.Value("user").(*model.User)
        task := &ArchiveCompressTask{
                TaskExtension: task.TaskExtension{
                        Creator: taskCreator,
                },
                SrcDirPath:   srcActualDirPath,
                Names:        sanitized,
                DstDirPath:   dstActualDirPath,
                ArchiveName:  archiveName,
                Password:     password,
                srcStorage:   srcStorage,
                dstStorage:   dstStorage,
                SrcStorageMp: srcStorage.GetStorage().MountPath,
                DstStorageMp: dstStorage.GetStorage().MountPath,
        }
        if ctx.Value(conf.NoTaskKey) != nil {
                if err := task.Run(); err != nil {
                        return nil, err
                }
                return nil, nil
        }
        ArchiveCompressTaskManager.Add(task)
        return task, nil
}

func archiveDriverExtract(ctx context.Context, path string, args model.ArchiveInnerArgs) (*model.Link, model.Obj, error) {
	storage, actualPath, err := op.GetStorageAndActualPath(path)
	if err != nil {
		return nil, nil, errors.WithMessage(err, "failed get storage")
	}
	return op.DriverExtract(ctx, storage, actualPath, args)
}

func archiveInternalExtract(ctx context.Context, path string, args model.ArchiveInnerArgs) (io.ReadCloser, int64, error) {
	storage, actualPath, err := op.GetStorageAndActualPath(path)
	if err != nil {
		return nil, 0, errors.WithMessage(err, "failed get storage")
	}
	return op.InternalExtract(ctx, storage, actualPath, args)
}
