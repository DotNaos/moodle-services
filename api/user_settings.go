package handler

import (
	"encoding/json"
	"io"
	"net/http"

	svc "github.com/DotNaos/moodle-services/pkg/moodleservices"
)

const maxUserSettingsBytes = 64 * 1024

// UserSettings stores and returns an opaque per-user settings JSON blob (chat
// course, model, reasoning effort, …). Keyed by Clerk user id.
func UserSettings(w http.ResponseWriter, r *http.Request) {
	if !svc.AllowMethods(w, r, http.MethodGet, http.MethodPut, http.MethodPost) {
		return
	}
	clerkUserID, ok := authorizeInternalRequest(w, r, true)
	if !ok {
		return
	}
	cfg := svc.LoadServerEnv()
	st, err := svc.OpenStoreFromEnv(cfg)
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	defer st.Close()

	if r.Method == http.MethodGet {
		settings, err := st.UserSettings(r.Context(), clerkUserID)
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		svc.WriteJSON(w, http.StatusOK, map[string]any{"settings": json.RawMessage(settings)})
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxUserSettingsBytes))
	if err != nil {
		svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "could not read request body"})
		return
	}
	var payload struct {
		Settings json.RawMessage `json:"settings"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || len(payload.Settings) == 0 || !json.Valid(payload.Settings) {
		svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "a valid settings object is required"})
		return
	}
	if err := st.UpsertUserSettings(r.Context(), clerkUserID, payload.Settings); err != nil {
		svc.WriteError(w, err)
		return
	}
	svc.WriteJSON(w, http.StatusOK, map[string]any{"settings": payload.Settings})
}
