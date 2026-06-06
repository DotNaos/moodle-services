package moodle

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const defaultPDFVisionModel = "gpt-5.4-mini"

func extractPDFTextWithCodexVision(data []byte, options PDFTextExtractionOptions) (string, error) {
	model := strings.TrimSpace(options.VisionModel)
	if model == "" {
		model = strings.TrimSpace(os.Getenv("MOODLE_PDF_VISION_MODEL"))
	}
	if model == "" {
		model = defaultPDFVisionModel
	}

	pages, cleanup, err := renderPDFPagesForVision(data, options)
	if err != nil {
		return "", err
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), codexVisionRunTimeout(len(pages)))
	defer cancel()

	client, err := newCodexAppServerClient(ctx, options.VisionCodexCommand)
	if err != nil {
		return "", err
	}
	defer client.Close()

	if err := client.Initialize(ctx); err != nil {
		return "", err
	}
	threadID, err := client.StartOCRThread(ctx, model)
	if err != nil {
		return "", err
	}

	var output strings.Builder
	for i, page := range pages {
		pageText, err := client.ExtractTextFromImage(ctx, threadID, page)
		if err != nil {
			return "", fmt.Errorf("vision extraction failed for page %d: %w", i+1, err)
		}
		pageText = strings.TrimSpace(pageText)
		if pageText == "" {
			continue
		}
		if output.Len() > 0 {
			output.WriteString("\n\n")
		}
		output.WriteString(pageText)
	}
	text := strings.TrimSpace(output.String())
	if text == "" {
		return "", errors.New("vision extraction produced no text")
	}
	return text, nil
}

func renderPDFPagesForVision(data []byte, options PDFTextExtractionOptions) ([]string, func(), error) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		return nil, nil, fmt.Errorf("vision dependency missing: pdftoppm not found in PATH")
	}

	tempDir, err := os.MkdirTemp("", "moodle-services-pdf-vision-*")
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tempDir) }

	pdfPath := filepath.Join(tempDir, "input.pdf")
	if err := os.WriteFile(pdfPath, data, 0o600); err != nil {
		cleanup()
		return nil, nil, err
	}

	dpi := options.VisionDPI
	if dpi <= 0 {
		dpi = 180
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	prefix := filepath.Join(tempDir, "page")
	args := []string{"-png", "-r", strconv.Itoa(dpi)}
	if options.VisionMaxPages > 0 {
		args = append(args, "-f", "1", "-l", strconv.Itoa(options.VisionMaxPages))
	}
	args = append(args, pdfPath, prefix)
	_, stderr, err := runExternalCommand(ctx, "pdftoppm", args...)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("pdftoppm failed: %w (%s)", err, stderr)
	}

	pages, err := filepath.Glob(filepath.Join(tempDir, "page-*.png"))
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	if len(pages) == 0 {
		cleanup()
		return nil, nil, errors.New("pdftoppm produced no page images")
	}
	sort.Slice(pages, func(i, j int) bool {
		return pageIndexFromPath(pages[i]) < pageIndexFromPath(pages[j])
	})
	return pages, cleanup, nil
}

func codexVisionRunTimeout(pageCount int) time.Duration {
	if pageCount < 1 {
		pageCount = 1
	}
	return time.Duration(pageCount)*2*time.Minute + time.Minute
}
