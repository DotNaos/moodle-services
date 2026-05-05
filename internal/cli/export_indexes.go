package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type exportSemesterIndex struct {
	Semester  string                 `yaml:"semester"`
	RunID     string                 `yaml:"run_id"`
	Courses   []exportCourseSummary  `yaml:"courses"`
	Calendar  *exportCalendarSummary `yaml:"calendar,omitempty"`
	Materials []exportMaterialRecord `yaml:"materials"`
}

type exportCalendarSummary struct {
	EventCount            int    `yaml:"event_count"`
	IndexRepoPath         string `yaml:"index_repo_path"`
	RawCurrentDriveLink   string `yaml:"raw_current_drive_link,omitempty"`
	SearchIndexDrivePath  string `yaml:"search_index_drive_path"`
	SearchEventsDrivePath string `yaml:"search_events_drive_path"`
}

type exportCourseSummary struct {
	Slug          string `yaml:"slug"`
	Name          string `yaml:"name"`
	CourseID      string `yaml:"course_id"`
	MaterialCount int    `yaml:"material_count"`
}

func uploadExportNavigation(ctx context.Context, uploader exportDriveUploader, run exportRunContext, courses []exportCourse, records []exportMaterialRecord, calendar exportCalendarIndex) error {
	root, err := uploader.EnsureFolderPath(ctx, nil)
	if err != nil {
		return err
	}
	searchFolder, err := uploader.EnsureFolderPath(ctx, []string{"_search"})
	if err != nil {
		return err
	}
	semesterFolder, err := uploader.EnsureFolderPath(ctx, []string{run.Semester})
	if err != nil {
		return err
	}
	currentFolder, err := uploader.EnsureFolderPath(ctx, []string{run.Semester, "current"})
	if err != nil {
		return err
	}

	if _, err := uploader.UploadText(ctx, rootREADME(), root.ID, "README.md", true); err != nil {
		return err
	}
	if _, err := uploader.UploadText(ctx, exportTutorial(), root.ID, "TUTORIAL.md", true); err != nil {
		return err
	}
	if _, err := uploader.UploadText(ctx, rootTOC(courses), root.ID, "TABLE_OF_CONTENTS.md", true); err != nil {
		return err
	}
	if _, err := uploader.UploadText(ctx, fmt.Sprintf("semester: %s\nlatest_run: %s\nstatus: complete\nupdated_at: %q\n", run.Semester, run.RunID, isoUTC(time.Now())), root.ID, "latest.yaml", true); err != nil {
		return err
	}
	if _, err := uploader.UploadText(ctx, searchREADME(), searchFolder.ID, "README.md", true); err != nil {
		return err
	}
	if err := uploadSearchIndexes(ctx, uploader, searchFolder.ID, records); err != nil {
		return err
	}
	if _, err := uploader.UploadText(ctx, semesterREADME(run.Semester, courses), semesterFolder.ID, "README.md", true); err != nil {
		return err
	}
	if _, err := uploader.UploadText(ctx, rootTOC(courses), semesterFolder.ID, "TABLE_OF_CONTENTS.md", true); err != nil {
		return err
	}
	if _, err := uploader.UploadText(ctx, currentREADME(run.Semester), currentFolder.ID, "README.md", true); err != nil {
		return err
	}
	if _, err := uploader.UploadText(ctx, semesterBriefing(run.Semester, run.RunID, courses, records, calendar), currentFolder.ID, "semester.briefing.md", true); err != nil {
		return err
	}
	index := buildSemesterIndex(run, courses, records, calendar)
	indexYAML, err := yaml.Marshal(index)
	if err != nil {
		return err
	}
	if _, err := uploader.UploadText(ctx, string(indexYAML), currentFolder.ID, "semester.index.yaml", true); err != nil {
		return err
	}
	if _, err := uploader.UploadText(ctx, semesterIndexMarkdown(index), currentFolder.ID, "semester.index.md", true); err != nil {
		return err
	}
	return nil
}

func uploadCourseCurrentDocs(ctx context.Context, uploader exportDriveUploader, folderID string, course exportCourse, records []exportMaterialRecord) error {
	if _, err := uploader.UploadText(ctx, courseREADME(course), folderID, "README.md", true); err != nil {
		return err
	}
	if _, err := uploader.UploadText(ctx, courseBriefing(course, records), folderID, "course.briefing.md", true); err != nil {
		return err
	}
	data, err := yaml.Marshal(map[string]any{
		"course_slug": course.Slug,
		"course_name": course.Title,
		"course_id":   fmt.Sprintf("%d", course.ID),
		"materials":   records,
	})
	if err != nil {
		return err
	}
	if _, err := uploader.UploadText(ctx, string(data), folderID, "course.index.yaml", true); err != nil {
		return err
	}
	if _, err := uploader.UploadText(ctx, courseMaterialsMarkdown(course, records), folderID, "materials.index.md", true); err != nil {
		return err
	}
	return nil
}

func uploadSearchIndexes(ctx context.Context, uploader exportDriveUploader, folderID string, records []exportMaterialRecord) error {
	sorted := append([]exportMaterialRecord(nil), records...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	yamlData, err := yaml.Marshal(sorted)
	if err != nil {
		return err
	}
	if _, err := uploader.UploadText(ctx, string(yamlData), folderID, "query.index.yaml", true); err != nil {
		return err
	}
	var jsonl strings.Builder
	for _, record := range sorted {
		line, err := json.Marshal(record)
		if err != nil {
			return err
		}
		jsonl.Write(line)
		jsonl.WriteByte('\n')
	}
	if _, err := uploader.UploadText(ctx, jsonl.String(), folderID, "query.index.jsonl", true); err != nil {
		return err
	}
	if _, err := uploader.UploadText(ctx, allMaterialsMarkdown(sorted), folderID, "all-materials.index.md", true); err != nil {
		return err
	}
	if _, err := uploader.UploadText(ctx, byCourseMarkdown(sorted), folderID, "by-course.md", true); err != nil {
		return err
	}
	if _, err := uploader.UploadText(ctx, byTypeMarkdown(sorted), folderID, "by-type.md", true); err != nil {
		return err
	}
	return nil
}

func buildSemesterIndex(run exportRunContext, courses []exportCourse, records []exportMaterialRecord, calendar exportCalendarIndex) exportSemesterIndex {
	counts := map[string]int{}
	for _, record := range records {
		counts[record.CourseSlug]++
	}
	summaries := make([]exportCourseSummary, 0, len(courses))
	for _, course := range courses {
		summaries = append(summaries, exportCourseSummary{
			Slug:          course.Slug,
			Name:          course.Title,
			CourseID:      fmt.Sprintf("%d", course.ID),
			MaterialCount: counts[course.Slug],
		})
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].Slug < summaries[j].Slug })
	index := exportSemesterIndex{Semester: run.Semester, RunID: run.RunID, Courses: summaries, Materials: records}
	if strings.TrimSpace(calendar.Semester) != "" {
		index.Calendar = &exportCalendarSummary{
			EventCount:            calendar.EventCount,
			IndexRepoPath:         filepath.ToSlash(filepath.Join(run.Semester, "calendar", "calendar.index.yaml")),
			RawCurrentDriveLink:   calendar.RawICS.CurrentDriveLink,
			SearchIndexDrivePath:  "_search/calendar.index.yaml",
			SearchEventsDrivePath: "_search/calendar.events.jsonl",
		}
	}
	return index
}

func rootREADME() string {
	return "# FHGR Moodle Export\n\nThis folder is the searchable export of FHGR Moodle course material and school calendar events.\n\nStart here:\n\n- `_search/query.index.yaml` for machine-readable material lookup.\n- `_search/calendar.index.yaml` for machine-readable calendar lookup.\n- `_search/all-materials.index.md` for a readable list of every exported material.\n- `FS26/current/` for the latest current-semester view.\n- `FS26/<run>/` for immutable export runs.\n\nThe `current/` folders are overwritten on each successful export. Timestamped run folders are append-only.\n"
}

func exportTutorial() string {
	return "# How To Use This Export\n\n## Find a topic\n\nOpen `_search/query.index.yaml` or `_search/all-materials.index.md` and search for a keyword, course name, file type, Moodle section, or material title.\n\n## Find a calendar event\n\nOpen `_search/calendar.index.yaml` or `FS26/current/calendar/calendar.index.md` and search for a course name, date, room, weekday, or event title.\n\n## Read a course\n\nOpen `FS26/current/<course>/README.md`, then `course.briefing.md`, then `materials.index.md`.\n\n## Inspect originals\n\nUse each course's `raw/` folder for original PDFs, slides, documents, and other files.\n\n## Read extracted text\n\nUse each course's `text/` folder for Markdown text extracted from readable resources.\n\n## Inspect visual content\n\nUse each course's `images/` folder for rendered PDF pages and slides. `thumbnails/` contains the first rendered page per material.\n\n## Verify freshness\n\nCheck `FS26/latest.yaml` and `FS26/current/semester.briefing.md` for the latest completed run.\n"
}

func rootTOC(courses []exportCourse) string {
	var b strings.Builder
	b.WriteString("# Table Of Contents\n\n## FS26\n\n")
	for _, course := range courses {
		b.WriteString("- ")
		b.WriteString(course.Title)
		b.WriteString(" (`")
		b.WriteString(course.Slug)
		b.WriteString("`)\n")
	}
	return b.String()
}

func searchREADME() string {
	return "# Search Indexes\n\nUse this folder when you need to find material or calendar events quickly.\n\n- `query.index.yaml`: full machine-readable material index with metadata, Drive links, text links, and image links.\n- `query.index.jsonl`: one material per line for external indexing tools.\n- `calendar.index.yaml`: machine-readable school calendar index.\n- `calendar.events.jsonl`: one calendar event per line for external indexing tools.\n- `all-materials.index.md`: readable table of all materials.\n- `by-course.md`: material list grouped by course.\n- `by-type.md`: material list grouped by file/material type.\n"
}

func semesterREADME(semester string, courses []exportCourse) string {
	return "# " + semester + "\n\nThis folder contains the current semester export and immutable timestamped runs.\n\n- `current/`: stable latest view for ChatGPT and humans.\n- `<timestamp-run-id>/`: immutable export history.\n- `latest.yaml`: pointer to the latest completed run.\n\n" + rootTOC(courses)
}

func currentREADME(semester string) string {
	return "# " + semester + " Current Export\n\nThis folder is overwritten after each successful export and is the best entry point for ChatGPT.\n\n- `semester.briefing.md`: short overview.\n- `semester.index.yaml`: machine-readable semester index.\n- `semester.index.md`: readable semester index.\n- `calendar/`: school calendar events for the semester.\n- `<course>/`: course-specific raw files, text, images, thumbnails, and indexes.\n"
}

func semesterBriefing(semester string, runID string, courses []exportCourse, records []exportMaterialRecord, calendar exportCalendarIndex) string {
	return fmt.Sprintf("# %s Briefing\n\nLatest run: `%s`\n\nCourses: %d\nMaterials: %d\nCalendar events: %d\n\nUse `_search/` for global lookup and this `current/` folder for the latest material and calendar.\n", semester, runID, len(courses), len(records), calendar.EventCount)
}

func courseREADME(course exportCourse) string {
	return "# " + course.Title + "\n\n- `course.briefing.md`: short course overview.\n- `materials.index.md`: readable list of exported materials.\n- `course.index.yaml`: machine-readable course index.\n- `raw/`: original files.\n- `text/`: extracted Markdown text.\n- `images/`: rendered pages and slides.\n- `thumbnails/`: first rendered page per material.\n"
}

func courseBriefing(course exportCourse, records []exportMaterialRecord) string {
	return fmt.Sprintf("# %s Briefing\n\nCourse slug: `%s`\nMoodle course ID: `%d`\nMaterials: %d\n\nUse `materials.index.md` for navigation, `text/` for reading, `images/` for visual inspection, and `raw/` for originals.\n", course.Title, course.Slug, course.ID, len(records))
}

func semesterIndexMarkdown(index exportSemesterIndex) string {
	return allMaterialsMarkdown(index.Materials)
}

func courseMaterialsMarkdown(course exportCourse, records []exportMaterialRecord) string {
	return "# Materials Index: " + course.Title + "\n\n" + materialTable(records)
}

func allMaterialsMarkdown(records []exportMaterialRecord) string {
	return "# All Materials Index\n\n" + materialTable(records)
}

func byCourseMarkdown(records []exportMaterialRecord) string {
	grouped := map[string][]exportMaterialRecord{}
	for _, record := range records {
		grouped[record.CourseSlug] = append(grouped[record.CourseSlug], record)
	}
	keys := sortedKeys(grouped)
	var b strings.Builder
	b.WriteString("# Materials By Course\n\n")
	for _, key := range keys {
		b.WriteString("## " + key + "\n\n")
		b.WriteString(materialTable(grouped[key]))
		b.WriteString("\n")
	}
	return b.String()
}

func byTypeMarkdown(records []exportMaterialRecord) string {
	grouped := map[string][]exportMaterialRecord{}
	for _, record := range records {
		grouped[record.Type] = append(grouped[record.Type], record)
	}
	keys := sortedKeys(grouped)
	var b strings.Builder
	b.WriteString("# Materials By Type\n\n")
	for _, key := range keys {
		b.WriteString("## " + key + "\n\n")
		b.WriteString(materialTable(grouped[key]))
		b.WriteString("\n")
	}
	return b.String()
}

func materialTable(records []exportMaterialRecord) string {
	var b strings.Builder
	b.WriteString("| Course | Section | Type | Title | Raw | Text | Images |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, record := range records {
		b.WriteString("| ")
		b.WriteString(escapeMD(record.CourseSlug))
		b.WriteString(" | ")
		b.WriteString(escapeMD(record.MoodleSection))
		b.WriteString(" | ")
		b.WriteString(escapeMD(record.Type))
		b.WriteString(" | ")
		b.WriteString(escapeMD(record.Title))
		b.WriteString(" | ")
		b.WriteString(mdLink("raw", record.RawCurrentLink))
		b.WriteString(" | ")
		b.WriteString(mdLink("text", record.TextCurrentLink))
		b.WriteString(" | ")
		b.WriteString(mdLink(fmt.Sprintf("%d images", record.ImageCount), record.ImagesCurrentLink))
		b.WriteString(" |\n")
	}
	return b.String()
}

func mdLink(label string, href string) string {
	if href == "" {
		return ""
	}
	return "[" + label + "](" + href + ")"
}

func escapeMD(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "|", "\\|")
	return value
}

func sortedKeys[T any](input map[string][]T) []string {
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
