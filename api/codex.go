package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	contract "github.com/DotNaos/moodle-services/pkg/apicontracts"
	svc "github.com/DotNaos/moodle-services/pkg/moodleservices"
	"github.com/DotNaos/moodle-services/pkg/studypipeline"
)

func Codex(w http.ResponseWriter, r *http.Request) {
	if !svc.AllowMethods(w, r, http.MethodGet, http.MethodPost, http.MethodDelete) {
		return
	}
	action := r.URL.Query().Get("action")
	switch action {
	case "status":
		clerkUserID, ok := authorizeInternalRequest(w, r, true)
		if !ok {
			return
		}
		authenticated, detail, err := studypipeline.CodexAuthenticated(r.Context(), clerkUserID, "")
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		svc.WriteJSON(w, http.StatusOK, map[string]any{
			"authenticated": authenticated,
			"provider":      "moodle-services",
			"detail":        detail,
		})
	case "auth":
		clerkUserID, ok := authorizeInternalRequest(w, r, true)
		if !ok {
			return
		}
		if r.Method == http.MethodDelete {
			if err := studypipeline.CodexLogout(r.Context(), clerkUserID, ""); err != nil {
				svc.WriteError(w, err)
				return
			}
			svc.WriteJSON(w, http.StatusOK, map[string]any{"authenticated": false})
			return
		}
		if r.Method != http.MethodPost {
			svc.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		start, err := studypipeline.StartCodexDeviceAuth(r.Context(), clerkUserID, "")
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		if start.Authenticated {
			svc.WriteJSON(w, http.StatusOK, map[string]any{"authenticated": true})
			return
		}
		svc.WriteJSON(w, http.StatusOK, map[string]any{
			"type":             "device_code",
			"verificationUri":  start.VerificationURI,
			"userCode":         start.UserCode,
			"expiresInSeconds": start.ExpiresInSeconds,
		})
	case "auth-callback":
		svc.WriteJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "Codex device-code sign-in does not use a callback endpoint.",
		})
	case "models":
		clerkUserID, ok := authorizeInternalRequest(w, r, true)
		if !ok {
			return
		}
		authenticated, _, err := studypipeline.CodexAuthenticated(r.Context(), clerkUserID, "")
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		if !authenticated {
			svc.WriteJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "Connect ChatGPT before loading the Codex model catalog.",
			})
			return
		}
		models, err := studypipeline.CodexModelCatalog(r.Context(), clerkUserID, "")
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		svc.WriteJSON(w, http.StatusOK, models)
	case "files":
		if r.Method == http.MethodGet {
			svc.WriteJSON(w, http.StatusOK, map[string]any{"files": []string{}})
			return
		}
		svc.WriteJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "Codex file storage has moved to moodle-services, but it is not configured on this deployment yet.",
		})
	case "run":
		handleCodexRun(w, r)
	default:
		svc.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "unknown Codex action"})
	}
}

func handleCodexRun(w http.ResponseWriter, r *http.Request) {
	clerkUserID, ok := authorizeInternalRequest(w, r, true)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		svc.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var body contract.CodexRunRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "Request body must be JSON."})
		return
	}
	if body.Stream || r.Header.Get("Accept") == "application/x-ndjson" {
		handleCodexRunStream(w, r, clerkUserID, body)
		return
	}
	result, err := studypipeline.RunCodexChat(r.Context(), contractToCodexChatInput(clerkUserID, body, nil))
	if err != nil {
		writeCodexRunError(w, err)
		return
	}
	svc.WriteJSON(w, http.StatusOK, result)
}

func handleCodexRunStream(w http.ResponseWriter, r *http.Request, clerkUserID string, body contract.CodexRunRequest) {
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	writeCodexStreamEvent(w, map[string]any{"type": "thread", "threadId": nil})

	result, err := studypipeline.RunCodexChat(r.Context(), contractToCodexChatInput(clerkUserID, body, func(event contract.StudyPipelineRefineEvent) {
		message := event.Message
		if event.Error != "" {
			message = event.Error
		}
		if message == "" {
			message = "Codex is working."
		}
		writeCodexStreamEvent(w, map[string]any{
			"type":   "tool",
			"title":  message,
			"status": "running",
		})
	}))
	if err != nil {
		writeCodexStreamEvent(w, map[string]any{
			"type":  "error",
			"error": codexRunErrorMessage(err),
		})
		return
	}
	writeCodexStreamEvent(w, map[string]any{
		"type":          "done",
		"threadId":      nil,
		"finalResponse": result.FinalResponse,
		"actions":       result.Actions,
	})
}

func contractToCodexChatInput(clerkUserID string, body contract.CodexRunRequest, emit func(contract.StudyPipelineRefineEvent)) studypipeline.CodexChatInput {
	return studypipeline.CodexChatInput{
		UserID:          clerkUserID,
		Prompt:          body.Prompt,
		Images:          body.Images,
		Model:           body.Model,
		ReasoningEffort: body.ReasoningEffort,
		OutputSchema:    body.OutputSchema,
		Emit:            emit,
	}
}

func writeCodexRunError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, studypipeline.ErrCodexNotAuthenticated) {
		status = http.StatusUnauthorized
	}
	svc.WriteJSON(w, status, map[string]string{"error": codexRunErrorMessage(err)})
}

func codexRunErrorMessage(err error) string {
	if errors.Is(err, studypipeline.ErrCodexNotAuthenticated) {
		return studypipeline.ErrCodexNotAuthenticated.Error()
	}
	return err.Error()
}

func writeCodexStreamEvent(w http.ResponseWriter, event map[string]any) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	_, _ = w.Write(append(data, '\n'))
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}
