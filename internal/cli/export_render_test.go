package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLooksLikeExportTextRejectsBinaryData(t *testing.T) {
	data := []byte{0x50, 0x4b, 0x03, 0x04, 0x00, 0x14, 0x08, 0x00}

	if looksLikeExportText(data) {
		t.Fatal("expected binary ZIP-like data to be rejected")
	}
}

func TestIsExportPlainTextResourceAcceptsTextContent(t *testing.T) {
	data := []byte("title,score\nalpha,42\n")

	if !isExportPlainTextResource("csv", "text/csv", data) {
		t.Fatal("expected CSV text to be accepted")
	}
}

func TestIsExportPlainTextResourceRejectsPowerPointContent(t *testing.T) {
	data := []byte{0x50, 0x4b, 0x03, 0x04, 0x00, 0x14, 0x08, 0x00}

	if isExportPlainTextResource("pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation", data) {
		t.Fatal("expected PPTX content to be rejected")
	}
}

func TestResetExportTextDirRemovesStaleFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "materials-text")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stale.md"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := resetExportTextDir(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("expected stale text directory to be removed, got %v", err)
	}
}

func TestSanitizeExportTextRemovesUnreadableControls(t *testing.T) {
	input := "alpha\x00 beta\x02\nkeep\tspacing"

	got := sanitizeExportText(input)
	if got != "alpha  beta \nkeep\tspacing" {
		t.Fatalf("unexpected sanitized text: %q", got)
	}
}
