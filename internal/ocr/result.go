package ocr

import (
	"encoding/json"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	StatusSuccess = "success"
	StatusFailed  = "failed"
)

func ParseOutput(provider Provider, outputDir string, exitCode int, timedOut bool, durationMs int64) RunResult {
	result := RunResult{
		Engine:       provider.ID,
		Status:       StatusSuccess,
		Images:       []ImageArtifact{},
		ArtifactsDir: outputDir,
		Warnings:     []string{},
		DurationMs:   durationMs,
		OutputFiles:  listOutputFiles(outputDir),
		ExitCode:     exitCode,
	}
	if exitCode != 0 || timedOut {
		result.Status = StatusFailed
	}
	if timedOut {
		result.Warnings = append(result.Warnings, "timeout reached")
	}
	if exitCode != 0 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("provider exited non-zero: %d", exitCode))
	}

	result.Markdown = readText(filepath.Join(outputDir, "output.md"))
	result.HTML = readText(filepath.Join(outputDir, "output.html"))
	result.Text = readText(filepath.Join(outputDir, "text.txt"))
	if rawJSON := readText(filepath.Join(outputDir, "output.json")); strings.TrimSpace(rawJSON) != "" {
		var parsed any
		if err := json.Unmarshal([]byte(rawJSON), &parsed); err == nil {
			result.JSON = parsed
		} else {
			result.Warnings = append(result.Warnings, "output.json could not be parsed")
		}
	}
	result.Images = listImages(filepath.Join(outputDir, "images"))
	result.Warnings = append(result.Warnings, DetectWarnings(provider, result)...)
	result.Warnings = uniqueStrings(result.Warnings)
	return result
}

func DetectWarnings(provider Provider, result RunResult) []string {
	var warnings []string
	markdown := strings.TrimSpace(result.Markdown)
	if markdown == "" {
		warnings = append(warnings, "missing output.md")
	} else if len([]rune(markdown)) < 80 {
		warnings = append(warnings, "empty or very short Markdown")
	}
	if provider.ExtractsImages && len(result.Images) == 0 {
		warnings = append(warnings, "no images extracted")
	}
	combined := strings.ToLower(strings.Join([]string{result.Markdown, result.HTML, result.Text}, "\n"))
	for _, marker := range []string{
		"formula not decoded",
		"[missing_ocr]",
		"ocr failed",
		"failed to parse",
		"placeholder text",
	} {
		if strings.Contains(combined, marker) {
			warnings = append(warnings, "output contains failed placeholder text")
			break
		}
	}
	return warnings
}

func readText(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func listImages(dir string) []ImageArtifact {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []ImageArtifact{}
	}
	images := []ImageArtifact{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		mimeType := mime.TypeByExtension(ext)
		switch ext {
		case ".png", ".jpg", ".jpeg", ".webp", ".gif", ".tif", ".tiff", ".bmp":
			images = append(images, ImageArtifact{Path: path, MimeType: mimeType})
		}
	}
	sort.Slice(images, func(i, j int) bool { return images[i].Path < images[j].Path })
	return images
}

func listOutputFiles(dir string) []string {
	files := []string{}
	_ = filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr == nil {
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(files)
	return files
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}
