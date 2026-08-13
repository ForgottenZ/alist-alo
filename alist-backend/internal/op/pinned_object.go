package op

import (
	"github.com/alist-org/alist/v3/internal/db"
	"github.com/alist-org/alist/v3/pkg/utils"
)

func SetPinnedPath(path string, pinned bool) error {
	return db.SetPinnedPath(utils.FixAndCleanPath(path), pinned)
}

func GetPinnedPaths(paths []string) (map[string]struct{}, error) {
	for i := range paths {
		paths[i] = utils.FixAndCleanPath(paths[i])
	}
	return db.GetPinnedPaths(paths)
}
