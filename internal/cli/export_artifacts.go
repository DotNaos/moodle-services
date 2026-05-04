package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DotNaos/moodle-services/internal/moodle"
)

const defaultExportImageLimit = 80

type exportMaterialRecord struct {
	ID                 string             `yaml:"id" json:"id"`
	Semester           string             `yaml:"semester" json:"semester"`
	CourseSlug         string             `yaml:"course_slug" json:"course_slug"`
	CourseName         string             `yaml:"course_name" json:"course_name"`
	CourseID           string             `yaml:"course_id" json:"course_id"`
	ResourceID         string             `yaml:"resource_id" json:"resource_id"`
	Title              string             `yaml:"title" json:"title"`
	Type               string             `yaml:"type" json:"type"`
	Category           string             `yaml:"category" json:"category"`
	MoodleSection      string             `yaml:"moodle_section,omitempty" json:"moodle_section,omitempty"`
	MoodleURL          string             `yaml:"moodle_url,omitempty" json:"moodle_url,omitempty"`
	OriginalFilename   string             `yaml:"original_filename" json:"original_filename"`
	SHA256             string             `yaml:"sha256,omitempty" json:"sha256,omitempty"`
	SizeBytes          int64              `yaml:"size_bytes,omitempty" json:"size_bytes,omitempty"`
	RunID              string             `yaml:"run_id" json:"run_id"`
	RawRunDriveLink    string             `yaml:"raw_run_drive_link,omitempty" json:"raw_run_drive_link,omitempty"`
	RawCurrentLink     string             `yaml:"raw_current_drive_link,omitempty" json:"raw_current_drive_link,omitempty"`
	TextCurrentLink    string             `yaml:"text_current_drive_link,omitempty" json:"text_current_drive_link,omitempty"`
	ImagesCurrentLink  string             `yaml:"images_current_drive_link,omitempty" json:"images_current_drive_link,omitempty"`
	ThumbnailLink      string             `yaml:"thumbnail_drive_link,omitempty" json:"thumbnail_drive_link,omitempty"`
	TextRepoPath       string             `yaml:"text_repo_path,omitempty" json:"text_repo_path,omitempty"`
	ImageCount         int                `yaml:"image_count" json:"image_count"`
	ImageStatus        string             `yaml:"image_status,omitempty" json:"image_status,omitempty"`
	ImageError         string             `yaml:"image_error,omitempty" json:"image_error,omitempty"`
	Images             []exportImageEntry `yaml:"images,omitempty" json:"images,omitempty"`
	BackedUpAt         string             `yaml:"exported_at" json:"exported_at"`
	SourceMoodleFields map[string]string  `yaml:"source_moodle_metadata,omitempty" json:"source_moodle_metadata,omitempty"`
}

type exportImageEntry struct {
	Name      string `yaml:"name" json:"name"`
	DriveID   string `yaml:"drive_id,omitempty" json:"drive_id,omitempty"`
	DriveLink string `yaml:"drive_link,omitempty" json:"drive_link,omitempty"`
}

func exportCourseDriveArtifacts(ctx context.Context, client *moodle.Client, uploader exportDriveUploader, run exportRunContext, course exportCourse, resources []moodle.Resource, textResults []exportTextExtraction, tempDir string) ([]exportMaterialRecord, error) {
	courseID := fmt.Sprintf("%d", course.ID)
	runRawFolder, err := uploader.EnsureFolderPath(ctx, []string{run.Semester, "runs", run.RunID, "raw-files", course.Slug})
	if err != nil {
		return nil, err
	}
	currentCourseFolder, err := uploader.EnsureFolderPath(ctx, []string{run.Semester, "current", course.Slug})
	if err != nil {
		return nil, err
	}
	currentRawFolder, err := uploader.EnsureFolderPath(ctx, []string{run.Semester, "current", course.Slug, "raw"})
	if err != nil {
		return nil, err
	}
	currentTextFolder, err := uploader.EnsureFolderPath(ctx, []string{run.Semester, "current", course.Slug, "text"})
	if err != nil {
		return nil, err
	}
	currentImagesFolder, err := uploader.EnsureFolderPath(ctx, []string{run.Semester, "current", course.Slug, "images"})
	if err != nil {
		return nil, err
	}
	currentThumbsFolder, err := uploader.EnsureFolderPath(ctx, []string{run.Semester, "current", course.Slug, "thumbnails"})
	if err != nil {
		return nil, err
	}

	rawDir := filepath.Join(tempDir, "raw-files", course.Slug)
	imageRoot := filepath.Join(tempDir, "images", course.Slug)
	records := make([]exportMaterialRecord, 0)
	textByResource := map[string]string{}
	for _, result := range textResults {
		if result.Status == "ok" && result.Path != "" {
			textByResource[result.ResourceID] = result.Path
		}
	}

	for _, resource := range resources {
		if resource.Type != "resource" || strings.TrimSpace(resource.ID) == "" {
			continue
		}
		fmt.Fprintf(os.Stderr, "exporting Moodle material: %s/%s %s\n", course.Slug, resource.ID, resource.Name)
		filename := exportResourceFilename(resource)
		rawPath := filepath.Join(rawDir, filename)
		if err := os.MkdirAll(filepath.Dir(rawPath), 0o755); err != nil {
			return records, err
		}
		result, err := client.DownloadFileToBuffer(resource.URL)
		if err != nil {
			records = append(records, failedExportRecord(run, course, courseID, resource, "raw_download_failed", err))
			continue
		}
		if err := os.WriteFile(rawPath, result.Data, 0o644); err != nil {
			return records, err
		}
		sum := sha256.Sum256(result.Data)
		rawRun, err := uploader.UploadFile(ctx, rawPath, runRawFolder.ID, filename, false)
		if err != nil {
			records = append(records, failedExportRecord(run, course, courseID, resource, "raw_run_upload_failed", err))
			continue
		}
		rawCurrent, err := uploader.UploadFile(ctx, rawPath, currentRawFolder.ID, filename, true)
		if err != nil {
			records = append(records, failedExportRecord(run, course, courseID, resource, "raw_current_upload_failed", err))
			continue
		}

		record := exportMaterialRecord{
			ID:               run.Semester + "/" + course.Slug + "/" + resource.ID,
			Semester:         run.Semester,
			CourseSlug:       course.Slug,
			CourseName:       course.Title,
			CourseID:         courseID,
			ResourceID:       resource.ID,
			Title:            resource.Name,
			Type:             normalizedExportType(resource.FileType, filename),
			Category:         classifyExportMaterial(resource),
			MoodleSection:    resource.SectionName,
			MoodleURL:        resource.URL,
			OriginalFilename: filename,
			SHA256:           hex.EncodeToString(sum[:]),
			SizeBytes:        int64(len(result.Data)),
			RunID:            run.RunID,
			RawRunDriveLink:  rawRun.WebViewLink,
			RawCurrentLink:   rawCurrent.WebViewLink,
			ImageStatus:      "skipped",
			BackedUpAt:       isoUTC(time.Now()),
			SourceMoodleFields: map[string]string{
				"content_type": result.ContentType,
				"file_type":    resource.FileType,
			},
		}

		if textPath := textByResource[resource.ID]; textPath != "" {
			textUpload, err := uploader.UploadFile(ctx, filepath.Join(course.Dir, textPath), currentTextFolder.ID, filepath.Base(textPath), true)
			if err == nil {
				record.TextRepoPath = filepath.ToSlash(filepath.Join(course.Slug, textPath))
				record.TextCurrentLink = textUpload.WebViewLink
			} else {
				record.ImageError = appendExportError(record.ImageError, "text upload failed: "+err.Error())
			}
		}

		images, imageStatus, imageErr := renderExportImages(rawPath, record.Type, filepath.Join(imageRoot, slugifyExportName(resource.ID+"-"+resource.Name)))
		record.ImageStatus = imageStatus
		if imageErr != nil {
			record.ImageError = imageErr.Error()
		}
		if len(images) > 0 {
			materialImageFolder, err := uploader.EnsureFolderPath(ctx, []string{run.Semester, "current", course.Slug, "images", slugifyExportName(resource.ID + "-" + resource.Name)})
			if err != nil {
				record.ImageStatus = "image_folder_failed"
				record.ImageError = appendExportError(record.ImageError, err.Error())
			} else {
				record.ImagesCurrentLink = materialImageFolder.WebViewLink
				for idx, imagePath := range images {
					name := filepath.Base(imagePath)
					imageUpload, err := uploader.UploadFile(ctx, imagePath, materialImageFolder.ID, name, true)
					if err != nil {
						record.ImageStatus = "partial"
						record.ImageError = appendExportError(record.ImageError, "image upload failed: "+err.Error())
						break
					}
					record.Images = append(record.Images, exportImageEntry{Name: name, DriveID: imageUpload.ID, DriveLink: imageUpload.WebViewLink})
					if idx == 0 {
						thumb, err := uploader.UploadFile(ctx, imagePath, currentThumbsFolder.ID, slugifyExportName(resource.ID+"-"+resource.Name)+".png", true)
						if err != nil {
							record.ImageStatus = "partial"
							record.ImageError = appendExportError(record.ImageError, "thumbnail upload failed: "+err.Error())
							continue
						}
						record.ThumbnailLink = thumb.WebViewLink
					}
				}
				record.ImageCount = len(record.Images)
			}
		}
		records = append(records, record)
	}

	if err := uploadCourseCurrentDocs(ctx, uploader, currentCourseFolder.ID, course, records); err != nil {
		return records, err
	}
	_ = currentImagesFolder
	return records, nil
}

func appendExportError(existing string, next string) string {
	existing = strings.TrimSpace(existing)
	next = strings.TrimSpace(next)
	if existing == "" {
		return next
	}
	if next == "" {
		return existing
	}
	return existing + "; " + next
}

func failedExportRecord(run exportRunContext, course exportCourse, courseID string, resource moodle.Resource, status string, err error) exportMaterialRecord {
	return exportMaterialRecord{
		ID:            run.Semester + "/" + course.Slug + "/" + resource.ID,
		Semester:      run.Semester,
		CourseSlug:    course.Slug,
		CourseName:    course.Title,
		CourseID:      courseID,
		ResourceID:    resource.ID,
		Title:         resource.Name,
		Type:          normalizedExportType(resource.FileType, resource.Name),
		Category:      classifyExportMaterial(resource),
		MoodleSection: resource.SectionName,
		MoodleURL:     resource.URL,
		RunID:         run.RunID,
		ImageStatus:   status,
		ImageError:    err.Error(),
		BackedUpAt:    isoUTC(time.Now()),
	}
}

func exportResourceFilename(resource moodle.Resource) string {
	base := buildResourceFilename(resource)
	ext := filepath.Ext(base)
	slug := slugifyExportName(resource.ID + "-" + strings.TrimSuffix(base, ext))
	if slug == "" {
		slug = "resource-" + resource.ID
	}
	return slug + strings.ToLower(ext)
}

func normalizedExportType(fileType string, filename string) string {
	value := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(fileType)), ".")
	if value == "" {
		value = strings.TrimPrefix(strings.ToLower(filepath.Ext(filename)), ".")
	}
	if value == "" {
		return "unknown"
	}
	return value
}

func classifyExportMaterial(resource moodle.Resource) string {
	text := strings.ToLower(resource.Name + " " + resource.SectionName)
	switch {
	case strings.Contains(text, "aufgabe"), strings.Contains(text, "exercise"), strings.Contains(text, "worksheet"):
		return "exercise"
	case strings.Contains(text, "lösung"), strings.Contains(text, "loesung"), strings.Contains(text, "solution"):
		return "solution"
	case strings.Contains(text, "folie"), strings.Contains(text, "slide"), strings.Contains(text, "präsentation"), strings.Contains(text, "presentation"):
		return "slides"
	case strings.Contains(text, "skript"), strings.Contains(text, "script"), strings.Contains(text, "reading"):
		return "reading"
	default:
		return "material"
	}
}

func renderExportImages(rawPath string, fileType string, outDir string) ([]string, string, error) {
	kind := normalizedExportType(fileType, rawPath)
	switch kind {
	case "pdf":
		return renderPDFImages(rawPath, outDir)
	case "ppt", "pptx", "doc", "docx":
		pdfPath, err := convertOfficeToPDF(rawPath, outDir)
		if err != nil {
			return nil, "skipped", err
		}
		return renderPDFImages(pdfPath, outDir)
	default:
		return nil, "skipped", nil
	}
}

func renderPDFImages(pdfPath string, outDir string) ([]string, string, error) {
	pdftoppm, err := exec.LookPath("pdftoppm")
	if err != nil {
		return nil, "skipped", fmt.Errorf("pdftoppm not available")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, "failed", err
	}
	limit := exportImageLimit()
	prefix := filepath.Join(outDir, "page")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	args := []string{"-png", "-r", "144"}
	if limit > 0 {
		args = append(args, "-f", "1", "-l", strconv.Itoa(limit))
	}
	args = append(args, pdfPath, prefix)
	if output, err := exec.CommandContext(ctx, pdftoppm, args...).CombinedOutput(); err != nil {
		return nil, "failed", fmt.Errorf("pdftoppm failed: %s", strings.TrimSpace(string(output)))
	}
	images, err := filepath.Glob(prefix + "-*.png")
	if err != nil {
		return nil, "failed", err
	}
	sort.Strings(images)
	if len(images) == 0 {
		return nil, "empty", nil
	}
	return images, "ok", nil
}

func convertOfficeToPDF(rawPath string, outDir string) (string, error) {
	soffice, err := exec.LookPath("soffice")
	if err != nil {
		soffice, err = exec.LookPath("libreoffice")
		if err != nil {
			return "", fmt.Errorf("LibreOffice not available")
		}
	}
	convertDir := filepath.Join(outDir, "_converted")
	if err := os.MkdirAll(convertDir, 0o755); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	output, err := exec.CommandContext(ctx, soffice, "--headless", "--convert-to", "pdf", "--outdir", convertDir, rawPath).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("LibreOffice conversion failed: %s", strings.TrimSpace(string(output)))
	}
	pdfPath := filepath.Join(convertDir, strings.TrimSuffix(filepath.Base(rawPath), filepath.Ext(rawPath))+".pdf")
	if _, err := os.Stat(pdfPath); err != nil {
		return "", err
	}
	return pdfPath, nil
}

func exportImageLimit() int {
	raw := strings.TrimSpace(os.Getenv("MOODLE_EXPORT_MAX_IMAGES_PER_RESOURCE"))
	if raw == "" {
		return defaultExportImageLimit
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return defaultExportImageLimit
	}
	return value
}
