package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DotNaos/moodle-services/internal/moodle"
)

func TestCleanExtractedTextWithTimeoutFallsBackToRaw(t *testing.T) {
	input := "  raw text  "
	got := cleanExtractedTextWithTimeout(input, 0)
	if strings.TrimSpace(got) != "raw text" {
		t.Fatalf("expected raw fallback, got %q", got)
	}
}

func TestCleanExtractedTextWithTimeoutCleansWhenAllowed(t *testing.T) {
	input := "hello-\nworld"
	got := cleanExtractedTextWithTimeout(input, 50*time.Millisecond)
	if got != "helloworld" {
		t.Fatalf("expected cleaned text, got %q", got)
	}
}

func TestPrintCommandArgsAllowCourseOutlineSelector(t *testing.T) {
	if err := printCmd.Args(printCmd, []string{"42"}); err != nil {
		t.Fatalf("expected single course selector to be valid, got %v", err)
	}
}

func TestBuildPrintOCROptionsFromFlags(t *testing.T) {
	oldEngine := printOCREngine
	oldOut := printOCROutDir
	oldFormat := printOCRFormat
	oldKeep := printOCRKeepArtifacts
	oldTimeout := printOCRTimeoutSeconds
	oldPlatform := printOCRDockerPlatform
	oldGPU := printOCRGPU
	oldFormula := printOCRFormula
	oldCode := printOCRCode
	oldVerbose := printOCRVerbose
	t.Cleanup(func() {
		printOCREngine = oldEngine
		printOCROutDir = oldOut
		printOCRFormat = oldFormat
		printOCRKeepArtifacts = oldKeep
		printOCRTimeoutSeconds = oldTimeout
		printOCRDockerPlatform = oldPlatform
		printOCRGPU = oldGPU
		printOCRFormula = oldFormula
		printOCRCode = oldCode
		printOCRVerbose = oldVerbose
	})

	printOCREngine = "all"
	printOCROutDir = "/tmp/ocr"
	printOCRFormat = "json"
	printOCRKeepArtifacts = true
	printOCRTimeoutSeconds = 900
	printOCRDockerPlatform = "linux/amd64"
	printOCRGPU = true
	printOCRFormula = true
	printOCRCode = true
	printOCRVerbose = true

	opts := buildPrintOCROptions()
	if opts.Engine != "all" || opts.OutputDir != "/tmp/ocr" || opts.Timeout != 900*time.Second {
		t.Fatalf("unexpected OCR options: %#v", opts)
	}
	if !opts.KeepArtifacts || !opts.GPU || !opts.Formula || !opts.Code || !opts.Verbose {
		t.Fatalf("expected boolean OCR options to be true: %#v", opts)
	}
	if opts.DockerPlatform != "linux/amd64" {
		t.Fatalf("unexpected docker platform: %#v", opts)
	}
}

func TestRunPrintCoursePageWithClientReturnsCourseOutline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/my/":
			_, _ = w.Write([]byte(`{"sesskey":"sess-123"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/lib/ajax/service.php":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
				{
					"error": false,
					"data": {
						"courses": [
							{
								"id": 42,
								"fullname": "Algorithmen des wissenschaftlichen Rechnens",
								"shortname": "AWR",
								"coursecategory": "FS26",
								"viewurl": "https://example.com/course/view.php?id=42",
								"courseimage": ""
							}
						]
					}
				}
			]`))
		case r.Method == http.MethodGet && r.URL.Path == "/course/view.php" && r.URL.Query().Get("id") == "42":
			_, _ = w.Write([]byte(`
				<li id="section-1" data-id="1" data-sectionname="Thema 1: Sparse Grids">
					<div class="summarytext"><p>Einfuhrung in die Woche.</p></div>
					<li class="activity label modtype_label">
						<div class="contentwithoutlink"><p>Bitte zuerst das Aufgabenblatt lesen.</p></div>
					</li>
					<li class="activity resource modtype_resource">
						<div data-activityname="Folien Teil 1">
							<a href="https://example.com/mod/resource/view.php?id=100"></a>
							<span class="activitybadge">PDF</span>
							<span class="resourcelinkdetails">Hochgeladen 20.03.2026 15:30</span>
						</div>
					</li>
				</li>
			`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	session := moodle.Session{SchoolID: moodle.ActiveSchoolID, Cookies: "MoodleSession=test"}
	client, err := moodle.NewClient(session)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client.BaseURL = server.URL

	result, err := runPrintCoursePageWithClient(client, "42")
	if err != nil {
		t.Fatalf("runPrintCoursePageWithClient: %v", err)
	}

	if result.Action != "print-course-page" {
		t.Fatalf("expected course-page action, got %q", result.Action)
	}
	if result.CourseID != "42" {
		t.Fatalf("expected course id 42, got %q", result.CourseID)
	}
	for _, want := range []string{
		"Thema 1: Sparse Grids",
		"Einfuhrung in die Woche.",
		"Bitte zuerst das Aufgabenblatt lesen.",
		"- Folien Teil 1 · PDF · 2026-03-20T15:30:00+01:00",
	} {
		if !strings.Contains(result.Text, want) {
			t.Fatalf("expected %q in course outline, got %q", want, result.Text)
		}
	}
}
