package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DotNaos/moodle-services/internal/api"
	"github.com/go-chi/chi/v5"
)

func TestAPICommandRoutesAreCuratedDataEndpoints(t *testing.T) {
	routes := buildAPICommandRoutes()
	paths := map[string]api.CommandRoute{}
	for _, route := range routes {
		if !strings.HasPrefix(route.APIPath, "/api/") {
			t.Fatalf("expected API route under /api, got %q", route.APIPath)
		}
		if strings.HasPrefix(route.APIPath, "/api/cli/") {
			t.Fatalf("legacy /api/cli route should not be exposed: %q", route.APIPath)
		}
		if route.Method != http.MethodGet {
			t.Fatalf("expected curated data routes to use GET, got %s for %s", route.Method, route.APIPath)
		}
		paths[route.APIPath] = route
	}

	for _, expected := range []string{
		"/api/version",
		"/api/timetable",
		"/api/current-lecture",
		"/api/nav",
		"/api/courses/{courseID}/page",
		"/api/courses/{courseID}/resources/{resourceID}/text",
		"/api/courses/{courseID}/resources/{resourceID}/ocr",
		"/api/mobile/qr/inspect",
	} {
		if _, ok := paths[expected]; !ok {
			t.Fatalf("expected curated API route %s, got %#v", expected, paths)
		}
	}
}

func TestCourseResourceOCRRouteBuildsCommandArguments(t *testing.T) {
	route := findAPICommandRoute(t, "/api/courses/{courseID}/resources/{resourceID}/ocr")
	req := httptest.NewRequest(http.MethodGet, "/api/courses/123/resources/456/ocr?engine=all&timeout=900&gpu=true&docker-platform=linux/amd64&formula=true&verbose=true", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("courseID", "123")
	rctx.URLParams.Add("resourceID", "456")
	req = req.WithContext(contextWithRoute(req, rctx))

	args, err := route.Arguments(req, api.CommandRequest{})
	if err != nil {
		t.Fatalf("Arguments: %v", err)
	}
	if got, want := strings.Join(args, " "), "123 456 --engine all --timeout 900 --docker-platform linux/amd64 --gpu --formula --verbose"; got != want {
		t.Fatalf("arguments = %q, want %q", got, want)
	}
}

func TestAPICommandRoutesExcludeSideEffectCommands(t *testing.T) {
	for _, route := range buildAPICommandRoutes() {
		command := strings.Join(route.CommandPath, " ")
		for _, excluded := range []string{
			"open",
			"download",
			"export",
			"login",
			"serve",
			"completion",
			"bootstrap",
			"skill",
			"update",
			"config set",
			"logs",
		} {
			if command == excluded || strings.HasPrefix(command, excluded+" ") {
				t.Fatalf("side-effect command %q should not be exposed as API route %s", command, route.APIPath)
			}
		}
	}
}

func TestCourseResourceTextRouteBuildsCommandArguments(t *testing.T) {
	route := findAPICommandRoute(t, "/api/courses/{courseID}/resources/{resourceID}/text")
	req := httptest.NewRequest(http.MethodGet, "/api/courses/123/resources/456/text?raw=true", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("courseID", "123")
	rctx.URLParams.Add("resourceID", "456")
	req = req.WithContext(contextWithRoute(req, rctx))

	args, err := route.Arguments(req, api.CommandRequest{})
	if err != nil {
		t.Fatalf("Arguments: %v", err)
	}
	if got, want := strings.Join(args, " "), "123 456 --raw"; got != want {
		t.Fatalf("arguments = %q, want %q", got, want)
	}
}

func TestNavRouteRequiresPathQuery(t *testing.T) {
	route := findAPICommandRoute(t, "/api/nav")
	req := httptest.NewRequest(http.MethodGet, "/api/nav", nil)
	if _, err := route.Arguments(req, api.CommandRequest{}); err == nil {
		t.Fatal("expected missing path query to fail")
	}
}

func findAPICommandRoute(t *testing.T, path string) api.CommandRoute {
	t.Helper()
	for _, route := range buildAPICommandRoutes() {
		if route.APIPath == path {
			return route
		}
	}
	t.Fatalf("route %s not found", path)
	return api.CommandRoute{}
}

func contextWithRoute(req *http.Request, rctx *chi.Context) context.Context {
	return context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
}
