package store

import (
	"context"
	"database/sql/driver"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRecordStudyPipelineCreatesImmutableRuns(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	st := &Store{db: db}
	started := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)

	expectRecordRun(t, mock, "11111111-1111-1111-1111-111111111111", started)
	first, err := st.RecordStudyPipeline(context.Background(), StudyPipelineRecordInput{
		UserID:       "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		CourseID:     "22584",
		Stage:        "extracted",
		ArtifactRoot: "/srv/moodle-study/courses/22584",
		StartedAt:    started,
		FinishedAt:   started,
	})
	if err != nil {
		t.Fatalf("first RecordStudyPipeline: %v", err)
	}

	expectRecordRun(t, mock, "22222222-2222-2222-2222-222222222222", started.Add(time.Minute))
	second, err := st.RecordStudyPipeline(context.Background(), StudyPipelineRecordInput{
		UserID:       "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		CourseID:     "22584",
		Stage:        "extracted",
		ArtifactRoot: "/srv/moodle-study/courses/22584",
		StartedAt:    started.Add(time.Minute),
		FinishedAt:   started.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("second RecordStudyPipeline: %v", err)
	}

	if first.ID == second.ID {
		t.Fatalf("expected rerun to create a distinct immutable run, got %q", first.ID)
	}
	if first.Engine != "docling" || first.ConfigHash != "config:extracted:default" || first.Ownership != "shared" {
		t.Fatalf("unexpected first run metadata: %#v", first)
	}
	if second.ID != "22222222-2222-2222-2222-222222222222" {
		t.Fatalf("unexpected second run id: %#v", second)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestListStudyPipelineRunsKeepsOldFailedStaleAndUserOwnedRuns(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	st := &Store{db: db}
	now := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)

	runRows := sqlmock.NewRows(studyPipelineRunColumns()).
		AddRow("22222222-2222-2222-2222-222222222222", "source:moodle-course:22584", "22584", "", "sha256:new", "extracted", "docling", "config:docling:default", "shared", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "succeeded", "/srv/new", "", now, now, now, `[{"id":"artifact:new","kind":"ocr_text"}]`).
		AddRow("11111111-1111-1111-1111-111111111111", "source:moodle-course:22584", "22584", "", "sha256:old", "extracted", "pdftotext", "config:pdftotext:default", "shared", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "stale", "/srv/old", "", now, now, now.Add(-time.Minute), `[]`).
		AddRow("33333333-3333-3333-3333-333333333333", "source:moodle-course:22584", "22584", "947711", "sha256:user", "curated", "codex", "config:codex:user", "user_owned", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "failed", "/srv/user", "codex failed", now, now, now.Add(-2*time.Minute), `[]`)
	mock.ExpectQuery(regexp.QuoteMeta("from study_pipeline_runs")).
		WithArgs("22584", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa").
		WillReturnRows(runRows)
	mock.ExpectQuery(regexp.QuoteMeta("from active_run_selections")).
		WithArgs("source:moodle-course:22584").
		WillReturnRows(sqlmock.NewRows([]string{"source_id", "resource_id", "stage", "active_run_id", "selected_by", "selected_at", "reason"}).
			AddRow("source:moodle-course:22584", "", "extracted", "22222222-2222-2222-2222-222222222222", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", now, "latest successful run"))

	runs, selections, err := st.ListStudyPipelineRuns(context.Background(), "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "22584")
	if err != nil {
		t.Fatalf("ListStudyPipelineRuns: %v", err)
	}
	if len(runs) != 3 {
		t.Fatalf("expected all visible runs to remain accessible, got %d", len(runs))
	}
	if runs[1].Status != "stale" || runs[2].Ownership != "user_owned" || runs[2].Status != "failed" {
		t.Fatalf("expected stale, user-owned, and failed runs to be represented, got %#v", runs)
	}
	if len(selections) != 1 || selections[0].ActiveRunID != runs[0].ID {
		t.Fatalf("unexpected active selection: %#v", selections)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestSelectActiveStudyPipelineRunCanPointBackToOldRun(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	st := &Store{db: db}
	now := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta("from study_pipeline_runs")).
		WithArgs("11111111-1111-1111-1111-111111111111", "22584", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa").
		WillReturnRows(sqlmock.NewRows(studyPipelineRunColumns()).
			AddRow("11111111-1111-1111-1111-111111111111", "source:moodle-course:22584", "22584", "", "sha256:old", "extracted", "pdftotext", "config:pdftotext:default", "shared", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "succeeded", "/srv/old", "", now, now, now, `[]`))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("insert into active_run_selections")).
		WithArgs("source:moodle-course:22584", "", "extracted", "11111111-1111-1111-1111-111111111111", uuidArg("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"), "compare OCR engines").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	selection, err := st.SelectActiveStudyPipelineRun(context.Background(), "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "22584", "11111111-1111-1111-1111-111111111111", "compare OCR engines")
	if err != nil {
		t.Fatalf("SelectActiveStudyPipelineRun: %v", err)
	}
	if selection.ActiveRunID != "11111111-1111-1111-1111-111111111111" || selection.Reason != "compare OCR engines" {
		t.Fatalf("unexpected selection: %#v", selection)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestPublishStudyPipelineRunSelectsActiveRunAndAudits(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	st := &Store{db: db}
	now := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta("from study_pipeline_runs")).
		WithArgs("11111111-1111-1111-1111-111111111111", "22584", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa").
		WillReturnRows(sqlmock.NewRows(studyPipelineRunColumns()).
			AddRow("11111111-1111-1111-1111-111111111111", "source:moodle-course:22584", "22584", "", "sha256:old", "extracted", "pdftotext", "config:pdftotext:default", "shared", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "succeeded", "/srv/old", "", now, now, now, `[]`))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("insert into active_run_selections")).
		WithArgs("source:moodle-course:22584", "", "extracted", "11111111-1111-1111-1111-111111111111", uuidArg("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"), "publish after review").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(regexp.QuoteMeta("insert into study_pipeline_audit_events")).
		WithArgs("22584", uuidArg("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"), "run.published", "run", "11111111-1111-1111-1111-111111111111", uuidArg("11111111-1111-1111-1111-111111111111"), nullUUIDArg{}, "publish after review").
		WillReturnRows(sqlmock.NewRows(studyPipelineAuditColumns()).
			AddRow("66666666-6666-6666-6666-666666666666", "22584", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "run.published", "run", "11111111-1111-1111-1111-111111111111", "11111111-1111-1111-1111-111111111111", "", "publish after review", now))

	selection, audit, err := st.PublishStudyPipelineRun(context.Background(), "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "22584", "11111111-1111-1111-1111-111111111111", "publish after review")
	if err != nil {
		t.Fatalf("PublishStudyPipelineRun: %v", err)
	}
	if selection.ActiveRunID != "11111111-1111-1111-1111-111111111111" || audit.Action != "run.published" {
		t.Fatalf("unexpected publish result: %#v %#v", selection, audit)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestUnpublishStudyPipelineRunClearsSelectionAndAudits(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	st := &Store{db: db}
	now := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta("from study_pipeline_runs")).
		WithArgs("11111111-1111-1111-1111-111111111111", "22584").
		WillReturnRows(sqlmock.NewRows(studyPipelineRunColumns()).
			AddRow("11111111-1111-1111-1111-111111111111", "source:moodle-course:22584", "22584", "", "sha256:old", "extracted", "pdftotext", "config:pdftotext:default", "shared", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "succeeded", "/srv/old", "", now, now, now, `[]`))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("delete from active_run_selections")).
		WithArgs("source:moodle-course:22584", "", "extracted", "11111111-1111-1111-1111-111111111111").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("insert into study_pipeline_audit_events")).
		WithArgs("22584", uuidArg("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"), "run.unpublished", "run", "11111111-1111-1111-1111-111111111111", uuidArg("11111111-1111-1111-1111-111111111111"), nullUUIDArg{}, "hide broken output").
		WillReturnRows(sqlmock.NewRows(studyPipelineAuditColumns()).
			AddRow("66666666-6666-6666-6666-666666666666", "22584", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "run.unpublished", "run", "11111111-1111-1111-1111-111111111111", "11111111-1111-1111-1111-111111111111", "", "hide broken output", now))
	mock.ExpectCommit()

	audit, err := st.UnpublishStudyPipelineRun(context.Background(), "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "22584", "11111111-1111-1111-1111-111111111111", "hide broken output")
	if err != nil {
		t.Fatalf("UnpublishStudyPipelineRun: %v", err)
	}
	if audit.Action != "run.unpublished" {
		t.Fatalf("unexpected audit result: %#v", audit)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func expectRecordRun(t *testing.T, mock sqlmock.Sqlmock, runID string, at time.Time) {
	t.Helper()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("insert into study_pipeline_runs")).
		WillReturnRows(sqlmock.NewRows(studyPipelineRunColumns()).
			AddRow(runID, "source:moodle-course:22584", "22584", "", "", "extracted", "docling", "config:extracted:default", "shared", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "succeeded", "/srv/moodle-study/courses/22584", "", at, at, at, `[{"id":"artifact:summary:22584:extracted","kind":"course_summary","uri":"/srv/moodle-study/courses/22584"}]`))
	mock.ExpectExec(regexp.QuoteMeta("insert into active_run_selections")).
		WithArgs("source:moodle-course:22584", "", "extracted", runID, uuidArg("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"), "latest successful run").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
}

func studyPipelineRunColumns() []string {
	return []string{
		"id", "source_id", "course_id", "resource_id", "file_hash", "stage", "engine", "config_hash",
		"ownership", "created_by", "status", "artifact_root", "error", "started_at", "finished_at", "created_at", "artifact_refs",
	}
}

type uuidArg string

func (u uuidArg) Match(value driver.Value) bool {
	return string(u) == value
}
