package handles

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestGetChunkUploadTempPath(t *testing.T) {
	tempDir := "/tmp/chunk_upload"
	uploadID := strings.Repeat("very-long-upload-id-", 30)

	got := getChunkUploadTempPath(tempDir, uploadID)
	base := filepath.Base(got)

	if len(base) != len("0123456789abcdef0123456789abcdef.part") {
		t.Fatalf("unexpected temp file name length: %d", len(base))
	}
	if !strings.HasSuffix(base, ".part") {
		t.Fatalf("temp file should use .part suffix: %s", base)
	}
	if filepath.Dir(got) != tempDir {
		t.Fatalf("temp path should remain in tempDir: %s", got)
	}
}
