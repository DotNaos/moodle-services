package moodleservice

import (
	"context"
	"os"
	"testing"
)

func TestMobileSessionFromCredentialsLive(t *testing.T) {
	if os.Getenv("MOODLE_LIVE_CREDENTIAL_TEST") != "1" {
		t.Skip("set MOODLE_LIVE_CREDENTIAL_TEST=1 with MOODLE_USERNAME and MOODLE_PASSWORD to run")
	}
	username := os.Getenv("MOODLE_USERNAME")
	password := os.Getenv("MOODLE_PASSWORD")
	if username == "" || password == "" {
		t.Fatal("MOODLE_USERNAME and MOODLE_PASSWORD are required")
	}

	session, siteInfo, err := MobileSessionFromCredentials(context.Background(), WebexCredentials{
		Username: username,
		Password: password,
	})
	if err != nil {
		t.Fatalf("MobileSessionFromCredentials: %v", err)
	}
	if session.Token == "" {
		t.Fatal("expected mobile token")
	}
	if session.UserID == 0 || siteInfo.UserID == 0 {
		t.Fatal("expected user id")
	}
}
