package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLooksLikeBackupTextRejectsBinaryData(t *testing.T) {
	data := []byte{0x50, 0x4b, 0x03, 0x04, 0x00, 0x14, 0x08, 0x00}

	if looksLikeBackupText(data) {
		t.Fatal("expected binary ZIP-like data to be rejected")
	}
}

func TestIsBackupPlainTextResourceAcceptsTextContent(t *testing.T) {
	data := []byte("title,score\nalpha,42\n")

	if !isBackupPlainTextResource("csv", "text/csv", data) {
		t.Fatal("expected CSV text to be accepted")
	}
}

func TestIsBackupPlainTextResourceRejectsPowerPointContent(t *testing.T) {
	data := []byte{0x50, 0x4b, 0x03, 0x04, 0x00, 0x14, 0x08, 0x00}

	if isBackupPlainTextResource("pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation", data) {
		t.Fatal("expected PPTX content to be rejected")
	}
}

func TestResetBackupTextDirRemovesStaleFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "materials-text")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stale.md"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := resetBackupTextDir(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("expected stale text directory to be removed, got %v", err)
	}
}
