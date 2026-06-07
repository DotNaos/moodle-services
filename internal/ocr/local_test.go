package ocr

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	bin := buildPdftotextHelper(t, dir)
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

func buildPdftotextHelper(t *testing.T, dir string) string {
	t.Helper()
	source := filepath.Join(dir, "pdftotext-helper.go")
	binary := filepath.Join(dir, "pdftotext-helper")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	code := `package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) != 4 || os.Args[1] != "-layout" {
		fmt.Fprintf(os.Stderr, "unexpected args: %v\n", os.Args[1:])
		os.Exit(2)
	}
	fmt.Fprintf(os.Stderr, "called %s %s %s\n", os.Args[1], os.Args[2], os.Args[3])
	if err := os.WriteFile(os.Args[3], []byte("Extracted PDF text\nSecond line\n"), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
`
	if err := os.WriteFile(source, []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-o", binary, source)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build helper: %v\n%s", err, output)
	}
	return binary
}
