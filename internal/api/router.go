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
	"github.com/DotNaos/moodle-services/internal/auth"
	"github.com/DotNaos/moodle-services/internal/moodle"
	contract "github.com/DotNaos/moodle-services/pkg/apicontracts"
	"github.com/DotNaos/moodle-services/pkg/studypipeline"
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
	router.Get("/api/courses/{courseID}/study-pipeline", studyPipelineStatusRoute(opts))
	router.Post("/api/courses/{courseID}/study-pipeline", studyPipelineStageRoute(opts, "curated"))
	router.Post("/api/courses/{courseID}/study-pipeline/{stage}", studyPipelineStageRoute(opts, ""))
	router.Get("/api/courses/{courseID}/study-pipeline/status", studyPipelineStatusRoute(opts))
	router.Get("/api/courses/{courseID}/study-pipeline/script", studyPipelineScriptRoute(opts))
	router.Get("/api/courses/{courseID}/study-pipeline/task-view", studyPipelineTaskViewRoute(opts))
	router.Post("/api/courses/{courseID}/study-pipeline/refine", studyPipelineRefineRoute(opts))
	router.Get("/api/courses/{courseID}/study-pipeline/tasks/{taskID}/chat", studyPipelineChatRoute(opts))
	router.Post("/api/courses/{courseID}/study-pipeline/tasks/{taskID}/chat", studyPipelineChatRoute(opts))
	router.Post("/api/courses/{courseID}/study-pipeline/tasks/{taskID}/attempts", studyPipelineAttemptRoute(opts))
	router.Post("/api/courses/{courseID}/study-pipeline/tasks/{taskID}/status", studyPipelineTaskStatusRoute(opts))
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
	router.HandleFunc("/api/codex/status", withQuery(serverless.Codex, "action", "status"))
	router.HandleFunc("/api/codex/auth", withQuery(serverless.Codex, "action", "auth"))
	router.HandleFunc("/api/codex/auth/callback", withQuery(serverless.Codex, "action", "auth-callback"))
	router.HandleFunc("/api/codex/models", withQuery(serverless.Codex, "action", "models"))
	router.HandleFunc("/api/codex/files", withQuery(serverless.Codex, "action", "files"))
	router.HandleFunc("/api/codex/run", withQuery(serverless.Codex, "action", "run"))
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
	router.HandleFunc("/api/user/settings", serverless.UserSettings)
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

func studyPipelineStatusRoute(opts ServerOptions) http.HandlerFunc {
	localHandler := studyPipelineStatusHandler(opts)
	webHandler := withRouteQuery(serverless.Materials, map[string]string{
		"route": "study-pipeline",
	}, map[string]string{
		"courseID": "courseId",
	})
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSpace(r.Header.Get("X-Moodle-App-Key")) != "" || strings.TrimSpace(r.Header.Get("Authorization")) != "" {
			webHandler(w, r)
			return
		}
		if rejectHostedAnonymous(w, r) {
			return
		}
		localHandler(w, r)
	}
}

func studyPipelineStageRoute(opts ServerOptions, fallbackStage string) http.HandlerFunc {
	localHandler := studyPipelineStageHandler(opts, fallbackStage)
	webHandler := withRouteQuery(serverless.Materials, map[string]string{
		"route":  "study-pipeline",
		"action": "stage",
		"stage":  fallbackStage,
	}, map[string]string{
		"courseID": "courseId",
		"stage":    "stage",
	})
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSpace(r.Header.Get("X-Moodle-App-Key")) != "" || strings.TrimSpace(r.Header.Get("Authorization")) != "" {
			webHandler(w, r)
			return
		}
		if rejectHostedAnonymous(w, r) {
			return
		}
		localHandler(w, r)
	}
}

func studyPipelineScriptRoute(opts ServerOptions) http.HandlerFunc {
	return studyPipelineReadRoute(opts, "script")
}

func studyPipelineTaskViewRoute(opts ServerOptions) http.HandlerFunc {
	return studyPipelineReadRoute(opts, "task-view")
}

func studyPipelineRefineRoute(opts ServerOptions) http.HandlerFunc {
	localHandler := studyPipelineRefineHandler(opts)
	webHandler := withRouteQuery(serverless.Materials, map[string]string{
		"route":  "study-pipeline",
		"action": "refine",
	}, map[string]string{
		"courseID": "courseId",
	})
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSpace(r.Header.Get("X-Moodle-App-Key")) != "" || strings.TrimSpace(r.Header.Get("Authorization")) != "" {
			webHandler(w, r)
			return
		}
		if rejectHostedAnonymous(w, r) {
			return
		}
		localHandler(w, r)
	}
}

func studyPipelineChatRoute(opts ServerOptions) http.HandlerFunc {
	localHandler := studyPipelineTaskStateHandler(opts, "chat")
	webHandler := withRouteQuery(serverless.Materials, map[string]string{
		"route":  "study-pipeline",
		"action": "chat",
	}, map[string]string{
		"courseID": "courseId",
		"taskID":   "taskId",
	})
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSpace(r.Header.Get("X-Moodle-App-Key")) != "" || strings.TrimSpace(r.Header.Get("Authorization")) != "" {
			webHandler(w, r)
			return
		}
		if rejectHostedAnonymous(w, r) {
			return
		}
		localHandler(w, r)
	}
}

func studyPipelineAttemptRoute(opts ServerOptions) http.HandlerFunc {
	localHandler := studyPipelineTaskStateHandler(opts, "attempts")
	webHandler := withRouteQuery(serverless.Materials, map[string]string{
		"route":  "study-pipeline",
		"action": "attempts",
	}, map[string]string{
		"courseID": "courseId",
		"taskID":   "taskId",
	})
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSpace(r.Header.Get("X-Moodle-App-Key")) != "" || strings.TrimSpace(r.Header.Get("Authorization")) != "" {
			webHandler(w, r)
			return
		}
		if rejectHostedAnonymous(w, r) {
			return
		}
		localHandler(w, r)
	}
}

func studyPipelineTaskStatusRoute(opts ServerOptions) http.HandlerFunc {
	localHandler := studyPipelineTaskStateHandler(opts, "status")
	webHandler := withRouteQuery(serverless.Materials, map[string]string{
		"route":  "study-pipeline",
		"action": "task-status",
	}, map[string]string{
		"courseID": "courseId",
		"taskID":   "taskId",
	})
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSpace(r.Header.Get("X-Moodle-App-Key")) != "" || strings.TrimSpace(r.Header.Get("Authorization")) != "" {
			webHandler(w, r)
			return
		}
		if rejectHostedAnonymous(w, r) {
			return
		}
		localHandler(w, r)
	}
}

func studyPipelineReadRoute(opts ServerOptions, action string) http.HandlerFunc {
	localHandler := studyPipelineReadHandler(opts, action)
	webHandler := withRouteQuery(serverless.Materials, map[string]string{
		"route":  "study-pipeline",
		"action": action,
	}, map[string]string{
		"courseID": "courseId",
	})
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSpace(r.Header.Get("X-Moodle-App-Key")) != "" || strings.TrimSpace(r.Header.Get("Authorization")) != "" {
			webHandler(w, r)
			return
		}
		if rejectHostedAnonymous(w, r) {
			return
		}
		localHandler(w, r)
	}
}

func studyPipelineStatusHandler(opts ServerOptions) http.HandlerFunc {
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

		writeJSON(w, http.StatusOK, studypipeline.Status(courseID, resources, studypipeline.RunOptions{
			Now: time.Now(),
		}))
	}
}

func studyPipelineStageHandler(opts ServerOptions, fallbackStage string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		courseID, resources, downloader, ok := studyPipelineContext(w, r, opts)
		if !ok {
			return
		}
		stage := strings.TrimSpace(chi.URLParam(r, "stage"))
		if stage == "" {
			stage = fallbackStage
		}
		response, err := studypipeline.RunStage(courseID, resources, stage, studypipeline.RunOptions{
			Downloader: downloader,
			Now:        time.Now(),
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if err := recordLocalStudyPipeline(r.Context(), response); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, response)
	}
}

func studyPipelineReadHandler(opts ServerOptions, action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		courseID, resources, downloader, ok := studyPipelineContext(w, r, opts)
		if !ok {
			return
		}
		options := studypipeline.RunOptions{Downloader: downloader, Now: time.Now()}
		switch action {
		case "script":
			script, err := studypipeline.LoadScript(courseID, resources, options)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"courseId": courseID, "scriptMarkdown": script})
		case "task-view":
			includeScript := r.URL.Query().Get("includeScript") != "0"
			view, err := studypipeline.LoadTaskView(courseID, resources, includeScript, options)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, http.StatusOK, view)
		default:
			writeError(w, http.StatusNotFound, fmt.Errorf("unknown study pipeline action %q", action))
		}
	}
}

func studyPipelineTaskStateHandler(opts ServerOptions, action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		courseID := strings.TrimSpace(chi.URLParam(r, "courseID"))
		taskID := strings.TrimSpace(chi.URLParam(r, "taskID"))
		if courseID == "" || taskID == "" {
			writeError(w, http.StatusBadRequest, fmt.Errorf("courseID and taskID are required"))
			return
		}
		if action == "chat" {
			if r.Method == http.MethodGet {
				messages, err := studypipeline.Messages("", courseID, taskID)
				if err != nil && !errors.Is(err, os.ErrNotExist) {
					writeError(w, http.StatusInternalServerError, err)
					return
				}
				writeJSON(w, http.StatusOK, map[string]any{"messages": messages})
				return
			}
			var input struct {
				Role string `json:"role"`
				Text string `json:"text"`
			}
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			messages, err := studypipeline.AppendMessage("", courseID, taskID, input.Role, input.Text)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"messages": messages})
			return
		}
		if action == "status" {
			var input contract.StudyPipelineTaskStatusRequest
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if err := studypipeline.RecordTaskStatus("", courseID, taskID, input.Status); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"saved": true, "status": input.Status})
			return
		}

		var input contract.StudyPipelineAttempt
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			var wrapped struct {
				UserAnswer string                        `json:"userAnswer"`
				Verdict    contract.StudyPipelineVerdict `json:"verdict"`
			}
			if err := json.NewDecoder(r.Body).Decode(&wrapped); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			input.UserAnswer = wrapped.UserAnswer
			input.Verdict = wrapped.Verdict
		}
		if err := studypipeline.RecordAttempt("", courseID, taskID, input); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"saved": true})
	}
}

func studyPipelineRefineHandler(opts ServerOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		courseID, resources, downloader, ok := studyPipelineContext(w, r, opts)
		if !ok {
			return
		}
		var input contract.StudyPipelineRefineRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if strings.TrimSpace(input.Model) == "" {
			writeError(w, http.StatusBadRequest, fmt.Errorf("model is required; load /api/codex/models and pass one of the returned model ids"))
			return
		}
		if acceptsNDJSON(r) {
			streamStudyPipelineRefine(w, r, courseID, resources, downloader, input)
			return
		}
		response, err := studypipeline.RefineContent(r.Context(), courseID, resources, input, studypipeline.RunOptions{
			Downloader: downloader,
			Now:        time.Now(),
			UserID:     strings.TrimSpace(r.Header.Get("X-Clerk-User-Id")),
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, response)
	}
}

func streamStudyPipelineRefine(w http.ResponseWriter, r *http.Request, courseID string, resources []moodle.Resource, downloader studypipeline.Downloader, input contract.StudyPipelineRefineRequest) {
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	encoder := json.NewEncoder(w)
	emit := func(event contract.StudyPipelineRefineEvent) {
		_ = encoder.Encode(event)
		if flusher != nil {
			flusher.Flush()
		}
	}
	emit(contract.StudyPipelineRefineEvent{
		Type:            "queued",
		Message:         "Queued Codex refinement on the server.",
		Model:           strings.TrimSpace(input.Model),
		ReasoningEffort: strings.TrimSpace(input.ReasoningEffort),
	})
	response, err := studypipeline.RefineContent(r.Context(), courseID, resources, input, studypipeline.RunOptions{
		Downloader:  downloader,
		Now:         time.Now(),
		UserID:      strings.TrimSpace(r.Header.Get("X-Clerk-User-Id")),
		RefineEvent: emit,
	})
	if err != nil {
		emit(contract.StudyPipelineRefineEvent{
			Type:  "error",
			Error: studyRefineErrorMessage(err),
		})
		return
	}
	target := response.Target
	emit(contract.StudyPipelineRefineEvent{
		Type:           "done",
		Message:        "Codex refinement finished.",
		Target:         &target,
		ContentPreview: response.ContentPreview,
	})
}

func acceptsNDJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/x-ndjson")
}

func studyRefineErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if strings.Contains(message, "401 Unauthorized") || strings.Contains(message, "Missing bearer") || strings.Contains(message, "Not logged in") {
		return "Codex is not connected for this user. Connect ChatGPT before improving content."
	}
	return message
}

func studyPipelineContext(w http.ResponseWriter, r *http.Request, opts ServerOptions) (string, []moodle.Resource, studypipeline.Downloader, bool) {
	if rejectHostedAnonymous(w, r) {
		return "", nil, nil, false
	}
	courseID := strings.TrimSpace(chi.URLParam(r, "courseID"))
	if courseID == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("courseID is required"))
		return "", nil, nil, false
	}
	client, err := opts.ClientProvider()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return "", nil, nil, false
	}
	resources, _, err := client.FetchCourseResources(courseID)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return "", nil, nil, false
	}
	downloader, _ := client.(studypipeline.Downloader)
	return courseID, resources, downloader, true
}

func resolveLogWriter(writer io.Writer) io.Writer {
	if writer != nil {
		return writer
	}
	return os.Stderr
}

func healthHandler(opts ServerOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if hostedMode() {
			writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
			return
		}
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
		if hostedMode() {
			serverless.Courses(w, r)
			return
		}
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
		if hostedMode() {
			withQuery(serverless.Courses, "route", "categories")(w, r)
			return
		}
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
		if rejectHostedAnonymous(w, r) {
			return
		}
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

func rejectHostedAnonymous(w http.ResponseWriter, r *http.Request) bool {
	if !hostedMode() || auth.APIKeyFromRequest(r) != "" {
		return false
	}
	writeError(w, http.StatusUnauthorized, auth.ErrUnauthorized)
	return true
}

func hostedMode() bool {
	return strings.TrimSpace(os.Getenv("DATABASE_URL")) != ""
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
