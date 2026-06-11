package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	contract "github.com/DotNaos/moodle-services/pkg/apicontracts"
	svc "github.com/DotNaos/moodle-services/pkg/moodleservices"
	"github.com/DotNaos/moodle-services/pkg/studypipeline"
)

func Materials(w http.ResponseWriter, r *http.Request) {
	if !svc.AllowMethods(w, r, http.MethodGet, http.MethodPost) {
		return
	}
	courseID := strings.TrimSpace(r.URL.Query().Get("courseId"))
	if courseID == "" {
		svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "courseId query parameter is required"})
		return
	}
	cfg := svc.LoadServerEnv()
	var studyStore *svc.Store
	var studyUserID string
	if cfg.DatabaseURL != "" {
		st, user, _, err := svc.AuthenticatedUser(r, cfg)
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		defer func() { _ = st.Close() }()
		studyStore = st
		studyUserID = user.ID
	}
	service, closeFn, err := svc.ServiceForRequest(r, cfg)
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	defer closeFn()
	materials, err := service.ListMaterials(courseID)
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	if r.URL.Query().Get("route") == "study-pipeline" {
		handleStudyPipeline(w, r, service, courseID, materials, studyStore, studyUserID)
		return
	}
	if r.Method != http.MethodGet {
		svc.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	svc.WriteJSON(w, http.StatusOK, contract.MaterialsResponse{Materials: materials})
}

func handleStudyPipeline(w http.ResponseWriter, r *http.Request, service svc.Service, courseID string, materials []svc.Resource, studyStore *svc.Store, studyUserID string) {
	action := strings.TrimSpace(r.URL.Query().Get("action"))
	stage := strings.TrimSpace(r.URL.Query().Get("stage"))
	options := studypipeline.RunOptions{
		Downloader: service.Client,
		Now:        time.Now(),
	}

	switch action {
	case "", "status":
		if r.Method != http.MethodGet {
			stage = defaultStage(stage)
			response, err := studypipeline.RunStage(courseID, materials, stage, options)
			if err != nil {
				svc.WriteError(w, err)
				return
			}
			if err := svc.RecordStudyPipelineResponse(r.Context(), studyStore, studyUserID, response); err != nil {
				svc.WriteError(w, err)
				return
			}
			svc.WriteJSON(w, http.StatusOK, response)
			return
		}
		svc.WriteJSON(w, http.StatusOK, studypipeline.Status(courseID, materials, options))
	case "stage":
		if r.Method != http.MethodPost {
			svc.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		response, err := studypipeline.RunStage(courseID, materials, defaultStage(stage), options)
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		if err := svc.RecordStudyPipelineResponse(r.Context(), studyStore, studyUserID, response); err != nil {
			svc.WriteError(w, err)
			return
		}
		svc.WriteJSON(w, http.StatusOK, response)
	case "script":
		script, err := studypipeline.LoadScript(courseID, materials, options)
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		svc.WriteJSON(w, http.StatusOK, map[string]string{"courseId": courseID, "scriptMarkdown": script})
	case "task-view":
		includeScript := r.URL.Query().Get("includeScript") != "0"
		view, err := studypipeline.LoadTaskView(courseID, materials, includeScript, options)
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		svc.WriteJSON(w, http.StatusOK, view)
	case "refine":
		if r.Method != http.MethodPost {
			svc.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		var input contract.StudyPipelineRefineRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if strings.TrimSpace(input.Model) == "" {
			svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "model is required; load /api/codex/models and pass one of the returned model ids"})
			return
		}
		if strings.Contains(r.Header.Get("Accept"), "application/x-ndjson") {
			w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusNotImplemented)
			_, _ = w.Write([]byte(`{"type":"error","error":"streaming refinement is available on the API server router"}` + "\n"))
			return
		}
		options.UserID = studyUserID
		response, err := studypipeline.RefineContent(r.Context(), courseID, materials, input, options)
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		svc.WriteJSON(w, http.StatusOK, response)
	case "chat":
		taskID := strings.TrimSpace(r.URL.Query().Get("taskId"))
		if taskID == "" {
			svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "taskId query parameter is required"})
			return
		}
		if r.Method == http.MethodGet {
			messages, _ := studypipeline.Messages("", courseID, taskID)
			svc.WriteJSON(w, http.StatusOK, map[string]any{"messages": messages})
			return
		}
		var input struct {
			Role string `json:"role"`
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		messages, err := studypipeline.AppendMessage("", courseID, taskID, input.Role, input.Text)
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		svc.WriteJSON(w, http.StatusOK, map[string]any{"messages": messages})
	case "attempts":
		taskID := strings.TrimSpace(r.URL.Query().Get("taskId"))
		if taskID == "" {
			svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "taskId query parameter is required"})
			return
		}
		var input contract.StudyPipelineAttempt
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := studypipeline.RecordAttempt("", courseID, taskID, input); err != nil {
			svc.WriteError(w, err)
			return
		}
		svc.WriteJSON(w, http.StatusOK, map[string]bool{"saved": true})
	case "task-status":
		taskID := strings.TrimSpace(r.URL.Query().Get("taskId"))
		if taskID == "" {
			svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "taskId query parameter is required"})
			return
		}
		var input contract.StudyPipelineTaskStatusRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := studypipeline.RecordTaskStatus("", courseID, taskID, input.Status); err != nil {
			svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		svc.WriteJSON(w, http.StatusOK, map[string]any{"saved": true, "status": input.Status})
	default:
		svc.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "unknown study pipeline action"})
	}
}

func defaultStage(stage string) string {
	if strings.TrimSpace(stage) == "" {
		return "curated"
	}
	return strings.TrimSpace(stage)
}
