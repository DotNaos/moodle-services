package studypipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	contract "github.com/DotNaos/moodle-services/pkg/apicontracts"
)

type CodexChatInput struct {
	ArtifactRoot    string
	UserID          string
	Prompt          string
	Images          []contract.CodexRunImage
	Model           string
	ReasoningEffort string
	OutputSchema    json.RawMessage
	Emit            func(contract.StudyPipelineRefineEvent)
}

func RunCodexChat(ctx context.Context, input CodexChatInput) (contract.CodexRunResponse, error) {
	image := strings.TrimSpace(os.Getenv(EnvCodexDockerImage))
	if image == "" {
		return contract.CodexRunResponse{}, fmt.Errorf("%s is not configured", EnvCodexDockerImage)
	}
	prompt := strings.TrimSpace(input.Prompt)
	if prompt == "" {
		return contract.CodexRunResponse{}, fmt.Errorf("prompt is required")
	}
	authenticated, _, err := CodexAuthenticated(ctx, input.UserID, input.ArtifactRoot)
	if err != nil {
		return contract.CodexRunResponse{}, err
	}
	if !authenticated {
		return contract.CodexRunResponse{}, ErrCodexNotAuthenticated
	}

	model := sanitizeCodexModel(input.Model)
	reasoningEffort := sanitizeCodexOption(input.ReasoningEffort)
	if model == "" {
		catalog, err := CodexModelCatalog(ctx, input.UserID, input.ArtifactRoot)
		if err != nil {
			return contract.CodexRunResponse{}, err
		}
		model, reasoningEffort = selectDefaultCodexChatModel(catalog, reasoningEffort)
	}
	command := buildCodexChatCommand(model, reasoningEffort, len(input.OutputSchema) > 0)
	if input.Emit != nil {
		input.Emit(contract.StudyPipelineRefineEvent{
			Type:            "runner",
			Message:         "Starting Codex chat in the per-user Docker runner.",
			Model:           model,
			ReasoningEffort: reasoningEffort,
		})
	}

	output, err := runDockerCodexWithOptions(ctx, dockerCodexOptions{
		Image:           image,
		Command:         command,
		Model:           model,
		ReasoningEffort: reasoningEffort,
		ArtifactRoot:    input.ArtifactRoot,
		UserID:          input.UserID,
		Prompt:          buildCodexChatPrompt(prompt, input.Images),
		OutputPrefix:    "chat",
		OutputSchema:    input.OutputSchema,
		Emit:            input.Emit,
	})
	if err != nil {
		return contract.CodexRunResponse{}, fmt.Errorf("codex chat failed: %w", err)
	}
	return parseCodexChatOutput(output)
}

var ErrCodexNotAuthenticated = fmt.Errorf("Codex is not connected for this user. Connect ChatGPT before asking Codex questions.")

func buildCodexChatCommand(model string, reasoningEffort string, hasSchema bool) string {
	parts := []string{
		"codex exec --json --skip-git-repo-check --sandbox read-only",
	}
	if model != "" {
		parts = append(parts, `--model "$CODEX_MODEL"`)
	}
	if reasoningEffort != "" {
		parts = append(parts, `-c 'model_reasoning_effort="`+reasoningEffort+`"'`)
	}
	if hasSchema {
		parts = append(parts, `--output-schema "$CODEX_OUTPUT_SCHEMA_FILE"`)
	}
	parts = append(parts, `--output-last-message "$CODEX_OUTPUT_FILE" -`)
	return strings.Join(parts, " ")
}

func buildCodexChatPrompt(prompt string, images []contract.CodexRunImage) string {
	if len(images) == 0 {
		return prompt
	}
	names := make([]string, 0, len(images))
	for _, image := range images {
		name := strings.TrimSpace(image.Name)
		if name != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return prompt
	}
	return prompt + "\n\nAttached Moodle page screenshots were provided by the web UI: " + strings.Join(names, ", ") + "."
}

func selectDefaultCodexChatModel(catalog contract.CodexModelCatalogResponse, requestedReasoningEffort string) (string, string) {
	for _, option := range catalog.Models {
		model := sanitizeCodexModel(option.ID)
		if model == "" {
			continue
		}
		reasoningEffort := requestedReasoningEffort
		if reasoningEffort == "" {
			reasoningEffort = sanitizeCodexOption(option.DefaultReasoningEffort)
		}
		return model, reasoningEffort
	}
	return "", requestedReasoningEffort
}

func parseCodexChatOutput(output string) (contract.CodexRunResponse, error) {
	text := strings.TrimSpace(output)
	if text == "" {
		return contract.CodexRunResponse{}, fmt.Errorf("empty Codex response")
	}

	var raw struct {
		Answer        string                   `json:"answer"`
		FinalResponse string                   `json:"finalResponse"`
		Actions       []contract.CodexUIAction `json:"actions"`
	}
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		extracted := extractJSONObject(text)
		if extracted == "" || json.Unmarshal([]byte(extracted), &raw) != nil {
			return contract.CodexRunResponse{
				ThreadID:      nil,
				FinalResponse: text,
				Actions:       []contract.CodexUIAction{},
			}, nil
		}
	}

	answer := strings.TrimSpace(raw.FinalResponse)
	if answer == "" {
		answer = strings.TrimSpace(raw.Answer)
	}
	if answer == "" {
		answer = "Codex finished without a text answer."
	}
	return contract.CodexRunResponse{
		ThreadID:      nil,
		FinalResponse: answer,
		Actions:       normalizeCodexActions(raw.Actions),
	}, nil
}

func extractJSONObject(text string) string {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return ""
	}
	return text[start : end+1]
}

func normalizeCodexActions(actions []contract.CodexUIAction) []contract.CodexUIAction {
	out := []contract.CodexUIAction{}
	for _, action := range actions {
		action.Type = strings.TrimSpace(action.Type)
		switch action.Type {
		case "open_course", "load_course_resources", "open_moodle_course_page", "open_latest_pdf":
			if stringPtrValue(action.CourseID) == "" {
				continue
			}
		case "open_material":
			if stringPtrValue(action.MaterialID) == "" {
				continue
			}
		case "open_resource":
			if stringPtrValue(action.CourseID) == "" || stringPtrValue(action.ResourceID) == "" {
				continue
			}
		case "scroll_pdf_to_page":
			if action.Page == nil || *action.Page < 1 {
				continue
			}
		default:
			continue
		}
		out = append(out, action)
	}
	return out
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
