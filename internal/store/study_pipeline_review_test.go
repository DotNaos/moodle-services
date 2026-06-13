package store

import (
	"context"
	"database/sql/driver"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRecordStudyPipelineFeedback(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	st := &Store{db: db}
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta("insert into study_pipeline_feedback")).
		WithArgs(
			"22584",
			"task-1",
			"task",
			"image_missing",
			"Diagram fehlt.",
			nullUUIDArg{},
			nullUUIDArg{},
			uuidArg("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"),
		).
		WillReturnRows(sqlmock.NewRows(studyPipelineFeedbackColumns()).
			AddRow("44444444-4444-4444-4444-444444444444", "22584", "task-1", "task", "image_missing", "Diagram fehlt.", "", "", "open", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", now, now))

	record, err := st.RecordStudyPipelineFeedback(context.Background(), "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "22584", StudyPipelineFeedbackInput{
		TargetID:     "task-1",
		TargetKind:   "task",
		FeedbackType: "image_missing",
		Message:      "Diagram fehlt.",
	})
	if err != nil {
		t.Fatalf("RecordStudyPipelineFeedback: %v", err)
	}
	if record.FeedbackType != "image_missing" || record.Status != "open" {
		t.Fatalf("unexpected feedback record: %#v", record)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestListStudyPipelineReviewIncludesFeedbackAndProposals(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	st := &Store{db: db}
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta("from study_pipeline_feedback")).
		WithArgs("22584").
		WillReturnRows(sqlmock.NewRows(studyPipelineFeedbackColumns()).
			AddRow("44444444-4444-4444-4444-444444444444", "22584", "task-1", "task", "ocr_bad", "OCR ist falsch.", "", "", "open", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", now, now))
	mock.ExpectQuery(regexp.QuoteMeta("from study_pipeline_proposals")).
		WithArgs("22584").
		WillReturnRows(sqlmock.NewRows(studyPipelineProposalColumns()).
			AddRow("55555555-5555-5555-5555-555555555555", "22584", "task-1", "task", "Aufgabe 1", "Verbesserte Aufgabe", "", "", "gpt-5", "private", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", nil, now, now))
	mock.ExpectQuery(regexp.QuoteMeta("from study_pipeline_audit_events")).
		WithArgs("22584").
		WillReturnRows(sqlmock.NewRows(studyPipelineAuditColumns()).
			AddRow("66666666-6666-6666-6666-666666666666", "22584", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "proposal.promoted", "proposal", "55555555-5555-5555-5555-555555555555", "", "", "accepted", now))

	feedback, proposals, audit, err := st.ListStudyPipelineReview(context.Background(), "22584")
	if err != nil {
		t.Fatalf("ListStudyPipelineReview: %v", err)
	}
	if len(feedback) != 1 || feedback[0].FeedbackType != "ocr_bad" {
		t.Fatalf("unexpected feedback: %#v", feedback)
	}
	if len(proposals) != 1 || proposals[0].Status != "private" {
		t.Fatalf("unexpected proposals: %#v", proposals)
	}
	if len(audit) != 1 || audit[0].Action != "proposal.promoted" {
		t.Fatalf("unexpected audit: %#v", audit)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestSubmitStudyPipelineProposal(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	st := &Store{db: db}
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta("update study_pipeline_proposals")).
		WithArgs("55555555-5555-5555-5555-555555555555", "22584").
		WillReturnRows(sqlmock.NewRows(studyPipelineProposalColumns()).
			AddRow("55555555-5555-5555-5555-555555555555", "22584", "task-1", "task", "Aufgabe 1", "Verbesserte Aufgabe", "", "", "gpt-5", "submitted_for_review", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", now, now, now))

	proposal, err := st.SubmitStudyPipelineProposal(context.Background(), "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "22584", "55555555-5555-5555-5555-555555555555")
	if err != nil {
		t.Fatalf("SubmitStudyPipelineProposal: %v", err)
	}
	if proposal.Status != "submitted_for_review" || proposal.SubmittedAt == nil {
		t.Fatalf("unexpected proposal: %#v", proposal)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestPromoteStudyPipelineProposalWritesAudit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	st := &Store{db: db}
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("update study_pipeline_proposals")).
		WithArgs("promoted", "55555555-5555-5555-5555-555555555555", "22584").
		WillReturnRows(sqlmock.NewRows(studyPipelineProposalColumns()).
			AddRow("55555555-5555-5555-5555-555555555555", "22584", "task-1", "task", "Aufgabe 1", "Verbesserte Aufgabe", "11111111-1111-1111-1111-111111111111", "artifact:task-1", "gpt-5", "promoted", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", now, now, now))
	mock.ExpectQuery(regexp.QuoteMeta("insert into study_pipeline_audit_events")).
		WithArgs("22584", uuidArg("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"), "proposal.promoted", "proposal", "55555555-5555-5555-5555-555555555555", uuidArg("11111111-1111-1111-1111-111111111111"), "artifact:task-1", "accepted").
		WillReturnRows(sqlmock.NewRows(studyPipelineAuditColumns()).
			AddRow("66666666-6666-6666-6666-666666666666", "22584", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "proposal.promoted", "proposal", "55555555-5555-5555-5555-555555555555", "11111111-1111-1111-1111-111111111111", "artifact:task-1", "accepted", now))
	mock.ExpectCommit()

	proposal, audit, err := st.PromoteStudyPipelineProposal(context.Background(), "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "22584", "55555555-5555-5555-5555-555555555555", "accepted")
	if err != nil {
		t.Fatalf("PromoteStudyPipelineProposal: %v", err)
	}
	if proposal.Status != "promoted" || audit.Action != "proposal.promoted" {
		t.Fatalf("unexpected promotion result: %#v %#v", proposal, audit)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestResolveStudyPipelineFeedbackWritesAudit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	st := &Store{db: db}
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("update study_pipeline_feedback")).
		WithArgs("resolved", "44444444-4444-4444-4444-444444444444", "22584").
		WillReturnRows(sqlmock.NewRows(studyPipelineFeedbackColumns()).
			AddRow("44444444-4444-4444-4444-444444444444", "22584", "task-1", "task", "ocr_bad", "OCR ist falsch.", "11111111-1111-1111-1111-111111111111", "artifact:task-1", "resolved", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", now, now))
	mock.ExpectQuery(regexp.QuoteMeta("insert into study_pipeline_audit_events")).
		WithArgs("22584", uuidArg("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"), "feedback.resolved", "feedback", "44444444-4444-4444-4444-444444444444", uuidArg("11111111-1111-1111-1111-111111111111"), "artifact:task-1", "fixed in published output").
		WillReturnRows(sqlmock.NewRows(studyPipelineAuditColumns()).
			AddRow("66666666-6666-6666-6666-666666666666", "22584", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "feedback.resolved", "feedback", "44444444-4444-4444-4444-444444444444", "11111111-1111-1111-1111-111111111111", "artifact:task-1", "fixed in published output", now))
	mock.ExpectCommit()

	feedback, audit, err := st.ResolveStudyPipelineFeedback(context.Background(), "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "22584", "44444444-4444-4444-4444-444444444444", "fixed in published output")
	if err != nil {
		t.Fatalf("ResolveStudyPipelineFeedback: %v", err)
	}
	if feedback.Status != "resolved" || audit.Action != "feedback.resolved" {
		t.Fatalf("unexpected feedback result: %#v %#v", feedback, audit)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func studyPipelineFeedbackColumns() []string {
	return []string{
		"id", "course_id", "target_id", "target_kind", "feedback_type", "message",
		"source_run_id", "source_artifact_id", "status", "created_by", "created_at", "updated_at",
	}
}

func studyPipelineProposalColumns() []string {
	return []string{
		"id", "course_id", "target_id", "target_kind", "title", "content_preview",
		"source_run_id", "source_artifact_id", "model", "status", "created_by", "submitted_at", "created_at", "updated_at",
	}
}

func studyPipelineAuditColumns() []string {
	return []string{
		"id", "course_id", "actor_id", "action", "target_kind", "target_id",
		"source_run_id", "source_artifact_id", "message", "created_at",
	}
}

type nullUUIDArg struct{}

func (nullUUIDArg) Match(value driver.Value) bool {
	return value == nil
}
