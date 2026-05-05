package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DotNaos/moodle-services/internal/moodle"
	"gopkg.in/yaml.v3"
)

type exportCalendarResult struct {
	Index exportCalendarIndex
	Raw   exportCalendarRaw
}

type exportCalendarRaw struct {
	Filename         string `yaml:"filename" json:"filename"`
	SHA256           string `yaml:"sha256" json:"sha256"`
	SizeBytes        int64  `yaml:"size_bytes" json:"size_bytes"`
	RunDriveID       string `yaml:"run_drive_id,omitempty" json:"run_drive_id,omitempty"`
	RunDriveLink     string `yaml:"run_drive_link,omitempty" json:"run_drive_link,omitempty"`
	CurrentDriveID   string `yaml:"current_drive_id,omitempty" json:"current_drive_id,omitempty"`
	CurrentDriveLink string `yaml:"current_drive_link,omitempty" json:"current_drive_link,omitempty"`
}

type exportCalendarIndex struct {
	Semester    string                 `yaml:"semester" json:"semester"`
	RunID       string                 `yaml:"run_id" json:"run_id"`
	Timezone    string                 `yaml:"timezone" json:"timezone"`
	WindowStart string                 `yaml:"window_start" json:"window_start"`
	WindowEnd   string                 `yaml:"window_end" json:"window_end"`
	EventCount  int                    `yaml:"event_count" json:"event_count"`
	RawICS      exportCalendarRaw      `yaml:"raw_ics" json:"raw_ics"`
	Events      []exportCalendarRecord `yaml:"events" json:"events"`
	GeneratedAt string                 `yaml:"generated_at" json:"generated_at"`
}

type exportCalendarRecord struct {
	ID             string `yaml:"id" json:"id"`
	Semester       string `yaml:"semester" json:"semester"`
	RunID          string `yaml:"run_id" json:"run_id"`
	UID            string `yaml:"uid,omitempty" json:"uid,omitempty"`
	Title          string `yaml:"title" json:"title"`
	Description    string `yaml:"description,omitempty" json:"description,omitempty"`
	Location       string `yaml:"location,omitempty" json:"location,omitempty"`
	Start          string `yaml:"start" json:"start"`
	End            string `yaml:"end" json:"end"`
	Date           string `yaml:"date" json:"date"`
	Weekday        string `yaml:"weekday" json:"weekday"`
	CourseSlug     string `yaml:"course_slug,omitempty" json:"course_slug,omitempty"`
	CourseName     string `yaml:"course_name,omitempty" json:"course_name,omitempty"`
	CourseID       string `yaml:"course_id,omitempty" json:"course_id,omitempty"`
	Category       string `yaml:"category" json:"category"`
	SearchText     string `yaml:"search_text" json:"search_text"`
	SourceCalendar string `yaml:"source_calendar" json:"source_calendar"`
}

func exportCalendarRun(ctx context.Context, uploader exportDriveUploader, run exportRunContext, cfg schoolExportConfig, courses []exportCourse, calendarURL string, tempDir string) (exportCalendarResult, error) {
	calendarURL = strings.TrimSpace(calendarURL)
	if calendarURL == "" {
		return exportCalendarResult{}, fmt.Errorf("calendar URL not set. Run: moodle config set --calendar-url <url>")
	}

	location := exportTimezone(cfg)
	from, to := exportSemesterWindow(run.Semester, location)
	data, err := moodle.FetchCalendarData(calendarURL)
	if err != nil {
		return exportCalendarResult{}, err
	}
	events, err := moodle.ParseCalendarEvents(data, from, to)
	if err != nil {
		return exportCalendarResult{}, err
	}
	sort.Slice(events, func(i, j int) bool {
		return events[i].Start.Before(events[j].Start)
	})

	calendarDir := filepath.Join(run.Workspace, run.Semester, "calendar")
	if err := os.MkdirAll(calendarDir, 0o755); err != nil {
		return exportCalendarResult{}, err
	}
	rawPath := filepath.Join(tempDir, "calendar.ics")
	if err := os.WriteFile(rawPath, data, 0o600); err != nil {
		return exportCalendarResult{}, err
	}
	sum := sha256.Sum256(data)
	raw := exportCalendarRaw{
		Filename:  filepath.Base(rawPath),
		SHA256:    hex.EncodeToString(sum[:]),
		SizeBytes: int64(len(data)),
	}

	runRawFolder, err := uploader.EnsureFolderPath(ctx, []string{run.Semester, "runs", run.RunID, "raw", "calendar"})
	if err != nil {
		return exportCalendarResult{}, err
	}
	currentRawFolder, err := uploader.EnsureFolderPath(ctx, []string{run.Semester, "current", "calendar", "raw"})
	if err != nil {
		return exportCalendarResult{}, err
	}
	runRaw, err := uploader.UploadFile(ctx, rawPath, runRawFolder.ID, raw.Filename, false)
	if err != nil {
		return exportCalendarResult{}, err
	}
	currentRaw, err := uploader.UploadFile(ctx, rawPath, currentRawFolder.ID, raw.Filename, true)
	if err != nil {
		return exportCalendarResult{}, err
	}
	raw.RunDriveID = runRaw.ID
	raw.RunDriveLink = runRaw.WebViewLink
	raw.CurrentDriveID = currentRaw.ID
	raw.CurrentDriveLink = currentRaw.WebViewLink

	records := exportCalendarRecords(run, events, courses, location)
	index := exportCalendarIndex{
		Semester:    run.Semester,
		RunID:       run.RunID,
		Timezone:    location.String(),
		WindowStart: from.Format(time.RFC3339),
		WindowEnd:   to.Format(time.RFC3339),
		EventCount:  len(records),
		RawICS:      raw,
		Events:      records,
		GeneratedAt: isoUTC(time.Now()),
	}

	if err := writeCalendarRepoFiles(calendarDir, index); err != nil {
		return exportCalendarResult{}, err
	}
	if err := uploadCalendarDriveFiles(ctx, uploader, run, index); err != nil {
		return exportCalendarResult{}, err
	}
	return exportCalendarResult{Index: index, Raw: raw}, nil
}

func exportTimezone(cfg schoolExportConfig) *time.Location {
	name := strings.TrimSpace(cfg.Timezone)
	if name == "" {
		name = "Europe/Zurich"
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return time.Local
	}
	return location
}

func exportSemesterWindow(semester string, location *time.Location) (time.Time, time.Time) {
	semester = strings.ToUpper(strings.TrimSpace(semester))
	if len(semester) != 4 {
		now := time.Now().In(location)
		return time.Date(now.Year(), time.January, 1, 0, 0, 0, 0, location), time.Date(now.Year()+1, time.January, 1, 0, 0, 0, 0, location)
	}
	year, err := strconv.Atoi("20" + semester[2:])
	if err != nil {
		now := time.Now().In(location)
		year = now.Year()
	}
	switch semester[:2] {
	case "FS":
		return time.Date(year, time.January, 1, 0, 0, 0, 0, location), time.Date(year, time.September, 1, 0, 0, 0, 0, location)
	case "HS":
		return time.Date(year, time.August, 1, 0, 0, 0, 0, location), time.Date(year+1, time.February, 1, 0, 0, 0, 0, location)
	default:
		return time.Date(year, time.January, 1, 0, 0, 0, 0, location), time.Date(year+1, time.January, 1, 0, 0, 0, 0, location)
	}
}

func exportCalendarRecords(run exportRunContext, events []moodle.CalendarEvent, courses []exportCourse, location *time.Location) []exportCalendarRecord {
	records := make([]exportCalendarRecord, 0, len(events))
	for index, event := range events {
		start := event.Start.In(location)
		end := event.End.In(location)
		course := matchCalendarCourse(event, courses)
		record := exportCalendarRecord{
			ID:             calendarRecordID(run.Semester, event, index),
			Semester:       run.Semester,
			RunID:          run.RunID,
			UID:            event.UID,
			Title:          strings.TrimSpace(event.Summary),
			Description:    strings.TrimSpace(event.Description),
			Location:       strings.TrimSpace(event.Location),
			Start:          start.Format(time.RFC3339),
			End:            end.Format(time.RFC3339),
			Date:           start.Format("2006-01-02"),
			Weekday:        start.Weekday().String(),
			Category:       classifyCalendarEvent(event),
			SourceCalendar: "FHGR calendar subscription",
		}
		if course != nil {
			record.CourseSlug = course.Slug
			record.CourseName = course.Title
			record.CourseID = fmt.Sprintf("%d", course.ID)
		}
		record.SearchText = strings.TrimSpace(strings.Join([]string{
			record.Title,
			record.Description,
			record.Location,
			record.CourseSlug,
			record.CourseName,
			record.Date,
			record.Weekday,
			record.Category,
		}, " "))
		records = append(records, record)
	}
	return records
}

func calendarRecordID(semester string, event moodle.CalendarEvent, index int) string {
	key := strings.TrimSpace(event.UID)
	if key == "" {
		key = fmt.Sprintf("%s-%s-%d", event.Start.Format("20060102T150405"), event.Summary, index)
	}
	return semester + "/calendar/" + slugifyExportName(key)
}

func matchCalendarCourse(event moodle.CalendarEvent, courses []exportCourse) *exportCourse {
	text := strings.ToLower(event.Summary + " " + event.Description)
	for i := range courses {
		title := strings.ToLower(courses[i].Title)
		slug := strings.ReplaceAll(strings.ToLower(courses[i].Slug), "-", " ")
		short := strings.ToLower(courses[i].Shortname)
		if title != "" && strings.Contains(text, title) {
			return &courses[i]
		}
		if slug != "" && strings.Contains(text, slug) {
			return &courses[i]
		}
		if short != "" && strings.Contains(text, short) {
			return &courses[i]
		}
	}
	return nil
}

func classifyCalendarEvent(event moodle.CalendarEvent) string {
	text := strings.ToLower(event.Summary + " " + event.Description)
	switch {
	case strings.Contains(text, "exam"), strings.Contains(text, "pruefung"), strings.Contains(text, "prüfung"):
		return "exam"
	case strings.Contains(text, "deadline"), strings.Contains(text, "abgabe"):
		return "deadline"
	default:
		return "lecture"
	}
}

func writeCalendarRepoFiles(dir string, index exportCalendarIndex) error {
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(calendarREADME(index)), 0o644); err != nil {
		return err
	}
	data, err := yaml.Marshal(index)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "calendar.index.yaml"), data, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "calendar.index.md"), []byte(calendarIndexMarkdown(index)), 0o644)
}

func uploadCalendarDriveFiles(ctx context.Context, uploader exportDriveUploader, run exportRunContext, index exportCalendarIndex) error {
	currentFolder, err := uploader.EnsureFolderPath(ctx, []string{run.Semester, "current", "calendar"})
	if err != nil {
		return err
	}
	searchFolder, err := uploader.EnsureFolderPath(ctx, []string{"_search"})
	if err != nil {
		return err
	}
	indexYAML, err := yaml.Marshal(index)
	if err != nil {
		return err
	}
	jsonl, err := calendarJSONL(index.Events)
	if err != nil {
		return err
	}
	if _, err := uploader.UploadText(ctx, calendarREADME(index), currentFolder.ID, "README.md", true); err != nil {
		return err
	}
	if _, err := uploader.UploadText(ctx, string(indexYAML), currentFolder.ID, "calendar.index.yaml", true); err != nil {
		return err
	}
	if _, err := uploader.UploadText(ctx, calendarIndexMarkdown(index), currentFolder.ID, "calendar.index.md", true); err != nil {
		return err
	}
	if _, err := uploader.UploadText(ctx, jsonl, currentFolder.ID, "calendar.events.jsonl", true); err != nil {
		return err
	}
	if _, err := uploader.UploadText(ctx, string(indexYAML), searchFolder.ID, "calendar.index.yaml", true); err != nil {
		return err
	}
	if _, err := uploader.UploadText(ctx, jsonl, searchFolder.ID, "calendar.events.jsonl", true); err != nil {
		return err
	}
	if _, err := uploader.UploadText(ctx, calendarIndexMarkdown(index), searchFolder.ID, "calendar.index.md", true); err != nil {
		return err
	}
	return nil
}

func calendarJSONL(records []exportCalendarRecord) (string, error) {
	var b strings.Builder
	for _, record := range records {
		line, err := json.Marshal(record)
		if err != nil {
			return "", err
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	return b.String(), nil
}

func calendarREADME(index exportCalendarIndex) string {
	return fmt.Sprintf("# Calendar Export\n\nThis folder contains the school calendar events for `%s`.\n\n- `calendar.index.yaml`: machine-readable event index with dates, locations, matching course names, and search text.\n- `calendar.index.md`: readable event list.\n- `calendar.events.jsonl`: one event per line for indexing tools on Google Drive.\n- `raw/calendar.ics`: original calendar subscription export on Google Drive only.\n\nWindow: `%s` to `%s`\nEvents: %d\n", index.Semester, index.WindowStart, index.WindowEnd, index.EventCount)
}

func calendarIndexMarkdown(index exportCalendarIndex) string {
	var b strings.Builder
	b.WriteString("# Calendar Index\n\n")
	b.WriteString(fmt.Sprintf("- Semester: `%s`\n", index.Semester))
	b.WriteString(fmt.Sprintf("- Run: `%s`\n", index.RunID))
	b.WriteString(fmt.Sprintf("- Events: `%d`\n", index.EventCount))
	b.WriteString(fmt.Sprintf("- Window: `%s` to `%s`\n\n", index.WindowStart, index.WindowEnd))
	b.WriteString("| Date | Time | Course | Title | Location |\n")
	b.WriteString("| --- | --- | --- | --- | --- |\n")
	for _, event := range index.Events {
		startTime := ""
		endTime := ""
		if start, err := time.Parse(time.RFC3339, event.Start); err == nil {
			startTime = start.Format("15:04")
		}
		if end, err := time.Parse(time.RFC3339, event.End); err == nil {
			endTime = end.Format("15:04")
		}
		b.WriteString("| ")
		b.WriteString(escapeMD(event.Date))
		b.WriteString(" | ")
		b.WriteString(escapeMD(strings.Trim(startTime+"-"+endTime, "-")))
		b.WriteString(" | ")
		b.WriteString(escapeMD(event.CourseSlug))
		b.WriteString(" | ")
		b.WriteString(escapeMD(event.Title))
		b.WriteString(" | ")
		b.WriteString(escapeMD(event.Location))
		b.WriteString(" |\n")
	}
	return b.String()
}
