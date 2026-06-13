package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

type StudyPipelineFeedbackInput struct {
	TargetID         string
	TargetKind       string
	FeedbackType     string
	Message          string
	SourceRunID      string
	SourceArtifactID string
}

type StudyPipelineFeedbackRecord struct {
	ID               string    `json:"id"`
	CourseID         string    `json:"courseId"`
	TargetID         string    `json:"targetId"`
	TargetKind       string    `json:"targetKind"`
	FeedbackType     string    `json:"feedbackType"`
	Message          string    `json:"message"`
	SourceRunID      string    `json:"sourceRunId,omitempty"`
	SourceArtifactID string    `json:"sourceArtifactId,omitempty"`
	Status           string    `json:"status"`
	CreatedBy        string    `json:"createdBy,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

type StudyPipelineProposalInput struct {
	TargetID         string
	TargetKind       string
	Title            string
	ContentPreview   string
	SourceRunID      string
	SourceArtifactID string
	Model            string
}

type StudyPipelineProposalRecord struct {
	ID               string     `json:"id"`
	CourseID         string     `json:"courseId"`
	TargetID         string     `json:"targetId"`
	TargetKind       string     `json:"targetKind"`
	Title            string     `json:"title"`
	ContentPreview   string     `json:"contentPreview"`
	SourceRunID      string     `json:"sourceRunId,omitempty"`
	SourceArtifactID string     `json:"sourceArtifactId,omitempty"`
	Model            string     `json:"model,omitempty"`
	Status           string     `json:"status"`
	CreatedBy        string     `json:"createdBy,omitempty"`
	SubmittedAt      *time.Time `json:"submittedAt,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

func (s *Store) RecordStudyPipelineFeedback(ctx context.Context, userID string, courseID string, input StudyPipelineFeedbackInput) (StudyPipelineFeedbackRecord, error) {
	if s == nil || s.db == nil {
		return StudyPipelineFeedbackRecord{}, sql.ErrConnDone
	}
	input.TargetID = strings.TrimSpace(input.TargetID)
	input.TargetKind = strings.TrimSpace(input.TargetKind)
	input.FeedbackType = strings.TrimSpace(input.FeedbackType)
	if strings.TrimSpace(courseID) == "" || input.TargetID == "" || input.TargetKind == "" || input.FeedbackType == "" {
		return StudyPipelineFeedbackRecord{}, sql.ErrNoRows
	}
	row := s.db.QueryRowContext(ctx, `
		insert into study_pipeline_feedback (
			course_id, target_id, target_kind, feedback_type, message,
			source_run_id, source_artifact_id, status, created_by
		)
		values ($1, $2, $3, $4, $5, $6, $7, 'open', $8)
		returning id::text, course_id, target_id, target_kind, feedback_type, message,
			coalesce(source_run_id::text, ''), coalesce(source_artifact_id, ''), status,
			coalesce(created_by::text, ''), created_at, updated_at
	`, strings.TrimSpace(courseID), input.TargetID, input.TargetKind, input.FeedbackType,
		strings.TrimSpace(input.Message), nullableUUID(input.SourceRunID), nullString(input.SourceArtifactID), nullableUUID(userID))
	return scanStudyPipelineFeedback(row)
}

func (s *Store) RecordStudyPipelineProposal(ctx context.Context, userID string, courseID string, input StudyPipelineProposalInput) (StudyPipelineProposalRecord, error) {
	if s == nil || s.db == nil {
		return StudyPipelineProposalRecord{}, sql.ErrConnDone
	}
	input.TargetID = strings.TrimSpace(input.TargetID)
	input.TargetKind = strings.TrimSpace(input.TargetKind)
	input.Title = strings.TrimSpace(input.Title)
	if strings.TrimSpace(courseID) == "" || input.TargetID == "" || input.TargetKind == "" {
		return StudyPipelineProposalRecord{}, sql.ErrNoRows
	}
	if input.Title == "" {
		input.Title = input.TargetKind + " " + input.TargetID
	}
	row := s.db.QueryRowContext(ctx, `
		insert into study_pipeline_proposals (
			course_id, target_id, target_kind, title, content_preview,
			source_run_id, source_artifact_id, model, status, created_by
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8, 'private', $9)
		returning id::text, course_id, target_id, target_kind, title, content_preview,
			coalesce(source_run_id::text, ''), coalesce(source_artifact_id, ''), coalesce(model, ''),
			status, coalesce(created_by::text, ''), submitted_at, created_at, updated_at
	`, strings.TrimSpace(courseID), input.TargetID, input.TargetKind, input.Title,
		strings.TrimSpace(input.ContentPreview), nullableUUID(input.SourceRunID), nullString(input.SourceArtifactID),
		nullString(input.Model), nullableUUID(userID))
	return scanStudyPipelineProposal(row)
}

func (s *Store) SubmitStudyPipelineProposal(ctx context.Context, userID string, courseID string, proposalID string) (StudyPipelineProposalRecord, error) {
	if s == nil || s.db == nil {
		return StudyPipelineProposalRecord{}, sql.ErrConnDone
	}
	row := s.db.QueryRowContext(ctx, `
		update study_pipeline_proposals
		set status = 'submitted_for_review',
			submitted_at = coalesce(submitted_at, now()),
			updated_at = now()
		where id = $1::uuid and course_id = $2
		returning id::text, course_id, target_id, target_kind, title, content_preview,
			coalesce(source_run_id::text, ''), coalesce(source_artifact_id, ''), coalesce(model, ''),
			status, coalesce(created_by::text, ''), submitted_at, created_at, updated_at
	`, strings.TrimSpace(proposalID), strings.TrimSpace(courseID))
	return scanStudyPipelineProposal(row)
}

func (s *Store) ListStudyPipelineReview(ctx context.Context, courseID string) ([]StudyPipelineFeedbackRecord, []StudyPipelineProposalRecord, error) {
	if s == nil || s.db == nil || strings.TrimSpace(courseID) == "" {
		return nil, nil, nil
	}
	feedbackRows, err := s.db.QueryContext(ctx, `
		select id::text, course_id, target_id, target_kind, feedback_type, message,
			coalesce(source_run_id::text, ''), coalesce(source_artifact_id, ''), status,
			coalesce(created_by::text, ''), created_at, updated_at
		from study_pipeline_feedback
		where course_id = $1
		order by created_at desc, id desc
	`, strings.TrimSpace(courseID))
	if err != nil {
		return nil, nil, err
	}
	defer feedbackRows.Close()

	feedback := []StudyPipelineFeedbackRecord{}
	for feedbackRows.Next() {
		item, err := scanStudyPipelineFeedback(feedbackRows)
		if err != nil {
			return nil, nil, err
		}
		feedback = append(feedback, item)
	}
	if err := feedbackRows.Err(); err != nil {
		return nil, nil, err
	}

	proposalRows, err := s.db.QueryContext(ctx, `
		select id::text, course_id, target_id, target_kind, title, content_preview,
			coalesce(source_run_id::text, ''), coalesce(source_artifact_id, ''), coalesce(model, ''),
			status, coalesce(created_by::text, ''), submitted_at, created_at, updated_at
		from study_pipeline_proposals
		where course_id = $1
		order by created_at desc, id desc
	`, strings.TrimSpace(courseID))
	if err != nil {
		return nil, nil, err
	}
	defer proposalRows.Close()

	proposals := []StudyPipelineProposalRecord{}
	for proposalRows.Next() {
		item, err := scanStudyPipelineProposal(proposalRows)
		if err != nil {
			return nil, nil, err
		}
		proposals = append(proposals, item)
	}
	return feedback, proposals, proposalRows.Err()
}

type studyPipelineReviewScanner interface {
	Scan(dest ...any) error
}

func scanStudyPipelineFeedback(row studyPipelineReviewScanner) (StudyPipelineFeedbackRecord, error) {
	var record StudyPipelineFeedbackRecord
	err := row.Scan(&record.ID, &record.CourseID, &record.TargetID, &record.TargetKind, &record.FeedbackType,
		&record.Message, &record.SourceRunID, &record.SourceArtifactID, &record.Status, &record.CreatedBy,
		&record.CreatedAt, &record.UpdatedAt)
	return record, err
}

func scanStudyPipelineProposal(row studyPipelineReviewScanner) (StudyPipelineProposalRecord, error) {
	var record StudyPipelineProposalRecord
	err := row.Scan(&record.ID, &record.CourseID, &record.TargetID, &record.TargetKind, &record.Title,
		&record.ContentPreview, &record.SourceRunID, &record.SourceArtifactID, &record.Model, &record.Status,
		&record.CreatedBy, &record.SubmittedAt, &record.CreatedAt, &record.UpdatedAt)
	return record, err
}
