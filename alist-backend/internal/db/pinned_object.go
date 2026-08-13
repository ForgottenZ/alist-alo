package db

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/alist-org/alist/v3/internal/model"
	"github.com/pkg/errors"
)

func pinnedPathHash(path string) string {
	sum := sha256.Sum256([]byte(path))
	return hex.EncodeToString(sum[:])
}

func SetPinnedPath(path string, pinned bool) error {
	pathHash := pinnedPathHash(path)
	if !pinned {
		return errors.WithStack(
			db.Where("path_hash = ?", pathHash).Delete(&model.PinnedObject{}).Error,
		)
	}
	item := &model.PinnedObject{PathHash: pathHash, Path: path}
	return errors.WithStack(db.Where("path_hash = ?", pathHash).FirstOrCreate(item).Error)
}

func GetPinnedPaths(paths []string) (map[string]struct{}, error) {
	result := make(map[string]struct{})
	if len(paths) == 0 {
		return result, nil
	}
	hashes := make([]string, 0, len(paths))
	for _, path := range paths {
		hashes = append(hashes, pinnedPathHash(path))
	}
	var items []model.PinnedObject
	if err := db.Where("path_hash IN ?", hashes).Find(&items).Error; err != nil {
		return nil, errors.Wrap(err, "failed get pinned paths")
	}
	for _, item := range items {
		result[item.Path] = struct{}{}
	}
	return result, nil
}
