package model

// PinnedObject records a virtual AList path that should be placed before
// unpinned objects when its parent directory is listed.
type PinnedObject struct {
	PathHash string `json:"-" gorm:"primaryKey;size:64"`
	Path     string `json:"path" gorm:"type:text;not null"`
}
