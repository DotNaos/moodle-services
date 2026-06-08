package handler

import (
	"encoding/json"
	"net/http"

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
	case "auth", "auth-callback":
		svc.WriteJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "Codex authentication has moved to moodle-services, but the Codex runner is not configured on this deployment yet.",
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
		var body struct {
			Stream bool `json:"stream"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Stream || r.Header.Get("Accept") == "application/x-ndjson" {
			w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusNotImplemented)
			_, _ = w.Write([]byte(`{"type":"error","error":"Codex runner is not configured in moodle-services yet."}` + "\n"))
			return
		}
		svc.WriteJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "Codex runner is not configured in moodle-services yet.",
		})
	default:
		svc.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "unknown Codex action"})
	}
}
