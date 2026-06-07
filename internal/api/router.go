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

	"github.com/DotNaos/moodle-services/internal/moodle"
	"github.com/DotNaos/moodle-services/internal/studypipeline"
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
	StudyWorkspace string
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
		localCORSMiddleware,
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
	router.Get("/api/study-pipeline/courses", studyPipelineCoursesHandler(opts))
	router.Get("/api/study-pipeline/courses/{courseSlug}", studyPipelineCourseHandler(opts))
	registerCommandRoutes(router, opts)

	return router, nil
}

func localCORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "authorization, content-type, x-moodle-app-key")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func studyPipelineCoursesHandler(opts ServerOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		payload, err := studypipeline.Scan(studypipeline.Options{
			Workspace: firstNonEmpty(r.URL.Query().Get("workspace"), opts.StudyWorkspace),
			Term:      r.URL.Query().Get("term"),
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, payload)
	}
}

func studyPipelineCourseHandler(opts ServerOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		courseSlug := strings.TrimSpace(chi.URLParam(r, "courseSlug"))
		if courseSlug == "" {
			writeError(w, http.StatusBadRequest, fmt.Errorf("courseSlug is required"))
			return
		}
		payload, err := studypipeline.Scan(studypipeline.Options{
			Workspace: firstNonEmpty(r.URL.Query().Get("workspace"), opts.StudyWorkspace),
			Term:      r.URL.Query().Get("term"),
			Course:    courseSlug,
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if len(payload.Courses) == 0 {
			writeError(w, http.StatusNotFound, fmt.Errorf("course %q was not found in the study pipeline workspace", courseSlug))
			return
		}
		writeJSON(w, http.StatusOK, payload.Courses[0])
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
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
