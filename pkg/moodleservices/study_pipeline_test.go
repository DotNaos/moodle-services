package moodleservices

import (
	"testing"

	"github.com/DotNaos/moodle-services/internal/store"
	contract "github.com/DotNaos/moodle-services/pkg/apicontracts"
)

func TestStudyPipelineRunOutcomeUsesCurationAccountability(t *testing.T) {
	cases := []struct {
		name       string
		response   contract.StudyPipelineResponse
		wantStatus string
		wantError  string
	}{
		{
			name: "non curated response succeeds",
			response: contract.StudyPipelineResponse{
				Stage: "extracted",
			},
			wantStatus: "succeeded",
		},
		{
			name: "complete curated response succeeds",
			response: contract.StudyPipelineResponse{
				Stage: "curated",
				CurationChecklist: &store.StudyPipelineCurationChecklist{
					Status: "complete",
				},
				ElementDecisions: []store.StudyPipelineElementDecision{
					{SourceElementID: "text-1", Outcome: "used_in_output"},
					{SourceElementID: "image-1", Outcome: "ignored"},
				},
			},
			wantStatus: "succeeded",
		},
		{
			name: "incomplete checklist fails",
			response: contract.StudyPipelineResponse{
				Stage: "curated",
				CurationChecklist: &store.StudyPipelineCurationChecklist{
					Status: "incomplete",
				},
			},
			wantStatus: "failed",
			wantError:  "curation checklist incomplete",
		},
		{
			name: "unaccounted element fails",
			response: contract.StudyPipelineResponse{
				Stage: "curated",
				CurationChecklist: &store.StudyPipelineCurationChecklist{
					Status: "complete",
				},
				ElementDecisions: []store.StudyPipelineElementDecision{
					{SourceElementID: "image-1", Outcome: "needs_review"},
				},
			},
			wantStatus: "failed",
			wantError:  "element accountability incomplete",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotStatus, gotError := studyPipelineRunOutcome(tc.response)
			if gotStatus != tc.wantStatus || gotError != tc.wantError {
				t.Fatalf("unexpected outcome status=%q error=%q", gotStatus, gotError)
			}
		})
	}
}
