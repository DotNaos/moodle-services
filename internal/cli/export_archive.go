package cli

import (
	"archive/zip"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type exportArchiveManifest struct {
	Semester       string                 `yaml:"semester" json:"semester"`
	RunID          string                 `yaml:"run_id" json:"run_id"`
	GeneratedAt    string                 `yaml:"generated_at" json:"generated_at"`
	Courses        []exportCourseSummary  `yaml:"courses" json:"courses"`
	Calendar       *exportCalendarSummary `yaml:"calendar,omitempty" json:"calendar,omitempty"`
	Materials      []exportMaterialRecord `yaml:"materials" json:"materials"`
	RawFileCount   int                    `yaml:"raw_file_count" json:"raw_file_count"`
	RawZipCount    int                    `yaml:"raw_zip_count" json:"raw_zip_count"`
	TextFileCount  int                    `yaml:"text_file_count" json:"text_file_count"`
	IndexFileCount int                    `yaml:"index_file_count" json:"index_file_count"`
}

type goodNotesZipIndexEntry struct {
	Section string
	Title   string
}

func resolveExportArchivePath(output string, run exportRunContext) (string, error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return "", fmt.Errorf("archive output path is empty")
	}
	if filepath.Ext(output) == ".zip" {
		return filepath.Abs(output)
	}
	filename := fmt.Sprintf("fhgr-moodle-%s-%s.zip", run.Semester, run.RunID)
	return filepath.Abs(filepath.Join(output, filename))
}

func normalizeExportArchiveProfile(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "full", nil
	}
	switch value {
	case "full", "goodnotes":
		return value, nil
	default:
		return "", fmt.Errorf("unsupported archive profile %q; use full or goodnotes", value)
	}
}

func writeExportArchive(run exportRunContext, courses []exportCourse, records []exportMaterialRecord, manifests []exportCourseManifest, calendar exportCalendarIndex, tempDir string, outputPath string, profile string) error {
	if outputPath == "" {
		return fmt.Errorf("archive output path is empty")
	}
	profile, err := normalizeExportArchiveProfile(profile)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}

	tempPath := outputPath + ".tmp"
	_ = os.Remove(tempPath)
	file, err := os.Create(tempPath)
	if err != nil {
		return err
	}

	writer := zip.NewWriter(file)
	archive := exportArchiveWriter{writer: writer, added: map[string]struct{}{}}

	if profile == "goodnotes" {
		if err := writeGoodNotesArchiveContents(&archive, run, courses, records, tempDir); err != nil {
			return closeFailedArchive(file, writer, tempPath, err)
		}
		return closeCompletedArchive(file, writer, tempPath, outputPath)
	}

	if err := archive.addText("README.md", exportArchiveREADME(run, len(courses), len(records), calendar.EventCount)); err != nil {
		return closeFailedArchive(file, writer, tempPath, err)
	}
	semesterIndex := buildSemesterIndex(run, courses, records, calendar)
	if err := archive.addYAML("manifest.yaml", buildExportArchiveManifest(run, semesterIndex, records, tempDir)); err != nil {
		return closeFailedArchive(file, writer, tempPath, err)
	}
	if err := archive.addYAML("semester.index.yaml", semesterIndex); err != nil {
		return closeFailedArchive(file, writer, tempPath, err)
	}
	if err := archive.addText("semester.index.md", semesterIndexMarkdown(semesterIndex)); err != nil {
		return closeFailedArchive(file, writer, tempPath, err)
	}
	if err := archive.addText("materials/all-materials.index.md", allMaterialsMarkdown(records)); err != nil {
		return closeFailedArchive(file, writer, tempPath, err)
	}
	if err := archive.addText("materials/by-course.md", byCourseMarkdown(records)); err != nil {
		return closeFailedArchive(file, writer, tempPath, err)
	}
	if err := archive.addText("materials/by-type.md", byTypeMarkdown(records)); err != nil {
		return closeFailedArchive(file, writer, tempPath, err)
	}
	if err := archive.addCourseFiles(courses); err != nil {
		return closeFailedArchive(file, writer, tempPath, err)
	}
	if err := archive.addRawCourseZips(tempDir, manifests); err != nil {
		return closeFailedArchive(file, writer, tempPath, err)
	}
	if err := archive.addRawFiles(tempDir); err != nil {
		return closeFailedArchive(file, writer, tempPath, err)
	}
	if err := archive.addCalendarFiles(run, tempDir); err != nil {
		return closeFailedArchive(file, writer, tempPath, err)
	}

	return closeCompletedArchive(file, writer, tempPath, outputPath)
}

type exportArchiveWriter struct {
	writer *zip.Writer
	added  map[string]struct{}
}

func (a *exportArchiveWriter) addText(name string, text string) error {
	entry, err := a.create(name)
	if err != nil {
		return err
	}
	_, err = io.WriteString(entry, text)
	return err
}

func (a *exportArchiveWriter) addYAML(name string, value any) error {
	text, err := yamlString(value)
	if err != nil {
		return err
	}
	return a.addText(name, text)
}

func (a *exportArchiveWriter) addFile(sourcePath string, archivePath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

	entry, err := a.create(archivePath)
	if err != nil {
		return err
	}
	_, err = io.Copy(entry, source)
	return err
}

func (a *exportArchiveWriter) create(name string) (io.Writer, error) {
	cleaned, err := safeArchivePath(name)
	if err != nil {
		return nil, err
	}
	if _, exists := a.added[cleaned]; exists {
		return nil, fmt.Errorf("duplicate archive path: %s", cleaned)
	}
	a.added[cleaned] = struct{}{}
	header := &zip.FileHeader{Name: cleaned, Method: zip.Deflate}
	header.SetModTime(time.Now().UTC())
	return a.writer.CreateHeader(header)
}

func (a *exportArchiveWriter) createUnique(name string) (io.Writer, error) {
	cleaned, err := safeArchivePath(name)
	if err != nil {
		return nil, err
	}
	candidate := cleaned
	for i := 2; ; i++ {
		if _, exists := a.added[candidate]; !exists {
			a.added[candidate] = struct{}{}
			header := &zip.FileHeader{Name: candidate, Method: zip.Deflate}
			header.SetModTime(time.Now().UTC())
			return a.writer.CreateHeader(header)
		}
		ext := filepath.Ext(cleaned)
		base := strings.TrimSuffix(cleaned, ext)
		candidate = fmt.Sprintf("%s %d%s", base, i, ext)
	}
}

func (a *exportArchiveWriter) addCourseFiles(courses []exportCourse) error {
	for _, course := range courses {
		for _, item := range []struct {
			source string
			target string
		}{
			{source: filepath.Join(course.Dir, "MOODLE.md"), target: "MOODLE.md"},
			{source: filepath.Join(course.Dir, "moodle-course.yaml"), target: "moodle-course.yaml"},
			{source: filepath.Join(course.Dir, "materials.index.yaml"), target: "materials.index.yaml"},
		} {
			if _, err := os.Stat(item.source); err == nil {
				if err := a.addFile(item.source, filepath.ToSlash(filepath.Join("courses", course.Slug, item.target))); err != nil {
					return err
				}
			} else if !os.IsNotExist(err) {
				return err
			}
		}
		textDir := filepath.Join(course.Dir, "materials-text")
		entries, err := os.ReadDir(textDir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
				continue
			}
			source := filepath.Join(textDir, entry.Name())
			target := filepath.ToSlash(filepath.Join("courses", course.Slug, "text", sanitizeArchiveFilename(entry.Name())))
			if err := a.addFile(source, target); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *exportArchiveWriter) addRawCourseZips(tempDir string, manifests []exportCourseManifest) error {
	for _, manifest := range manifests {
		name := strings.TrimSpace(manifest.RawZipFilename)
		if name == "" {
			continue
		}
		source := filepath.Join(tempDir, name)
		if _, err := os.Stat(source); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		target := filepath.ToSlash(filepath.Join("raw-course-zips", sanitizeArchiveFilename(name)))
		if err := a.addFile(source, target); err != nil {
			return err
		}
	}
	return nil
}

func (a *exportArchiveWriter) addRawFiles(tempDir string) error {
	rawRoot := filepath.Join(tempDir, "raw-files")
	if _, err := os.Stat(rawRoot); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var files []string
	if err := filepath.WalkDir(rawRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		files = append(files, path)
		return nil
	}); err != nil {
		return err
	}
	sort.Strings(files)
	for _, source := range files {
		rel, err := filepath.Rel(rawRoot, source)
		if err != nil {
			return err
		}
		relParts := strings.Split(filepath.ToSlash(rel), "/")
		if len(relParts) < 2 {
			continue
		}
		courseSlug := sanitizeArchiveFilename(relParts[0])
		parts := []string{"courses", courseSlug, "raw"}
		for _, part := range relParts[1:] {
			if part == "" {
				continue
			}
			parts = append(parts, sanitizeArchiveFilename(part))
		}
		if err := a.addFile(source, filepath.ToSlash(filepath.Join(parts...))); err != nil {
			return err
		}
	}
	return nil
}

func (a *exportArchiveWriter) addCalendarFiles(run exportRunContext, tempDir string) error {
	rawICS := filepath.Join(tempDir, "calendar.ics")
	if _, err := os.Stat(rawICS); err == nil {
		if err := a.addFile(rawICS, filepath.ToSlash(filepath.Join("calendar", "raw", "calendar.ics"))); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	calendarDir := filepath.Join(run.Workspace, run.Semester, "calendar")
	for _, name := range []string{"README.md", "calendar.index.yaml", "calendar.index.md"} {
		source := filepath.Join(calendarDir, name)
		if _, err := os.Stat(source); err == nil {
			if err := a.addFile(source, filepath.ToSlash(filepath.Join("calendar", name))); err != nil {
				return err
			}
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func writeGoodNotesArchiveContents(archive *exportArchiveWriter, run exportRunContext, courses []exportCourse, records []exportMaterialRecord, tempDir string) error {
	pdfRecords := goodNotesPDFRecords(records)
	if len(pdfRecords) == 0 {
		return archive.addGoodNotesCourseZips(run, courses, tempDir)
	}
	for _, record := range pdfRecords {
		source := filepath.Join(tempDir, "raw-files", record.CourseSlug, record.OriginalFilename)
		if _, err := os.Stat(source); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		section := strings.TrimSpace(record.MoodleSection)
		if section == "" {
			section = "Unsortiert"
		}
		if shouldSkipGoodNotesSection(section) {
			continue
		}
		target := filepath.ToSlash(filepath.Join(
			run.Semester,
			sanitizeGoodNotesComponent(record.CourseName),
			sanitizeGoodNotesComponent(section),
			goodNotesPDFName(record.Title),
		))
		if err := archive.addFile(source, target); err != nil {
			return err
		}
	}
	return nil
}

func (a *exportArchiveWriter) addGoodNotesCourseZips(run exportRunContext, courses []exportCourse, tempDir string) error {
	for _, course := range courses {
		zipPath := filepath.Join(tempDir, course.Slug+".zip")
		reader, err := zip.OpenReader(zipPath)
		if err != nil {
			return err
		}
		index := parseGoodNotesCourseZipIndex(reader.File)
		files := append([]*zip.File(nil), reader.File...)
		sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
		for _, file := range files {
			if file.FileInfo().IsDir() || !strings.EqualFold(filepath.Ext(file.Name), ".pdf") {
				continue
			}
			if err := a.addGoodNotesZipFile(run, course, file, index); err != nil {
				_ = reader.Close()
				return err
			}
		}
		if err := reader.Close(); err != nil {
			return err
		}
	}
	return nil
}

func (a *exportArchiveWriter) addGoodNotesZipFile(run exportRunContext, course exportCourse, file *zip.File, index map[string]goodNotesZipIndexEntry) error {
	targetParts, ok := goodNotesZipTargetParts(run, course, file.Name, index)
	if !ok {
		return nil
	}
	source, err := file.Open()
	if err != nil {
		return err
	}
	defer source.Close()
	entry, err := a.createUnique(filepath.ToSlash(filepath.Join(targetParts...)))
	if err != nil {
		return err
	}
	_, err = io.Copy(entry, source)
	return err
}

func goodNotesZipTargetParts(run exportRunContext, course exportCourse, rawName string, index map[string]goodNotesZipIndexEntry) ([]string, bool) {
	rawParts := strings.Split(filepath.ToSlash(rawName), "/")
	parts := make([]string, 0, len(rawParts))
	for _, part := range rawParts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		parts = append(parts, part)
	}
	if len(parts) > 1 && strings.HasPrefix(strings.ToLower(parts[0]), "kurs") {
		parts = parts[1:]
	}

	activityFolder := ""
	if len(parts) > 0 {
		activityFolder = parts[0]
	}
	meta := index[activityFolder]
	section := strings.TrimSpace(meta.Section)
	if section == "" {
		section = "Unsortiert"
	}
	if shouldSkipGoodNotesSection(section) {
		return nil, false
	}
	title := strings.TrimSpace(meta.Title)
	if title == "" {
		title = fallbackGoodNotesTitleFromZipPath(parts)
	}

	return []string{
		sanitizeGoodNotesComponent(run.Semester),
		sanitizeGoodNotesComponent(course.Title),
		sanitizeGoodNotesComponent(section),
		goodNotesPDFName(title),
	}, true
}

func countGoodNotesCourseZipPDFs(courses []exportCourse, tempDir string) (int, error) {
	count := 0
	for _, course := range courses {
		zipPath := filepath.Join(tempDir, course.Slug+".zip")
		reader, err := zip.OpenReader(zipPath)
		if err != nil {
			return 0, err
		}
		for _, file := range reader.File {
			if !file.FileInfo().IsDir() && strings.EqualFold(filepath.Ext(file.Name), ".pdf") {
				count++
			}
		}
		if err := reader.Close(); err != nil {
			return 0, err
		}
	}
	return count, nil
}

func goodNotesPDFRecords(records []exportMaterialRecord) []exportMaterialRecord {
	pdfs := make([]exportMaterialRecord, 0)
	for _, record := range records {
		if strings.EqualFold(record.Type, "pdf") || strings.EqualFold(filepath.Ext(record.OriginalFilename), ".pdf") {
			pdfs = append(pdfs, record)
		}
	}
	sort.Slice(pdfs, func(i, j int) bool {
		left := pdfs[i].CourseSlug + "/" + pdfs[i].MoodleSection + "/" + pdfs[i].OriginalFilename
		right := pdfs[j].CourseSlug + "/" + pdfs[j].MoodleSection + "/" + pdfs[j].OriginalFilename
		return left < right
	})
	return pdfs
}

func parseGoodNotesCourseZipIndex(files []*zip.File) map[string]goodNotesZipIndexEntry {
	mainIndex := ""
	for _, file := range files {
		if file.FileInfo().IsDir() || !strings.HasSuffix(file.Name, "/index.html") {
			continue
		}
		if strings.Count(strings.TrimSuffix(file.Name, "/index.html"), "/") == 0 {
			mainIndex = file.Name
			break
		}
	}
	if mainIndex == "" {
		return map[string]goodNotesZipIndexEntry{}
	}
	for _, file := range files {
		if file.Name != mainIndex {
			continue
		}
		source, err := file.Open()
		if err != nil {
			return map[string]goodNotesZipIndexEntry{}
		}
		data, err := io.ReadAll(io.LimitReader(source, 10*1024*1024))
		_ = source.Close()
		if err != nil {
			return map[string]goodNotesZipIndexEntry{}
		}
		return parseGoodNotesCourseIndexHTML(string(data))
	}
	return map[string]goodNotesZipIndexEntry{}
}

var goodNotesCourseIndexTokenPattern = regexp.MustCompile(`(?is)<h3[^>]*>(.*?)</h3>|<li>\s*<a\s+[^>]*href=["']([^"']+)["'][^>]*>(.*?)</a>\s*</li>`)

func parseGoodNotesCourseIndexHTML(content string) map[string]goodNotesZipIndexEntry {
	out := map[string]goodNotesZipIndexEntry{}
	section := "Allgemeine Informationen"
	for _, match := range goodNotesCourseIndexTokenPattern.FindAllStringSubmatch(content, -1) {
		if match[1] != "" {
			section = normalizeGoodNotesHTMLText(match[1])
			if section == "" {
				section = "Allgemeine Informationen"
			}
			continue
		}
		href := strings.TrimSpace(html.UnescapeString(match[2]))
		title := cleanGoodNotesActivityTitle(normalizeGoodNotesHTMLText(match[3]))
		folder := goodNotesActivityFolderFromHref(href)
		if folder == "" || title == "" {
			continue
		}
		out[folder] = goodNotesZipIndexEntry{Section: section, Title: title}
	}
	return out
}

func goodNotesActivityFolderFromHref(href string) string {
	href = strings.TrimPrefix(strings.TrimSpace(href), "./")
	href = strings.TrimSuffix(href, "/index.html")
	if strings.Contains(href, "/") || href == "" {
		return ""
	}
	return href
}

func cleanGoodNotesActivityTitle(title string) string {
	title = strings.TrimSpace(title)
	title = regexp.MustCompile(`\s+\((Datei|File|Link/URL|Forum|Text- und Medienfeld|Externes Tool)\)\s*$`).ReplaceAllString(title, "")
	return strings.TrimSpace(title)
}

func normalizeGoodNotesHTMLText(fragment string) string {
	text := regexp.MustCompile(`(?is)<[^>]+>`).ReplaceAllString(fragment, " ")
	text = html.UnescapeString(text)
	text = strings.ReplaceAll(text, "\u00a0", " ")
	return strings.Join(strings.Fields(text), " ")
}

func shouldSkipGoodNotesSection(section string) bool {
	normalized := strings.ToLower(sanitizeGoodNotesComponent(section))
	normalized = strings.ReplaceAll(normalized, "-", " ")
	normalized = strings.Join(strings.Fields(normalized), " ")
	return normalized == "allgemeine informationen" || normalized == "allgemeine information" || normalized == "general"
}

func fallbackGoodNotesTitleFromZipPath(parts []string) string {
	if len(parts) > 0 {
		title := parts[0]
		title = regexp.MustCompile(`\.\d+$`).ReplaceAllString(title, "")
		title = strings.TrimPrefix(title, "Datei_")
		title = strings.ReplaceAll(title, "_", " ")
		title = strings.ReplaceAll(title, "...", "")
		if strings.TrimSpace(title) != "" {
			return strings.TrimSpace(title)
		}
	}
	if len(parts) > 0 {
		return strings.TrimSpace(parts[len(parts)-1])
	}
	return "Datei"
}

func goodNotesPDFName(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Datei"
	}
	if strings.EqualFold(filepath.Ext(title), ".pdf") {
		title = strings.TrimSuffix(title, filepath.Ext(title))
	}
	return sanitizeGoodNotesComponent(title) + ".pdf"
}

func buildExportArchiveManifest(run exportRunContext, index exportSemesterIndex, records []exportMaterialRecord, tempDir string) exportArchiveManifest {
	return exportArchiveManifest{
		Semester:       run.Semester,
		RunID:          run.RunID,
		GeneratedAt:    isoUTC(time.Now()),
		Courses:        index.Courses,
		Calendar:       index.Calendar,
		Materials:      records,
		RawFileCount:   countFiles(filepath.Join(tempDir, "raw-files")),
		RawZipCount:    countFilesMatching(tempDir, ".zip"),
		TextFileCount:  countTextFiles(index.Courses, run.Workspace, run.Semester),
		IndexFileCount: len(index.Courses)*3 + 6,
	}
}

func countFiles(root string) int {
	count := 0
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			count++
		}
		return nil
	})
	return count
}

func countFilesMatching(root string, ext string) int {
	count := 0
	entries, err := os.ReadDir(root)
	if err != nil {
		return 0
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ext) {
			count++
		}
	}
	return count
}

func countTextFiles(courses []exportCourseSummary, workspace string, semester string) int {
	total := 0
	for _, course := range courses {
		total += countFiles(filepath.Join(workspace, semester, course.Slug, "materials-text"))
	}
	return total
}

func safeArchivePath(name string) (string, error) {
	cleaned := filepath.ToSlash(filepath.Clean(strings.TrimSpace(name)))
	if cleaned == "." || cleaned == "" {
		return "", fmt.Errorf("empty archive path")
	}
	if strings.HasPrefix(cleaned, "../") || cleaned == ".." || strings.HasPrefix(cleaned, "/") {
		return "", fmt.Errorf("unsafe archive path: %s", name)
	}
	return cleaned, nil
}

func sanitizeArchiveFilename(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	base := strings.TrimSuffix(name, filepath.Ext(name))
	slug := slugifyExportName(base)
	if slug == "" {
		slug = "file"
	}
	return slug + ext
}

func sanitizeArchiveFolderName(name string) string {
	slug := slugifyExportName(name)
	if slug == "" {
		return "folder"
	}
	return slug
}

func sanitizeGoodNotesComponent(name string) string {
	name = html.UnescapeString(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "\u00a0", " ")
	replacer := strings.NewReplacer(
		"/", "-",
		"\\", "-",
		":", " -",
		"*", "",
		"?", "",
		"\"", "'",
		"<", "(",
		">", ")",
		"|", "-",
	)
	name = replacer.Replace(name)
	name = strings.Join(strings.Fields(name), " ")
	name = strings.Trim(name, ". ")
	if name == "" {
		return "Untitled"
	}
	runes := []rune(name)
	if len(runes) > 120 {
		name = strings.TrimSpace(string(runes[:120]))
	}
	return name
}

func exportArchiveREADME(run exportRunContext, courseCount int, materialCount int, eventCount int) string {
	return fmt.Sprintf("# FHGR Moodle Offline Export\n\n"+
		"Semester: %s\n"+
		"Run: %s\n\n"+
		"This archive contains sanitized offline copies of the exported Moodle material.\n\n"+
		"- `courses/<course>/raw/`: original Moodle files with safe filenames.\n"+
		"- `courses/<course>/text/`: extracted Markdown text when available.\n"+
		"- `raw-course-zips/`: one Moodle course zip per exported course.\n"+
		"- `calendar/`: calendar index files and the raw ICS file.\n"+
		"- `materials/`: readable material indexes grouped by course and type.\n"+
		"- `manifest.yaml` and `semester.index.yaml`: machine-readable indexes.\n\n"+
		"Courses: %d\n"+
		"Materials: %d\n"+
		"Calendar events: %d\n",
		run.Semester, run.RunID, courseCount, materialCount, eventCount)
}

func closeCompletedArchive(file *os.File, writer *zip.Writer, tempPath string, outputPath string) error {
	if err := writer.Close(); err != nil {
		_ = file.Close()
		_ = os.Remove(tempPath)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	if err := os.Rename(tempPath, outputPath); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return nil
}

func closeFailedArchive(file *os.File, writer *zip.Writer, tempPath string, err error) error {
	_ = writer.Close()
	_ = file.Close()
	_ = os.Remove(tempPath)
	return err
}
