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
	if course.HeroImage != "https://moodle.example.test/course.png?token=test-token" {
		t.Fatalf("hero image not normalized: %#v", course)
	}
}

func TestMobileClientFetchCoursesUsesOverviewSVGWhenCourseImageIsEmpty(t *testing.T) {
	const svgDataURI = "data:image/svg+xml;base64,PHN2Zy8+"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		switch r.Form.Get("wsfunction") {
		case "core_enrol_get_users_courses":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":        22577,
					"fullname":  "Data Science und Informatik bei Banken (cds-305) FS26",
					"shortname": "(cds-305) FS26",
					"category":  1885,
					"overviewfiles": []map[string]any{
						{
							"filename": "course.svg",
							"fileurl":  svgDataURI,
							"mimetype": "image/svg+xml",
						},
					},
				},
			})
		case "core_course_get_categories":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case "core_course_get_enrolled_courses_by_timeline_classification":
			_ = json.NewEncoder(w).Encode(map[string]any{"courses": []map[string]any{}})
		case "core_course_get_courses_by_field":
			_ = json.NewEncoder(w).Encode(map[string]any{"courses": []map[string]any{}})
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
	if courses[0].HeroImage != "" {
		t.Fatalf("expected generated SVG overview image to be ignored, got %q", courses[0].HeroImage)
	}
}

func TestMobileClientFetchCoursesUsesTimelineImageWhenUserCourseImageIsEmpty(t *testing.T) {
	const svgDataURI = "data:image/svg+xml;base64,PHN2Zy8+"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		switch r.Form.Get("wsfunction") {
		case "core_enrol_get_users_courses":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":        22577,
					"fullname":  "Data Science und Informatik bei Banken (cds-305) FS26",
					"shortname": "(cds-305) FS26",
					"category":  1885,
				},
			})
		case "core_course_get_categories":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case "core_course_get_enrolled_courses_by_timeline_classification":
			if r.Form.Get("classification") != "all" {
				t.Fatalf("unexpected classification %q", r.Form.Get("classification"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"courses": []map[string]any{
					{
						"id":          22577,
						"fullname":    "Data Science und Informatik bei Banken (cds-305) FS26",
						"shortname":   "(cds-305) FS26",
						"category":    1885,
						"courseimage": svgDataURI,
					},
				},
			})
		case "core_course_get_courses_by_field":
			_ = json.NewEncoder(w).Encode(map[string]any{"courses": []map[string]any{}})
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
	if courses[0].HeroImage != "" {
		t.Fatalf("expected generated SVG timeline image to be ignored, got %q", courses[0].HeroImage)
	}
}

func TestMobileClientFetchCoursesPrefersTimelineSVGOverGeneratedCourseImage(t *testing.T) {
	const svgDataURI = "data:image/svg+xml;base64,PHN2Zy8+"
	const generatedSVG = "https://moodle.fhgr.ch/pluginfile.php/1267822/course/generated/course.svg"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		switch r.Form.Get("wsfunction") {
		case "core_enrol_get_users_courses":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":          22577,
					"fullname":    "Data Science und Informatik bei Banken (cds-305) FS26",
					"shortname":   "(cds-305) FS26",
					"category":    1885,
					"courseimage": generatedSVG,
				},
			})
		case "core_course_get_categories":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case "core_course_get_enrolled_courses_by_timeline_classification":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"courses": []map[string]any{
					{
						"id":          22577,
						"fullname":    "Data Science und Informatik bei Banken (cds-305) FS26",
						"shortname":   "(cds-305) FS26",
						"category":    1885,
						"visible":     true,
						"courseimage": svgDataURI,
					},
				},
			})
		case "core_course_get_courses_by_field":
			_ = json.NewEncoder(w).Encode(map[string]any{"courses": []map[string]any{}})
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
	if courses[0].HeroImage != "" {
		t.Fatalf("expected generated SVG course images to be ignored, got %q", courses[0].HeroImage)
	}
}

func TestMobileClientFetchCoursesReplacesGeneratedTimelineImageWithCourseDetailImage(t *testing.T) {
	const generatedSVG = "https://moodle.fhgr.ch/pluginfile.php/1267822/course/generated/course.svg"
	const detailImage = "https://moodle.fhgr.ch/pluginfile.php/1267822/course/overviewfiles/banner.jpg"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		switch r.Form.Get("wsfunction") {
		case "core_enrol_get_users_courses":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":          22577,
					"fullname":    "Data Science und Informatik bei Banken (cds-305) FS26",
					"shortname":   "(cds-305) FS26",
					"category":    1885,
					"courseimage": generatedSVG,
				},
			})
		case "core_course_get_categories":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case "core_course_get_enrolled_courses_by_timeline_classification":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"courses": []map[string]any{
					{
						"id":          22577,
						"fullname":    "Data Science und Informatik bei Banken (cds-305) FS26",
						"shortname":   "(cds-305) FS26",
						"category":    1885,
						"courseimage": generatedSVG,
					},
				},
			})
		case "core_course_get_courses_by_field":
			if r.Form.Get("field") != "id" || r.Form.Get("value") != "22577" {
				t.Fatalf("unexpected course detail query field=%q value=%q", r.Form.Get("field"), r.Form.Get("value"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"courses": []map[string]any{
					{
						"id":          22577,
						"fullname":    "Data Science und Informatik bei Banken (cds-305) FS26",
						"shortname":   "(cds-305) FS26",
						"category":    1885,
						"courseimage": detailImage,
					},
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
	expected := detailImage + "?token=test-token"
	if courses[0].HeroImage != expected {
		t.Fatalf("expected detail course image %q, got %q", expected, courses[0].HeroImage)
	}
}

func TestMobileClientFetchCoursesReplacesInlineSVGTimelineImageWithCourseDetailImage(t *testing.T) {
	const svgDataURI = "data:image/svg+xml;base64,PHN2Zy8+"
	const detailImage = "https://moodle.fhgr.ch/pluginfile.php/1267822/course/overviewfiles/banner.jpg"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		switch r.Form.Get("wsfunction") {
		case "core_enrol_get_users_courses":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":          22577,
					"fullname":    "Data Science und Informatik bei Banken (cds-305) FS26",
					"shortname":   "(cds-305) FS26",
					"category":    1885,
					"courseimage": svgDataURI,
				},
			})
		case "core_course_get_categories":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case "core_course_get_enrolled_courses_by_timeline_classification":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"courses": []map[string]any{
					{
						"id":          22577,
						"fullname":    "Data Science und Informatik bei Banken (cds-305) FS26",
						"shortname":   "(cds-305) FS26",
						"category":    1885,
						"courseimage": svgDataURI,
					},
				},
			})
		case "core_course_get_courses_by_field":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"courses": []map[string]any{
					{
						"id":          22577,
						"fullname":    "Data Science und Informatik bei Banken (cds-305) FS26",
						"shortname":   "(cds-305) FS26",
						"category":    1885,
						"courseimage": detailImage,
					},
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
	expected := detailImage + "?token=test-token"
	if courses[0].HeroImage != expected {
		t.Fatalf("expected detail course image %q, got %q", expected, courses[0].HeroImage)
	}
}

func TestMobileCourseImagePrefersOverviewFileOverGeneratedCourseImage(t *testing.T) {
	const generatedSVG = "https://moodle.fhgr.ch/pluginfile.php/1267822/course/generated/course.svg"
	const overviewImage = "https://moodle.fhgr.ch/pluginfile.php/1267822/course/overviewfiles/banner.jpg"
	image := mobileCourseImage(MobileCourse{
		CourseImage: generatedSVG,
		OverviewFiles: []MobileCourseOverview{
			{FileURL: overviewImage},
		},
	}, "test-token")

	expected := overviewImage + "?token=test-token"
	if image != expected {
		t.Fatalf("expected overview image %q, got %q", expected, image)
	}
}

func TestMobileClientFetchCoursesUsesCourseDetailImageWhenListsAreEmpty(t *testing.T) {
	const svgDataURI = "data:image/svg+xml;base64,PHN2Zy8+"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		switch r.Form.Get("wsfunction") {
		case "core_enrol_get_users_courses":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":        22577,
					"fullname":  "Data Science und Informatik bei Banken (cds-305) FS26",
					"shortname": "(cds-305) FS26",
					"category":  1885,
				},
			})
		case "core_course_get_categories":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case "core_course_get_enrolled_courses_by_timeline_classification":
			_ = json.NewEncoder(w).Encode(map[string]any{"courses": []map[string]any{}})
		case "core_course_get_courses_by_field":
			if r.Form.Get("field") != "id" || r.Form.Get("value") != "22577" {
				t.Fatalf("unexpected course detail query field=%q value=%q", r.Form.Get("field"), r.Form.Get("value"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"courses": []map[string]any{
					{
						"id":          22577,
						"fullname":    "Data Science und Informatik bei Banken (cds-305) FS26",
						"shortname":   "(cds-305) FS26",
						"category":    1885,
						"courseimage": svgDataURI,
					},
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
	if courses[0].HeroImage != "" {
		t.Fatalf("expected generated SVG course detail image to be ignored, got %q", courses[0].HeroImage)
	}
}
