package cleaner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetDirSize(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "cleaner_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	testFile := filepath.Join(tempDir, "sample.txt")
	content := []byte("hello world 12345")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	size := getDirSize(tempDir, 3)
	if size != int64(len(content)) {
		t.Errorf("expected size %d, got %d", len(content), size)
	}
}

func TestTargetCachesDefinitions(t *testing.T) {
	if len(targetCaches) == 0 {
		t.Error("expected targetCaches to contain cache targets")
	}

	for _, c := range targetCaches {
		if c.name == "" || c.rel == "" {
			t.Errorf("invalid cache target definition: %+v", c)
		}
	}
}
