package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DotNaos/moodle-services/internal/moodle"
)

func TestExportSemesterWindow(t *testing.T) {
	location := time.FixedZone("test", 3600)
	from, to := exportSemesterWindow("FS26", location)
	if got, want := from.Format("2006-01-02"), "2026-01-01"; got != want {
		t.Fatalf("FS start = %s, want %s", got, want)
	}
	if got, want := to.Format("2006-01-02"), "2026-09-01"; got != want {
		t.Fatalf("FS end = %s, want %s", got, want)
	}

	from, to = exportSemesterWindow("HS26", location)
	if got, want := from.Format("2006-01-02"), "2026-08-01"; got != want {
		t.Fatalf("HS start = %s, want %s", got, want)
	}
	if got, want := to.Format("2006-01-02"), "2027-02-01"; got != want {
		t.Fatalf("HS end = %s, want %s", got, want)
	}
}

func TestExportCalendarRecordsMatchCourses(t *testing.T) {
	location := time.FixedZone("test", 3600)
	run := exportRunContext{Semester: "FS26", RunID: "run-1"}
	courses := []exportCourse{{
		ID:        22585,
		Slug:      "deep-learning",
		Title:     "Deep Learning",
		Shortname: "DL",
	}}
	events := []moodle.CalendarEvent{{
		UID:      "event-1",
		Summary:  "Deep Learning",
		Location: "B1.03",
		Start:    time.Date(2026, 5, 15, 13, 30, 0, 0, location),
		End:      time.Date(2026, 5, 15, 15, 0, 0, 0, location),
	}}

	records := exportCalendarRecords(run, events, courses, location)
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	record := records[0]
	if record.CourseSlug != "deep-learning" {
		t.Fatalf("course slug = %q, want deep-learning", record.CourseSlug)
	}
	if !strings.Contains(record.SearchText, "B1.03") {
		t.Fatalf("search text does not contain location: %q", record.SearchText)
	}
	if record.Category != "lecture" {
		t.Fatalf("category = %q, want lecture", record.Category)
	}
}

func TestExportCalendarRunWritesRepoAndDriveArtifacts(t *testing.T) {
	const ics = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:calendar-test-1\r\nDTSTAMP:20260501T080000Z\r\nSUMMARY:Deep Learning\r\nLOCATION:B1.03\r\nDTSTART:20260515T113000Z\r\nDTEND:20260515T130000Z\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/calendar")
		_, _ = w.Write([]byte(ics))
	}))
	defer server.Close()

	workspace := t.TempDir()
	run := exportRunContext{
		Semester:  "FS26",
		RunID:     "run-1",
		StartedAt: time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC),
		Workspace: workspace,
	}
	cfg := schoolExportConfig{Timezone: "Europe/Zurich"}
	courses := []exportCourse{{
		ID:    22585,
		Slug:  "deep-learning",
		Title: "Deep Learning",
	}}

	result, err := exportCalendarRun(context.Background(), newDryRunExportDriveUploader(), run, cfg, courses, server.URL, t.TempDir())
	if err != nil {
		t.Fatalf("exportCalendarRun returned error: %v", err)
	}
	if result.Index.EventCount != 1 {
		t.Fatalf("event count = %d, want 1", result.Index.EventCount)
	}
	if result.Index.Events[0].CourseSlug != "deep-learning" {
		t.Fatalf("course slug = %q, want deep-learning", result.Index.Events[0].CourseSlug)
	}
	if result.Index.RawICS.CurrentDriveLink == "" {
		t.Fatalf("expected current raw ICS Drive link")
	}

	for _, name := range []string{"README.md", "calendar.index.yaml", "calendar.index.md"} {
		path := filepath.Join(workspace, "FS26", "calendar", name)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to exist: %v", path, err)
		}
	}
}
