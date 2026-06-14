package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DotNaos/moodle-services/internal/moodle"
	contract "github.com/DotNaos/moodle-services/pkg/apicontracts"
	"github.com/DotNaos/moodle-services/pkg/studypipeline"
)

type stubClient struct {
	validateErr error
	courses     []moodle.Course
	categories  []moodle.Category
	resources   map[string][]moodle.Resource
}

func (s stubClient) ValidateSession() error {
	return s.validateErr
}

func (s stubClient) FetchCourses() ([]moodle.Course, error) {
	return s.courses, nil
}

func (s stubClient) FetchCourseResources(courseID string) ([]moodle.Resource, string, error) {
	if s.resources == nil {
		return nil, "", fmt.Errorf("no resources configured")
	}
	res, ok := s.resources[courseID]
	if !ok {
		return nil, "", fmt.Errorf("course %s not found", courseID)
	}
	return res, "", nil
}

func (s stubClient) FetchCategories() ([]moodle.Category, error) {
	return s.categories, nil
}

func TestStudyPipelineHandlerBuildsCoursePlan(t *testing.T) {
	t.Setenv(studypipeline.EnvArtifactRoot, t.TempDir())
	router, err := NewRouter(ServerOptions{
		ClientProvider: func() (Client, error) {
			return stubClient{
				resources: map[string][]moodle.Resource{
					"22584": {
						{ID: "1", Name: "01 Memory Hierarchy", FileType: "pdf", SectionID: "s1"},
						{ID: "2", Name: "Aufgabenblatt 01", FileType: "pdf", SectionID: "s1"},
						{ID: "3", Name: "Lösung Aufgabenblatt 01", FileType: "pdf", SectionID: "s1"},
					},
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/courses/22584/study-pipeline", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}
	var payload struct {
		Status  string `json:"status"`
		Summary struct {
			Tasks           int `json:"tasks"`
			LinkedSolutions int `json:"linkedSolutions"`
		} `json:"summary"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Status != "curated-ready" || payload.Summary.Tasks != 1 || payload.Summary.LinkedSolutions != 1 {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestStudyPipelinePlanRouteRunsStagesServerSide(t *testing.T) {
	t.Setenv(studypipeline.EnvArtifactRoot, t.TempDir())
	router, err := NewRouter(ServerOptions{
		ClientProvider: func() (Client, error) {
			return stubClient{
				resources: map[string][]moodle.Resource{
					"22584": {
						{ID: "2", Name: "Aufgabenblatt 01", FileType: "pdf", SectionID: "s1"},
						{ID: "3", Name: "Lösung Aufgabenblatt 01", FileType: "pdf", SectionID: "s1"},
					},
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/courses/22584/study-pipeline/plan",
		strings.NewReader(`{"mode":"from","startStage":"extracted"}`),
	)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}
	var payload contract.StudyPipelinePlanResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Status != "succeeded" || len(payload.Steps) != 2 {
		t.Fatalf("unexpected plan payload: %#v", payload)
	}
	if payload.Steps[0].Stage != "extracted" || payload.Steps[0].Status != "succeeded" {
		t.Fatalf("unexpected first step: %#v", payload.Steps[0])
	}
	if payload.Steps[1].Stage != "curated" || payload.Steps[1].Status != "succeeded" {
		t.Fatalf("unexpected second step: %#v", payload.Steps[1])
	}
	if payload.Response == nil || payload.Response.Stage != "curated" {
		t.Fatalf("expected final curated response, got %#v", payload.Response)
	}
}

func TestStudyPipelineInventoryRouteBuildsTaskGroups(t *testing.T) {
	t.Setenv(studypipeline.EnvArtifactRoot, t.TempDir())
	router, err := NewRouter(ServerOptions{
		ClientProvider: func() (Client, error) {
			return stubClient{
				resources: map[string][]moodle.Resource{
					"22584": {
						{ID: "1", Name: "Teil 01 Memory Hierarchy", FileType: "pdf", SectionID: "s1"},
						{ID: "2", Name: "Aufgabenblatt 01", FileType: "pdf", SectionID: "s1"},
						{ID: "3", Name: "Lösung Aufgabenblatt 01", FileType: "pdf", SectionID: "s1"},
						{ID: "4", Name: "Modulbeschreibung", FileType: "pdf", SectionID: "s0"},
					},
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/courses/22584/study-pipeline/inventory", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}
	var payload contract.CourseInventoryResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Summary.TaskGroups != 1 || payload.Summary.PairedTaskGroups != 1 || payload.Summary.References != 1 {
		t.Fatalf("unexpected inventory summary: %#v", payload.Summary)
	}
	if len(payload.TaskGroups) != 1 || payload.TaskGroups[0].Solution == nil || payload.TaskGroups[0].Solution.ID != "3" {
		t.Fatalf("unexpected task group: %#v", payload.TaskGroups)
	}
}

func TestStudyPipelineExtractedDocumentsRouteBuildsStructure(t *testing.T) {
	t.Setenv(studypipeline.EnvArtifactRoot, t.TempDir())
	router, err := NewRouter(ServerOptions{
		ClientProvider: func() (Client, error) {
			return stubClient{
				resources: map[string][]moodle.Resource{
					"22584": {
						{ID: "2", Name: "Aufgabenblatt 01", FileType: "pdf", SectionID: "s1"},
					},
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/courses/22584/study-pipeline/extracted-documents", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}
	var payload contract.ExtractedDocumentsResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Summary.TotalDocuments != 1 || payload.Summary.TotalPages != 1 || payload.Summary.TotalBlocks == 0 {
		t.Fatalf("unexpected extracted summary: %#v", payload.Summary)
	}
	if len(payload.Documents) != 1 || len(payload.Documents[0].Pages) != 1 {
		t.Fatalf("unexpected documents: %#v", payload.Documents)
	}
	if payload.Documents[0].Pages[0].Blocks[0].Type != "heading" {
		t.Fatalf("expected placeholder heading block, got %#v", payload.Documents[0].Pages[0].Blocks)
	}
}

func TestStudyPipelineExtractedAssetRouteServesCourseAsset(t *testing.T) {
	root := t.TempDir()
	t.Setenv(studypipeline.EnvArtifactRoot, root)
	assetPath := filepath.Join(root, "courses", "22584", "extracted", "runs", "run-1", "assets", "page.png")
	if err := os.MkdirAll(filepath.Dir(assetPath), 0o755); err != nil {
		t.Fatalf("mkdir asset dir: %v", err)
	}
	if err := os.WriteFile(assetPath, []byte{0x89, 0x50, 0x4e, 0x47}, 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}
	router, err := NewRouter(ServerOptions{
		ClientProvider: func() (Client, error) {
			return stubClient{
				resources: map[string][]moodle.Resource{"22584": nil},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/courses/22584/study-pipeline/extracted-asset?path="+url.QueryEscape(assetPath), nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "image/png") {
		t.Fatalf("expected image/png content type, got %q", got)
	}
	if rec.Body.String() != string([]byte{0x89, 0x50, 0x4e, 0x47}) {
		t.Fatalf("unexpected body: %v", rec.Body.Bytes())
	}
}

func TestStudyPipelineRequiresCourseID(t *testing.T) {
	router, err := NewRouter(ServerOptions{
		ClientProvider: func() (Client, error) {
			return stubClient{}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/courses/%20/study-pipeline", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestHealthHandlerOK(t *testing.T) {
	router, err := NewRouter(ServerOptions{
		ClientProvider: func() (Client, error) {
			return stubClient{}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	var payload map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["status"] != "ok" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestServerlessParityAuthRoutesAreRegistered(t *testing.T) {
	t.Setenv("MOODLE_WEB_INTERNAL_SECRET", "")
	router, err := NewRouter(ServerOptions{
		ClientProvider: func() (Client, error) {
			return stubClient{}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	for _, tc := range []struct {
		method string
		path   string
	}{
		{method: http.MethodPost, path: "/api/auth/clerk/qr/exchange"},
		{method: http.MethodPost, path: "/api/auth/clerk/login"},
		{method: http.MethodPost, path: "/api/auth/clerk/session"},
		{method: http.MethodPost, path: "/api/auth/clerk/mobile/bridge/start"},
		{method: http.MethodGet, path: "/api/auth/clerk/mobile/bridge/status?challenge=test"},
		{method: http.MethodPost, path: "/api/auth/clerk/mobile/bridge/complete"},
		{method: http.MethodGet, path: "/api/auth/clerk/codex/state"},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
		router.ServeHTTP(rec, req)

		if rec.Code == http.StatusNotFound {
			t.Fatalf("%s %s returned 404; expected Docker router to mirror Vercel rewrite", tc.method, tc.path)
		}
	}
}

func TestServerlessParityWebRoutesAreRegistered(t *testing.T) {
	t.Setenv("MOODLE_WEB_INTERNAL_SECRET", "")
	router, err := NewRouter(ServerOptions{
		ClientProvider: func() (Client, error) {
			return stubClient{}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	for _, tc := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/.well-known/oauth-protected-resource"},
		{method: http.MethodGet, path: "/.well-known/oauth-authorization-server"},
		{method: http.MethodPost, path: "/oauth/register"},
		{method: http.MethodGet, path: "/oauth/authorize"},
		{method: http.MethodPost, path: "/oauth/authorize/complete"},
		{method: http.MethodPost, path: "/oauth/token"},
		{method: http.MethodGet, path: "/api/docs"},
		{method: http.MethodPost, path: "/api/mcp"},
		{method: http.MethodGet, path: "/api/openapi.json"},
		{method: http.MethodGet, path: "/api/me"},
		{method: http.MethodGet, path: "/api/keys"},
		{method: http.MethodGet, path: "/api/search?q=moodle"},
		{method: http.MethodGet, path: "/api/courses/22584/materials"},
		{method: http.MethodGet, path: "/api/courses/22584/materials/974595/text"},
		{method: http.MethodGet, path: "/api/courses/22584/materials/974595/pdf"},
		{method: http.MethodGet, path: "/api/courses/22584/recordings"},
		{method: http.MethodGet, path: "/api/calendar"},
		{method: http.MethodPost, path: "/api/webex/credentials"},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
		router.ServeHTTP(rec, req)

		if rec.Code == http.StatusNotFound {
			t.Fatalf("%s %s returned 404; expected Docker router to mirror Vercel rewrite", tc.method, tc.path)
		}
	}
}

func TestOpenAPIHandler(t *testing.T) {
	router, err := NewRouter(ServerOptions{
		ClientProvider: func() (Client, error) {
			return stubClient{}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	req.Host = "api.localhost:8080"
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("expected JSON content type, got %q", got)
	}

	var payload map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["openapi"] != "3.0.3" {
		t.Fatalf("unexpected openapi version: %#v", payload["openapi"])
	}

	servers, ok := payload["servers"].([]any)
	if !ok || len(servers) != 1 {
		t.Fatalf("unexpected servers payload: %#v", payload["servers"])
	}
	server, ok := servers[0].(map[string]any)
	if !ok || server["url"] != "http://api.localhost:8080" {
		t.Fatalf("unexpected server url: %#v", servers[0])
	}

	paths, ok := payload["paths"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected paths payload: %#v", payload["paths"])
	}
	if _, ok := paths["/healthz"]; !ok {
		t.Fatalf("expected /healthz path, got %#v", paths)
	}
	if _, ok := paths["/api/courses"]; !ok {
		t.Fatalf("expected /api/courses path, got %#v", paths)
	}
}

func TestScalarDocsHandler(t *testing.T) {
	router, err := NewRouter(ServerOptions{
		ClientProvider: func() (Client, error) {
			return stubClient{}, nil
		},
		CommandRoutes: []CommandRoute{
			{
				APIPath:     "/api/version",
				Method:      http.MethodGet,
				CommandPath: []string{"version"},
				Summary:     "Show version information",
			},
		},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/docs", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("expected HTML content type, got %q", got)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "@scalar/api-reference") {
		t.Fatalf("expected Scalar script in body, got %q", body)
	}
	if !strings.Contains(body, openAPIPath) {
		t.Fatalf("expected OpenAPI path in body, got %q", body)
	}
}

func TestCuratedCommandEndpoint(t *testing.T) {
	called := false
	var gotPath []string
	var gotArgs []string

	router, err := NewRouter(ServerOptions{
		ClientProvider: func() (Client, error) {
			return stubClient{}, nil
		},
		CommandRoutes: []CommandRoute{
			{
				APIPath:     "/api/version",
				Method:      http.MethodGet,
				CommandPath: []string{"version"},
				Summary:     "Show version information",
				Arguments: func(r *http.Request, _ CommandRequest) ([]string, error) {
					if r.URL.Query().Get("check") == "true" {
						return []string{"--check"}, nil
					}
					return nil, nil
				},
			},
		},
		CommandRunner: func(_ context.Context, commandPath []string, arguments []string, stdout io.Writer, _ io.Writer) error {
			called = true
			gotPath = append([]string{}, commandPath...)
			gotArgs = append([]string{}, arguments...)
			_, err := io.WriteString(stdout, `{"version":"v1.2.3"}`)
			return err
		},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/version?check=true", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if !called {
		t.Fatal("expected command runner to be invoked")
	}
	if strings.Join(gotPath, " ") != "version" {
		t.Fatalf("unexpected command path: %#v", gotPath)
	}
	if strings.Join(gotArgs, " ") != "--check" {
		t.Fatalf("unexpected command arguments: %#v", gotArgs)
	}

	var payload map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["version"] != "v1.2.3" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestOpenAPIIncludesCuratedCommandEndpoints(t *testing.T) {
	router, err := NewRouter(ServerOptions{
		ClientProvider: func() (Client, error) {
			return stubClient{}, nil
		},
		CommandRoutes: []CommandRoute{
			{
				APIPath:     "/api/version",
				Method:      http.MethodGet,
				CommandPath: []string{"version"},
				Summary:     "Show version information",
			},
		},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	router.ServeHTTP(rec, req)

	var payload map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	paths, ok := payload["paths"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected paths payload: %#v", payload["paths"])
	}
	if _, ok := paths["/api/version"]; !ok {
		t.Fatalf("expected curated command path, got %#v", paths)
	}
	if _, ok := paths["/api/cli/version"]; ok {
		t.Fatalf("legacy /api/cli path should not be documented: %#v", paths)
	}
	versionPath, ok := paths["/api/version"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected /api/version path shape: %#v", paths["/api/version"])
	}
	if _, ok := versionPath["get"]; !ok {
		t.Fatalf("expected /api/version to document GET, got %#v", versionPath)
	}
	if _, ok := versionPath["post"]; ok {
		t.Fatalf("expected /api/version not to document POST, got %#v", versionPath)
	}
}

func TestHealthHandlerExpiredSession(t *testing.T) {
	router, err := NewRouter(ServerOptions{
		ClientProvider: func() (Client, error) {
			return stubClient{validateErr: moodle.ErrSessionExpired}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestCoursesHandler(t *testing.T) {
	wantCourses := []moodle.Course{
		{ID: 1, Fullname: "Course A", Category: "Cat", CategoryID: 42, HeroImage: "https://moodle.test/banner.jpg"},
	}
	router, err := NewRouter(ServerOptions{
		ClientProvider: func() (Client, error) {
			return stubClient{courses: wantCourses}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/courses", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	var got []moodle.Course
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != len(wantCourses) || got[0].ID != wantCourses[0].ID {
		t.Fatalf("unexpected courses: %#v", got)
	}
	if got[0].CategoryID != 42 || got[0].HeroImage == "" {
		t.Fatalf("course normalization was not preserved: %#v", got[0])
	}
}

func TestCategoriesHandler(t *testing.T) {
	wantCategories := []moodle.Category{
		{ID: 42, Name: "FS26", ParentID: 10, Path: "/7/10/42", Depth: 3},
	}
	router, err := NewRouter(ServerOptions{
		ClientProvider: func() (Client, error) {
			return stubClient{categories: wantCategories}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/categories", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	var got []moodle.Category
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != len(wantCategories) || got[0].Name != "FS26" {
		t.Fatalf("unexpected categories: %#v", got)
	}
}

func TestCourseResourcesHandler(t *testing.T) {
	resource := moodle.Resource{ID: "42", Name: "Slide"}
	router, err := NewRouter(ServerOptions{
		ClientProvider: func() (Client, error) {
			return stubClient{
				resources: map[string][]moodle.Resource{
					"123": {resource},
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/courses/123/resources", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	var got []moodle.Resource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 1 || got[0].ID != resource.ID {
		t.Fatalf("unexpected resources: %#v", got)
	}
}
