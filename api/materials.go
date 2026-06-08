package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/DotNaos/moodle-services/internal/moodle"
	"github.com/DotNaos/moodle-services/internal/store"
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

func handleStudyPipeline(w http.ResponseWriter, r *http.Request, service svc.Service, courseID string, materials []moodle.Resource, studyStore *svc.Store, studyUserID string) {
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
			if err := recordStudyPipeline(r, studyStore, studyUserID, response); err != nil {
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
		if err := recordStudyPipeline(r, studyStore, studyUserID, response); err != nil {
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
	default:
		svc.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "unknown study pipeline action"})
	}
}

func recordStudyPipeline(r *http.Request, st *svc.Store, userID string, response contract.StudyPipelineResponse) error {
	if st == nil || strings.TrimSpace(userID) == "" {
		return nil
	}
	return st.RecordStudyPipeline(r.Context(), store.StudyPipelineRecordInput{
		UserID:       userID,
		CourseID:     response.CourseID,
		Stage:        response.Stage,
		ArtifactRoot: response.ArtifactRoot,
		Summary:      response.Summary,
		Materials:    studyMaterialRecords(response.Materials),
		TaskLinks:    studyTaskLinkRecords(response.TaskLinks),
	})
}

func studyMaterialRecords(materials []contract.StudyPipelineMaterial) []store.StudyPipelineMaterialRecord {
	out := make([]store.StudyPipelineMaterialRecord, 0, len(materials))
	for _, material := range materials {
		out = append(out, store.StudyPipelineMaterialRecord{
			ID:             material.ID,
			Name:           material.Name,
			URL:            material.URL,
			ResourceType:   material.ResourceType,
			FileType:       material.FileType,
			SectionID:      material.SectionID,
			SectionName:    material.SectionName,
			Classification: material.Type,
		})
	}
	return out
}

func studyTaskLinkRecords(links []contract.StudyPipelineTaskLink) []store.StudyPipelineTaskLinkRecord {
	out := make([]store.StudyPipelineTaskLinkRecord, 0, len(links))
	for _, link := range links {
		record := store.StudyPipelineTaskLinkRecord{
			TaskResourceID: link.Task.ID,
			Status:         link.Status,
		}
		if link.Solution != nil {
			record.SolutionResourceID = link.Solution.ID
		}
		out = append(out, record)
	}
	return out
}

func defaultStage(stage string) string {
	if strings.TrimSpace(stage) == "" {
		return "curated"
	}
	return strings.TrimSpace(stage)
}
