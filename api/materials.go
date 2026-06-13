package handler

import (
	"context"
	"encoding/json"
	"errors"
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
			applyStageRequestOptions(r, &options)
			stage = defaultStage(stage)
			response, err := studypipeline.RunStage(courseID, materials, stage, options)
			if err != nil {
				if recordErr := recordStudyPipelineFailure(r.Context(), studyStore, studyUserID, courseID, stage, options, err); recordErr != nil {
					svc.WriteError(w, recordErr)
					return
				}
				svc.WriteError(w, err)
				return
			}
			if run, err := svc.RecordStudyPipelineResponse(r.Context(), studyStore, studyUserID, response); err != nil {
				svc.WriteError(w, err)
				return
			} else if run.ID != "" {
				response.Run = &run
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
		applyStageRequestOptions(r, &options)
		response, err := studypipeline.RunStage(courseID, materials, defaultStage(stage), options)
		if err != nil {
			if recordErr := recordStudyPipelineFailure(r.Context(), studyStore, studyUserID, courseID, defaultStage(stage), options, err); recordErr != nil {
				svc.WriteError(w, recordErr)
				return
			}
			svc.WriteError(w, err)
			return
		}
		if run, err := svc.RecordStudyPipelineResponse(r.Context(), studyStore, studyUserID, response); err != nil {
			svc.WriteError(w, err)
			return
		} else if run.ID != "" {
			response.Run = &run
		}
		svc.WriteJSON(w, http.StatusOK, response)
	case "runs":
		if r.Method != http.MethodGet {
			svc.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		runs, selections, err := studyStore.ListStudyPipelineRuns(r.Context(), studyUserID, courseID)
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		svc.WriteJSON(w, http.StatusOK, contract.StudyPipelineRunsResponse{
			CourseID:         courseID,
			Runs:             runs,
			ActiveSelections: selections,
		})
	case "select-run":
		if r.Method != http.MethodPost {
			svc.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		if studyStore == nil {
			svc.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "pipeline run storage is not configured"})
			return
		}
		runID := strings.TrimSpace(r.URL.Query().Get("runId"))
		if runID == "" {
			svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "runId query parameter is required"})
			return
		}
		var input contract.StudyPipelineSelectRunRequest
		_ = json.NewDecoder(r.Body).Decode(&input)
		selection, err := studyStore.SelectActiveStudyPipelineRun(r.Context(), studyUserID, courseID, runID, input.Reason)
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		svc.WriteJSON(w, http.StatusOK, contract.StudyPipelineSelectRunResponse{Selection: selection})
	case "review":
		if r.Method != http.MethodGet {
			svc.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		feedback, proposals, audit, err := studyStore.ListStudyPipelineReview(r.Context(), courseID)
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		svc.WriteJSON(w, http.StatusOK, contract.StudyPipelineReviewResponse{
			CourseID:  courseID,
			Feedback:  feedback,
			Proposals: proposals,
			Audit:     audit,
		})
	case "feedback":
		if r.Method != http.MethodPost {
			svc.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		if studyStore == nil {
			svc.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "pipeline review storage is not configured"})
			return
		}
		var input contract.StudyPipelineFeedbackRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		feedback, err := studyStore.RecordStudyPipelineFeedback(r.Context(), studyUserID, courseID, svc.StudyPipelineFeedbackInput{
			TargetID:         input.TargetID,
			TargetKind:       input.TargetKind,
			FeedbackType:     input.FeedbackType,
			Message:          input.Message,
			SourceRunID:      input.SourceRunID,
			SourceArtifactID: input.SourceArtifactID,
		})
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		svc.WriteJSON(w, http.StatusOK, contract.StudyPipelineFeedbackResponse{Feedback: feedback})
	case "proposals":
		if r.Method != http.MethodPost {
			svc.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		if studyStore == nil {
			svc.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "pipeline review storage is not configured"})
			return
		}
		var input contract.StudyPipelineProposalRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		proposal, err := studyStore.RecordStudyPipelineProposal(r.Context(), studyUserID, courseID, svc.StudyPipelineProposalInput{
			TargetID:         input.TargetID,
			TargetKind:       input.TargetKind,
			Title:            input.Title,
			ContentPreview:   input.ContentPreview,
			SourceRunID:      input.SourceRunID,
			SourceArtifactID: input.SourceArtifactID,
			Model:            input.Model,
		})
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		svc.WriteJSON(w, http.StatusOK, contract.StudyPipelineProposalResponse{Proposal: proposal})
	case "submit-proposal":
		if r.Method != http.MethodPost {
			svc.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		if studyStore == nil {
			svc.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "pipeline review storage is not configured"})
			return
		}
		proposalID := strings.TrimSpace(r.URL.Query().Get("proposalId"))
		if proposalID == "" {
			svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "proposalId query parameter is required"})
			return
		}
		proposal, err := studyStore.SubmitStudyPipelineProposal(r.Context(), studyUserID, courseID, proposalID)
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		svc.WriteJSON(w, http.StatusOK, contract.StudyPipelineProposalResponse{Proposal: proposal})
	case "promote-proposal":
		if r.Method != http.MethodPost {
			svc.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		if studyStore == nil {
			svc.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "pipeline review storage is not configured"})
			return
		}
		proposalID := strings.TrimSpace(r.URL.Query().Get("proposalId"))
		if proposalID == "" {
			svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "proposalId query parameter is required"})
			return
		}
		input := moderationRequest(r)
		proposal, audit, err := studyStore.PromoteStudyPipelineProposal(r.Context(), studyUserID, courseID, proposalID, input.Reason)
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		svc.WriteJSON(w, http.StatusOK, contract.StudyPipelineProposalModerationResponse{Proposal: proposal, Audit: audit})
	case "dismiss-proposal":
		if r.Method != http.MethodPost {
			svc.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		if studyStore == nil {
			svc.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "pipeline review storage is not configured"})
			return
		}
		proposalID := strings.TrimSpace(r.URL.Query().Get("proposalId"))
		if proposalID == "" {
			svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "proposalId query parameter is required"})
			return
		}
		input := moderationRequest(r)
		proposal, audit, err := studyStore.DismissStudyPipelineProposal(r.Context(), studyUserID, courseID, proposalID, input.Reason)
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		svc.WriteJSON(w, http.StatusOK, contract.StudyPipelineProposalModerationResponse{Proposal: proposal, Audit: audit})
	case "resolve-feedback":
		if r.Method != http.MethodPost {
			svc.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		if studyStore == nil {
			svc.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "pipeline review storage is not configured"})
			return
		}
		feedbackID := strings.TrimSpace(r.URL.Query().Get("feedbackId"))
		if feedbackID == "" {
			svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "feedbackId query parameter is required"})
			return
		}
		input := moderationRequest(r)
		feedback, audit, err := studyStore.ResolveStudyPipelineFeedback(r.Context(), studyUserID, courseID, feedbackID, input.Reason)
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		svc.WriteJSON(w, http.StatusOK, contract.StudyPipelineFeedbackModerationResponse{Feedback: feedback, Audit: audit})
	case "dismiss-feedback":
		if r.Method != http.MethodPost {
			svc.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		if studyStore == nil {
			svc.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "pipeline review storage is not configured"})
			return
		}
		feedbackID := strings.TrimSpace(r.URL.Query().Get("feedbackId"))
		if feedbackID == "" {
			svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "feedbackId query parameter is required"})
			return
		}
		input := moderationRequest(r)
		feedback, audit, err := studyStore.DismissStudyPipelineFeedback(r.Context(), studyUserID, courseID, feedbackID, input.Reason)
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		svc.WriteJSON(w, http.StatusOK, contract.StudyPipelineFeedbackModerationResponse{Feedback: feedback, Audit: audit})
	case "publish-run":
		if r.Method != http.MethodPost {
			svc.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		if studyStore == nil {
			svc.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "pipeline run storage is not configured"})
			return
		}
		runID := strings.TrimSpace(r.URL.Query().Get("runId"))
		if runID == "" {
			svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "runId query parameter is required"})
			return
		}
		input := moderationRequest(r)
		selection, audit, err := studyStore.PublishStudyPipelineRun(r.Context(), studyUserID, courseID, runID, input.Reason)
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		svc.WriteJSON(w, http.StatusOK, contract.StudyPipelinePublishRunResponse{Selection: &selection, Audit: audit})
	case "unpublish-run":
		if r.Method != http.MethodPost {
			svc.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		if studyStore == nil {
			svc.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "pipeline run storage is not configured"})
			return
		}
		runID := strings.TrimSpace(r.URL.Query().Get("runId"))
		if runID == "" {
			svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "runId query parameter is required"})
			return
		}
		input := moderationRequest(r)
		audit, err := studyStore.UnpublishStudyPipelineRun(r.Context(), studyUserID, courseID, runID, input.Reason)
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		svc.WriteJSON(w, http.StatusOK, contract.StudyPipelinePublishRunResponse{Audit: audit})
	case "inventory":
		if r.Method != http.MethodGet {
			svc.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		inventory, err := studypipeline.LoadInventory(courseID, materials, options)
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		svc.WriteJSON(w, http.StatusOK, inventory)
	case "extracted-documents":
		if r.Method != http.MethodGet {
			svc.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		documents, err := studypipeline.LoadExtractedDocuments(courseID, materials, options)
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		svc.WriteJSON(w, http.StatusOK, documents)
	case "extracted-asset":
		if r.Method != http.MethodGet {
			svc.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		data, contentType, err := studypipeline.OpenExtractedAsset(courseID, r.URL.Query().Get("path"), options)
		if err != nil {
			switch {
			case errors.Is(err, studypipeline.ErrInvalidExtractedAssetPath):
				svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			case errors.Is(err, studypipeline.ErrExtractedAssetNotFound):
				svc.WriteJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			default:
				svc.WriteError(w, err)
			}
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "private, max-age=3600")
		_, _ = w.Write(data)
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
			options.UserID = studyUserID
			options.RefineEvent = emit
			response, err := studypipeline.RefineContent(r.Context(), courseID, materials, input, options)
			if err != nil {
				emit(contract.StudyPipelineRefineEvent{
					Type:  "error",
					Error: err.Error(),
				})
				return
			}
			if studyStore != nil {
				_, _ = studyStore.RecordStudyPipelineProposal(r.Context(), studyUserID, courseID, svc.StudyPipelineProposalInput{
					TargetID:       response.Target.ID,
					TargetKind:     response.Target.Kind,
					Title:          response.Target.Title,
					ContentPreview: response.ContentPreview,
					Model:          response.Target.Model,
				})
			}
			target := response.Target
			emit(contract.StudyPipelineRefineEvent{
				Type:           "done",
				Message:        "Codex refinement finished.",
				Target:         &target,
				ContentPreview: response.ContentPreview,
			})
			return
		}
		options.UserID = studyUserID
		response, err := studypipeline.RefineContent(r.Context(), courseID, materials, input, options)
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		if studyStore != nil {
			_, _ = studyStore.RecordStudyPipelineProposal(r.Context(), studyUserID, courseID, svc.StudyPipelineProposalInput{
				TargetID:       response.Target.ID,
				TargetKind:     response.Target.Kind,
				Title:          response.Target.Title,
				ContentPreview: response.ContentPreview,
				Model:          response.Target.Model,
			})
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

func applyStageRequestOptions(r *http.Request, options *studypipeline.RunOptions) {
	if r.Body == nil {
		return
	}
	var input contract.StudyPipelineStageRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		return
	}
	options.Engine = input.Engine
	options.ConfigHash = input.ConfigHash
}

func moderationRequest(r *http.Request) contract.StudyPipelineModerationRequest {
	var input contract.StudyPipelineModerationRequest
	if r.Body == nil {
		return input
	}
	_ = json.NewDecoder(r.Body).Decode(&input)
	return input
}

func defaultStage(stage string) string {
	if strings.TrimSpace(stage) == "" {
		return "curated"
	}
	return strings.TrimSpace(stage)
}

func recordStudyPipelineFailure(ctx context.Context, st *svc.Store, userID string, courseID string, stage string, options studypipeline.RunOptions, runErr error) error {
	if st == nil || strings.TrimSpace(userID) == "" || strings.TrimSpace(courseID) == "" {
		return nil
	}
	now := time.Now().UTC()
	_, err := st.RecordStudyPipeline(ctx, svc.StudyPipelineRecordInput{
		UserID:       userID,
		CourseID:     courseID,
		Stage:        defaultStage(stage),
		Engine:       options.Engine,
		ConfigHash:   options.ConfigHash,
		ArtifactRoot: studypipeline.CourseArtifactRoot("", courseID),
		Status:       "failed",
		Error:        errorMessage(runErr),
		StartedAt:    now,
		FinishedAt:   now,
	})
	return err
}

func errorMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
