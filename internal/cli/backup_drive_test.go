package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGoogleDriveAccessTokenUsesOAuthCredentialsWhenPresent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if got := r.Form.Get("grant_type"); got != "refresh_token" {
			t.Fatalf("grant_type = %q, want refresh_token", got)
		}
		if got := r.Form.Get("client_id"); got != "client-id" {
			t.Fatalf("client_id = %q, want client-id", got)
		}
		if got := r.Form.Get("client_secret"); got != "client-secret" {
			t.Fatalf("client_secret = %q, want client-secret", got)
		}
		if got := r.Form.Get("refresh_token"); got != "refresh-token" {
			t.Fatalf("refresh_token = %q, want refresh-token", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "access-token"})
	}))
	defer server.Close()

	t.Setenv("GOOGLE_DRIVE_OAUTH_CREDENTIALS_JSON", `{
		"type": "authorized_user",
		"client_id": "client-id",
		"client_secret": "client-secret",
		"refresh_token": "refresh-token",
		"token_uri": "`+server.URL+`"
	}`)
	t.Setenv("GOOGLE_DRIVE_SERVICE_ACCOUNT_JSON", "")

	token, err := googleDriveAccessToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if token != "access-token" {
		t.Fatalf("token = %q, want access-token", token)
	}
}

func TestGoogleDriveAccessTokenRequiresCredentials(t *testing.T) {
	t.Setenv("GOOGLE_DRIVE_OAUTH_CREDENTIALS_JSON", "")
	t.Setenv("GOOGLE_DRIVE_SERVICE_ACCOUNT_JSON", "")

	_, err := googleDriveAccessToken(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}
