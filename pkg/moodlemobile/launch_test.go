package moodlemobile

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestBuildLaunchURL(t *testing.T) {
	launch, err := BuildLaunchURL("https://moodle.fhgr.ch/", DefaultLaunchService, "passport-123", "studyreplay")
	if err != nil {
		t.Fatalf("BuildLaunchURL: %v", err)
	}
	if launch.SiteURL != "https://moodle.fhgr.ch" {
		t.Fatalf("unexpected site URL %q", launch.SiteURL)
	}
	if !strings.HasPrefix(launch.URL, "https://moodle.fhgr.ch/admin/tool/mobile/launch.php?") {
		t.Fatalf("unexpected launch URL %q", launch.URL)
	}
	for _, want := range []string{"service=moodle_mobile_app", "passport=passport-123", "urlscheme=studyreplay", "confirmed=0", "oauthsso=0"} {
		if !strings.Contains(launch.URL, want) {
			t.Fatalf("launch URL %q missing %q", launch.URL, want)
		}
	}
	if launch.SiteID != ExpectedSiteID("https://moodle.fhgr.ch", "passport-123") {
		t.Fatalf("unexpected site id %q", launch.SiteID)
	}
}

func TestParseCallback(t *testing.T) {
	payload := "site-id-123:::public-token-456:::private-token-789"
	callback := "studyreplay://token=" + base64.StdEncoding.EncodeToString([]byte(payload))

	token, err := ParseCallback(callback)
	if err != nil {
		t.Fatalf("ParseCallback: %v", err)
	}
	if token.Scheme != "studyreplay" || token.SiteID != "site-id-123" {
		t.Fatalf("unexpected callback %#v", token)
	}
	if token.Token != "public-token-456" || token.PrivateToken != "private-token-789" {
		t.Fatalf("unexpected tokens %#v", token)
	}
}

func TestParseCallbackRejectsInvalidBase64(t *testing.T) {
	_, err := ParseCallback("studyreplay://token=not-valid-!!!")
	if err == nil {
		t.Fatal("expected invalid base64 to fail")
	}
}
