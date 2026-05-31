package moodleservices

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/DotNaos/moodle-services/pkg/chatgptapp"
)

const (
	EnvDatabaseURL       = "DATABASE_URL"
	EnvEncryptionKey     = "APP_ENCRYPTION_KEY"
	EnvAPIKeyHashSecret  = "API_KEY_HASH_SECRET"
	EnvLegacyAPIKeyHash  = "MCP_API_KEY_HASH"
	EnvCalendarURL       = "MOODLE_CALENDAR_URL"
	EnvMobileSessionJSON = "MOODLE_MOBILE_SESSION_JSON"
)

const OAuthAccessTokenPrefix = "moodle_oauth_"

type ServerEnv struct {
	DatabaseURL   string
	EncryptionKey string
	HashSecret    []byte
	CalendarURL   string
}

func LoadServerEnv() ServerEnv {
	encryptionKey := strings.TrimSpace(os.Getenv(EnvEncryptionKey))
	hashSecret := strings.TrimSpace(os.Getenv(EnvAPIKeyHashSecret))
	if hashSecret == "" {
		hashSecret = encryptionKey
	}
	return ServerEnv{
		DatabaseURL:   strings.TrimSpace(os.Getenv(EnvDatabaseURL)),
		EncryptionKey: encryptionKey,
		HashSecret:    []byte(hashSecret),
		CalendarURL:   strings.TrimSpace(os.Getenv(EnvCalendarURL)),
	}
}

func OpenStoreFromEnv(cfg ServerEnv) (*Store, error) {
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("%s is not configured", EnvDatabaseURL)
	}
	return OpenStore(cfg.DatabaseURL)
}

func EncryptionBox(cfg ServerEnv) (Box, error) {
	if cfg.EncryptionKey == "" {
		return Box{}, fmt.Errorf("%s is not configured", EnvEncryptionKey)
	}
	return NewBox(cfg.EncryptionKey)
}

func AuthenticatedUser(r *http.Request, cfg ServerEnv) (*Store, User, string, error) {
	apiKey := APIKeyFromRequest(r)
	if apiKey == "" {
		return nil, User{}, "", ErrUnauthorized
	}
	st, err := OpenStoreFromEnv(cfg)
	if err != nil {
		return nil, User{}, "", err
	}
	user, err := st.UserForAPIKey(r.Context(), apiKey, cfg.HashSecret)
	if err != nil {
		_ = st.Close()
		return nil, User{}, "", err
	}
	return st, user, apiKey, nil
}

func ServiceForRequest(r *http.Request, cfg ServerEnv) (Service, func(), error) {
	apiKey := APIKeyFromRequest(r)
	if apiKey == "" {
		return Service{}, nil, ErrUnauthorized
	}
	if cfg.DatabaseURL == "" {
		if expectedHash := strings.TrimSpace(os.Getenv(EnvLegacyAPIKeyHash)); expectedHash != "" && !ConstantTimeEqual(HashAPIKey(apiKey), expectedHash) {
			return Service{}, nil, ErrUnauthorized
		}
		client, err := chatgptapp.ClientFromEnv()
		if err != nil {
			return Service{}, nil, err
		}
		return Service{Client: client, CalendarURL: cfg.CalendarURL}, func() {}, nil
	}
	st, err := OpenStoreFromEnv(cfg)
	if err != nil {
		return Service{}, nil, err
	}
	closeFn := func() { _ = st.Close() }
	var credentials MoodleCredentials
	if strings.HasPrefix(apiKey, OAuthAccessTokenPrefix) {
		credentials, err = st.MoodleCredentialsForOAuthAccessToken(r.Context(), apiKey, cfg.HashSecret)
	} else {
		credentials, err = st.MoodleCredentialsForAPIKey(r.Context(), apiKey, cfg.HashSecret)
	}
	if err != nil {
		closeFn()
		return Service{}, nil, err
	}
	box, err := EncryptionBox(cfg)
	if err != nil {
		closeFn()
		return Service{}, nil, err
	}
	sessionJSON, err := box.DecryptString(credentials.EncryptedMobileSessionJSON)
	if err != nil {
		closeFn()
		return Service{}, nil, err
	}
	var session MobileSession
	if err := json.Unmarshal([]byte(sessionJSON), &session); err != nil {
		closeFn()
		return Service{}, nil, fmt.Errorf("decode stored Moodle session: %w", err)
	}
	client, err := NewMobileClient(session, session.ResolvedSchoolID())
	if err != nil {
		closeFn()
		return Service{}, nil, err
	}
	calendarURL := cfg.CalendarURL
	if credentials.EncryptedCalendarURL != "" {
		if decrypted, err := box.DecryptString(credentials.EncryptedCalendarURL); err == nil {
			calendarURL = decrypted
		}
	}
	webexSessionJSON := ""
	if credentials.EncryptedWebexSessionJSON != "" {
		if decrypted, err := box.DecryptString(credentials.EncryptedWebexSessionJSON); err == nil {
			webexSessionJSON = decrypted
		}
	}
	return Service{Client: client, CalendarURL: calendarURL, WebexSessionJSON: webexSessionJSON}, closeFn, nil
}

func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func WriteError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, ErrUnauthorized):
		status = http.StatusUnauthorized
	case errors.Is(err, ErrNotFound):
		status = http.StatusNotFound
	}
	WriteJSON(w, status, map[string]string{"error": err.Error()})
}

func AllowMethods(w http.ResponseWriter, r *http.Request, methods ...string) bool {
	SetServiceCORS(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return false
	}
	for _, method := range methods {
		if r.Method == method {
			return true
		}
	}
	WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	return false
}

func SetServiceCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "authorization, content-type, x-moodle-app-key")
}
