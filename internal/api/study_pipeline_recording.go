package api

import (
	"context"
	"os"
	"strings"

	"github.com/DotNaos/moodle-services/internal/store"
	contract "github.com/DotNaos/moodle-services/pkg/apicontracts"
)

func recordLocalStudyPipeline(ctx context.Context, response contract.StudyPipelineResponse) error {
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
	return st.RecordStudyPipeline(ctx, store.StudyPipelineRecordInput{
		UserID:       userID,
		CourseID:     response.CourseID,
		Stage:        response.Stage,
		ArtifactRoot: response.ArtifactRoot,
		Summary:      response.Summary,
		Materials:    studyPipelineMaterialRecords(response.Materials),
		TaskLinks:    studyPipelineTaskLinkRecords(response.TaskLinks),
	})
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
