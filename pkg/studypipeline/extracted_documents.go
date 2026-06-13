package studypipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DotNaos/moodle-services/internal/moodle"
	contract "github.com/DotNaos/moodle-services/pkg/apicontracts"
)

const extractedDocumentEngine = "baseline-pdftotext-pdftoppm"

var (
	ErrInvalidExtractedAssetPath = errors.New("invalid extracted asset path")
	ErrExtractedAssetNotFound    = errors.New("extracted asset not found")
)

var (
	pagePreviewRe     = regexp.MustCompile(`-(\d+)\.png$`)
	numberedListRe    = regexp.MustCompile(`^\d+[\.)]\s+`)
	mathLikeRe        = regexp.MustCompile(`(?i)(\\\(|\\\[|\$[^$]+\$|∑|∫|√|≈|≤|≥|=)`)
	blankLineSplitter = regexp.MustCompile(`\n\s*\n+`)
)

func LoadExtractedDocuments(courseID string, resources []moodle.Resource, options RunOptions) (contract.ExtractedDocumentsResponse, error) {
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}
	root := strings.TrimSpace(options.Root)
	if root == "" {
		root = ArtifactRootFromEnv()
	}
	if cached, ok, err := readLatestExtractedDocuments(root, courseID); err != nil {
		return contract.ExtractedDocumentsResponse{}, err
	} else if ok {
		return cached, nil
	}
	if !hasExtractedArtifacts(root, courseID) {
		if err := writeRaw(root, courseID, resources, options.Downloader); err != nil {
			return contract.ExtractedDocumentsResponse{}, err
		}
		if err := writeExtracted(root, courseID, resources, options.Downloader); err != nil {
			return contract.ExtractedDocumentsResponse{}, err
		}
	}
	return writeExtractedDocumentRun(root, courseID, resources, now)
}

func readLatestExtractedDocuments(root string, courseID string) (contract.ExtractedDocumentsResponse, bool, error) {
	path := filepath.Join(courseDir(root, courseID), "extracted", "latest-documents.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return contract.ExtractedDocumentsResponse{}, false, nil
		}
		return contract.ExtractedDocumentsResponse{}, false, err
	}
	var response contract.ExtractedDocumentsResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return contract.ExtractedDocumentsResponse{}, false, err
	}
	return response, true, nil
}

func OpenExtractedAsset(courseID string, assetPath string, options RunOptions) ([]byte, string, error) {
	root := strings.TrimSpace(options.Root)
	if root == "" {
		root = ArtifactRootFromEnv()
	}
	courseRoot, err := filepath.Abs(CourseArtifactRoot(root, courseID))
	if err != nil {
		return nil, "", err
	}
	assetPath = strings.TrimSpace(assetPath)
	if assetPath == "" {
		return nil, "", ErrInvalidExtractedAssetPath
	}
	if !filepath.IsAbs(assetPath) {
		assetPath = filepath.Join(courseRoot, filepath.FromSlash(assetPath))
	}
	resolved, err := filepath.Abs(assetPath)
	if err != nil {
		return nil, "", err
	}
	if resolved != courseRoot && !strings.HasPrefix(resolved, courseRoot+string(filepath.Separator)) {
		return nil, "", ErrInvalidExtractedAssetPath
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", ErrExtractedAssetNotFound
		}
		return nil, "", err
	}
	contentType := mime.TypeByExtension(filepath.Ext(resolved))
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	return data, contentType, nil
}

func writeExtractedDocumentRun(root string, courseID string, resources []moodle.Resource, now time.Time) (contract.ExtractedDocumentsResponse, error) {
	if now.IsZero() {
		now = time.Now()
	}
	runID := "baseline-" + now.UTC().Format("20060102T150405Z")
	runDir := filepath.Join(courseDir(root, courseID), "extracted", "runs", runID)
	if err := os.MkdirAll(filepath.Join(runDir, "documents"), 0o755); err != nil {
		return contract.ExtractedDocumentsResponse{}, err
	}
	if err := os.MkdirAll(filepath.Join(runDir, "assets"), 0o755); err != nil {
		return contract.ExtractedDocumentsResponse{}, err
	}

	plan := Build(courseID, resources, "extracted-structured", now)
	resourcesByID := map[string]moodle.Resource{}
	for _, resource := range resources {
		resourcesByID[resource.ID] = resource
	}

	documents := make([]contract.PDFDocument, 0)
	for _, material := range plan.Materials {
		if !shouldStructureMaterial(material) {
			continue
		}
		resource := resourcesByID[material.ID]
		document, err := buildExtractedDocument(root, courseID, runID, runDir, material, resource)
		if err != nil {
			return contract.ExtractedDocumentsResponse{}, err
		}
		documents = append(documents, document)
		if err := writeJSONFile(filepath.Join(runDir, "documents", safeSegment(document.ID)+".json"), document); err != nil {
			return contract.ExtractedDocumentsResponse{}, err
		}
	}

	response := contract.ExtractedDocumentsResponse{
		CourseID:     courseID,
		RunID:        runID,
		GeneratedAt:  now.UTC().Format(time.RFC3339),
		Engine:       extractedDocumentEngine,
		ArtifactRoot: courseDir(root, courseID),
		Documents:    documents,
	}
	response.Summary, response.Diagnostics = summarizeExtractedDocuments(documents)
	if err := writeJSONFile(filepath.Join(runDir, "documents.json"), response); err != nil {
		return contract.ExtractedDocumentsResponse{}, err
	}
	if err := writeJSONFile(filepath.Join(courseDir(root, courseID), "extracted", "latest-documents.json"), response); err != nil {
		return contract.ExtractedDocumentsResponse{}, err
	}
	return response, nil
}

func shouldStructureMaterial(material contract.StudyPipelineMaterial) bool {
	switch material.Type {
	case "slide", "script", "task", "solution":
		return true
	default:
		return false
	}
}

func buildExtractedDocument(root string, courseID string, runID string, runDir string, material contract.StudyPipelineMaterial, resource moodle.Resource) (contract.PDFDocument, error) {
	rawPath := rawPathForResource(root, courseID, resource)
	extractedPath := extractedPathForMaterial(root, courseID, material)
	content := extractedContentForMaterial(root, courseID, material)
	documentID := material.ID
	if strings.TrimSpace(documentID) == "" {
		documentID = safeSegment(material.Name)
	}
	assetDir := filepath.Join(runDir, "assets", safeSegment(documentID+"-"+material.Name))

	assets := make([]contract.DocumentAsset, 0)
	warnings := []string{}
	previewAssets, previewWarnings := renderPagePreviewAssets(rawPath, assetDir)
	assets = append(assets, previewAssets...)
	warnings = append(warnings, previewWarnings...)
	imageAssets, imageWarnings := extractEmbeddedImageAssets(rawPath, assetDir)
	assets = append(assets, imageAssets...)
	warnings = append(warnings, imageWarnings...)

	pages := buildDocumentPages(material, content, previewAssets)
	diagnostics := diagnosticsForDocument(pages, imageAssets, warnings)
	return contract.PDFDocument{
		ID:            documentID,
		Resource:      material,
		RunID:         runID,
		Engine:        extractedDocumentEngine,
		Status:        "machine-extracted",
		SourcePath:    filepath.ToSlash(rawPath),
		ExtractedPath: filepath.ToSlash(extractedPath),
		Pages:         pages,
		Assets:        assets,
		Diagnostics:   diagnostics,
	}, nil
}

func rawPathForResource(root string, courseID string, resource moodle.Resource) string {
	return filepath.Join(courseDir(root, courseID), "raw", "materials", safeSegment(resource.SectionName), resourceFileName(resource))
}

func renderPagePreviewAssets(rawPath string, assetDir string) ([]contract.DocumentAsset, []string) {
	if !strings.EqualFold(filepath.Ext(rawPath), ".pdf") || strings.TrimSpace(rawPath) == "" {
		return nil, nil
	}
	if _, err := os.Stat(rawPath); err != nil {
		return nil, []string{"raw PDF is not available for page preview rendering"}
	}
	pdftoppm, err := exec.LookPath("pdftoppm")
	if err != nil {
		return nil, []string{"pdftoppm is not available; page previews were skipped"}
	}
	outDir := filepath.Join(assetDir, "pages")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, []string{err.Error()}
	}
	prefix := filepath.Join(outDir, "page")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	output, err := exec.CommandContext(ctx, pdftoppm, "-png", "-r", "144", rawPath, prefix).CombinedOutput()
	if err != nil {
		return nil, []string{"pdftoppm failed: " + strings.TrimSpace(string(output))}
	}
	paths, err := filepath.Glob(prefix + "-*.png")
	if err != nil {
		return nil, []string{err.Error()}
	}
	sort.Slice(paths, func(i, j int) bool {
		return pageNumberFromPreviewPath(paths[i]) < pageNumberFromPreviewPath(paths[j])
	})
	assets := make([]contract.DocumentAsset, 0, len(paths))
	for _, path := range paths {
		pageNumber := pageNumberFromPreviewPath(path)
		assets = append(assets, contract.DocumentAsset{
			ID:         fmt.Sprintf("page-%03d-preview", pageNumber),
			Kind:       "page_preview",
			Path:       filepath.ToSlash(path),
			PageNumber: pageNumber,
			MimeType:   "image/png",
			Role:       "page_preview",
		})
	}
	return assets, nil
}

func extractEmbeddedImageAssets(rawPath string, assetDir string) ([]contract.DocumentAsset, []string) {
	if !strings.EqualFold(filepath.Ext(rawPath), ".pdf") || strings.TrimSpace(rawPath) == "" {
		return nil, nil
	}
	if _, err := os.Stat(rawPath); err != nil {
		return nil, nil
	}
	pdfimages, err := exec.LookPath("pdfimages")
	if err != nil {
		return nil, []string{"pdfimages is not available; embedded image extraction was skipped"}
	}
	outDir := filepath.Join(assetDir, "images")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, []string{err.Error()}
	}
	prefix := filepath.Join(outDir, "image")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	output, err := exec.CommandContext(ctx, pdfimages, "-png", rawPath, prefix).CombinedOutput()
	if err != nil {
		return nil, []string{"pdfimages failed: " + strings.TrimSpace(string(output))}
	}
	paths, err := filepath.Glob(prefix + "-*.png")
	if err != nil {
		return nil, []string{err.Error()}
	}
	sort.Strings(paths)
	assets := make([]contract.DocumentAsset, 0, len(paths))
	for index, path := range paths {
		assets = append(assets, contract.DocumentAsset{
			ID:       fmt.Sprintf("embedded-image-%03d", index+1),
			Kind:     "embedded_image",
			Path:     filepath.ToSlash(path),
			MimeType: "image/png",
			Role:     "extracted_image",
		})
	}
	return assets, nil
}

func pageNumberFromPreviewPath(path string) int {
	matches := pagePreviewRe.FindStringSubmatch(filepath.Base(path))
	if len(matches) != 2 {
		return 0
	}
	value, _ := strconv.Atoi(matches[1])
	return value
}

func buildDocumentPages(material contract.StudyPipelineMaterial, content string, previewAssets []contract.DocumentAsset) []contract.PDFPage {
	pageTexts := splitExtractedPages(content)
	if len(pageTexts) == 0 {
		pageTexts = []string{""}
	}
	for len(pageTexts) < len(previewAssets) {
		pageTexts = append(pageTexts, "")
	}
	pages := make([]contract.PDFPage, 0, len(pageTexts))
	for index, text := range pageTexts {
		pageNumber := index + 1
		previewAssetID := ""
		for _, asset := range previewAssets {
			if asset.PageNumber == pageNumber {
				previewAssetID = asset.ID
				break
			}
		}
		blocks := blocksForPage(material, pageNumber, text)
		page := contract.PDFPage{
			ID:             fmt.Sprintf("%s-page-%03d", material.ID, pageNumber),
			PageNumber:     pageNumber,
			Text:           strings.TrimSpace(text),
			Markdown:       strings.TrimSpace(text),
			PreviewAssetID: previewAssetID,
			Blocks:         blocks,
		}
		page.Diagnostics = diagnosticsForPage(page)
		pages = append(pages, page)
	}
	return pages
}

func splitExtractedPages(content string) []string {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	if strings.Contains(content, "\f") {
		parts := strings.Split(content, "\f")
		pages := make([]string, 0, len(parts))
		for _, part := range parts {
			pages = append(pages, strings.TrimSpace(part))
		}
		return pages
	}
	return []string{content}
}

func blocksForPage(material contract.StudyPipelineMaterial, pageNumber int, text string) []contract.DocumentBlock {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	parts := blankLineSplitter.Split(text, -1)
	blocks := make([]contract.DocumentBlock, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		blockType, label, confidence := classifyDocumentBlock(material, part)
		blockIndex := len(blocks) + 1
		blocks = append(blocks, contract.DocumentBlock{
			ID:         fmt.Sprintf("%s-p%03d-b%03d", material.ID, pageNumber, blockIndex),
			PageNumber: pageNumber,
			Type:       blockType,
			Label:      label,
			Text:       strings.TrimSpace(stripMarkdownMarkers(part)),
			Markdown:   part,
			Source:     "extracted_text",
			Confidence: confidence,
		})
	}
	return blocks
}

func classifyDocumentBlock(material contract.StudyPipelineMaterial, text string) (string, string, string) {
	lines := nonEmptyLines(text)
	if len(lines) == 0 {
		return "unknown", "empty", "low"
	}
	if strings.HasPrefix(lines[0], "```") || allLinesIndented(lines) {
		return "code", semanticLabel(material, "code"), "medium"
	}
	if len(lines) > 1 && strings.Contains(text, "|") {
		return "table", semanticLabel(material, "table"), "medium"
	}
	if allListLines(lines) {
		return "list", semanticLabel(material, "list"), "high"
	}
	if mathLikeRe.MatchString(text) && len([]rune(text)) < 240 {
		return "formula", semanticLabel(material, "formula"), "medium"
	}
	if strings.HasPrefix(lines[0], "#") || looksLikeHeading(lines) {
		return "heading", semanticLabel(material, "heading"), "medium"
	}
	return "paragraph", semanticLabel(material, "paragraph"), "high"
}

func semanticLabel(material contract.StudyPipelineMaterial, fallback string) string {
	switch material.Type {
	case "task":
		return "task_" + fallback
	case "solution":
		return "solution_" + fallback
	case "slide", "script":
		return "lecture_" + fallback
	default:
		return fallback
	}
}

func nonEmptyLines(text string) []string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func allLinesIndented(lines []string) bool {
	if len(lines) == 0 {
		return false
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "\t") {
			return false
		}
	}
	return true
}

func allListLines(lines []string) bool {
	if len(lines) == 0 {
		return false
	}
	for _, line := range lines {
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || numberedListRe.MatchString(line) {
			continue
		}
		return false
	}
	return true
}

func looksLikeHeading(lines []string) bool {
	if len(lines) != 1 {
		return false
	}
	line := strings.TrimSpace(strings.TrimPrefix(lines[0], "#"))
	if len([]rune(line)) > 120 {
		return false
	}
	if strings.HasSuffix(line, ".") || strings.HasSuffix(line, ",") || strings.HasSuffix(line, ";") {
		return false
	}
	return len(strings.Fields(line)) <= 12
}

func stripMarkdownMarkers(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "#")
		line = strings.TrimPrefix(line, "- ")
		line = strings.TrimPrefix(line, "* ")
		lines[i] = strings.TrimSpace(line)
	}
	return strings.Join(lines, "\n")
}

func diagnosticsForPage(page contract.PDFPage) contract.ExtractedDocumentDiagnostics {
	diagnostics := contract.ExtractedDocumentDiagnostics{}
	if strings.TrimSpace(page.Text) == "" {
		diagnostics.PagesMissingText = []int{page.PageNumber}
		if page.PreviewAssetID != "" {
			diagnostics.VisualOnlyPages = []int{page.PageNumber}
		}
	}
	for _, block := range page.Blocks {
		if block.Type == "unknown" {
			diagnostics.UnknownBlocks = append(diagnostics.UnknownBlocks, block.ID)
		}
	}
	return diagnostics
}

func diagnosticsForDocument(pages []contract.PDFPage, imageAssets []contract.DocumentAsset, warnings []string) contract.ExtractedDocumentDiagnostics {
	diagnostics := contract.ExtractedDocumentDiagnostics{
		ExtractedImageAssets: len(imageAssets),
		Warnings:             compactStrings(warnings),
	}
	for _, page := range pages {
		diagnostics.PagesMissingText = append(diagnostics.PagesMissingText, page.Diagnostics.PagesMissingText...)
		diagnostics.VisualOnlyPages = append(diagnostics.VisualOnlyPages, page.Diagnostics.VisualOnlyPages...)
		diagnostics.UnknownBlocks = append(diagnostics.UnknownBlocks, page.Diagnostics.UnknownBlocks...)
	}
	for _, asset := range imageAssets {
		diagnostics.UnusedImageAssets = append(diagnostics.UnusedImageAssets, asset.ID)
	}
	return diagnostics
}

func summarizeExtractedDocuments(documents []contract.PDFDocument) (contract.ExtractedDocumentsSummary, contract.ExtractedDocumentDiagnostics) {
	summary := contract.ExtractedDocumentsSummary{TotalDocuments: len(documents)}
	diagnostics := contract.ExtractedDocumentDiagnostics{}
	for _, document := range documents {
		summary.TotalPages += len(document.Pages)
		summary.EmbeddedImageAssets += document.Diagnostics.ExtractedImageAssets
		diagnostics.PagesMissingText = append(diagnostics.PagesMissingText, document.Diagnostics.PagesMissingText...)
		diagnostics.VisualOnlyPages = append(diagnostics.VisualOnlyPages, document.Diagnostics.VisualOnlyPages...)
		diagnostics.UnknownBlocks = append(diagnostics.UnknownBlocks, document.Diagnostics.UnknownBlocks...)
		diagnostics.UnusedImageAssets = append(diagnostics.UnusedImageAssets, document.Diagnostics.UnusedImageAssets...)
		diagnostics.Warnings = append(diagnostics.Warnings, document.Diagnostics.Warnings...)
		for _, asset := range document.Assets {
			if asset.Kind == "page_preview" {
				summary.PagePreviewAssets++
			}
		}
		for _, page := range document.Pages {
			summary.TotalBlocks += len(page.Blocks)
		}
	}
	summary.PagesMissingText = len(diagnostics.PagesMissingText)
	summary.VisualOnlyPages = len(diagnostics.VisualOnlyPages)
	summary.UnknownBlocks = len(diagnostics.UnknownBlocks)
	diagnostics.ExtractedImageAssets = summary.EmbeddedImageAssets
	diagnostics.Warnings = compactStrings(diagnostics.Warnings)
	return summary, diagnostics
}

func compactStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
