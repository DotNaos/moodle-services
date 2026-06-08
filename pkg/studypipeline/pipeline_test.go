package studypipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DotNaos/moodle-services/internal/moodle"
)

func TestBuildClassifiesAndLinksCourseMaterials(t *testing.T) {
	payload := Build("22584", []moodle.Resource{
		{ID: "1", Name: "01 Memory Hierarchy", FileType: "pdf", SectionID: "s1", SectionName: "Week 1"},
		{ID: "2", Name: "Aufgabenblatt 01", FileType: "pdf", SectionID: "s1", SectionName: "Week 1"},
		{ID: "3", Name: "Loesung Aufgabenblatt 01", FileType: "pdf", SectionID: "s1", SectionName: "Week 1"},
		{ID: "4", Name: "Aufgabenblatt 02", FileType: "pdf", SectionID: "s2", SectionName: "Week 2"},
	}, "created", time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC))

	if payload.CourseID != "22584" || payload.Status != "created" {
		t.Fatalf("unexpected payload identity: %#v", payload)
	}
	if payload.Summary.Slides != 1 || payload.Summary.Tasks != 2 || payload.Summary.Solutions != 1 {
		t.Fatalf("unexpected summary: %#v", payload.Summary)
	}
	if payload.Summary.LinkedSolutions != 1 || payload.Summary.MissingSolutions != 1 {
		t.Fatalf("unexpected solution summary: %#v", payload.Summary)
	}
	if len(payload.TaskLinks) != 2 {
		t.Fatalf("expected two task links, got %d", len(payload.TaskLinks))
	}
	if payload.TaskLinks[0].Solution == nil || payload.TaskLinks[0].Solution.ID != "3" {
		t.Fatalf("expected first task to link solution 3, got %#v", payload.TaskLinks[0])
	}
	if got := payload.MissingSolutions[0].ID; got != "4" {
		t.Fatalf("expected task 4 to miss solution, got %q", got)
	}
}

func TestBuildRecognizesGermanUmlauts(t *testing.T) {
	payload := Build("1", []moodle.Resource{
		{ID: "1", Name: "Übung 03", FileType: "pdf"},
		{ID: "2", Name: "Musterlösung 03", FileType: "pdf"},
	}, "", time.Unix(0, 0))

	if payload.Summary.Tasks != 1 || payload.Summary.Solutions != 1 || payload.Summary.LinkedSolutions != 1 {
		t.Fatalf("unexpected summary: %#v", payload.Summary)
	}
}

func TestBuildRecognizesCourseSpecificTaskNames(t *testing.T) {
	payload := Build("22576", []moodle.Resource{
		{ID: "1", Name: "Arbeitsauftrag", FileType: "docx", SectionName: "Woche 3"},
		{ID: "2", Name: "Auftrag Abschlussarbeit & Inhaltsverzeichnis", FileType: "docx", SectionName: "Woche 3"},
		{ID: "3", Name: "Bewertungskriterien Schlusspräsentation", FileType: "xlsx", SectionName: "Woche 3"},
		{ID: "4", Name: "Powerpoint Vorlage", FileType: "pptx", SectionName: "Allgemein"},
	}, "", time.Unix(0, 0))

	if payload.Summary.Tasks != 2 {
		t.Fatalf("expected two real task-like resources, got summary %#v", payload.Summary)
	}
	if payload.Summary.Other != 2 {
		t.Fatalf("expected criteria/template to stay non-task, got summary %#v", payload.Summary)
	}
}

func TestLoadTaskViewDoesNotGenerateFakeTasksWhenCourseHasNoTaskSheets(t *testing.T) {
	resources := []moodle.Resource{
		{ID: "1", Name: "Folien 1.1 - Einführung", FileType: "pdf", SectionName: "Termin 1"},
		{ID: "2", Name: "Powerpoint Vorlage", FileType: "pptx", SectionName: "Allgemein"},
		{ID: "3", Name: "Bewertungskriterien", FileType: "pdf", SectionName: "Leistungsnachweis"},
	}
	view, err := LoadTaskView("17503", resources, true, RunOptions{
		Root: t.TempDir(),
		Now:  time.Unix(0, 0),
	})
	if err != nil {
		t.Fatalf("LoadTaskView: %v", err)
	}
	if len(view.Sheets) != 0 {
		t.Fatalf("expected no fake generated task sheets, got %#v", view.Sheets)
	}
}

func TestCuratedStageDoesNotDownloadRawMaterials(t *testing.T) {
	_, err := RunStage("17503", []moodle.Resource{
		{ID: "1", Name: "Folien 1.1 - Einführung", URL: "https://example.invalid/material.pdf", FileType: "pdf"},
	}, "curated", RunOptions{
		Root:       t.TempDir(),
		Now:        time.Unix(0, 0),
		Downloader: failingDownloader{},
	})
	if err != nil {
		t.Fatalf("curated stage should not call downloader: %v", err)
	}
}

type failingDownloader struct{}

func (failingDownloader) DownloadFileToBuffer(string) (moodle.DownloadResult, error) {
	return moodle.DownloadResult{}, fmt.Errorf("downloader should not be called")
}

func TestCuratedStageBuildsScriptFromExtractedContent(t *testing.T) {
	root := t.TempDir()
	courseID := "22585"
	resources := []moodle.Resource{
		{ID: "1", Name: "Neural Networks", FileType: "pdf", SectionName: "Week 1"},
		{ID: "2", Name: "Aufgabenblatt 01", FileType: "pdf", SectionName: "Week 1"},
	}
	writeExtractedFixture(t, root, courseID, "slides", "1-Neural Networks", "Hidden layers transform tensors into useful representations.")
	writeExtractedFixture(t, root, courseID, "tasks", "2-Aufgabenblatt 01", "Berechnen Sie die Anzahl Parameter des neuronalen Netzes.")

	view, err := LoadTaskView(courseID, resources, true, RunOptions{
		Root: root,
		Now:  time.Unix(0, 0),
	})
	if err != nil {
		t.Fatalf("LoadTaskView: %v", err)
	}
	if !strings.Contains(view.ScriptMarkdown, "Hidden layers transform tensors") {
		t.Fatalf("expected script to include extracted slide content, got %q", view.ScriptMarkdown)
	}
	if strings.Contains(view.ScriptMarkdown, "ready for the Codex cleanup stage") {
		t.Fatalf("script still contains placeholder cleanup text: %q", view.ScriptMarkdown)
	}
	if len(view.Sheets) != 1 || !strings.Contains(view.Sheets[0].Tasks[0].PromptMarkdown, "Berechnen Sie die Anzahl Parameter") {
		t.Fatalf("expected task prompt to include extracted task text, got %#v", view.Sheets)
	}
}

func TestCuratedStageRemovesStaleGeneratedTaskFiles(t *testing.T) {
	root := t.TempDir()
	courseID := "19489"
	staleTaskPath := filepath.Join(root, "courses", courseID, "curated", "tasks", "task-old.mdx")
	if err := os.MkdirAll(filepath.Dir(staleTaskPath), 0o755); err != nil {
		t.Fatalf("mkdir stale task: %v", err)
	}
	if err := os.WriteFile(staleTaskPath, []byte("This task was detected from Moodle material."), 0o644); err != nil {
		t.Fatalf("write stale task: %v", err)
	}

	view, err := LoadTaskView(courseID, []moodle.Resource{
		{ID: "1", Name: "Einführungsfolien", FileType: "pdf", SectionName: "Week 1"},
	}, false, RunOptions{
		Root: root,
		Now:  time.Unix(0, 0),
	})
	if err != nil {
		t.Fatalf("LoadTaskView: %v", err)
	}
	if len(view.Sheets) != 0 {
		t.Fatalf("expected no task sheets, got %#v", view.Sheets)
	}
	if _, err := os.Stat(staleTaskPath); !os.IsNotExist(err) {
		t.Fatalf("expected stale generated task file to be removed, stat err: %v", err)
	}
}

func TestExtractedTextDoesNotUseRawPDFBytesWhenExtractionFails(t *testing.T) {
	content := extractedText(moodle.Resource{
		ID:       "1",
		Name:     "Broken PDF",
		FileType: "pdf",
	}, moodle.DownloadResult{
		Data:        []byte("%PDF-1.7\nxref\ntrailer\n%%EOF"),
		ContentType: "application/pdf",
	})

	if strings.Contains(content, "%PDF-1.7") || strings.Contains(content, "xref") {
		t.Fatalf("expected raw PDF bytes to be excluded, got %q", content)
	}
	if !strings.Contains(content, "No text could be extracted from Broken PDF") {
		t.Fatalf("expected extraction failure marker, got %q", content)
	}
}

func writeExtractedFixture(t *testing.T, root string, courseID string, dirName string, name string, body string) {
	t.Helper()
	path := filepath.Join(root, "courses", safeSegment(courseID), "extracted", dirName, safeSegment(name)+".mdx")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	content := strings.Join([]string{
		"---",
		"status: extracted",
		"---",
		"",
		body,
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}
