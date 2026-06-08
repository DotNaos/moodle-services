package studypipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/DotNaos/moodle-services/internal/moodle"
	contract "github.com/DotNaos/moodle-services/pkg/apicontracts"
)

const (
	EnvArtifactRoot          = "MOODLE_STUDY_ARTIFACT_ROOT"
	EnvCodexCommand          = "MOODLE_STUDY_CODEX_COMMAND"
	EnvCodexDockerImage      = "MOODLE_STUDY_CODEX_DOCKER_IMAGE"
	EnvCodexModelCandidates  = "MOODLE_STUDY_CODEX_MODEL_CANDIDATES"
	EnvCodexContainerCommand = "MOODLE_STUDY_CODEX_CONTAINER_COMMAND"
	DefaultArtifactRoot      = "/srv/moodle-study"
)

type Downloader interface {
	DownloadFileToBuffer(url string) (moodle.DownloadResult, error)
}

type RunOptions struct {
	Root       string
	Now        time.Time
	Downloader Downloader
	UserID     string
	Refiner    ContentRefiner
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
	ArtifactRoot string
	CourseID     string
	UserID       string
	Kind         string
	TargetID     string
	Title        string
	Content      string
}

type RefineOutput struct {
	Content string
	Model   string
}

var unsafePathRe = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

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
	refiner := options.Refiner
	if refiner == nil {
		refiner = DockerCodexRefiner{}
	}
	output, err := refiner.Refine(ctx, RefineInput{
		ArtifactRoot: root,
		CourseID:     courseID,
		UserID:       options.UserID,
		Kind:         kind,
		TargetID:     targetID,
		Title:        material.Name,
		Content:      content,
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
	models := codexModelCandidates()
	if len(models) == 0 {
		return RefineOutput{}, fmt.Errorf("%s is not configured", EnvCodexModelCandidates)
	}
	command := strings.TrimSpace(os.Getenv(EnvCodexContainerCommand))
	if command == "" {
		command = `codex exec --skip-git-repo-check --sandbox read-only --model "$CODEX_MODEL" -`
	}
	prompt := buildRefinePrompt(input)
	var errs []string
	for _, model := range models {
		output, err := runDockerCodex(ctx, image, command, model, input.ArtifactRoot, input.UserID, prompt)
		if err == nil && strings.TrimSpace(output) != "" {
			return RefineOutput{Content: strings.TrimSpace(output), Model: model}, nil
		}
		if err != nil {
			errs = append(errs, model+": "+err.Error())
		} else {
			errs = append(errs, model+": empty response")
		}
	}
	return RefineOutput{}, fmt.Errorf("codex refinement failed for all configured models: %s", strings.Join(errs, "; "))
}

func codexModelCandidates() []string {
	raw := strings.TrimSpace(os.Getenv(EnvCodexModelCandidates))
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '\n' || r == ';' })
	models := []string{}
	for _, part := range parts {
		if model := strings.TrimSpace(part); model != "" {
			models = append(models, model)
		}
	}
	return models
}

func runDockerCodex(ctx context.Context, image string, command string, model string, artifactRoot string, userID string, prompt string) (string, error) {
	userSegment := safeSegment(firstNonEmpty(userID, "anonymous"))
	stateRoot := filepath.Join(firstNonEmpty(artifactRoot, ArtifactRootFromEnv()), "codex-users", userSegment)
	if err := os.MkdirAll(stateRoot, 0o700); err != nil {
		return "", err
	}
	_ = os.Chown(stateRoot, 10001, 10001)
	_ = os.Chmod(stateRoot, 0o700)
	args := []string{
		"run", "--rm", "-i",
		"--user", "0:0",
		"-e", "CODEX_MODEL=" + model,
		"-e", "HOME=/home/codex",
		"-e", "CODEX_HOME=/home/codex/.codex",
		"-v", stateRoot + ":/home/codex/.codex",
		image,
		"sh", "-lc", command,
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdin = strings.NewReader(prompt)
	output, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err != nil {
		return "", fmt.Errorf("%w (%s)", err, compactProcessOutput(text))
	}
	return text, nil
}

func buildRefinePrompt(input RefineInput) string {
	kindLabel := "script chapter"
	if input.Kind == "task" {
		kindLabel = "task sheet"
	}
	return strings.Join([]string{
		"You are cleaning Moodle course material for a study UI.",
		"Rewrite the given " + kindLabel + " into polished, useful Markdown.",
		"Preserve all factual content, equations, variables, code snippets, task requirements, deadlines, and source intent.",
		"Improve structure, headings, lists, LaTeX math formatting, and readability.",
		"Use KaTeX-compatible LaTeX with inline `$...$` and display `$$...$$` where appropriate.",
		"Do not invent new facts. If text is clearly garbled, keep the best faithful reconstruction.",
		"Return only the improved Markdown content, without meta commentary.",
		"",
		"Course ID: " + input.CourseID,
		"Title: " + input.Title,
		"Target ID: " + input.TargetID,
		"",
		"Extracted source:",
		input.Content,
	}, "\n")
}

func compactProcessOutput(output string) string {
	output = strings.Join(strings.Fields(output), " ")
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
