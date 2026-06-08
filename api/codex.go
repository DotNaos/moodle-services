package handler

import (
	"encoding/json"
	"net/http"

	svc "github.com/DotNaos/moodle-services/pkg/moodleservices"
)

func Codex(w http.ResponseWriter, r *http.Request) {
	if !svc.AllowMethods(w, r, http.MethodGet, http.MethodPost, http.MethodDelete) {
		return
	}
	action := r.URL.Query().Get("action")
	switch action {
	case "status":
		svc.WriteJSON(w, http.StatusOK, map[string]any{
			"authenticated": false,
			"provider":      "moodle-services",
		})
	case "auth", "auth-callback":
		svc.WriteJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "Codex authentication has moved to moodle-services, but the Codex runner is not configured on this deployment yet.",
		})
	case "models":
		svc.WriteJSON(w, http.StatusOK, map[string]any{
			"models": []map[string]string{},
		})
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
