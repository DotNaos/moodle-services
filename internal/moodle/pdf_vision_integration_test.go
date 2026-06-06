package moodle

import (
	"os"
	"strings"
	"testing"
)

func TestExtractPDFTextWithCodexVisionIntegration(t *testing.T) {
	if strings.TrimSpace(os.Getenv("MOODLE_PDF_VISION_CODEX_INTEGRATION")) == "" {
		t.Skip("set MOODLE_PDF_VISION_CODEX_INTEGRATION=1 to run Codex vision extraction")
	}
	path := strings.TrimSpace(os.Getenv("MOODLE_PDF_VISION_TEST_FILE"))
	if path == "" {
		t.Skip("set MOODLE_PDF_VISION_TEST_FILE to run vision extraction against a real PDF")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read test PDF: %v", err)
	}
	text, err := ExtractPDFTextWithOptions(data, PDFTextExtractionOptions{
		UseVision:      true,
		VisionModel:    "gpt-5.4-mini",
		VisionMaxPages: 1,
	})
	if err != nil {
		t.Fatalf("vision extraction failed: %v", err)
	}

	for _, expected := range []string{
		"moodle list courses",
		"curl -X POST",
		"Semesterinformation",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected vision text to contain %q, got:\n%s", expected, text)
		}
	}
}
