package codexstate

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	svc "github.com/DotNaos/moodle-services/pkg/moodleservices"
)

type adminUsersResponse struct {
	DefaultQuotaBytes int64           `json:"defaultQuotaBytes"`
	MaxQuotaBytes     int64           `json:"maxQuotaBytes"`
	Users             []svc.AdminUser `json:"users"`
}

type adminUserResponse struct {
	DefaultQuotaBytes int64         `json:"defaultQuotaBytes"`
	MaxQuotaBytes     int64         `json:"maxQuotaBytes"`
	User              svc.AdminUser `json:"user"`
}

type updateAdminUserRequest struct {
	UserID               string `json:"userId"`
	CodexStateQuotaBytes *int64 `json:"codexStateQuotaBytes,omitempty"`
	ResetCodexStateQuota bool   `json:"resetCodexStateQuota,omitempty"`
	IsAdmin              *bool  `json:"isAdmin,omitempty"`
}

func HandleAdmin(w http.ResponseWriter, r *http.Request, clerkUserID string) {
	if !svc.AllowMethods(w, r, http.MethodGet, http.MethodPatch) {
		return
	}
	cfg := svc.LoadServerEnv()
	store, err := svc.OpenStoreFromEnv(cfg)
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	defer store.Close()
	isAdmin, err := store.UserIsAdmin(r.Context(), clerkUserID, cfg.IsConfiguredAdminClerkUser(clerkUserID))
	if errors.Is(err, svc.ErrNotFound) {
		svc.WriteError(w, svc.ErrUnauthorized)
		return
	}
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	if !isAdmin {
		svc.WriteError(w, svc.ErrUnauthorized)
		return
	}
	switch r.Method {
	case http.MethodGet:
		users, err := store.ListAdminUsers(r.Context(), cfg.CodexStateUserQuotaBytes, cfg.CodexStateAdminQuotaBytes)
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		svc.WriteJSON(w, http.StatusOK, adminUsersResponse{
			DefaultQuotaBytes: cfg.CodexStateUserQuotaBytes,
			MaxQuotaBytes:     svc.MaxCodexStateUserQuotaBytes,
			Users:             users,
		})
	case http.MethodPatch:
		var input updateAdminUserRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
			return
		}
		userID := strings.TrimSpace(input.UserID)
		if userID == "" {
			svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "userId is required"})
			return
		}
		if input.ResetCodexStateQuota && input.CodexStateQuotaBytes != nil {
			svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "quota can be set or reset, not both"})
			return
		}
		if input.CodexStateQuotaBytes != nil {
			quotaBytes := *input.CodexStateQuotaBytes
			if quotaBytes <= 0 || quotaBytes > svc.MaxCodexStateUserQuotaBytes {
				svc.WriteJSON(w, http.StatusBadRequest, map[string]any{
					"error":         "codexStateQuotaBytes must be between 1 byte and the maximum quota",
					"maxQuotaBytes": svc.MaxCodexStateUserQuotaBytes,
				})
				return
			}
		}
		user, err := store.UpdateAdminUser(r.Context(), userID, input.CodexStateQuotaBytes, input.ResetCodexStateQuota, input.IsAdmin, cfg.CodexStateUserQuotaBytes, cfg.CodexStateAdminQuotaBytes)
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		svc.WriteJSON(w, http.StatusOK, adminUserResponse{
			DefaultQuotaBytes: cfg.CodexStateUserQuotaBytes,
			MaxQuotaBytes:     svc.MaxCodexStateUserQuotaBytes,
			User:              user,
		})
	}
}
