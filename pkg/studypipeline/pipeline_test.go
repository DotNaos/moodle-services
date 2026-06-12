package studypipeline

import (
	"context"
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
	if err != nil {
		t.Fatalf("curated stage should not call downloader: %v", err)
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
	if err != nil {
		t.Fatalf("RunStage curated: %v", err)
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
