package api

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/DotNaos/moodle-services/internal/store"
	contract "github.com/DotNaos/moodle-services/pkg/apicontracts"
	"github.com/DotNaos/moodle-services/pkg/studypipeline"
)

func recordLocalStudyPipeline(ctx context.Context, response *contract.StudyPipelineResponse) error {
	if response == nil {
		return nil
	}
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return nil
	}
	st, err := store.Open(databaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	userID, err := st.EnsureStudyPipelineSystemUser(ctx)
	if err != nil {
		return err
	}
	run, err := st.RecordStudyPipeline(ctx, store.StudyPipelineRecordInput{
		UserID:       userID,
		CourseID:     response.CourseID,
		Stage:        response.Stage,
		Engine:       response.Engine,
		ConfigHash:   response.ConfigHash,
		ArtifactRoot: response.ArtifactRoot,
		Summary:      response.Summary,
		Materials:    studyPipelineMaterialRecords(response.Materials),
		TaskLinks:    studyPipelineTaskLinkRecords(response.TaskLinks),
	})
	if err != nil {
		return err
	}
	if run.ID != "" {
		response.Run = &run
	}
	return nil
}

func recordLocalStudyPipelineFailure(ctx context.Context, courseID string, stage string, options studypipeline.RunOptions, err error) error {
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" || strings.TrimSpace(courseID) == "" {
		return nil
	}
	st, openErr := store.Open(databaseURL)
	if openErr != nil {
		return openErr
	}
	defer func() { _ = st.Close() }()

	userID, ensureErr := st.EnsureStudyPipelineSystemUser(ctx)
	if ensureErr != nil {
		return ensureErr
	}
	now := time.Now().UTC()
	_, recordErr := st.RecordStudyPipeline(ctx, store.StudyPipelineRecordInput{
		UserID:       userID,
		CourseID:     courseID,
		Stage:        defaultStudyPipelineStage(stage),
		Engine:       options.Engine,
		ConfigHash:   options.ConfigHash,
		ArtifactRoot: studypipeline.CourseArtifactRoot("", courseID),
		Status:       "failed",
		Error:        errorMessage(err),
		StartedAt:    now,
		FinishedAt:   now,
	})
	return recordErr
}

func studyPipelineMaterialRecords(materials []contract.StudyPipelineMaterial) []store.StudyPipelineMaterialRecord {
	records := make([]store.StudyPipelineMaterialRecord, 0, len(materials))
	for _, material := range materials {
		records = append(records, store.StudyPipelineMaterialRecord{
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
	return records
}

func defaultStudyPipelineStage(stage string) string {
	stage = strings.TrimSpace(stage)
	if stage == "" {
		return "curated"
	}
	return stage
}

func errorMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func studyPipelineTaskLinkRecords(links []contract.StudyPipelineTaskLink) []store.StudyPipelineTaskLinkRecord {
	records := make([]store.StudyPipelineTaskLinkRecord, 0, len(links))
	for _, link := range links {
		record := store.StudyPipelineTaskLinkRecord{
			TaskResourceID: link.Task.ID,
			Status:         link.Status,
		}
		if link.Solution != nil {
			record.SolutionResourceID = link.Solution.ID
		}
		records = append(records, record)
	}
	return records
}
