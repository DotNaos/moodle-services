package moodle

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestExtractCourseListImagesKeepsGeneratedSVGDataURI(t *testing.T) {
	const svgDataURI = `data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciPjwvc3ZnPg==`
	html := `<li class="course-listitem" data-course-id="22577">
		<div class="list-image" style="background-image: url(&quot;` + svgDataURI + `&quot;);"></div>
	</li>`

	images := ExtractCourseListImages(html)
	if images[22577] != svgDataURI {
		t.Fatalf("expected SVG data URI, got %q", images[22577])
	}
}

func TestFetchCoursesFallsBackToCourseListSVGWhenAPIImageIsEmpty(t *testing.T) {
	const svgDataURI = `data:image/svg+xml;base64,PHN2Zy8+`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/my/":
			_, _ = w.Write([]byte(`{"sesskey":"testkey"}`))
		case "/lib/ajax/service.php":
			_ = json.NewEncoder(w).Encode([]moodleAPIResponse{
				{
					Data: &moodleAPIData{
						Courses: []moodleAPICourse{
							{
								ID:             22577,
								Fullname:       "Data Science und Informatik bei Banken (cds-305) FS26",
								Shortname:      "(cds-305) FS26",
								CourseCategory: "FS26",
								ViewURL:        "https://moodle.example.test/course/view.php?id=22577",
								CourseImage:    "",
							},
							{
								ID:             22585,
								Fullname:       "Deep Learning (cds-108) FS26",
								Shortname:      "(cds-108) FS26",
								CourseCategory: "FS26",
								ViewURL:        "https://moodle.example.test/course/view.php?id=22585",
								CourseImage:    "https://moodle.example.test/deep-learning.png",
							},
						},
					},
				},
			})
		case "/my/courses.php":
			_, _ = w.Write([]byte(`<li class="course-listitem" data-course-id="22577">
				<div class="list-image" style="background-image: url(&quot;` + svgDataURI + `&quot;);"></div>
			</li>`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	client := &Client{
		BaseURL: server.URL,
		Cookies: "MoodleSession=test",
		http:    server.Client(),
	}
	client.http.Timeout = 60 * time.Second

	courses, err := client.FetchCourses()
	if err != nil {
		t.Fatalf("FetchCourses: %v", err)
	}
	if len(courses) != 2 {
		t.Fatalf("expected two courses, got %#v", courses)
	}
	if courses[0].HeroImage != svgDataURI {
		t.Fatalf("expected fallback SVG data URI, got %q", courses[0].HeroImage)
	}
	if courses[1].HeroImage != "https://moodle.example.test/deep-learning.png" {
		t.Fatalf("existing course image was overwritten: %q", courses[1].HeroImage)
	}
}
