package moodle

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestSaveMobileSessionUsesPrivatePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mobile-session.json")
	session := MobileSession{
		SiteURL:   "https://moodle.example.test",
		UserID:    42,
		Token:     "test-token",
		CreatedAt: time.Unix(100, 0).UTC(),
	}

	if err := SaveMobileSession(path, session); err != nil {
		t.Fatalf("SaveMobileSession: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat mobile session: %v", err)
	}
	if got := info.Mode().Perm(); runtime.GOOS != "windows" && got != 0o600 {
		t.Fatalf("expected 0600 permissions, got %o", got)
	}

	loaded, err := LoadMobileSession(path)
	if err != nil {
		t.Fatalf("LoadMobileSession: %v", err)
	}
	if loaded.Token != session.Token {
		t.Fatalf("unexpected token %q", loaded.Token)
	}
}

func TestMobileClientFetchCoursesNormalizesCategoryAndHeroImage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		switch r.Form.Get("wsfunction") {
		case "core_enrol_get_users_courses":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":          22585,
					"fullname":    "Deep Learning (cds-108) FS26",
					"shortname":   "(cds-108) FS26",
					"category":    1885,
					"courseimage": "https://moodle.example.test/course.png",
				},
			})
		case "core_course_get_categories":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":       1885,
					"name":     "FS26",
					"idnumber": "",
					"parent":   1157,
					"path":     "/7/1157/1885",
					"depth":    3,
				},
			})
		default:
			t.Fatalf("unexpected wsfunction %q", r.Form.Get("wsfunction"))
		}
	}))
	defer server.Close()

	client, err := NewMobileClient(MobileSession{
		SiteURL: server.URL,
		UserID:  22388,
		Token:   "test-token",
	}, ActiveSchoolID)
	if err != nil {
		t.Fatalf("NewMobileClient: %v", err)
	}

	courses, err := client.FetchCourses()
	if err != nil {
		t.Fatalf("FetchCourses: %v", err)
	}
	if len(courses) != 1 {
		t.Fatalf("expected one course, got %#v", courses)
	}
	course := courses[0]
	if course.Category != "FS26" || course.CategoryID != 1885 {
		t.Fatalf("category not normalized: %#v", course)
	}
	if course.HeroImage != "https://moodle.example.test/course.png" {
		t.Fatalf("hero image not normalized: %#v", course)
	}
}
