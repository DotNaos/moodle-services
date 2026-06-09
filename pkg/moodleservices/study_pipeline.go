package moodleservices

import (
	"context"
	"strings"

	contract "github.com/DotNaos/moodle-services/pkg/apicontracts"
)

func RecordStudyPipelineResponse(ctx context.Context, st *Store, userID string, response contract.StudyPipelineResponse) error {
	if st == nil || strings.TrimSpace(userID) == "" {
		return nil
	}
	materials := make([]StudyPipelineMaterialRecord, 0, len(response.Materials))
	for _, material := range response.Materials {
		materials = append(materials, StudyPipelineMaterialRecord{
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
	links := make([]StudyPipelineTaskLinkRecord, 0, len(response.TaskLinks))
	for _, link := range response.TaskLinks {
		record := StudyPipelineTaskLinkRecord{
			TaskResourceID: link.Task.ID,
			Status:         link.Status,
		}
		if link.Solution != nil {
			record.SolutionResourceID = link.Solution.ID
		}
		links = append(links, record)
	}
	return st.RecordStudyPipeline(ctx, StudyPipelineRecordInput{
		UserID:       userID,
		CourseID:     response.CourseID,
		Stage:        response.Stage,
		ArtifactRoot: response.ArtifactRoot,
		Summary:      response.Summary,
		Materials:    materials,
		TaskLinks:    links,
	})
}
