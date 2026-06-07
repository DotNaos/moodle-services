package ocr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizePdftotextMarkdown(t *testing.T) {
	got := string(normalizePdftotextMarkdown([]byte("\n  hello\nworld  \n")))
	if got != "hello\nworld\n" {
		t.Fatalf("normalizePdftotextMarkdown = %q", got)
	}
}

func TestLocalExecutorRunsPdftotextCompatibleBinary(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "pdftotext")
	script := "#!/bin/sh\nprintf 'called %s %s %s\\n' \"$1\" \"$2\" \"$3\" >&2\nprintf 'Extracted PDF text\\nSecond line\\n' > \"$3\"\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(dir, "input.pdf")
	if err := os.WriteFile(input, []byte("%PDF-1.4\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "out")
	provider, _ := ProviderByID("pdftotext")

	result, err := (LocalExecutor{PdftotextBinary: bin}).Run(t.Context(), provider, input, output, Options{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status != StatusSuccess {
		t.Fatalf("expected success, got %#v", result)
	}
	if strings.TrimSpace(result.Markdown) != "Extracted PDF text\nSecond line" {
		t.Fatalf("unexpected markdown: %q", result.Markdown)
	}
	if !containsString(result.OutputFiles, "output.md") || !containsString(result.OutputFiles, "text.txt") {
		t.Fatalf("expected normalized output files, got %#v", result.OutputFiles)
	}
	if containsString(result.Warnings, "no images extracted") {
		t.Fatalf("did not expect image warning: %#v", result.Warnings)
	}
}
