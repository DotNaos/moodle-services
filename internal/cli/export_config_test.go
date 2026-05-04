package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DotNaos/moodle-services/internal/moodle"
)

func TestBuildExportRunContextUsesUTCAndGitHubRunID(t *testing.T) {
	t.Setenv("GITHUB_RUN_ID", "123456789")
	t.Setenv("GITHUB_RUN_ATTEMPT", "2")

	ctx := buildExportRunContext("/tmp/school", "FS26", time.Date(2026, 5, 3, 13, 42, 18, 0, time.UTC))

	if ctx.RunID != "2026-05-03-134218-123456789" {
		t.Fatalf("unexpected run id: %s", ctx.RunID)
	}
	if ctx.GitHubRunAttempt != "2" {
		t.Fatalf("unexpected run attempt: %s", ctx.GitHubRunAttempt)
	}
}

func TestSemestersToProcessSkipsCompletedOldSemesters(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "FS26"))
	mustMkdir(t, filepath.Join(root, "HS25"))
	cfg := schoolExportConfig{CurrentTerm: "FS26"}
	index := exportIndex{Semesters: map[string]exportSemesterRef{
		"HS25": {Status: exportStatusComplete},
	}}

	got, err := semestersToProcess(root, cfg, index, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "FS26" {
		t.Fatalf("unexpected semesters: %#v", got)
	}
}

func TestSemestersToProcessIncludesMissingOldSemesters(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "FS26"))
	mustMkdir(t, filepath.Join(root, "HS25"))
	cfg := schoolExportConfig{CurrentTerm: "FS26"}
	index := exportIndex{Semesters: map[string]exportSemesterRef{}}

	got, err := semestersToProcess(root, cfg, index, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "FS26" || got[1] != "HS25" {
		t.Fatalf("unexpected semesters: %#v", got)
	}
}

func TestExportCourseSlugUsesSchoolOverrides(t *testing.T) {
	cfg := schoolExportConfig{}
	cfg.Moodle.CourseSlugOverrides = map[string]string{"19489": "design-thinking"}

	got := exportCourseSlug(testMoodleCourse(19489, "Design Thinking FS26", "DT FS26"), cfg)
	if got != "design-thinking" {
		t.Fatalf("unexpected slug: %s", got)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func testMoodleCourse(id int, fullname string, shortname string) moodle.Course {
	return moodle.Course{ID: id, Fullname: fullname, Shortname: shortname, Category: "FS26"}
}
