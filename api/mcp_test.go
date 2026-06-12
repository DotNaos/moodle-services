package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DotNaos/moodle-services/pkg/chatgptapp"
	svc "github.com/DotNaos/moodle-services/pkg/moodleservices"
)

func TestMCPServiceFromRequestRejectsGlobalSessionFallbackWithoutDatabase(t *testing.T) {
	t.Setenv("MOODLE_MOBILE_SESSION_JSON", `{"siteUrl":"https://moodle.fhgr.ch","userId":123,"token":"global-token"}`)

	apiKey := "moodle_test_key"
	t.Setenv("MCP_API_KEY_HASH", chatgptapp.HashAPIKey(apiKey))
	request := httptest.NewRequest(http.MethodPost, "/api/mcp?key="+apiKey, nil)
	cfg, err := chatgptapp.LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv: %v", err)
	}

	service, returnedKey, status, err := serviceFromRequest(request, cfg)
	if err == nil {
		t.Fatalf("serviceFromRequest unexpectedly returned service: %#v", service)
	}
	if returnedKey != "" {
		t.Fatalf("serviceFromRequest returned API key %q despite rejecting the request", returnedKey)
	}
	if status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", status, http.StatusInternalServerError)
	}
	if !errors.Is(err, svc.ErrDatabaseNotConfigured) {
		t.Fatalf("serviceFromRequest error = %v, want ErrDatabaseNotConfigured", err)
	}
}
