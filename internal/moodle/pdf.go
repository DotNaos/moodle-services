package moodle

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	pdf "github.com/ledongthuc/pdf"
)

var reOCRPageSuffix = regexp.MustCompile(`-(\d+)\.png$`)

type PDFTextExtractionOptions struct {
	UseVision          bool
	VisionModel        string
	VisionMaxPages     int
	VisionDPI          int
	VisionCodexCommand string
}

func ExtractPDFText(data []byte) (string, error) {
	nativeText, nativeErr := extractPDFTextNative(data)
	nativeText = strings.TrimSpace(nativeText)

	if shouldAttemptOCR(nativeText, nativeErr) {
		ocrText, ocrErr := extractPDFTextOCR(data)
		ocrText = strings.TrimSpace(ocrText)

		if shouldPreferOCR(nativeText, nativeErr, ocrText, ocrErr) {
			return ocrText, nil
		}
		if nativeErr != nil {
			return "", fmt.Errorf("pdf text extraction failed (native + ocr): native=%w, ocr=%v", nativeErr, ocrErr)
		}
	}

	if nativeErr != nil {
		return "", nativeErr
	}
	return nativeText, nil
}

func ExtractPDFTextWithOptions(data []byte, options PDFTextExtractionOptions) (string, error) {
	if options.UseVision {
		return extractPDFTextWithCodexVision(data, options)
	}
	return ExtractPDFText(data)
}

func extractPDFTextNative(data []byte) (string, error) {
	if text, err := extractPDFTextWithPdftotext(data); err == nil && strings.TrimSpace(text) != "" {
		return text, nil
	}
	reader := bytes.NewReader(data)
	r, err := pdf.NewReader(reader, int64(len(data)))
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	b, err := r.GetPlainText()
	if err != nil {
		return "", err
	}
	if _, err := buf.ReadFrom(b); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func extractPDFTextWithPdftotext(data []byte) (string, error) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		return "", fmt.Errorf("pdftotext not found in PATH")
	}

	tempDir, err := os.MkdirTemp("", "moodle-services-pdftotext-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tempDir)

	pdfPath := filepath.Join(tempDir, "input.pdf")
	outPath := filepath.Join(tempDir, "output.txt")
	if err := os.WriteFile(pdfPath, data, 0o600); err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	_, stderr, err := runExternalCommand(ctx, "pdftotext", "-layout", pdfPath, outPath)
	if err != nil {
		return "", fmt.Errorf("pdftotext failed: %w (%s)", err, stderr)
	}
	output, err := os.ReadFile(outPath)
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func extractPDFTextOCR(data []byte) (string, error) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		return "", fmt.Errorf("ocr dependency missing: pdftoppm not found in PATH")
	}
	if _, err := exec.LookPath("tesseract"); err != nil {
		return "", fmt.Errorf("ocr dependency missing: tesseract not found in PATH")
	}

	tempDir, err := os.MkdirTemp("", "moodle-services-ocr-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tempDir)

	pdfPath := filepath.Join(tempDir, "input.pdf")
	if err := os.WriteFile(pdfPath, data, 0o600); err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	prefix := filepath.Join(tempDir, "page")
	_, stderr, err := runExternalCommand(ctx, "pdftoppm", "-png", "-r", "300", pdfPath, prefix)
	if err != nil {
		return "", fmt.Errorf("pdftoppm failed: %w (%s)", err, stderr)
	}

	pages, err := filepath.Glob(filepath.Join(tempDir, "page-*.png"))
	if err != nil {
		return "", err
	}
	if len(pages) == 0 {
		return "", errors.New("pdftoppm produced no page images")
	}

	sort.Slice(pages, func(i, j int) bool {
		return pageIndexFromPath(pages[i]) < pageIndexFromPath(pages[j])
	})

	lang := selectTesseractLanguage(runExternalCommand(ctx, "tesseract", "--list-langs"))

	var output strings.Builder
	for _, page := range pages {
		text, err := runTesseractOCR(ctx, page, lang)
		if err != nil {
			return "", err
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		if output.Len() > 0 {
			output.WriteString("\n\n")
		}
		output.WriteString(text)
	}

	result := strings.TrimSpace(output.String())
	if result == "" {
		return "", errors.New("ocr produced no text")
	}
	return result, nil
}

func runTesseractOCR(ctx context.Context, imagePath string, lang string) (string, error) {
	args := []string{imagePath, "stdout", "--oem", "1", "--psm", "3"}
	if lang != "" {
		args = append(args, "-l", lang)
	}
	text, stderr, err := runExternalCommand(ctx, "tesseract", args...)
	if err == nil {
		return text, nil
	}

	// Retry once with default language if the selected language is unavailable.
	if lang != "" {
		text, fallbackStderr, fallbackErr := runExternalCommand(ctx, "tesseract", imagePath, "stdout", "--oem", "1", "--psm", "3")
		if fallbackErr == nil {
			return text, nil
		}
		return "", fmt.Errorf("tesseract failed for %s: %w (%s; fallback: %s)", filepath.Base(imagePath), fallbackErr, stderr, fallbackStderr)
	}
	return "", fmt.Errorf("tesseract failed for %s: %w (%s)", filepath.Base(imagePath), err, stderr)
}

func runExternalCommand(ctx context.Context, name string, args ...string) (stdout string, stderr string, err error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var out bytes.Buffer
	var errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return out.String(), strings.TrimSpace(errBuf.String()), err
}

func shouldAttemptOCR(nativeText string, nativeErr error) bool {
	if nativeErr != nil {
		return true
	}
	nativeText = strings.TrimSpace(nativeText)
	if nativeText == "" {
		return true
	}
	if len(strings.Fields(nativeText)) < 40 {
		return true
	}
	return textQualityScore(nativeText) < 0.86
}

func shouldPreferOCR(nativeText string, nativeErr error, ocrText string, ocrErr error) bool {
	if ocrErr != nil || strings.TrimSpace(ocrText) == "" {
		return false
	}
	if nativeErr != nil || strings.TrimSpace(nativeText) == "" {
		return true
	}

	nativeScore := textQualityScore(nativeText)
	ocrScore := textQualityScore(ocrText)
	if ocrScore > nativeScore+0.08 {
		return true
	}
	return ocrScore >= nativeScore-0.02 && len(ocrText) > len(nativeText)*13/10
}

func textQualityScore(text string) float64 {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}

	var total int
	var useful int
	for _, r := range text {
		if unicode.IsSpace(r) {
			continue
		}
		total++
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune(".,;:!?()[]{}\"'`-_/\\@#%&*+=<>", r) {
			useful++
		}
	}
	if total == 0 {
		return 0
	}

	score := float64(useful) / float64(total)
	wordCount := len(strings.Fields(text))
	switch {
	case wordCount >= 200:
		score += 0.08
	case wordCount >= 80:
		score += 0.04
	case wordCount < 20:
		score -= 0.15
	}

	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

func selectTesseractLanguage(stdout string, _ string, err error) string {
	if err != nil {
		return ""
	}

	hasDE := false
	hasEN := false
	for _, line := range strings.Split(stdout, "\n") {
		switch strings.TrimSpace(line) {
		case "deu":
			hasDE = true
		case "eng":
			hasEN = true
		}
	}
	if hasDE && hasEN {
		return "deu+eng"
	}
	if hasDE {
		return "deu"
	}
	if hasEN {
		return "eng"
	}
	return ""
}

func pageIndexFromPath(path string) int {
	base := filepath.Base(path)
	m := reOCRPageSuffix.FindStringSubmatch(base)
	if len(m) < 2 {
		return 1 << 30
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 1 << 30
	}
	return n
}
