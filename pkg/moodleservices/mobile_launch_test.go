package moodleservices

import (
	"encoding/base64"
	"testing"
)

func TestMobileLaunchFacade(t *testing.T) {
	launch, err := BuildMobileLaunchURL("https://moodle.fhgr.ch/", DefaultMobileLaunchService, "passport-123", DefaultMobileLaunchScheme)
	if err != nil {
		t.Fatalf("BuildMobileLaunchURL: %v", err)
	}

	payload := launch.SiteID + ":::public-token:::private-token"
	callback := launch.URLScheme + "://token=" + base64.StdEncoding.EncodeToString([]byte(payload))
	token, err := ParseMobileLaunchCallback(callback)
	if err != nil {
		t.Fatalf("ParseMobileLaunchCallback: %v", err)
	}
	mobileToken := MobileTokenFromLaunch(launch.SiteURL, token)
	if mobileToken.SiteURL != "https://moodle.fhgr.ch" {
		t.Fatalf("unexpected site URL %q", mobileToken.SiteURL)
	}
	if mobileToken.Token != "public-token" || mobileToken.PrivateToken != "private-token" {
		t.Fatalf("unexpected token %#v", mobileToken)
	}
}
