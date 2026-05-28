package moodlemobile

import "testing"

func TestSessionFromToken(t *testing.T) {
	session := SessionFromToken(Token{
		SiteURL:      "https://moodle.fhgr.ch/",
		UserID:       123,
		Token:        "public-token",
		PrivateToken: "private-token",
	})
	if err := ValidateSession(session); err != nil {
		t.Fatalf("ValidateSession: %v", err)
	}
	if session.SiteURL != "https://moodle.fhgr.ch" {
		t.Fatalf("unexpected site URL %q", session.SiteURL)
	}
}

func TestTokenRESTEndpoint(t *testing.T) {
	endpoint := (Token{SiteURL: "https://moodle.fhgr.ch/"}).RESTEndpoint()
	if endpoint != "https://moodle.fhgr.ch/webservice/rest/server.php?moodlewsrestformat=json" {
		t.Fatalf("unexpected endpoint %q", endpoint)
	}
}
