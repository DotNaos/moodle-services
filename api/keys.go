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

	if r.URL.Query().Get("route") == "webex-credentials" {
		if !svc.AllowMethods(w, r, http.MethodPost) {
			return
		}
		var input contract.WebexCredentialsRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
			return
		}
		if strings.TrimSpace(input.Username) == "" || strings.TrimSpace(input.Password) == "" {
			svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "username and password are required"})
			return
		}
		service, closeFn, err := svc.ServiceForRequest(r, cfg)
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		defer closeFn()
		sessionJSON, err := service.CreateWebexBrowserSession(r.Context(), input.CourseID, svc.WebexCredentials{
			Username: strings.TrimSpace(input.Username),
			Password: input.Password,
		})
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		box, err := svc.EncryptionBox(cfg)
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		encryptedSessionJSON, err := box.EncryptString(sessionJSON)
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		// Persist the credentials (encrypted) so an expired session can be
		// re-created silently without asking the user to sign in again.
		credentialsJSON, err := json.Marshal(map[string]string{
			"username": strings.TrimSpace(input.Username),
			"password": input.Password,
		})
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		encryptedCredentials, err := box.EncryptString(string(credentialsJSON))
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		if err := st.UpsertWebexSession(r.Context(), svc.UpsertWebexSessionInput{
			UserID:                    user.ID,
			EncryptedWebexSessionJSON: encryptedSessionJSON,
			EncryptedWebexCredentials: encryptedCredentials,
		}); err != nil {
			svc.WriteError(w, err)
			return
		}
		svc.WriteJSON(w, http.StatusOK, contract.WebexCredentialsResponse{SavedSession: true})
		return
	}

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
