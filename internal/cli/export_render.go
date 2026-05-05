package cli

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/DotNaos/moodle-services/internal/moodle"
	"gopkg.in/yaml.v3"
)

type exportCourseManifest struct {
	Semester             string            `yaml:"semester" json:"semester"`
	CourseID             string            `yaml:"course_id" json:"courseId"`
	CourseSlug           string            `yaml:"course_slug" json:"courseSlug"`
	CourseName           string            `yaml:"course_name" json:"courseName"`
	RunID                string            `yaml:"run_id" json:"runId"`
	GoogleDriveFolderID  string            `yaml:"google_drive_folder_id" json:"googleDriveFolderId"`
	GoogleDriveFileIDs   []string          `yaml:"google_drive_file_ids" json:"googleDriveFileIds"`
	GoogleDriveLink      string            `yaml:"google_drive_link" json:"googleDriveLink"`
	RawZipFilename       string            `yaml:"raw_zip_filename" json:"rawZipFilename"`
	SHA256               string            `yaml:"sha256" json:"sha256"`
	ExportStatus         string            `yaml:"export_status" json:"exportStatus"`
	BackedUpAt           string            `yaml:"backed_up_at" json:"backedUpAt"`
	SourceMoodleMetadata map[string]string `yaml:"source_moodle_metadata" json:"sourceMoodleMetadata"`
}

type exportMaterialIndex struct {
	Semester             string                 `yaml:"semester"`
	CourseID             string                 `yaml:"course_id"`
	CourseSlug           string                 `yaml:"course_slug"`
	CourseName           string                 `yaml:"course_name"`
	RunID                string                 `yaml:"run_id"`
	RawZipFilename       string                 `yaml:"raw_zip_filename"`
	RawZipSHA256         string                 `yaml:"raw_zip_sha256"`
	RawGoogleDriveFileID string                 `yaml:"raw_google_drive_file_id"`
	RawGoogleDriveLink   string                 `yaml:"raw_google_drive_link"`
	Materials            []exportZipEntry       `yaml:"materials"`
	MoodleResources      []moodle.Resource      `yaml:"moodle_resources"`
	TextExtraction       []exportTextExtraction `yaml:"text_extraction"`
}

type exportZipEntry struct {
	Path           string `yaml:"path"`
	CompressedSize uint64 `yaml:"compressed_size"`
	Size           uint64 `yaml:"size"`
	CRC            string `yaml:"crc"`
}

type exportTextExtraction struct {
	ResourceID string `yaml:"resource_id"`
	Status     string `yaml:"status"`
	Path       string `yaml:"path,omitempty"`
	Error      string `yaml:"error,omitempty"`
}

var errExportUnsupportedTextResource = errors.New("unsupported non-text resource")

type exportRunYAML struct {
	Semester         string `yaml:"semester"`
	Run              string `yaml:"run"`
	GitHubRunID      string `yaml:"github_run_id"`
	GitHubRunAttempt string `yaml:"github_run_attempt"`
	Status           string `yaml:"status"`
	StartedAt        string `yaml:"started_at"`
	CompletedAt      string `yaml:"completed_at"`
}

type exportLatestYAML struct {
	Semester  string `yaml:"semester"`
	LatestRun string `yaml:"latest_run"`
	Status    string `yaml:"status"`
	UpdatedAt string `yaml:"updated_at"`
}

func ensureExportCourseFiles(course exportCourse) error {
	if err := os.MkdirAll(course.Dir, 0o755); err != nil {
		return err
	}
	readme := filepath.Join(course.Dir, "README.md")
	if _, err := os.Stat(readme); os.IsNotExist(err) {
		content := fmt.Sprintf("# %s\n\n- Moodle: [%s](%s)\n- Moodle snapshot: see `MOODLE.md` after running the sync.\n", course.Title, course.Fullname, course.ViewURL)
		if err := os.WriteFile(readme, []byte(content), 0o644); err != nil {
			return err
		}
	}
	tasks := filepath.Join(course.Dir, "TASKS.md")
	if _, err := os.Stat(tasks); os.IsNotExist(err) {
		content := fmt.Sprintf("# %s Tasks\n\n## Manual Tasks\n\n## Moodle Sync\n\n<!-- BEGIN MOODLE SYNC -->\n<!-- END MOODLE SYNC -->\n", course.Title)
		return os.WriteFile(tasks, []byte(content), 0o644)
	}
	return nil
}

func writeExportCourseSnapshot(course exportCourse, readerText string, resources []moodle.Resource) error {
	if strings.TrimSpace(readerText) == "" {
		readerText = "No readable course content found."
	}
	md := "# " + course.Title + "\n\n"
	if course.ViewURL != "" {
		md += "Source: " + course.ViewURL + "\n\n"
	}
	md += strings.TrimSpace(readerText) + "\n"
	if err := os.WriteFile(filepath.Join(course.Dir, "MOODLE.md"), []byte(md), 0o644); err != nil {
		return err
	}
	snapshot := map[string]any{
		"id":        course.ID,
		"title":     course.Title,
		"fullname":  course.Fullname,
		"shortname": course.Shortname,
		"category":  course.Category,
		"url":       course.ViewURL,
		"resources": resources,
	}
	data, err := yaml.Marshal(snapshot)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(course.Dir, "moodle-course.yaml"), data, 0o644)
}

func writeExportMaterialIndex(course exportCourse, run exportRunContext, zipPath string, zipSHA string, upload exportDriveFile, resources []moodle.Resource, textResults []exportTextExtraction) (exportMaterialIndex, error) {
	entries, err := readExportZipEntries(zipPath)
	if err != nil {
		return exportMaterialIndex{}, err
	}
	index := exportMaterialIndex{
		Semester:             run.Semester,
		CourseID:             fmt.Sprintf("%d", course.ID),
		CourseSlug:           course.Slug,
		CourseName:           course.Title,
		RunID:                run.RunID,
		RawZipFilename:       filepath.Base(zipPath),
		RawZipSHA256:         zipSHA,
		RawGoogleDriveFileID: upload.ID,
		RawGoogleDriveLink:   upload.WebViewLink,
		Materials:            entries,
		MoodleResources:      resources,
		TextExtraction:       textResults,
	}
	data, err := yaml.Marshal(index)
	if err != nil {
		return exportMaterialIndex{}, err
	}
	return index, os.WriteFile(filepath.Join(course.Dir, "materials.index.yaml"), data, 0o644)
}

func readExportZipEntries(path string) ([]exportZipEntry, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	entries := make([]exportZipEntry, 0, len(reader.File))
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		entries = append(entries, exportZipEntry{
			Path:           file.Name,
			CompressedSize: file.CompressedSize64,
			Size:           file.UncompressedSize64,
			CRC:            fmt.Sprintf("%08x", file.CRC32),
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

func sha256File(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func extractExportResourceTexts(client *moodle.Client, course exportCourse, resources []moodle.Resource) []exportTextExtraction {
	results := make([]exportTextExtraction, 0)
	textDir := filepath.Join(course.Dir, "materials-text")
	if err := resetExportTextDir(textDir); err != nil {
		return []exportTextExtraction{{Status: "failed", Error: err.Error()}}
	}
	for _, resource := range resources {
		if resource.Type != "resource" || strings.TrimSpace(resource.ID) == "" {
			continue
		}
		text, err := renderExportResourceText(client, resource)
		if err != nil {
			status := "failed"
			if errors.Is(err, errExportUnsupportedTextResource) {
				status = "skipped"
			}
			results = append(results, exportTextExtraction{ResourceID: resource.ID, Status: status, Error: err.Error()})
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" {
			results = append(results, exportTextExtraction{ResourceID: resource.ID, Status: "empty"})
			continue
		}
		if err := os.MkdirAll(textDir, 0o755); err != nil {
			results = append(results, exportTextExtraction{ResourceID: resource.ID, Status: "failed", Error: err.Error()})
			continue
		}
		filename := slugifyExportName(resource.Name)
		if filename == "" {
			filename = "resource-" + resource.ID
		}
		path := filepath.Join(textDir, filename+".md")
		content := "# " + resource.Name + "\n\n" + text + "\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			results = append(results, exportTextExtraction{ResourceID: resource.ID, Status: "failed", Error: err.Error()})
			continue
		}
		rel, _ := filepath.Rel(course.Dir, path)
		results = append(results, exportTextExtraction{ResourceID: resource.ID, Status: "ok", Path: filepath.ToSlash(rel)})
	}
	return results
}

func resetExportTextDir(textDir string) error {
	if strings.TrimSpace(textDir) == "" || textDir == string(filepath.Separator) {
		return fmt.Errorf("refusing to reset invalid materials text directory")
	}
	if err := os.RemoveAll(textDir); err != nil {
		return err
	}
	return nil
}

func renderExportResourceText(client *moodle.Client, resource moodle.Resource) (string, error) {
	result, err := client.DownloadFileToBuffer(resource.URL)
	if err != nil {
		return "", err
	}
	fileType := strings.ToLower(strings.TrimSpace(resource.FileType))
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(result.ContentType, ";")[0]))
	if fileType == "pdf" || strings.Contains(contentType, "pdf") {
		text, err := moodle.ExtractPDFText(result.Data)
		if err != nil {
			return "", err
		}
		return sanitizeExportText(cleanExtractedTextWithTimeout(text, 2*time.Second)), nil
	}
	if !isExportPlainTextResource(fileType, contentType, result.Data) {
		return "", fmt.Errorf("%w: %s (%s)", errExportUnsupportedTextResource, resource.Name, result.ContentType)
	}
	return sanitizeExportText(string(result.Data)), nil
}

func isExportPlainTextResource(fileType string, contentType string, data []byte) bool {
	switch strings.TrimPrefix(strings.ToLower(fileType), ".") {
	case "txt", "md", "markdown", "csv", "tsv", "log", "xml", "html", "htm", "json", "yaml", "yml":
		return looksLikeExportText(data)
	}
	switch {
	case strings.HasPrefix(contentType, "text/"):
		return looksLikeExportText(data)
	case contentType == "application/json",
		contentType == "application/xml",
		contentType == "application/yaml",
		contentType == "application/x-yaml",
		contentType == "application/xhtml+xml":
		return looksLikeExportText(data)
	default:
		return false
	}
}

func looksLikeExportText(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	if !utf8.Valid(data) {
		return false
	}
	sampleData := data
	if len(data) > 4096 {
		sampleData = data[:4096]
		for len(sampleData) > 0 && !utf8.Valid(sampleData) {
			sampleData = sampleData[:len(sampleData)-1]
		}
	}
	sample := string(sampleData)
	var total int
	var suspicious int
	for _, r := range sample {
		total++
		if r == 0 {
			return false
		}
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			suspicious++
		}
	}
	if total == 0 {
		return true
	}
	return suspicious*100/total < 5
}

func sanitizeExportText(input string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return ' '
		}
		return r
	}, input)
}

func renderExportReport(run exportRunContext, status string, manifests []exportCourseManifest, calendarEvents int, failures []string) string {
	lines := []string{
		"# FHGR Moodle Export Report: " + run.Semester,
		"",
		"- Run: `" + run.RunID + "`",
		"- Status: `" + status + "`",
		fmt.Sprintf("- Courses processed: %d", len(manifests)),
		fmt.Sprintf("- Calendar events exported: %d", calendarEvents),
		fmt.Sprintf("- Failures: %d", len(failures)),
		"",
		"## Courses",
		"",
	}
	if len(manifests) == 0 {
		lines = append(lines, "- None")
	}
	for _, item := range manifests {
		lines = append(lines, fmt.Sprintf("- %s (%s): `%s` sha256 `%s`", item.CourseSlug, item.CourseID, item.RawZipFilename, item.SHA256))
	}
	if len(failures) > 0 {
		lines = append(lines, "", "## Failures", "")
		for _, failure := range failures {
			lines = append(lines, "- "+failure)
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func yamlString(value any) (string, error) {
	data, err := yaml.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func isoUTC(t time.Time) string {
	return t.UTC().Truncate(time.Second).Format(time.RFC3339)
}
