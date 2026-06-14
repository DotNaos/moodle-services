package studypipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DotNaos/moodle-services/internal/moodle"
	contract "github.com/DotNaos/moodle-services/pkg/apicontracts"
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

func TestBuildDoesNotPairNeighborSolutionBySectionWhenTaskHasNumber(t *testing.T) {
	payload := Build("22584", []moodle.Resource{
		{ID: "9", Name: "Aufgabenblatt 09", FileType: "pdf", SectionID: "s4", SectionName: "Nachrichtengekoppelte Systeme"},
		{ID: "10", Name: "Aufgabenblatt 10", FileType: "pdf", SectionID: "s4", SectionName: "Nachrichtengekoppelte Systeme"},
		{ID: "10s", Name: "Aufgabenblatt 10 -- Lösung", FileType: "pdf", SectionID: "s4", SectionName: "Nachrichtengekoppelte Systeme"},
	}, "", time.Unix(0, 0))

	if payload.Summary.LinkedSolutions != 1 || payload.Summary.MissingSolutions != 1 {
		t.Fatalf("unexpected solution summary: %#v", payload.Summary)
	}
	for _, link := range payload.TaskLinks {
		if link.Task.ID == "9" && link.Solution != nil {
			t.Fatalf("task 9 should not receive task 10 solution: %#v", link)
		}
		if link.Task.ID == "10" && (link.Solution == nil || link.Solution.ID != "10s") {
			t.Fatalf("task 10 should receive its own solution: %#v", link)
		}
	}
}

func TestBuildInventoryGroupsTasksSolutionsAndReferences(t *testing.T) {
	inventory := BuildInventory("22584", []moodle.Resource{
		{ID: "1", Name: "Teil 01 Memory Hierarchy", FileType: "pdf", SectionID: "s1", SectionName: "Woche 1"},
		{ID: "2", Name: "Aufgabenblatt 01", FileType: "pdf", SectionID: "s1", SectionName: "Woche 1"},
		{ID: "3", Name: "Lösung Aufgabenblatt 01", FileType: "pdf", SectionID: "s1", SectionName: "Woche 1"},
		{ID: "4", Name: "Aufgabenblatt 09", FileType: "pdf", SectionID: "s4", SectionName: "Woche 4"},
		{ID: "5", Name: "Aufgabenblatt 10", FileType: "pdf", SectionID: "s4", SectionName: "Woche 4"},
		{ID: "6", Name: "Aufgabenblatt 10 -- Lösung", FileType: "pdf", SectionID: "s4", SectionName: "Woche 4"},
		{ID: "7", Name: "Modulbeschreibung", FileType: "pdf", SectionID: "s0", SectionName: "Allgemein"},
		{ID: "8", Name: "Forum Fragen", Type: "forum", SectionID: "s0", SectionName: "Allgemein"},
		{ID: "9", Name: "Externes Werkzeug", Type: "url", SectionID: "s0", SectionName: "Allgemein"},
	}, time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC))

	if inventory.CourseID != "22584" || inventory.GeneratedAt != "2026-06-12T10:00:00Z" {
		t.Fatalf("unexpected inventory identity: %#v", inventory)
	}
	if inventory.Summary.TotalResources != 9 || inventory.Summary.LectureMaterial != 1 || inventory.Summary.TaskGroups != 3 {
		t.Fatalf("unexpected summary: %#v", inventory.Summary)
	}
	if inventory.Summary.PairedTaskGroups != 2 || inventory.Summary.MissingSolutionGroups != 1 || inventory.Summary.AmbiguousTaskGroups != 0 {
		t.Fatalf("unexpected pairing summary: %#v", inventory.Summary)
	}
	if inventory.Summary.References != 1 || inventory.Summary.Interactions != 1 || inventory.Summary.Unknown != 1 {
		t.Fatalf("unexpected non-task summary: %#v", inventory.Summary)
	}

	if len(inventory.TaskGroups) != 3 {
		t.Fatalf("expected three task groups, got %d", len(inventory.TaskGroups))
	}
	firstGroup := inventory.TaskGroups[0]
	if firstGroup.ID != "task-group-1" || firstGroup.PairingStatus != "paired" {
		t.Fatalf("unexpected first group: %#v", firstGroup)
	}
	if firstGroup.Solution == nil || firstGroup.Solution.ID != "3" {
		t.Fatalf("expected first group to link solution 3, got %#v", firstGroup)
	}
	secondGroup := inventory.TaskGroups[1]
	if secondGroup.ID != "task-group-9" || secondGroup.PairingStatus != "missing_solution" {
		t.Fatalf("unexpected second group: %#v", secondGroup)
	}
	thirdGroup := inventory.TaskGroups[2]
	if thirdGroup.ID != "task-group-10" || thirdGroup.Solution == nil || thirdGroup.Solution.ID != "6" {
		t.Fatalf("expected task 10 to link its own solution, got %#v", thirdGroup)
	}
	if len(inventory.Unknown) != 1 || inventory.Unknown[0].ID != "9" {
		t.Fatalf("expected unknown resource 7 to be preserved, got %#v", inventory.Unknown)
	}
}

func TestLoadInventoryPersistsCourseInventory(t *testing.T) {
	root := t.TempDir()
	courseID := "22584"
	_, err := LoadInventory(courseID, []moodle.Resource{
		{ID: "2", Name: "Aufgabenblatt 01", FileType: "pdf", SectionID: "s1"},
		{ID: "3", Name: "Lösung Aufgabenblatt 01", FileType: "pdf", SectionID: "s1"},
	}, RunOptions{
		Root: root,
		Now:  time.Unix(0, 0),
	})
	if err != nil {
		t.Fatalf("LoadInventory: %v", err)
	}

	path := filepath.Join(root, "courses", courseID, "inventory", "course-inventory.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read inventory: %v", err)
	}
	var persisted contract.CourseInventoryResponse
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("decode inventory: %v", err)
	}
	if persisted.Summary.PairedTaskGroups != 1 || persisted.TaskGroups[0].Solution == nil {
		t.Fatalf("unexpected persisted inventory: %#v", persisted)
	}
}

func TestLoadExtractedDocumentsBuildsRenderableStructure(t *testing.T) {
	root := t.TempDir()
	courseID := "22584"
	resources := []moodle.Resource{
		{ID: "2", Name: "Aufgabenblatt 01", FileType: "pdf", SectionName: "Einführung"},
	}
	writeExtractedFixture(t, root, courseID, "tasks", "2-Aufgabenblatt 01", strings.Join([]string{
		"# Aufgabe 1",
		"",
		"- Teil A",
		"- Teil B",
		"",
		"E = mc^2",
	}, "\n"))

	response, err := LoadExtractedDocuments(courseID, resources, RunOptions{
		Root: root,
		Now:  time.Date(2026, 6, 12, 10, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("LoadExtractedDocuments: %v", err)
	}
	if response.RunID != "baseline-20260612T103000Z" || response.Engine == "" {
		t.Fatalf("unexpected run metadata: %#v", response)
	}
	if response.Summary.TotalDocuments != 1 || response.Summary.TotalPages != 1 || response.Summary.TotalBlocks != 3 {
		t.Fatalf("unexpected summary: %#v", response.Summary)
	}
	document := response.Documents[0]
	if document.Resource.ID != "2" || document.Status != "machine-extracted" {
		t.Fatalf("unexpected document: %#v", document)
	}
	blocks := document.Pages[0].Blocks
	if blocks[0].Type != "heading" || blocks[1].Type != "list" || blocks[2].Type != "formula" {
		t.Fatalf("unexpected blocks: %#v", blocks)
	}
	latestPath := filepath.Join(root, "courses", courseID, "extracted", "latest-documents.json")
	if _, err := os.Stat(latestPath); err != nil {
		t.Fatalf("expected latest document structure to be written: %v", err)
	}

	cached, err := LoadExtractedDocuments(courseID, resources, RunOptions{
		Root: root,
		Now:  time.Date(2026, 6, 12, 10, 31, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("LoadExtractedDocuments cached: %v", err)
	}
	if cached.RunID != response.RunID {
		t.Fatalf("expected cached document run %q, got %q", response.RunID, cached.RunID)
	}
	unexpectedRunPath := filepath.Join(root, "courses", courseID, "extracted", "runs", "baseline-20260612T103100Z")
	if _, err := os.Stat(unexpectedRunPath); !os.IsNotExist(err) {
		t.Fatalf("expected cached read not to create a new run, stat err=%v", err)
	}
}

func TestOpenExtractedAssetServesOnlyCourseArtifacts(t *testing.T) {
	root := t.TempDir()
	courseID := "22584"
	assetPath := filepath.Join(root, "courses", courseID, "extracted", "runs", "run-1", "assets", "page.png")
	if err := os.MkdirAll(filepath.Dir(assetPath), 0o755); err != nil {
		t.Fatalf("mkdir asset dir: %v", err)
	}
	if err := os.WriteFile(assetPath, []byte{0x89, 0x50, 0x4e, 0x47}, 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}

	data, contentType, err := OpenExtractedAsset(courseID, assetPath, RunOptions{Root: root})
	if err != nil {
		t.Fatalf("OpenExtractedAsset: %v", err)
	}
	if string(data) != string([]byte{0x89, 0x50, 0x4e, 0x47}) || !strings.Contains(contentType, "image/png") {
		t.Fatalf("unexpected asset response data=%v contentType=%q", data, contentType)
	}

	outsidePath := filepath.Join(root, "other-course.png")
	if err := os.WriteFile(outsidePath, []byte("nope"), 0o644); err != nil {
		t.Fatalf("write outside asset: %v", err)
	}
	_, _, err = OpenExtractedAsset(courseID, outsidePath, RunOptions{Root: root})
	if !errors.Is(err, ErrInvalidExtractedAssetPath) {
		t.Fatalf("expected invalid asset path, got %v", err)
	}
}

func TestLoadTaskViewDoesNotGenerateFakeTasksWhenCourseHasNoTaskSheets(t *testing.T) {
	root := t.TempDir()
	courseID := "17503"
	resources := []moodle.Resource{
		{ID: "1", Name: "Folien 1.1 - Einführung", FileType: "pdf", SectionName: "Termin 1"},
		{ID: "2", Name: "Powerpoint Vorlage", FileType: "pptx", SectionName: "Allgemein"},
		{ID: "3", Name: "Bewertungskriterien", FileType: "pdf", SectionName: "Leistungsnachweis"},
	}
	writeExtractedFixture(t, root, courseID, "slides", "1-Folien 1.1 - Einführung", "course slide text")

	view, err := LoadTaskView(courseID, resources, true, RunOptions{
		Root: root,
		Now:  time.Unix(0, 0),
	})
	if err != nil {
		t.Fatalf("LoadTaskView: %v", err)
	}
	if len(view.Sheets) != 0 {
		t.Fatalf("expected no fake generated task sheets, got %#v", view.Sheets)
	}
}

func TestRecordTaskStatusPersistsDoneProgress(t *testing.T) {
	root := t.TempDir()
	courseID := "22584"
	resources := []moodle.Resource{
		{ID: "2", Name: "Aufgabenblatt 01", FileType: "pdf", SectionName: "Einführung"},
	}
	writeExtractedFixture(t, root, courseID, "tasks", "2-Aufgabenblatt 01", "task text")
	id := taskID(contract.StudyPipelineMaterial{ID: "2", Name: "Aufgabenblatt 01"})
	if err := RecordTaskStatus(root, courseID, id, "done"); err != nil {
		t.Fatalf("RecordTaskStatus: %v", err)
	}

	view, err := LoadTaskView(courseID, resources, false, RunOptions{
		Root: root,
		Now:  time.Unix(0, 0),
	})
	if err != nil {
		t.Fatalf("LoadTaskView: %v", err)
	}
	if got := view.Sheets[0].Tasks[0].Status; got != "done" {
		t.Fatalf("status = %q, want done", got)
	}
	if view.Progress.Done != 1 || view.Progress.Checked != 1 || view.Progress.Open != 0 {
		t.Fatalf("unexpected progress: %#v", view.Progress)
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
	if err == nil {
		t.Fatalf("expected curated stage to fail without complete element accountability")
	}
	if strings.Contains(err.Error(), "downloader should not be called") {
		t.Fatalf("curated stage called downloader: %v", err)
	}
	if !strings.Contains(err.Error(), "element accountability incomplete") {
		t.Fatalf("expected accountability error, got %v", err)
	}
}

func TestRunStageCarriesRequestedEngineMetadata(t *testing.T) {
	response, err := RunStage("22584", []moodle.Resource{
		{ID: "1", Name: "Aufgabenblatt 01", FileType: "pdf"},
	}, "extracted", RunOptions{
		Root:       t.TempDir(),
		Now:        time.Unix(0, 0),
		Engine:     "marker",
		ConfigHash: "config:extracted:marker:layout-v1",
	})
	if err != nil {
		t.Fatalf("RunStage extracted: %v", err)
	}
	if response.Engine != "marker" || response.ConfigHash != "config:extracted:marker:layout-v1" {
		t.Fatalf("unexpected run metadata engine=%q config=%q", response.Engine, response.ConfigHash)
	}
}

type failingDownloader struct{}

func (failingDownloader) DownloadFileToBuffer(string) (moodle.DownloadResult, error) {
	return moodle.DownloadResult{}, fmt.Errorf("downloader should not be called")
}

type staticDownloader struct {
	data        []byte
	contentType string
}

func (downloader staticDownloader) DownloadFileToBuffer(string) (moodle.DownloadResult, error) {
	return moodle.DownloadResult{Data: downloader.data, ContentType: downloader.contentType}, nil
}

func TestCuratedStageRunsExtractionWhenMissing(t *testing.T) {
	root := t.TempDir()
	courseID := "22584"
	resources := []moodle.Resource{
		{ID: "1", Name: "Lecture Teil 03", URL: "https://example.invalid/teil-03.txt", FileType: "txt"},
	}
	_, err := RunStage(courseID, resources, "curated", RunOptions{
		Root:       root,
		Now:        time.Unix(0, 0),
		Downloader: staticDownloader{data: []byte("real extracted lecture text"), contentType: "text/plain"},
	})
	if err == nil {
		t.Fatalf("expected curated stage to fail without complete element accountability")
	}
	if !strings.Contains(err.Error(), "element accountability incomplete") {
		t.Fatalf("expected accountability error, got %v", err)
	}

	script, err := os.ReadFile(filepath.Join(root, "courses", courseID, "curated", "script", "Script.mdx"))
	if err != nil {
		t.Fatalf("read script: %v", err)
	}
	if !strings.Contains(string(script), "real extracted lecture text") {
		t.Fatalf("expected curated script to include extracted text, got %q", string(script))
	}
	if strings.Contains(string(script), "No extracted text was available") {
		t.Fatalf("curated script still contains missing extraction placeholder: %q", string(script))
	}
}

func TestStatusDoesNotReportCuratedWithoutExtractedArtifacts(t *testing.T) {
	root := t.TempDir()
	courseID := "22584"
	curatedTasksDir := filepath.Join(root, "courses", courseID, "curated", "tasks")
	if err := os.MkdirAll(curatedTasksDir, 0o755); err != nil {
		t.Fatalf("mkdir curated tasks: %v", err)
	}
	if err := os.WriteFile(filepath.Join(curatedTasksDir, "Tasks.mdx"), []byte("# Tasks\n"), 0o644); err != nil {
		t.Fatalf("write stale curated tasks: %v", err)
	}

	status := Status(courseID, nil, RunOptions{Root: root, Now: time.Unix(0, 0)})
	if status.Stage == "curated" || status.Status == "curated-ready" {
		t.Fatalf("stale curated artifacts without extraction reported ready: %#v", status)
	}
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
	if len(view.ScriptSections) != 1 || view.ScriptSections[0].Status != "machine-extracted" {
		t.Fatalf("expected script section status to be machine-extracted, got %#v", view.ScriptSections)
	}
	if view.Sheets[0].Tasks[0].ContentState.Status != "machine-extracted" {
		t.Fatalf("expected task status to be machine-extracted, got %#v", view.Sheets[0].Tasks[0].ContentState)
	}
}

func TestLoadTaskViewIncludesExtractedImageAssets(t *testing.T) {
	root := t.TempDir()
	courseID := "22584"
	now := time.Date(2026, 6, 14, 9, 10, 0, 0, time.UTC)
	resources := []moodle.Resource{
		{ID: "947711", Name: "Aufgabenblatt 01", FileType: "pdf", SectionName: "Week 1"},
	}
	writeExtractedFixture(t, root, courseID, "tasks", "947711-Aufgabenblatt 01", "Aufgabe 1\n\nMit Diagramm.")
	writeLatestExtractedDocumentFixture(t, root, courseID, contract.ExtractedDocumentsResponse{
		CourseID:     courseID,
		RunID:        "baseline-20260614T091000Z",
		GeneratedAt:  now.Format(time.RFC3339),
		Engine:       extractedDocumentEngine,
		ArtifactRoot: filepath.Join(root, "courses", courseID),
		Documents: []contract.PDFDocument{{
			ID: "947711",
			Resource: contract.StudyPipelineMaterial{
				ID:       "947711",
				Name:     "Aufgabenblatt 01",
				Type:     "task",
				FileType: "pdf",
			},
			RunID:  "baseline-20260614T091000Z",
			Engine: extractedDocumentEngine,
			Status: "machine-extracted",
			Pages: []contract.PDFPage{{
				ID:         "947711-page-001",
				PageNumber: 1,
				Text:       "Aufgabe 1\n\nMit Diagramm.",
				Markdown:   "Aufgabe 1\n\nMit Diagramm.",
				Blocks: []contract.DocumentBlock{{
					ID:         "947711-p001-b001",
					PageNumber: 1,
					Type:       "paragraph",
					Label:      "task_paragraph",
					Text:       "Mit Diagramm.",
					Markdown:   "Mit Diagramm.",
					Source:     "extracted_text",
					Confidence: "high",
				}},
			}},
			Assets: []contract.DocumentAsset{{
				ID:       "embedded-image-001",
				Kind:     "embedded_image",
				Path:     filepath.Join(root, "courses", courseID, "extracted", "runs", "baseline-20260614T091000Z", "assets", "947711-aufgabenblatt-01", "images", "image-000.png"),
				MimeType: "image/png",
				Role:     "extracted_image",
			}},
			Diagnostics: contract.ExtractedDocumentDiagnostics{
				ExtractedImageAssets: 1,
				UnusedImageAssets:    []string{"embedded-image-001"},
			},
		}},
	})

	view, err := LoadTaskView(courseID, resources, false, RunOptions{
		Root: root,
		Now:  now,
	})
	if err != nil {
		t.Fatalf("LoadTaskView: %v", err)
	}
	if len(view.Sheets) != 1 || len(view.Sheets[0].Tasks) != 1 {
		t.Fatalf("expected one task, got %#v", view.Sheets)
	}
	prompt := view.Sheets[0].Tasks[0].PromptMarkdown
	if !strings.Contains(prompt, "embedded-image-001") || !strings.Contains(prompt, "/api/study-pipeline/courses/22584/study-pipeline/extracted-asset?path=") {
		t.Fatalf("expected task view prompt to include extracted image figure, got %q", prompt)
	}
}

func TestRefineContentWritesSeparateImprovedArtifact(t *testing.T) {
	root := t.TempDir()
	courseID := "22585"
	resources := []moodle.Resource{
		{ID: "1", Name: "Neural Networks", FileType: "pdf", SectionName: "Week 1"},
	}
	writeExtractedFixture(t, root, courseID, "slides", "1-Neural Networks", "ugly extracted tensor text")

	response, err := RefineContent(context.Background(), courseID, resources, contractRefineRequest("script-section", "1"), RunOptions{
		Root:    root,
		Now:     time.Unix(0, 0),
		UserID:  "user-1",
		Refiner: fakeRefiner{content: "## Neural Networks\n\nCleaned text with $x$ and structure.", model: "test-model"},
	})
	if err != nil {
		t.Fatalf("RefineContent: %v", err)
	}
	if response.Target.Status != "codex-improved" || response.Target.Model != "test-model" {
		t.Fatalf("unexpected target state: %#v", response.Target)
	}

	extracted := extractedContentForMaterial(root, courseID, Build(courseID, resources, "", time.Unix(0, 0)).Materials[0])
	if extracted != "ugly extracted tensor text" {
		t.Fatalf("extracted content was modified: %q", extracted)
	}
	view, err := LoadTaskView(courseID, resources, true, RunOptions{Root: root, Now: time.Unix(0, 0)})
	if err != nil {
		t.Fatalf("LoadTaskView: %v", err)
	}
	if !strings.Contains(view.ScriptMarkdown, "Cleaned text with $x$") {
		t.Fatalf("expected improved content in script, got %q", view.ScriptMarkdown)
	}
	if strings.Contains(view.ScriptMarkdown, "ugly extracted tensor text") {
		t.Fatalf("expected improved content to replace display text only, got %q", view.ScriptMarkdown)
	}
	if len(view.ScriptSections) != 1 || view.ScriptSections[0].Status != "codex-improved" {
		t.Fatalf("expected improved section status, got %#v", view.ScriptSections)
	}
}

func TestRefineContentPassesCustomPromptToRefiner(t *testing.T) {
	root := t.TempDir()
	courseID := "22585"
	resources := []moodle.Resource{
		{ID: "1", Name: "CNN", FileType: "pdf", SectionName: "Week 1"},
	}
	writeExtractedFixture(t, root, courseID, "slides", "1-CNN", "extracted convolution text")
	refiner := &captureRefiner{content: "## CNN\n\nCleaned text.", model: "test-model"}

	_, err := RefineContent(context.Background(), courseID, resources, contract.StudyPipelineRefineRequest{
		Kind:         "script-section",
		TargetID:     "1",
		CustomPrompt: "Bitte deutsche Begriffe bevorzugen und wichtige Formeln stärker strukturieren.",
	}, RunOptions{
		Root:    root,
		Now:     time.Unix(0, 0),
		UserID:  "user-1",
		Refiner: refiner,
	})
	if err != nil {
		t.Fatalf("RefineContent: %v", err)
	}
	if refiner.input.CustomPrompt != "Bitte deutsche Begriffe bevorzugen und wichtige Formeln stärker strukturieren." {
		t.Fatalf("custom prompt was not forwarded: %q", refiner.input.CustomPrompt)
	}
}

func TestBuildRefinePromptIncludesCustomPromptAsGuidance(t *testing.T) {
	prompt := buildRefinePrompt(RefineInput{
		CourseID:     "22585",
		Kind:         "task",
		TargetID:     "task-1",
		Title:        "Aufgabe",
		CustomPrompt: "Mach die Aufgabenstellung prüfungsfreundlicher.",
		Content:      "Original source text.",
	})

	if !strings.Contains(prompt, "Additional user instructions for this refinement:") {
		t.Fatalf("custom prompt section missing: %s", prompt)
	}
	if !strings.Contains(prompt, "Mach die Aufgabenstellung prüfungsfreundlicher.") {
		t.Fatalf("custom prompt missing: %s", prompt)
	}
	if !strings.Contains(prompt, "Do not use them to add facts") {
		t.Fatalf("anti-hallucination guard missing: %s", prompt)
	}
}

func TestCuratedStageRemovesStaleGeneratedTaskFiles(t *testing.T) {
	root := t.TempDir()
	courseID := "19489"
	writeExtractedFixture(t, root, courseID, "slides", "1-Einführungsfolien", "slide text")
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

func TestReadCodexDeviceAuthStartParsesCLIOutput(t *testing.T) {
	output := strings.Join([]string{
		"Welcome to Codex [v\x1b[90m0.130.0\x1b[0m]",
		"Follow these steps to sign in with ChatGPT using device code authorization:",
		"1. Open this link in your browser and sign in to your account",
		"   \x1b[94mhttps://auth.openai.com/codex/device\x1b[0m",
		"2. Enter this one-time code \x1b[90m(expires in 15 minutes)\x1b[0m",
		"   \x1b[94mBGWE-JHZCL\x1b[0m",
	}, "\n")

	start, err := readCodexDeviceAuthStart(strings.NewReader(output))
	if err != nil {
		t.Fatalf("readCodexDeviceAuthStart: %v", err)
	}
	if start.VerificationURI != "https://auth.openai.com/codex/device" {
		t.Fatalf("unexpected verification URI: %q", start.VerificationURI)
	}
	if start.UserCode != "BGWE-JHZCL" {
		t.Fatalf("unexpected user code: %q", start.UserCode)
	}
	if start.ExpiresInSeconds != 15*60 {
		t.Fatalf("unexpected expiry: %d", start.ExpiresInSeconds)
	}
}

func TestParseCodexChatOutputUsesAnswerAndValidActions(t *testing.T) {
	courseID := "22585"
	resourceID := "949833"
	reason := "Open the task sheet the user asked for."
	output := `{"answer":"Opening the task sheet.","actions":[{"type":"open_resource","courseId":"22585","resourceId":"949833","reason":"Open the task sheet the user asked for."},{"type":"open_resource","courseId":null,"resourceId":"bad"},{"type":"unknown","courseId":"22585"}]}`

	result, err := parseCodexChatOutput(output)
	if err != nil {
		t.Fatalf("parseCodexChatOutput: %v", err)
	}
	if result.FinalResponse != "Opening the task sheet." {
		t.Fatalf("unexpected response: %q", result.FinalResponse)
	}
	if len(result.Actions) != 1 {
		t.Fatalf("expected one valid action, got %#v", result.Actions)
	}
	if result.Actions[0].Type != "open_resource" || *result.Actions[0].CourseID != courseID || *result.Actions[0].ResourceID != resourceID || *result.Actions[0].Reason != reason {
		t.Fatalf("unexpected action: %#v", result.Actions[0])
	}
}

func TestParseCodexChatOutputFallsBackToText(t *testing.T) {
	result, err := parseCodexChatOutput("Plain answer from Codex")
	if err != nil {
		t.Fatalf("parseCodexChatOutput: %v", err)
	}
	if result.FinalResponse != "Plain answer from Codex" || len(result.Actions) != 0 {
		t.Fatalf("unexpected fallback result: %#v", result)
	}
}

func TestSelectDefaultCodexChatModelUsesCatalog(t *testing.T) {
	model, effort := selectDefaultCodexChatModel(contract.CodexModelCatalogResponse{
		Models: []contract.CodexModelOption{{
			ID:                     "gpt-5.5",
			DefaultReasoningEffort: "high",
		}},
	}, "")
	if model != "gpt-5.5" || effort != "high" {
		t.Fatalf("default model = %q/%q, want gpt-5.5/high", model, effort)
	}
}

func TestDockerHostMountPathTranslatesStudyDataPath(t *testing.T) {
	t.Setenv("MOODLE_DOCKER_CONTAINER_DATA_DIR", "/data")
	t.Setenv("MOODLE_DOCKER_HOST_DATA_DIR", "/opt/platform/apps/moodle-staging/services-data")

	got := dockerHostMountPath("/data/study/codex-users/user_123")
	want := "/opt/platform/apps/moodle-staging/services-data/study/codex-users/user_123"
	if got != want {
		t.Fatalf("dockerHostMountPath = %q, want %q", got, want)
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

type fakeRefiner struct {
	content string
	model   string
}

func (f fakeRefiner) Refine(context.Context, RefineInput) (RefineOutput, error) {
	return RefineOutput{Content: f.content, Model: f.model}, nil
}

type captureRefiner struct {
	content string
	input   RefineInput
	model   string
}

func (f *captureRefiner) Refine(_ context.Context, input RefineInput) (RefineOutput, error) {
	f.input = input
	return RefineOutput{Content: f.content, Model: f.model}, nil
}

func contractRefineRequest(kind string, targetID string) contract.StudyPipelineRefineRequest {
	return contract.StudyPipelineRefineRequest{Kind: kind, TargetID: targetID}
}
