package moodle

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestBuildMobileLaunchURL(t *testing.T) {
	launch, err := BuildMobileLaunchURL("https://moodle.fhgr.ch/", DefaultMobileLaunchService, "passport-123", "moodlecli")
	if err != nil {
		t.Fatalf("BuildMobileLaunchURL: %v", err)
	}

	if launch.SiteURL != "https://moodle.fhgr.ch" {
		t.Fatalf("unexpected site URL %q", launch.SiteURL)
	}
	if !strings.HasPrefix(launch.URL, "https://moodle.fhgr.ch/admin/tool/mobile/launch.php?") {
		t.Fatalf("unexpected launch URL %q", launch.URL)
	}
	for _, want := range []string{
		"service=moodle_mobile_app",
		"passport=passport-123",
		"urlscheme=moodlecli",
		"confirmed=0",
		"oauthsso=0",
	} {
		if !strings.Contains(launch.URL, want) {
			t.Fatalf("launch URL %q missing %q", launch.URL, want)
		}
	}
	if launch.SiteID != ExpectedMobileLaunchSiteID("https://moodle.fhgr.ch", "passport-123") {
		t.Fatalf("unexpected site id %q", launch.SiteID)
	}
}

func TestParseMobileLaunchCallback(t *testing.T) {
	payload := "site-id-123:::public-token-456:::private-token-789"
	callback := "moodlecli://token=" + base64.StdEncoding.EncodeToString([]byte(payload))

	token, err := ParseMobileLaunchCallback(callback)
	if err != nil {
		t.Fatalf("ParseMobileLaunchCallback: %v", err)
	}
	if token.Scheme != "moodlecli" {
		t.Fatalf("unexpected scheme %q", token.Scheme)
	}
	if token.SiteID != "site-id-123" {
		t.Fatalf("unexpected site id %q", token.SiteID)
	}
	if token.Token != "public-token-456" {
		t.Fatalf("unexpected token %q", token.Token)
	}
	if token.PrivateToken != "private-token-789" {
		t.Fatalf("unexpected private token %q", token.PrivateToken)
	}
}

func TestParseMobileLaunchCallbackRejectsInvalidBase64(t *testing.T) {
	_, err := ParseMobileLaunchCallback("moodlecli://token=not-valid-!!!")
	if err == nil {
		t.Fatal("expected invalid base64 to fail")
	}
}
