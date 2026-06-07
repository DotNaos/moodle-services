package ocr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseOutputReadsNormalizedFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "images"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "output.md"), []byte("This is a long enough markdown output for a Moodle study PDF. It contains useful text and details."), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "output.html"), []byte("<p>Hello</p>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "output.json"), []byte(`{"ok":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "images", "page.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider, _ := ProviderByID("docling")
	result := ParseOutput(provider, dir, 0, false, 123)
	if result.Status != StatusSuccess {
		t.Fatalf("expected success, got %s", result.Status)
	}
	if result.Markdown == "" || result.HTML == "" || result.JSON == nil {
		t.Fatalf("expected parsed outputs, got %#v", result)
	}
	if len(result.Images) != 1 {
		t.Fatalf("expected one image, got %#v", result.Images)
	}
}

func TestDetectWarnings(t *testing.T) {
	result := RunResult{Engine: "docling", Markdown: "Formula not decoded", Images: []ImageArtifact{}}
	provider, _ := ProviderByID("docling")
	warnings := DetectWarnings(provider, result)
	for _, want := range []string{"empty or very short Markdown", "no images extracted", "output contains failed placeholder text"} {
		if !containsString(warnings, want) {
			t.Fatalf("expected warning %q in %#v", want, warnings)
		}
	}
}

func TestDetectWarningsSkipsImagesForTextOnlyProvider(t *testing.T) {
	provider, _ := ProviderByID("pdftotext")
	result := RunResult{Engine: "pdftotext", Markdown: strings.Repeat("text ", 30), Images: []ImageArtifact{}}
	warnings := DetectWarnings(provider, result)
	if containsString(warnings, "no images extracted") {
		t.Fatalf("did not expect image warning for text-only provider: %#v", warnings)
	}
}

func TestFormatComparisonWarningsTruncates(t *testing.T) {
	got := formatComparisonWarnings([]string{strings.Repeat("x", 800)})
	if len([]rune(got)) > 503 {
		t.Fatalf("warning was not truncated: %d", len([]rune(got)))
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("expected ellipsis, got %q", got)
	}
}

func TestFormatComparisonFilesSummarizesLongLists(t *testing.T) {
	files := make([]string, 0, 20)
	for i := 0; i < 20; i++ {
		files = append(files, filepath.Join("images", strings.Repeat("x", 80), "page.png"))
	}
	got := formatComparisonFiles(files)
	if len([]rune(got)) > 503 {
		t.Fatalf("file list was not truncated: %d", len([]rune(got)))
	}
	if !strings.Contains(got, "20 files total") {
		t.Fatalf("expected total file count, got %q", got)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
