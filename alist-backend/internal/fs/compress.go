package fs

import (
    "context"
    "fmt"
    "io"
    "os"
    stdpath "path"
    "strings"
    "time"

    "github.com/alist-org/alist/v3/internal/driver"
    "github.com/alist-org/alist/v3/internal/model"
    "github.com/alist-org/alist/v3/internal/op"
    "github.com/alist-org/alist/v3/internal/stream"
    "github.com/alist-org/alist/v3/pkg/utils"
    zip "github.com/yeka/zip"
)

func archiveCompress(ctx context.Context, srcDirPath string, names []string, dstDirPath, archiveName, password string) error {
    if len(names) == 0 {
        return fmt.Errorf("no source objects provided")
    }
    if strings.TrimSpace(archiveName) == "" {
        archiveName = fmt.Sprintf("压缩包-%d.zip", time.Now().Unix())
    }
    if !strings.HasSuffix(strings.ToLower(archiveName), ".zip") {
        archiveName = archiveName + ".zip"
    }
    srcStorage, srcActualDir, err := op.GetStorageAndActualPath(srcDirPath)
    if err != nil {
        return err
    }
    dstStorage, dstActualDir, err := op.GetStorageAndActualPath(dstDirPath)
    if err != nil {
        return err
    }
    tmpFile, err := os.CreateTemp("", "alist-compress-*.zip")
    if err != nil {
        return err
    }
    defer func() {
        _ = tmpFile.Close()
        _ = os.Remove(tmpFile.Name())
    }()
    writer := zip.NewWriter(tmpFile)
    for _, name := range names {
        actualPath := stdpath.Join(srcActualDir, name)
        if err := addToZip(ctx, srcStorage, actualPath, name, writer, password); err != nil {
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
        Name:     archiveName,
        Size:     info.Size(),
        Modified: time.Now(),
    }
    fileStream := &stream.FileStream{
        Ctx:     ctx,
        Obj:     obj,
        Reader:  tmpFile,
    }
    fileStream.Add(tmpFile)
    return op.Put(ctx, dstStorage, dstActualDir, fileStream, nil)
}

func addToZip(ctx context.Context, storage driver.Driver, actualPath, zipPath string, writer *zip.Writer, password string) error {
    obj, err := op.Get(ctx, storage, actualPath)
    if err != nil {
        return err
    }
    if obj.IsDir() {
        header := &zip.FileHeader{
            Name:   strings.TrimSuffix(zipPath, "/") + "/",
            Method: zip.Store,
        }
        header.SetModTime(obj.ModTime())
        if password != "" {
            header.SetPassword(password)
            header.SetEncryptionMethod(zip.AES256Encryption)
        }
        if _, err := writer.CreateHeader(header); err != nil {
            return err
        }
        children, err := op.List(ctx, storage, actualPath, model.ListArgs{})
        if err != nil {
            return err
        }
        for _, child := range children {
            childPath := stdpath.Join(actualPath, child.GetName())
            childZip := stdpath.Join(zipPath, child.GetName())
            if err := addToZip(ctx, storage, childPath, childZip, writer, password); err != nil {
                return err
            }
        }
        return nil
    }
    header := &zip.FileHeader{
        Name:   zipPath,
        Method: zip.Deflate,
    }
    header.SetModTime(obj.ModTime())
    header.UncompressedSize64 = uint64(obj.GetSize())
    if password != "" {
        header.SetPassword(password)
        header.SetEncryptionMethod(zip.AES256Encryption)
    }
    zipWriter, err := writer.CreateHeader(header)
    if err != nil {
        return err
    }
    link, fileObj, err := op.Link(ctx, storage, actualPath, model.LinkArgs{})
    if err != nil {
        return err
    }
    fs := stream.FileStream{Ctx: ctx, Obj: fileObj}
    ss, err := stream.NewSeekableStream(fs, link)
    if err != nil {
        return err
    }
    defer ss.Close()
    _, err = utils.CopyWithBuffer(zipWriter, ss)
    return err
}
