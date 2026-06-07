package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	serverless "github.com/DotNaos/moodle-services/api"
	"github.com/DotNaos/moodle-services/internal/moodle"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Client is the subset of the Moodle client used by the API server.
type Client interface {
	ValidateSession() error
	FetchCourses() ([]moodle.Course, error)
	FetchCourseResources(courseID string) ([]moodle.Resource, string, error)
}

type categoryClient interface {
	FetchCategories() ([]moodle.Category, error)
}

// ServerOptions configure the HTTP router.
type ServerOptions struct {
	ClientProvider func() (Client, error)
	CommandRoutes  []CommandRoute
	CommandRunner  CommandRunner
	LogWriter      io.Writer
	RequestTimeout time.Duration
}

// NewRouter builds a chi router exposing the REST API.
func NewRouter(opts ServerOptions) (*chi.Mux, error) {
	if opts.ClientProvider == nil {
		return nil, fmt.Errorf("client provider is required")
	}

	router := chi.NewRouter()
	requestTimeout := opts.RequestTimeout
	if requestTimeout <= 0 {
		requestTimeout = 30 * time.Minute
	}

	router.Use(
		middleware.RequestID,
		middleware.RealIP,
		middleware.RequestLogger(&middleware.DefaultLogFormatter{
			Logger:  log.New(resolveLogWriter(opts.LogWriter), "", log.LstdFlags),
			NoColor: true,
		}),
		middleware.Recoverer,
		middleware.Timeout(requestTimeout),
	)

	router.Get(openAPIPath, openAPIHandler(opts))
	router.Get(docsPath, scalarHandler())
	router.Get(scalarPath, scalarHandler())
	router.Get("/healthz", healthHandler(opts))
	router.Get("/api/courses", coursesHandler(opts))
	router.Get("/api/categories", categoriesHandler(opts))
	router.Get("/api/courses/{courseID}/resources", courseResourcesHandler(opts))
	registerServerlessParityRoutes(router)
	registerCommandRoutes(router, opts)

	return router, nil
}

func registerServerlessParityRoutes(router *chi.Mux) {
	router.HandleFunc("/.well-known/oauth-protected-resource", withQuery(serverless.Oauth, "route", "protected-resource"))
	router.HandleFunc("/.well-known/oauth-authorization-server", withQuery(serverless.Oauth, "route", "authorization-server"))
	router.HandleFunc("/oauth/register", withQuery(serverless.Oauth, "route", "register"))
	router.HandleFunc("/oauth/authorize", withQuery(serverless.Oauth, "route", "authorize"))
	router.HandleFunc("/oauth/authorize/complete", withQuery(serverless.Oauth, "route", "authorize-complete"))
	router.HandleFunc("/oauth/token", withQuery(serverless.Oauth, "route", "token"))
	router.HandleFunc("/api/docs", serverless.Docs)
	router.HandleFunc("/api/mcp", serverless.Handler)
	router.HandleFunc("/api/openapi.json", serverless.Openapi)
	router.HandleFunc("/api/me", serverless.Me)
	router.HandleFunc("/api/keys", serverless.Keys)
	router.HandleFunc("/api/search", serverless.Search)
	router.HandleFunc("/api/auth/qr/exchange", serverless.AuthQrExchange)
	router.HandleFunc("/api/auth/clerk/qr/exchange", withQuery(serverless.AuthQrExchange, "clerk", "1"))
	router.HandleFunc("/api/auth/clerk/login", withQuery(serverless.AuthQrExchange, "clerk", "login"))
	router.HandleFunc("/api/auth/clerk/session", withQuery(serverless.AuthQrExchange, "clerk", "session"))
	router.HandleFunc("/api/auth/clerk/mobile/bridge/start", withQuery(serverless.AuthQrExchange, "bridge", "start"))
	router.HandleFunc("/api/auth/clerk/mobile/bridge/status", withQuery(serverless.AuthQrExchange, "bridge", "status"))
	router.HandleFunc("/api/auth/clerk/mobile/bridge/complete", withQuery(serverless.AuthQrExchange, "bridge", "complete"))
	router.HandleFunc("/api/auth/clerk/codex/state", withQuery(serverless.AuthQrExchange, "codex", "state"))
	router.HandleFunc("/api/courses/{courseID}/materials", withPathQuery(serverless.Materials, map[string]string{
		"courseID": "courseId",
	}))
	router.HandleFunc("/api/courses/{courseID}/materials/{resourceID}/text", withPathQuery(serverless.MaterialText, map[string]string{
		"courseID":   "courseId",
		"resourceID": "resourceId",
	}))
	router.HandleFunc("/api/courses/{courseID}/materials/{resourceID}/pdf", withPathQuery(serverless.PDF, map[string]string{
		"courseID":   "courseId",
		"resourceID": "resourceId",
	}))
	router.HandleFunc("/api/courses/{courseID}/recordings", withRouteQuery(serverless.Courses, map[string]string{
		"route": "recordings",
	}, map[string]string{
		"courseID": "courseId",
	}))
	router.HandleFunc("/api/calendar", withQuery(serverless.Courses, "route", "calendar"))
	router.HandleFunc("/api/webex/credentials", withQuery(serverless.Keys, "route", "webex-credentials"))
}

func withQuery(next http.HandlerFunc, key string, value string) http.HandlerFunc {
	return withRouteQuery(next, map[string]string{key: value}, nil)
}

func withPathQuery(next http.HandlerFunc, pathToQuery map[string]string) http.HandlerFunc {
	return withRouteQuery(next, nil, pathToQuery)
}

func withRouteQuery(next http.HandlerFunc, staticQuery map[string]string, pathToQuery map[string]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		request := r.Clone(r.Context())
		urlCopy := *r.URL
		query := urlCopy.Query()
		for key, value := range staticQuery {
			query.Set(key, value)
		}
		for pathName, queryName := range pathToQuery {
			if value := strings.TrimSpace(chi.URLParam(r, pathName)); value != "" {
				query.Set(queryName, value)
			}
		}
		urlCopy.RawQuery = query.Encode()
		request.URL = &urlCopy
		next(w, request)
	}
}

func resolveLogWriter(writer io.Writer) io.Writer {
	if writer != nil {
		return writer
	}
	return os.Stderr
}

func healthHandler(opts ServerOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		client, err := opts.ClientProvider()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if err := client.ValidateSession(); err != nil {
			status := http.StatusBadGateway
			if errors.Is(err, moodle.ErrSessionExpired) {
				status = http.StatusUnauthorized
			}
			writeError(w, status, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func coursesHandler(opts ServerOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		client, err := opts.ClientProvider()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		courses, err := client.FetchCourses()
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusOK, courses)
	}
}

func categoriesHandler(opts ServerOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		client, err := opts.ClientProvider()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		categoriesClient, ok := client.(categoryClient)
		if !ok {
			writeError(w, http.StatusNotImplemented, fmt.Errorf("moodle client does not support categories"))
			return
		}
		categories, err := categoriesClient.FetchCategories()
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusOK, categories)
	}
}

func courseResourcesHandler(opts ServerOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		courseID := strings.TrimSpace(chi.URLParam(r, "courseID"))
		if courseID == "" {
			writeError(w, http.StatusBadRequest, fmt.Errorf("courseID is required"))
			return
		}

		client, err := opts.ClientProvider()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		resources, _, err := client.FetchCourseResources(courseID)
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusOK, resources)
	}
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
