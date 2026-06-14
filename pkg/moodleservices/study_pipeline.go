package moodleservices

import (
	"context"
	"strings"

	contract "github.com/DotNaos/moodle-services/pkg/apicontracts"
)

func RecordStudyPipelineResponse(ctx context.Context, st *Store, userID string, response contract.StudyPipelineResponse) (StudyPipelineRunRecord, error) {
	if st == nil || strings.TrimSpace(userID) == "" {
		return StudyPipelineRunRecord{}, nil
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
	status, runError := studyPipelineRunOutcome(response)
	run, err := st.RecordStudyPipeline(ctx, StudyPipelineRecordInput{
		UserID:            userID,
		CourseID:          response.CourseID,
		Stage:             response.Stage,
		Engine:            response.Engine,
		ConfigHash:        response.ConfigHash,
		ArtifactRoot:      response.ArtifactRoot,
		Status:            status,
		Error:             runError,
		ArtifactRefs:      response.ArtifactRefs,
		CurationChecklist: response.CurationChecklist,
		ElementDecisions:  response.ElementDecisions,
		Summary:           response.Summary,
		Materials:         materials,
		TaskLinks:         links,
	})
	if err != nil {
		return StudyPipelineRunRecord{}, err
	}
	return run, nil
}

func studyPipelineRunOutcome(response contract.StudyPipelineResponse) (string, string) {
	if response.CurationChecklist == nil {
		return "succeeded", ""
	}
	for _, decision := range response.ElementDecisions {
		switch strings.TrimSpace(decision.Outcome) {
		case "needs_review", "failed":
			return "failed", "element accountability incomplete"
		}
	}
	if strings.TrimSpace(response.CurationChecklist.Status) != "complete" {
		return "failed", "curation checklist incomplete"
	}
	return "succeeded", ""
}
