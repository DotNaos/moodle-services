package studypipeline

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DotNaos/moodle-services/internal/moodle"
	contract "github.com/DotNaos/moodle-services/pkg/apicontracts"
)

const (
	EnvArtifactRoot          = "MOODLE_STUDY_ARTIFACT_ROOT"
	EnvCodexCommand          = "MOODLE_STUDY_CODEX_COMMAND"
	EnvCodexDockerImage      = "MOODLE_STUDY_CODEX_DOCKER_IMAGE"
	EnvCodexContainerCommand = "MOODLE_STUDY_CODEX_CONTAINER_COMMAND"
	DefaultArtifactRoot      = "/srv/moodle-study"
)

type Downloader interface {
	DownloadFileToBuffer(url string) (moodle.DownloadResult, error)
}

type RunOptions struct {
	Root        string
	Now         time.Time
	Downloader  Downloader
	UserID      string
	Refiner     ContentRefiner
	RefineEvent func(contract.StudyPipelineRefineEvent)
}

type TaskMessage struct {
	ID        string `json:"id"`
	Role      string `json:"role"`
	Text      string `json:"text"`
	CreatedAt string `json:"createdAt"`
}

type ContentRefiner interface {
	Refine(ctx context.Context, input RefineInput) (RefineOutput, error)
}

type RefineInput struct {
	ArtifactRoot    string
	CourseID        string
	UserID          string
	Kind            string
	Model           string
	ReasoningEffort string
	CustomPrompt    string
	TargetID        string
	Title           string
	Content         string
	Emit            func(contract.StudyPipelineRefineEvent)
}

type RefineOutput struct {
	Content string
	Model   string
}

var unsafePathRe = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)
var ansiEscapeRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)
var deviceAuthUserCodeRe = regexp.MustCompile(`[A-Z0-9]{4}-[A-Z0-9]{5}`)
var codexAuthMu sync.Mutex
var codexAuthProcesses = map[string]time.Time{}

type CodexDeviceAuthStart struct {
	Authenticated    bool
	VerificationURI  string
	UserCode         string
	ExpiresInSeconds int
}

func ArtifactRootFromEnv() string {
	if value := strings.TrimSpace(os.Getenv(EnvArtifactRoot)); value != "" {
		return value
	}
	return DefaultArtifactRoot
}

func RunStage(courseID string, resources []moodle.Resource, stage string, options RunOptions) (contract.StudyPipelineResponse, error) {
	stage = strings.TrimSpace(stage)
	if stage == "" {
		stage = "raw"
	}
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}
	root := strings.TrimSpace(options.Root)
	if root == "" {
		root = ArtifactRootFromEnv()
	}

	switch stage {
	case "raw":
		if err := writeRaw(root, courseID, resources, options.Downloader); err != nil {
			return contract.StudyPipelineResponse{}, err
		}
	case "extracted":
		if err := writeRaw(root, courseID, resources, options.Downloader); err != nil {
			return contract.StudyPipelineResponse{}, err
		}
		if err := writeExtracted(root, courseID, resources, options.Downloader); err != nil {
			return contract.StudyPipelineResponse{}, err
		}
	case "curated":
		if err := writeCurated(root, courseID, resources, now); err != nil {
			return contract.StudyPipelineResponse{}, err
		}
	default:
		return contract.StudyPipelineResponse{}, fmt.Errorf("unknown study pipeline stage %q", stage)
	}

	response := Build(courseID, resources, stage+"-ready", now)
	response.Stage = stage
	response.ArtifactRoot = courseDir(root, courseID)
	return response, nil
}

func Status(courseID string, resources []moodle.Resource, options RunOptions) contract.StudyPipelineResponse {
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}
	root := strings.TrimSpace(options.Root)
	if root == "" {
		root = ArtifactRootFromEnv()
	}
	stage := ""
	status := "planned"
	switch {
	case fileExists(filepath.Join(courseDir(root, courseID), "curated", "tasks", "Tasks.mdx")):
		stage = "curated"
		status = "curated-ready"
	case dirExists(filepath.Join(courseDir(root, courseID), "extracted")):
		stage = "extracted"
		status = "extracted-ready"
	case fileExists(filepath.Join(courseDir(root, courseID), "raw", "resources.json")):
		stage = "raw"
		status = "raw-ready"
	}
	response := Build(courseID, resources, status, now)
	response.Stage = stage
	response.ArtifactRoot = courseDir(root, courseID)
	return response
}

func LoadTaskView(courseID string, resources []moodle.Resource, includeScript bool, options RunOptions) (contract.StudyPipelineTaskViewResponse, error) {
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}
	root := strings.TrimSpace(options.Root)
	if root == "" {
		root = ArtifactRootFromEnv()
	}
	if err := writeCurated(root, courseID, resources, now); err != nil {
		return contract.StudyPipelineTaskViewResponse{}, err
	}

	plan := Build(courseID, resources, "served", now)
	taskLinks := effectiveTaskLinks(plan.Materials, plan.TaskLinks)
	state, _ := readTaskState(root, courseID)
	sheets := make([]contract.StudyPipelineTaskSheet, 0, len(taskLinks))
	progress := contract.StudyPipelineProgress{}

	for _, link := range taskLinks {
		taskID := taskID(link.Task)
		attempt := state.Attempts[taskID]
		status := "open"
		var latest *contract.StudyPipelineAttempt
		if attempt != nil {
			status = attempt.Status
			latest = &contract.StudyPipelineAttempt{
				UserAnswer: attempt.UserAnswer,
				Verdict:    attempt.Verdict,
			}
		}
		switch status {
		case "checked":
			progress.Checked++
		case "correct":
			progress.Checked++
			progress.Correct++
		case "wrong":
			progress.Checked++
			progress.Wrong++
		case "needs_review":
			progress.Checked++
			progress.NeedsReview++
		default:
			progress.Open++
		}

		sheet := contract.StudyPipelineTaskSheet{
			ResourceID: link.Task.ID,
			Title:      link.Task.Name,
			Kind:       "task",
			Tasks: []contract.StudyPipelineTaskItem{{
				TaskID:           taskID,
				SourceResourceID: link.Task.ID,
				Title:            link.Task.Name,
				PromptMarkdown:   taskPrompt(root, courseID, link),
				ContentState:     taskContentState(root, courseID, link),
				Parts: []contract.StudyPipelineTaskPart{{
					ID:             taskID + "-main",
					Label:          "Aufgabe",
					PromptMarkdown: taskPrompt(root, courseID, link),
				}},
				LatestAttempt: latest,
				Status:        status,
			}},
		}
		if link.Solution != nil {
			sheet.SolutionResourceID = link.Solution.ID
			sheet.SolutionTitle = link.Solution.Name
			sheet.SolutionMarkdown = solutionPrompt(root, courseID, *link.Solution)
		}
		sheets = append(sheets, sheet)
	}

	script := ""
	if includeScript {
		script = loadScriptMarkdown(root, courseID, resources)
	}

	return contract.StudyPipelineTaskViewResponse{
		CourseID:       courseID,
		GeneratedAt:    now.UTC().Format(time.RFC3339),
		Source:         "moodle-services",
		ScriptMarkdown: script,
		ScriptSections: scriptContentStates(root, courseID, plan),
		Sheets:         sheets,
		Resources:      viewResources(plan.Materials),
		Progress:       progress,
	}, nil
}

func LoadScript(courseID string, resources []moodle.Resource, options RunOptions) (string, error) {
	root := strings.TrimSpace(options.Root)
	if root == "" {
		root = ArtifactRootFromEnv()
	}
	if err := writeCurated(root, courseID, resources, options.Now); err != nil {
		return "", err
	}
	return loadScriptMarkdown(root, courseID, resources), nil
}

func RefineContent(ctx context.Context, courseID string, resources []moodle.Resource, input contract.StudyPipelineRefineRequest, options RunOptions) (contract.StudyPipelineRefineResponse, error) {
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}
	root := strings.TrimSpace(options.Root)
	if root == "" {
		root = ArtifactRootFromEnv()
	}
	if err := writeCurated(root, courseID, resources, now); err != nil {
		return contract.StudyPipelineRefineResponse{}, err
	}

	plan := Build(courseID, resources, "refine", now)
	kind := normalizeRefineKind(input.Kind)
	targetID := strings.TrimSpace(input.TargetID)
	if targetID == "" {
		return contract.StudyPipelineRefineResponse{}, fmt.Errorf("targetId is required")
	}
	material, ok := findRefineMaterial(plan, kind, targetID)
	if !ok {
		return contract.StudyPipelineRefineResponse{}, fmt.Errorf("refine target %q was not found for %s", targetID, kind)
	}
	content := extractedContentForMaterial(root, courseID, material)
	if strings.TrimSpace(content) == "" {
		return contract.StudyPipelineRefineResponse{}, fmt.Errorf("no extracted content is available for %s", material.Name)
	}
	if options.RefineEvent != nil {
		options.RefineEvent(contract.StudyPipelineRefineEvent{
			Type:            "started",
			Message:         "Preparing extracted Moodle content for Codex.",
			Model:           strings.TrimSpace(input.Model),
			ReasoningEffort: strings.TrimSpace(input.ReasoningEffort),
		})
	}
	refiner := options.Refiner
	if refiner == nil {
		refiner = DockerCodexRefiner{}
	}
	output, err := refiner.Refine(ctx, RefineInput{
		ArtifactRoot:    root,
		CourseID:        courseID,
		UserID:          options.UserID,
		Kind:            kind,
		Model:           input.Model,
		ReasoningEffort: input.ReasoningEffort,
		CustomPrompt:    input.CustomPrompt,
		TargetID:        targetID,
		Title:           material.Name,
		Content:         content,
		Emit:            options.RefineEvent,
	})
	if err != nil {
		return contract.StudyPipelineRefineResponse{}, err
	}
	if strings.TrimSpace(output.Content) == "" {
		return contract.StudyPipelineRefineResponse{}, fmt.Errorf("codex returned empty refined content")
	}
	if err := writeImprovedContent(root, courseID, material, kind, output.Content, output.Model, now); err != nil {
		return contract.StudyPipelineRefineResponse{}, err
	}
	state := contentState(root, courseID, material, kind)
	if options.RefineEvent != nil {
		options.RefineEvent(contract.StudyPipelineRefineEvent{
			Type:           "saved",
			Message:        "Codex-improved content was saved separately from the extracted source.",
			Target:         &state,
			ContentPreview: previewMarkdown(output.Content, 1200),
		})
	}
	return contract.StudyPipelineRefineResponse{
		CourseID:       courseID,
		Target:         state,
		ContentPreview: previewMarkdown(output.Content, 1200),
	}, nil
}

func RecordAttempt(root string, courseID string, taskIDValue string, attempt contract.StudyPipelineAttempt) error {
	if strings.TrimSpace(root) == "" {
		root = ArtifactRootFromEnv()
	}
	state, _ := readTaskState(root, courseID)
	if state.Attempts == nil {
		state.Attempts = map[string]*taskAttempt{}
	}
	status := "checked"
	if attempt.Verdict.IsCorrect {
		status = "correct"
	} else if strings.TrimSpace(attempt.Verdict.FeedbackMarkdown) != "" {
		status = "needs_review"
	}
	state.Attempts[taskIDValue] = &taskAttempt{
		UserAnswer: attempt.UserAnswer,
		Verdict:    attempt.Verdict,
		Status:     status,
		UpdatedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	return writeTaskState(root, courseID, state)
}

func Messages(root string, courseID string, taskIDValue string) ([]TaskMessage, error) {
	if strings.TrimSpace(root) == "" {
		root = ArtifactRootFromEnv()
	}
	state, err := readTaskState(root, courseID)
	if err != nil {
		return nil, err
	}
	return append([]TaskMessage(nil), state.Messages[taskIDValue]...), nil
}

func AppendMessage(root string, courseID string, taskIDValue string, role string, text string) ([]TaskMessage, error) {
	if strings.TrimSpace(root) == "" {
		root = ArtifactRootFromEnv()
	}
	state, _ := readTaskState(root, courseID)
	if state.Messages == nil {
		state.Messages = map[string][]TaskMessage{}
	}
	message := TaskMessage{
		ID:        fmt.Sprintf("%s-%d", safeSegment(role), time.Now().UnixNano()),
		Role:      strings.TrimSpace(role),
		Text:      strings.TrimSpace(text),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	state.Messages[taskIDValue] = append(state.Messages[taskIDValue], message)
	if err := writeTaskState(root, courseID, state); err != nil {
		return nil, err
	}
	return state.Messages[taskIDValue], nil
}

func writeRaw(root string, courseID string, resources []moodle.Resource, downloader Downloader) error {
	dir := filepath.Join(courseDir(root, courseID), "raw")
	if err := os.MkdirAll(filepath.Join(dir, "materials"), 0o755); err != nil {
		return err
	}
	if err := writeJSONFile(filepath.Join(dir, "resources.json"), resources); err != nil {
		return err
	}
	index := Build(courseID, resources, "raw-indexed", time.Now())
	if err := writeJSONFile(filepath.Join(dir, "materials.index.json"), index); err != nil {
		return err
	}
	if downloader == nil {
		return nil
	}
	for _, resource := range resources {
		if strings.TrimSpace(resource.URL) == "" || resource.Type == "folder" {
			continue
		}
		download, err := downloader.DownloadFileToBuffer(resource.URL)
		if err != nil {
			continue
		}
		name := resourceFileName(resource)
		path := filepath.Join(dir, "materials", safeSegment(resource.SectionName), name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, download.Data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func writeExtracted(root string, courseID string, resources []moodle.Resource, downloader Downloader) error {
	dir := filepath.Join(courseDir(root, courseID), "extracted")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, resource := range resources {
		kind := classify(resource)
		targetDir := filepath.Join(dir, kind+"s")
		if kind == "other" {
			targetDir = filepath.Join(dir, "other")
		}
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			return err
		}
		text := extractedPlaceholder(courseID, resource)
		if downloader != nil && strings.TrimSpace(resource.URL) != "" && resource.Type != "folder" {
			if download, err := downloader.DownloadFileToBuffer(resource.URL); err == nil {
				text = extractedText(resource, download)
			}
		}
		path := filepath.Join(targetDir, safeSegment(resource.ID+"-"+resource.Name)+".mdx")
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func writeCurated(root string, courseID string, resources []moodle.Resource, now time.Time) error {
	if now.IsZero() {
		now = time.Now()
	}
	dir := filepath.Join(courseDir(root, courseID), "curated")
	if err := os.RemoveAll(filepath.Join(dir, "script")); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(dir, "tasks")); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "script"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "tasks", "solutions"), 0o755); err != nil {
		return err
	}
	plan := Build(courseID, resources, "curated", now)
	if err := os.WriteFile(filepath.Join(dir, "script", "Script.mdx"), []byte(buildScript(root, courseID, plan, now)), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "tasks", "Tasks.mdx"), []byte(buildTasksIndex(courseID, plan, now)), 0o644); err != nil {
		return err
	}
	for _, link := range effectiveTaskLinks(plan.Materials, plan.TaskLinks) {
		taskPath := filepath.Join(dir, "tasks", safeSegment(taskID(link.Task))+".mdx")
		if err := os.WriteFile(taskPath, []byte(taskPrompt(root, courseID, link)), 0o644); err != nil {
			return err
		}
		if link.Solution != nil {
			solutionPath := filepath.Join(dir, "tasks", "solutions", safeSegment(taskID(*link.Solution))+".mdx")
			if err := os.WriteFile(solutionPath, []byte(solutionPrompt(root, courseID, *link.Solution)), 0o644); err != nil {
				return err
			}
		}
	}
	if err := runCodexCleanupHook(courseID, dir); err != nil {
		return err
	}
	return nil
}

func runCodexCleanupHook(courseID string, curatedDir string) error {
	command := strings.TrimSpace(os.Getenv(EnvCodexCommand))
	if command == "" {
		return nil
	}
	cmd := exec.Command("sh", "-lc", command)
	cmd.Env = append(os.Environ(),
		"MOODLE_STUDY_COURSE_ID="+courseID,
		"MOODLE_STUDY_CURATED_DIR="+curatedDir,
		"MOODLE_STUDY_SCRIPT_PATH="+filepath.Join(curatedDir, "script", "Script.mdx"),
		"MOODLE_STUDY_TASKS_DIR="+filepath.Join(curatedDir, "tasks"),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("codex cleanup hook failed: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func extractedText(resource moodle.Resource, download moodle.DownloadResult) string {
	isPDF := strings.EqualFold(resource.FileType, "pdf") || strings.Contains(strings.ToLower(download.ContentType), "pdf")
	if isPDF {
		if extracted, err := moodle.ExtractPDFText(download.Data); err == nil {
			return frontmatter("extracted", resource, time.Now()) + "\n\n" + strings.TrimSpace(extracted) + "\n"
		}
	}
	text := ""
	if !isPDF {
		text = strings.TrimSpace(string(download.Data))
	}
	if text == "" {
		text = fmt.Sprintf("No text could be extracted from %s.", resource.Name)
	}
	return frontmatter("extracted", resource, time.Now()) + "\n\n" + text + "\n"
}

func extractedPlaceholder(courseID string, resource moodle.Resource) string {
	return frontmatter("extracted-placeholder", resource, time.Now()) + "\n\n" + strings.Join([]string{
		"# " + resource.Name,
		"",
		"This extracted file was created without a downloader in the current runtime.",
		"",
		"- Course: " + courseID,
		"- Resource: " + resource.ID,
		"- Type: " + classify(resource),
	}, "\n") + "\n"
}

func buildScript(root string, courseID string, plan contract.StudyPipelineResponse, now time.Time) string {
	var out strings.Builder
	out.WriteString("---\n")
	out.WriteString("status: server-curated-from-extracted\n")
	out.WriteString("ai_used: false\n")
	out.WriteString("codex_agent_status: pending-explicit-cleanup\n")
	out.WriteString("course_id: \"" + courseID + "\"\n")
	out.WriteString("generated_at: \"" + now.UTC().Format(time.RFC3339) + "\"\n")
	out.WriteString("---\n\n")
	out.WriteString("# Course Script\n\n")
	for _, material := range plan.Materials {
		if material.Type != "slide" && material.Type != "script" {
			continue
		}
		out.WriteString("## " + material.Name + "\n\n")
		out.WriteString("Source: [Moodle resource](moodle-resource:" + material.ID + ")\n\n")
		content := displayContentForMaterial(root, courseID, material, "script-section")
		if content == "" {
			out.WriteString("No extracted text was available for this Moodle resource.\n\n")
			continue
		}
		out.WriteString(content)
		out.WriteString("\n\n")
	}
	if out.Len() == 0 {
		out.WriteString("No script material was detected yet.\n")
	}
	return out.String()
}

func buildTasksIndex(courseID string, plan contract.StudyPipelineResponse, now time.Time) string {
	var out strings.Builder
	out.WriteString("---\n")
	out.WriteString("status: server-task-pass-1\n")
	out.WriteString("ai_used: false\n")
	out.WriteString("codex_agent_status: pending-explicit-cleanup\n")
	out.WriteString("course_id: \"" + courseID + "\"\n")
	out.WriteString("generated_at: \"" + now.UTC().Format(time.RFC3339) + "\"\n")
	out.WriteString("---\n\n")
	out.WriteString("# Tasks\n\n")
	for _, link := range effectiveTaskLinks(plan.Materials, plan.TaskLinks) {
		out.WriteString("- [" + link.Task.Name + "](" + safeSegment(taskID(link.Task)) + ".mdx)")
		if link.Solution != nil {
			out.WriteString(" - solution linked")
		} else {
			out.WriteString(" - solution missing")
		}
		out.WriteString("\n")
	}
	return out.String()
}

func taskPrompt(root string, courseID string, link contract.StudyPipelineTaskLink) string {
	content := displayContentForMaterial(root, courseID, link.Task, "task")
	state := contentState(root, courseID, link.Task, "task")
	lines := []string{
		"---",
		"status: " + state.Status,
		"ai_used: " + fmt.Sprintf("%t", state.Status == "codex-improved"),
		"course_id: \"" + courseID + "\"",
		"source_task: \"" + link.Task.ID + "\"",
		"---",
		"",
		"# " + link.Task.Name,
		"",
		"Source: [Moodle resource](moodle-resource:" + link.Task.ID + ")",
		"",
	}
	if content != "" {
		lines = append(lines, content)
	} else {
		lines = append(lines, "No extracted task text was available for this Moodle resource.")
	}
	if link.Solution != nil {
		lines = append(lines, "", "Linked solution: [moodle-resource:"+link.Solution.ID+"]")
	} else {
		lines = append(lines, "", "Solution status: missing in Moodle or not detected.")
	}
	return strings.Join(lines, "\n") + "\n"
}

func taskContentState(root string, courseID string, link contract.StudyPipelineTaskLink) contract.StudyPipelineContentRef {
	return contentState(root, courseID, link.Task, "task")
}

func effectiveTaskLinks(materials []contract.StudyPipelineMaterial, links []contract.StudyPipelineTaskLink) []contract.StudyPipelineTaskLink {
	if len(links) > 0 {
		return links
	}
	return nil
}

func solutionPrompt(root string, courseID string, resource contract.StudyPipelineMaterial) string {
	content := extractedContentForMaterial(root, courseID, resource)
	return strings.Join([]string{
		"---",
		"status: solution-from-extracted",
		"ai_used: false",
		"course_id: \"" + courseID + "\"",
		"source_solution: \"" + resource.ID + "\"",
		"---",
		"",
		"# " + resource.Name,
		"",
		"Source: [Moodle resource](moodle-resource:" + resource.ID + ")",
		"",
		firstNonEmpty(content, "No extracted solution text was available for this Moodle resource."),
	}, "\n") + "\n"
}

func scriptContentStates(root string, courseID string, plan contract.StudyPipelineResponse) []contract.StudyPipelineContentRef {
	states := []contract.StudyPipelineContentRef{}
	for _, material := range plan.Materials {
		if material.Type != "slide" && material.Type != "script" {
			continue
		}
		states = append(states, contentState(root, courseID, material, "script-section"))
	}
	return states
}

func displayContentForMaterial(root string, courseID string, material contract.StudyPipelineMaterial, kind string) string {
	if content := improvedContentForMaterial(root, courseID, material, kind); strings.TrimSpace(content) != "" {
		return content
	}
	return extractedContentForMaterial(root, courseID, material)
}

func improvedContentForMaterial(root string, courseID string, material contract.StudyPipelineMaterial, kind string) string {
	path := improvedPathForMaterial(root, courseID, material, kind)
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return stripFrontmatter(strings.TrimSpace(string(data)))
}

func writeImprovedContent(root string, courseID string, material contract.StudyPipelineMaterial, kind string, content string, model string, now time.Time) error {
	path := improvedPathForMaterial(root, courseID, material, kind)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	lines := []string{
		"---",
		"status: codex-improved",
		"ai_used: true",
		"course_id: \"" + courseID + "\"",
		"kind: \"" + kind + "\"",
		"target_id: \"" + material.ID + "\"",
		"model: \"" + strings.ReplaceAll(model, "\"", "\\\"") + "\"",
		"generated_at: \"" + now.UTC().Format(time.RFC3339) + "\"",
		"---",
		"",
		strings.TrimSpace(content),
		"",
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}

func contentState(root string, courseID string, material contract.StudyPipelineMaterial, kind string) contract.StudyPipelineContentRef {
	state := contract.StudyPipelineContentRef{
		ID:          material.ID,
		Kind:        kind,
		Title:       material.Name,
		Status:      "machine-extracted",
		StatusLabel: "Machine extracted",
		SourcePath:  filepath.ToSlash(extractedPathForMaterial(root, courseID, material)),
	}
	if metadata, ok := improvedMetadata(root, courseID, material, kind); ok {
		state.Status = "codex-improved"
		state.StatusLabel = "Codex improved"
		state.Model = metadata.Model
		state.UpdatedAt = metadata.GeneratedAt
		state.SourcePath = filepath.ToSlash(improvedPathForMaterial(root, courseID, material, kind))
	}
	return state
}

type improvedFileMetadata struct {
	Model       string
	GeneratedAt string
}

func improvedMetadata(root string, courseID string, material contract.StudyPipelineMaterial, kind string) (improvedFileMetadata, bool) {
	data, err := os.ReadFile(improvedPathForMaterial(root, courseID, material, kind))
	if err != nil {
		return improvedFileMetadata{}, false
	}
	frontmatter := frontmatterBlock(string(data))
	if frontmatter == "" {
		return improvedFileMetadata{}, true
	}
	return improvedFileMetadata{
		Model:       frontmatterValue(frontmatter, "model"),
		GeneratedAt: frontmatterValue(frontmatter, "generated_at"),
	}, true
}

func improvedPathForMaterial(root string, courseID string, material contract.StudyPipelineMaterial, kind string) string {
	dirName := "script"
	if kind == "task" {
		dirName = "tasks"
	}
	return filepath.Join(courseDir(root, courseID), "improved", dirName, safeSegment(material.ID+"-"+material.Name)+".mdx")
}

func normalizeRefineKind(kind string) string {
	switch strings.TrimSpace(kind) {
	case "task":
		return "task"
	default:
		return "script-section"
	}
}

func findRefineMaterial(plan contract.StudyPipelineResponse, kind string, targetID string) (contract.StudyPipelineMaterial, bool) {
	if kind == "task" {
		for _, link := range effectiveTaskLinks(plan.Materials, plan.TaskLinks) {
			if link.Task.ID == targetID || taskID(link.Task) == targetID {
				return link.Task, true
			}
		}
		return contract.StudyPipelineMaterial{}, false
	}
	for _, material := range plan.Materials {
		if (material.Type == "slide" || material.Type == "script") && material.ID == targetID {
			return material, true
		}
	}
	return contract.StudyPipelineMaterial{}, false
}

func extractedContentForMaterial(root string, courseID string, material contract.StudyPipelineMaterial) string {
	path := extractedPathForMaterial(root, courseID, material)
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return stripFrontmatter(strings.TrimSpace(string(data)))
}

func extractedPathForMaterial(root string, courseID string, material contract.StudyPipelineMaterial) string {
	dirName := material.Type + "s"
	if material.Type == "other" {
		dirName = "other"
	}
	return filepath.Join(courseDir(root, courseID), "extracted", dirName, safeSegment(material.ID+"-"+material.Name)+".mdx")
}

type DockerCodexRefiner struct{}

func (DockerCodexRefiner) Refine(ctx context.Context, input RefineInput) (RefineOutput, error) {
	image := strings.TrimSpace(os.Getenv(EnvCodexDockerImage))
	if image == "" {
		return RefineOutput{}, fmt.Errorf("%s is not configured", EnvCodexDockerImage)
	}
	model := sanitizeCodexModel(input.Model)
	if model == "" {
		return RefineOutput{}, fmt.Errorf("codex model is required; load /api/codex/models and pass one of the returned model ids")
	}
	command := strings.TrimSpace(os.Getenv(EnvCodexContainerCommand))
	if command == "" {
		reasoningConfig := ""
		if effort := sanitizeCodexOption(input.ReasoningEffort); effort != "" {
			reasoningConfig = ` -c 'model_reasoning_effort="` + effort + `"'`
		}
		command = `codex exec --json --skip-git-repo-check --sandbox read-only --model "$CODEX_MODEL"` + reasoningConfig + ` --output-last-message "$CODEX_OUTPUT_FILE" -`
	}
	prompt := buildRefinePrompt(input)
	if input.Emit != nil {
		input.Emit(contract.StudyPipelineRefineEvent{
			Type:            "runner",
			Message:         "Starting Codex in the per-user Docker runner.",
			Model:           model,
			ReasoningEffort: sanitizeCodexOption(input.ReasoningEffort),
		})
	}
	output, err := runDockerCodex(ctx, image, command, model, input.ReasoningEffort, input.ArtifactRoot, input.UserID, prompt, input.Emit)
	if err != nil {
		return RefineOutput{}, fmt.Errorf("codex refinement failed for model %s: %w", model, err)
	}
	if strings.TrimSpace(output) == "" {
		return RefineOutput{}, fmt.Errorf("codex refinement failed for model %s: empty response", model)
	}
	return RefineOutput{Content: strings.TrimSpace(output), Model: model}, nil
}

func CodexModelCatalog(ctx context.Context, userID string, root string) (contract.CodexModelCatalogResponse, error) {
	image := strings.TrimSpace(os.Getenv(EnvCodexDockerImage))
	if image == "" {
		return contract.CodexModelCatalogResponse{}, fmt.Errorf("%s is not configured", EnvCodexDockerImage)
	}
	output, err := runDockerCodex(ctx, image, "codex debug models", "", "", firstNonEmpty(root, ArtifactRootFromEnv()), userID, "", nil)
	if err != nil {
		return contract.CodexModelCatalogResponse{}, err
	}
	return contract.CodexModelCatalogResponse{Models: parseCodexModels(output)}, nil
}

func CodexAuthenticated(ctx context.Context, userID string, root string) (bool, string, error) {
	image := strings.TrimSpace(os.Getenv(EnvCodexDockerImage))
	if image == "" {
		return false, "", fmt.Errorf("%s is not configured", EnvCodexDockerImage)
	}
	output, err := runDockerCodex(ctx, image, "codex login status", "", "", firstNonEmpty(root, ArtifactRootFromEnv()), userID, "", nil)
	if err == nil {
		return true, strings.TrimSpace(output), nil
	}
	if strings.Contains(err.Error(), "Not logged in") || strings.Contains(output, "Not logged in") {
		return false, "Not logged in", nil
	}
	return false, strings.TrimSpace(output), nil
}

func CodexLogout(ctx context.Context, userID string, root string) error {
	image := strings.TrimSpace(os.Getenv(EnvCodexDockerImage))
	if image == "" {
		return fmt.Errorf("%s is not configured", EnvCodexDockerImage)
	}
	_, err := runDockerCodex(ctx, image, "codex logout", "", "", firstNonEmpty(root, ArtifactRootFromEnv()), userID, "", nil)
	if err != nil && !strings.Contains(err.Error(), "Not logged in") {
		return err
	}
	return nil
}

func StartCodexDeviceAuth(ctx context.Context, userID string, root string) (CodexDeviceAuthStart, error) {
	authenticated, _, err := CodexAuthenticated(ctx, userID, root)
	if err != nil {
		return CodexDeviceAuthStart{}, err
	}
	if authenticated {
		return CodexDeviceAuthStart{Authenticated: true}, nil
	}

	image := strings.TrimSpace(os.Getenv(EnvCodexDockerImage))
	if image == "" {
		return CodexDeviceAuthStart{}, fmt.Errorf("%s is not configured", EnvCodexDockerImage)
	}
	stateRoot, err := prepareCodexStateRoot(firstNonEmpty(root, ArtifactRootFromEnv()), userID)
	if err != nil {
		return CodexDeviceAuthStart{}, err
	}
	authKey := safeSegment(firstNonEmpty(userID, "anonymous"))
	if !claimCodexAuthProcess(authKey) {
		return CodexDeviceAuthStart{}, fmt.Errorf("ChatGPT sign-in is already running for this user")
	}

	loginCtx, cancel := context.WithTimeout(context.Background(), 16*time.Minute)
	args := []string{
		"run", "--rm", "-i",
		"--user", "0:0",
		"-e", "HOME=/home/codex",
		"-e", "CODEX_HOME=/home/codex/.codex",
		"-v", dockerHostMountPath(stateRoot) + ":/home/codex/.codex",
		image,
		"codex", "login", "--device-auth",
	}
	cmd := exec.CommandContext(loginCtx, "docker", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		releaseCodexAuthProcess(authKey)
		return CodexDeviceAuthStart{}, err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		cancel()
		releaseCodexAuthProcess(authKey)
		return CodexDeviceAuthStart{}, err
	}

	resultCh := make(chan CodexDeviceAuthStart, 1)
	errCh := make(chan error, 1)
	go func() {
		defer releaseCodexAuthProcess(authKey)
		defer cancel()
		start, parseErr := readCodexDeviceAuthStart(stdout)
		if parseErr != nil {
			errCh <- parseErr
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return
		}
		resultCh <- start
		_ = cmd.Wait()
	}()

	select {
	case start := <-resultCh:
		return start, nil
	case err := <-errCh:
		return CodexDeviceAuthStart{}, err
	case <-time.After(8 * time.Second):
		_ = cmd.Process.Kill()
		return CodexDeviceAuthStart{}, fmt.Errorf("ChatGPT sign-in did not produce a device code")
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		return CodexDeviceAuthStart{}, ctx.Err()
	}
}

func claimCodexAuthProcess(key string) bool {
	codexAuthMu.Lock()
	defer codexAuthMu.Unlock()
	if startedAt, ok := codexAuthProcesses[key]; ok && time.Since(startedAt) < 16*time.Minute {
		return false
	}
	codexAuthProcesses[key] = time.Now()
	return true
}

func releaseCodexAuthProcess(key string) {
	codexAuthMu.Lock()
	defer codexAuthMu.Unlock()
	delete(codexAuthProcesses, key)
}

func readCodexDeviceAuthStart(reader io.Reader) (CodexDeviceAuthStart, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var seen strings.Builder
	var verificationURI string
	var userCode string
	for scanner.Scan() {
		clean := strings.TrimSpace(ansiEscapeRe.ReplaceAllString(scanner.Text(), ""))
		if clean == "" {
			continue
		}
		seen.WriteString(clean)
		seen.WriteByte('\n')
		if strings.HasPrefix(clean, "http://") || strings.HasPrefix(clean, "https://") {
			verificationURI = strings.Fields(clean)[0]
		}
		if code := deviceAuthUserCodeRe.FindString(clean); code != "" {
			userCode = code
		}
		if verificationURI != "" && userCode != "" {
			return CodexDeviceAuthStart{VerificationURI: verificationURI, UserCode: userCode, ExpiresInSeconds: 15 * 60}, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return CodexDeviceAuthStart{}, err
	}
	return CodexDeviceAuthStart{}, fmt.Errorf("could not read ChatGPT device code from Codex login output: %s", compactProcessOutput(seen.String()))
}

func sanitizeCodexModel(value string) string {
	model := strings.TrimSpace(value)
	if model == "" || len([]rune(model)) > 160 || strings.ContainsAny(model, "\r\n\t") {
		return ""
	}
	return model
}

func sanitizeCodexOption(value string) string {
	option := strings.TrimSpace(value)
	if option == "" || len([]rune(option)) > 80 || strings.ContainsAny(option, "\r\n\t") || unsafePathRe.MatchString(option) {
		return ""
	}
	return option
}

func parseCodexModels(value string) []contract.CodexModelOption {
	var payload struct {
		Models []map[string]any `json:"models"`
	}
	if err := json.Unmarshal([]byte(value), &payload); err != nil {
		return nil
	}
	models := []contract.CodexModelOption{}
	for _, item := range payload.Models {
		id := stringValue(item["slug"])
		label := firstNonEmpty(stringValue(item["display_name"]), id)
		if id == "" || label == "" {
			continue
		}
		models = append(models, contract.CodexModelOption{
			ID:                     id,
			Label:                  label,
			Description:            stringValue(item["description"]),
			DefaultReasoningEffort: stringValue(item["default_reasoning_level"]),
			ReasoningEfforts:       parseReasoningEfforts(item["supported_reasoning_levels"]),
			SpeedTiers:             stringSliceValue(item["additional_speed_tiers"]),
		})
	}
	return models
}

func parseReasoningEfforts(value any) []contract.CodexReasoningOption {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	efforts := []contract.CodexReasoningOption{}
	for _, item := range items {
		record, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id := stringValue(record["effort"])
		if id == "" {
			continue
		}
		efforts = append(efforts, contract.CodexReasoningOption{
			ID:          id,
			Label:       reasoningEffortLabel(id),
			Description: stringValue(record["description"]),
		})
	}
	return efforts
}

func reasoningEffortLabel(value string) string {
	if value == "xhigh" {
		return "XHigh"
	}
	if value == "" {
		return ""
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func stringValue(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func stringSliceValue(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	values := []string{}
	for _, item := range items {
		text := stringValue(item)
		if text != "" {
			values = append(values, text)
		}
	}
	return values
}

func runDockerCodex(ctx context.Context, image string, command string, model string, reasoningEffort string, artifactRoot string, userID string, prompt string, emit func(contract.StudyPipelineRefineEvent)) (string, error) {
	return runDockerCodexWithOptions(ctx, dockerCodexOptions{
		Image:           image,
		Command:         command,
		Model:           model,
		ReasoningEffort: reasoningEffort,
		ArtifactRoot:    artifactRoot,
		UserID:          userID,
		Prompt:          prompt,
		OutputPrefix:    "refine",
		Emit:            emit,
	})
}

type dockerCodexOptions struct {
	Image           string
	Command         string
	Model           string
	ReasoningEffort string
	ArtifactRoot    string
	UserID          string
	Prompt          string
	OutputPrefix    string
	OutputSchema    []byte
	Emit            func(contract.StudyPipelineRefineEvent)
}

func runDockerCodexWithOptions(ctx context.Context, options dockerCodexOptions) (string, error) {
	stateRoot, err := prepareCodexStateRoot(firstNonEmpty(options.ArtifactRoot, ArtifactRootFromEnv()), options.UserID)
	if err != nil {
		return "", err
	}
	outputPrefix := safeSegment(firstNonEmpty(options.OutputPrefix, "codex"))
	outputPath := filepath.Join(stateRoot, "last-"+outputPrefix+"-"+safeSegment(firstNonEmpty(options.Model, "default"))+".md")
	schemaPath := filepath.Join(stateRoot, "last-"+outputPrefix+"-schema.json")
	if options.Prompt != "" {
		_ = os.Remove(outputPath)
	}
	if len(options.OutputSchema) > 0 {
		if err := os.WriteFile(schemaPath, options.OutputSchema, 0o600); err != nil {
			return "", err
		}
	} else {
		_ = os.Remove(schemaPath)
	}
	args := []string{
		"run", "--rm", "-i",
		"--user", "0:0",
		"-e", "CODEX_MODEL=" + options.Model,
		"-e", "CODEX_REASONING_EFFORT=" + sanitizeCodexOption(options.ReasoningEffort),
		"-e", "CODEX_OUTPUT_FILE=/home/codex/.codex/" + filepath.Base(outputPath),
		"-e", "CODEX_OUTPUT_SCHEMA_FILE=/home/codex/.codex/" + filepath.Base(schemaPath),
		"-e", "HOME=/home/codex",
		"-e", "CODEX_HOME=/home/codex/.codex",
		"-v", dockerHostMountPath(stateRoot) + ":/home/codex/.codex",
		options.Image,
		"sh", "-lc", options.Command,
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdin = strings.NewReader(options.Prompt)
	text, err := runCommandWithOptionalEvents(cmd, options.Emit)
	if fileOutput := readOptionalOutputFile(outputPath); fileOutput != "" {
		if err != nil {
			return fileOutput, fmt.Errorf("%w (%s)", err, compactProcessOutput(text))
		}
		return fileOutput, nil
	}
	if err != nil {
		return "", fmt.Errorf("%w (%s)", err, compactProcessOutput(text))
	}
	return text, nil
}

func prepareCodexStateRoot(artifactRoot string, userID string) (string, error) {
	userSegment := safeSegment(firstNonEmpty(userID, "anonymous"))
	stateRoot := filepath.Join(firstNonEmpty(artifactRoot, ArtifactRootFromEnv()), "codex-users", userSegment)
	if err := os.MkdirAll(stateRoot, 0o700); err != nil {
		return "", err
	}
	_ = os.Chown(stateRoot, 10001, 10001)
	_ = os.Chmod(stateRoot, 0o700)
	return stateRoot, nil
}

func runCommandWithOptionalEvents(cmd *exec.Cmd, emit func(contract.StudyPipelineRefineEvent)) (string, error) {
	if emit == nil {
		output, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(output)), err
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}

	lines := make(chan string, 32)
	done := make(chan struct{}, 2)
	scan := func(prefix string, reader io.Reader) {
		defer func() { done <- struct{}{} }()
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			if prefix != "" {
				line = prefix + line
			}
			lines <- line
		}
	}
	go scan("", stdout)
	go scan("stderr: ", stderr)
	go func() {
		<-done
		<-done
		close(lines)
	}()

	var output strings.Builder
	for line := range lines {
		output.WriteString(line)
		output.WriteByte('\n')
		emitCodexLineEvent(line, emit)
	}
	err = cmd.Wait()
	return strings.TrimSpace(output.String()), err
}

func readOptionalOutputFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func emitCodexLineEvent(line string, emit func(contract.StudyPipelineRefineEvent)) {
	if emit == nil {
		return
	}
	cleanLine := strings.TrimPrefix(line, "stderr: ")
	var event map[string]any
	if err := json.Unmarshal([]byte(cleanLine), &event); err == nil {
		refineEvent := contract.StudyPipelineRefineEvent{
			Type:    "codex",
			Message: codexEventMessage(event),
		}
		if category, title, status, id := classifyCodexToolEvent(event); category == "tool" {
			refineEvent.Category = "tool"
			refineEvent.ToolTitle = title
			refineEvent.ToolStatus = status
			refineEvent.ToolID = id
		} else {
			refineEvent.Category = "status"
		}
		emit(refineEvent)
		return
	}
	if strings.Contains(line, "ERROR") || strings.Contains(line, "Reconnecting") {
		emit(contract.StudyPipelineRefineEvent{
			Type:     "codex",
			Category: "status",
			Message:  humanCodexProcessMessage(line),
		})
	}
}

// classifyCodexToolEvent inspects a raw Codex `exec --json` event. If it
// represents a surfaced tool call (shell command, MCP tool, or web search) it
// returns category "tool" with a human title, normalized status and the item
// id. Everything else (session/turn lifecycle, reasoning, file changes, todo
// lists) is category "status".
func classifyCodexToolEvent(event map[string]any) (category, title, status, id string) {
	switch stringValue(event["type"]) {
	case "item.started", "item.updated", "item.completed":
	default:
		return "status", "", "", ""
	}
	item, ok := event["item"].(map[string]any)
	if !ok {
		return "status", "", "", ""
	}
	id = stringValue(item["id"])
	eventType := stringValue(event["type"])
	itemStatus := normalizeCodexToolStatus(stringValue(item["status"]), eventType)
	switch stringValue(item["type"]) {
	case "command_execution":
		command := compactCodexCommand(stringValue(item["command"]))
		if command == "" {
			command = "Shell command"
		}
		return "tool", command, itemStatus, id
	case "mcp_tool_call":
		name := strings.Trim(stringValue(item["server"])+"."+stringValue(item["tool"]), ".")
		if name == "" {
			name = "MCP tool"
		}
		return "tool", name, itemStatus, id
	case "web_search":
		// web_search items carry no status field; derive it from the wrapper.
		query := stringValue(item["query"])
		searchTitle := "Web search"
		if query != "" {
			searchTitle = "Web search: " + query
		}
		return "tool", searchTitle, normalizeCodexToolStatus("", eventType), id
	default:
		return "status", "", "", ""
	}
}

func normalizeCodexToolStatus(itemStatus, eventType string) string {
	switch itemStatus {
	case "completed":
		return "completed"
	case "failed":
		return "failed"
	case "in_progress":
		return "running"
	}
	if eventType == "item.completed" {
		return "completed"
	}
	return "running"
}

func compactCodexCommand(command string) string {
	command = strings.Join(strings.Fields(command), " ")
	const limit = 72
	if len([]rune(command)) <= limit {
		return command
	}
	return string([]rune(command)[:limit-3]) + "..."
}

func codexEventMessage(event map[string]any) string {
	eventType := stringValue(event["type"])
	switch eventType {
	case "thread.started":
		return "Codex session started."
	case "turn.started":
		return "Codex is reading the extracted content."
	case "turn.failed":
		if details, ok := event["error"].(map[string]any); ok {
			return humanCodexProcessMessage(stringValue(details["message"]))
		}
		return "Codex refinement failed."
	case "error":
		return humanCodexProcessMessage(stringValue(event["message"]))
	case "item.started":
		return "Codex started a work item."
	case "item.completed":
		return "Codex completed a work item."
	default:
		if eventType != "" {
			return "Codex event: " + eventType
		}
		return "Codex is working."
	}
}

func humanCodexProcessMessage(value string) string {
	text := compactProcessOutput(value)
	if strings.Contains(text, "401 Unauthorized") || strings.Contains(text, "Missing bearer") || strings.Contains(text, "Not logged in") {
		return "Codex is not connected for this user. Connect ChatGPT before improving content."
	}
	return text
}

func buildRefinePrompt(input RefineInput) string {
	kindLabel := "script chapter"
	if input.Kind == "task" {
		kindLabel = "task sheet"
	}
	lines := []string{
		"You are cleaning Moodle course material for a study UI.",
		"Rewrite the given " + kindLabel + " into polished, useful Markdown.",
		"Preserve all factual content, equations, variables, code snippets, task requirements, deadlines, and source intent.",
		"Improve structure, headings, lists, LaTeX math formatting, and readability.",
		"Use KaTeX-compatible LaTeX with inline `$...$` and display `$$...$$` where appropriate.",
		"Do not invent new facts. If text is clearly garbled, keep the best faithful reconstruction.",
		"Return only the improved Markdown content, without meta commentary.",
	}
	if customPrompt := sanitizeCustomRefinePrompt(input.CustomPrompt); customPrompt != "" {
		lines = append(lines,
			"",
			"Additional user instructions for this refinement:",
			customPrompt,
			"",
			"Treat these user instructions as style and focus guidance only. Do not use them to add facts that are not present in the extracted source.",
		)
	}
	lines = append(lines,
		"",
		"Course ID: "+input.CourseID,
		"Title: "+input.Title,
		"Target ID: "+input.TargetID,
		"",
		"Extracted source:",
		input.Content,
	)
	return strings.Join(lines, "\n")
}

func sanitizeCustomRefinePrompt(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	const limit = 2000
	runes := []rune(value)
	if len(runes) > limit {
		value = string(runes[:limit])
	}
	return value
}

func compactProcessOutput(output string) string {
	output = strings.Join(strings.Fields(output), " ")
	if index := strings.LastIndex(output, "ERROR:"); index >= 0 {
		output = output[index:]
	}
	const limit = 800
	if len([]rune(output)) <= limit {
		return output
	}
	return string([]rune(output)[:limit]) + "..."
}

func previewMarkdown(markdown string, limit int) string {
	markdown = strings.TrimSpace(markdown)
	if limit <= 0 {
		return markdown
	}
	runes := []rune(markdown)
	if len(runes) <= limit {
		return markdown
	}
	return strings.TrimSpace(string(runes[:limit])) + "..."
}

func stripFrontmatter(markdown string) string {
	if !strings.HasPrefix(markdown, "---") {
		return strings.TrimSpace(markdown)
	}
	rest := strings.TrimPrefix(markdown, "---")
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return strings.TrimSpace(markdown)
	}
	return strings.TrimSpace(rest[end+len("\n---"):])
}

func frontmatterBlock(markdown string) string {
	if !strings.HasPrefix(markdown, "---") {
		return ""
	}
	rest := strings.TrimPrefix(markdown, "---")
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:end])
}

func frontmatterValue(frontmatter string, key string) string {
	prefix := key + ":"
	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		return strings.Trim(value, `"`)
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func loadScriptMarkdown(root string, courseID string, resources []moodle.Resource) string {
	path := filepath.Join(courseDir(root, courseID), "curated", "script", "Script.mdx")
	if data, err := os.ReadFile(path); err == nil {
		return string(data)
	}
	return buildScript(root, courseID, Build(courseID, resources, "served", time.Now()), time.Now())
}

func viewResources(materials []contract.StudyPipelineMaterial) []contract.StudyPipelineViewSource {
	out := make([]contract.StudyPipelineViewSource, 0, len(materials))
	for _, material := range materials {
		out = append(out, contract.StudyPipelineViewSource{
			ResourceID: material.ID,
			Title:      material.Name,
			Kind:       material.Type,
		})
	}
	return out
}

type taskState struct {
	Attempts map[string]*taskAttempt  `json:"attempts"`
	Messages map[string][]TaskMessage `json:"messages"`
}

type taskAttempt struct {
	UserAnswer string                        `json:"userAnswer"`
	Verdict    contract.StudyPipelineVerdict `json:"verdict"`
	Status     string                        `json:"status"`
	UpdatedAt  string                        `json:"updatedAt"`
}

func readTaskState(root string, courseID string) (taskState, error) {
	state := taskState{
		Attempts: map[string]*taskAttempt{},
		Messages: map[string][]TaskMessage{},
	}
	data, err := os.ReadFile(statePath(root, courseID))
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return taskState{Attempts: map[string]*taskAttempt{}, Messages: map[string][]TaskMessage{}}, err
	}
	if state.Attempts == nil {
		state.Attempts = map[string]*taskAttempt{}
	}
	if state.Messages == nil {
		state.Messages = map[string][]TaskMessage{}
	}
	return state, nil
}

func writeTaskState(root string, courseID string, state taskState) error {
	path := statePath(root, courseID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeJSONFile(path, state)
}

func statePath(root string, courseID string) string {
	return filepath.Join(courseDir(root, courseID), "curated", "task-state.json")
}

func frontmatter(status string, resource moodle.Resource, now time.Time) string {
	return strings.Join([]string{
		"---",
		"status: " + status,
		"resource_id: \"" + resource.ID + "\"",
		"title: \"" + strings.ReplaceAll(resource.Name, "\"", "\\\"") + "\"",
		"resource_type: \"" + classify(resource) + "\"",
		"generated_at: \"" + now.UTC().Format(time.RFC3339) + "\"",
		"---",
	}, "\n")
}

func courseDir(root string, courseID string) string {
	return filepath.Join(root, "courses", safeSegment(courseID))
}

func safeSegment(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.ReplaceAll(value, "ä", "ae")
	value = strings.ReplaceAll(value, "ö", "oe")
	value = strings.ReplaceAll(value, "ü", "ue")
	value = strings.ReplaceAll(value, "ß", "ss")
	value = unsafePathRe.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-._")
	if value == "" {
		return "untitled"
	}
	if len(value) > 96 {
		value = value[:96]
	}
	return value
}

func resourceFileName(resource moodle.Resource) string {
	ext := strings.Trim(strings.ToLower(resource.FileType), ".")
	if ext == "" {
		ext = "bin"
	}
	return safeSegment(resource.ID+"-"+resource.Name) + "." + ext
}

func taskID(material contract.StudyPipelineMaterial) string {
	return "task-" + safeSegment(material.ID+"-"+material.Name)
}

func writeJSONFile(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func sortedResources(resources []moodle.Resource) []moodle.Resource {
	out := append([]moodle.Resource(nil), resources...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SectionName != out[j].SectionName {
			return out[i].SectionName < out[j].SectionName
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
