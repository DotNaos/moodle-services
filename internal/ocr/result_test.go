package ocr

import (
	"os"
	"path/filepath"
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
	warnings := DetectWarnings(result)
	for _, want := range []string{"empty or very short Markdown", "no images extracted", "output contains failed placeholder text"} {
		if !containsString(warnings, want) {
			t.Fatalf("expected warning %q in %#v", want, warnings)
		}
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
