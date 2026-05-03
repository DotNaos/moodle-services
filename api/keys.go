package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	contract "github.com/DotNaos/moodle-services/pkg/apicontracts"
	svc "github.com/DotNaos/moodle-services/pkg/moodleservices"
)

func Keys(w http.ResponseWriter, r *http.Request) {
	if !svc.AllowMethods(w, r, http.MethodGet, http.MethodPost, http.MethodDelete) {
		return
	}
	cfg := svc.LoadServerEnv()
	st, user, _, err := svc.AuthenticatedUser(r, cfg)
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	defer st.Close()

	switch r.Method {
	case http.MethodGet:
		records, err := st.ListAPIKeys(r.Context(), user.ID)
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		svc.WriteJSON(w, http.StatusOK, contract.ListAPIKeysResponse{Keys: records})
	case http.MethodPost:
		var input contract.CreateAPIKeyRequest
		_ = json.NewDecoder(r.Body).Decode(&input)
		if strings.TrimSpace(input.Name) == "" {
			input.Name = "API key"
		}
		if len(input.Scopes) == 0 {
			input.Scopes = []string{"moodle:read", "pdf:read", "calendar:read"}
		}
		apiKey, err := svc.GenerateAPIKey()
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		if input.RevokeExisting {
			if err := st.RevokeActiveAPIKeysForUser(r.Context(), user.ID); err != nil {
				svc.WriteError(w, err)
				return
			}
		}
		record, err := st.CreateAPIKey(r.Context(), user.ID, input.Name, apiKey, cfg.HashSecret, input.Scopes)
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		svc.WriteJSON(w, http.StatusOK, contract.CreateAPIKeyResponse{APIKey: apiKey, APIKeyRecord: record, RevokedExisting: input.RevokeExisting})
	case http.MethodDelete:
		keyID := strings.TrimSpace(r.URL.Query().Get("id"))
		if keyID == "" {
			svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "id query parameter is required"})
			return
		}
		err := st.RevokeAPIKey(r.Context(), user.ID, keyID)
		if errors.Is(err, sql.ErrNoRows) {
			svc.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "API key not found"})
			return
		}
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		svc.WriteJSON(w, http.StatusOK, contract.RevokeAPIKeyResponse{Revoked: true})
	}
}
