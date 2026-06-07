package studypipeline

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanDetectsCompleteCoursePipeline(t *testing.T) {
	root := t.TempDir()
	course := filepath.Join(root, "terms", "FS26", "courses", "high-performance-computing")
	writeFile(t, filepath.Join(course, "README.md"), "# High Performance Computing\n")
	writeFile(t, filepath.Join(course, ".raw", "Moodle.md"), "# Moodle\n")
	writeFile(t, filepath.Join(course, ".raw", "materials.index.yaml"), "materials: []\n")
	writeFile(t, filepath.Join(course, ".raw", "materials", "01", "deck.pdf"), "pdf")
	writeFile(t, filepath.Join(course, ".extracted", "script", "Script.mdx"), "# Extracted\n")
	writeFile(t, filepath.Join(course, ".extracted", "slides", "01.mdx"), "# Slide\n")
	writeFile(t, filepath.Join(course, ".extracted", "tasks", "01-task.mdx"), "# Task\n")
	writeFile(t, filepath.Join(course, ".extracted", "solutions", "01-solution.mdx"), "# Solution\n")
	writeFile(t, filepath.Join(course, ".extracted", "slides", "01.assets", "image.png"), "png")
	writeFile(t, filepath.Join(course, "script", "Script.mdx"), "# Script\n")
	writeFile(t, filepath.Join(course, "tasks", "01-task.mdx"), "---\nsolution_status: \"moodle-solution-available\"\n---\n# Task\n")
	writeFile(t, filepath.Join(course, "tasks", "solutions", "01-solution.mdx"), "# Solution\n")

	payload, err := Scan(Options{Workspace: root, Term: "FS26"})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if payload.Summary.Courses != 1 {
		t.Fatalf("expected one course, got %#v", payload.Summary)
	}
	got := payload.Courses[0]
	if got.Status != "complete" {
		t.Fatalf("expected complete status, got %q issues=%v", got.Status, got.Issues)
	}
	if got.Title != "High Performance Computing" {
		t.Fatalf("unexpected title %q", got.Title)
	}
	if got.Raw.Materials.Files != 1 || got.Extracted.Assets != 1 || got.Curated.Solutions.Files != 1 {
		t.Fatalf("unexpected counts: %#v", got)
	}
	if got.Curated.SolutionStates["moodle-solution-available"] != 1 {
		t.Fatalf("solution state not detected: %#v", got.Curated.SolutionStates)
	}
}

func TestScanReportsMissingCourseStages(t *testing.T) {
	root := t.TempDir()
	course := filepath.Join(root, "terms", "FS26", "courses", "empty-course")
	writeFile(t, filepath.Join(course, "README.md"), "# Empty Course\n")

	payload, err := Scan(Options{Workspace: root, Term: "FS26", Course: "empty-course"})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	got := payload.Courses[0]
	if got.Status == "complete" {
		t.Fatalf("expected incomplete course, got %#v", got)
	}
	if len(got.Issues) == 0 {
		t.Fatalf("expected missing quality gate issues")
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
