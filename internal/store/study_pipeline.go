package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

const studyPipelineSystemSiteURL = "internal://moodle-services/study-pipeline"

type StudyPipelineRecordInput struct {
	UserID            string
	CourseID          string
	SourceID          string
	ResourceID        string
	FileHash          string
	Stage             string
	Engine            string
	ConfigHash        string
	Ownership         string
	CreatedBy         string
	Status            string
	ArtifactRoot      string
	Error             string
	StartedAt         time.Time
	FinishedAt        time.Time
	ArtifactRefs      []StudyPipelineArtifactRef
	CurationChecklist *StudyPipelineCurationChecklist
	ElementDecisions  []StudyPipelineElementDecision
	Summary           any
	Materials         []StudyPipelineMaterialRecord
	TaskLinks         []StudyPipelineTaskLinkRecord
}

type StudyPipelineArtifactRef struct {
	ID         string         `json:"id"`
	Kind       string         `json:"kind"`
	URI        string         `json:"uri,omitempty"`
	StorageKey string         `json:"storageKey,omitempty"`
	Checksum   string         `json:"checksum,omitempty"`
	PageNumber int            `json:"pageNumber,omitempty"`
	BlockID    string         `json:"blockId,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type StudyPipelineElementDecision struct {
	ID                        string         `json:"id,omitempty"`
	SourceElementID           string         `json:"sourceElementId,omitempty"`
	SourceArtifactID          string         `json:"sourceArtifactId,omitempty"`
	SourceAssetID             string         `json:"sourceAssetId,omitempty"`
	SourcePageImageArtifactID string         `json:"sourcePageImageArtifactId,omitempty"`
	OutputArtifactID          string         `json:"outputArtifactId,omitempty"`
	ElementKind               string         `json:"elementKind,omitempty"`
	Outcome                   string         `json:"outcome"`
	Reason                    string         `json:"reason,omitempty"`
	DecidedBy                 string         `json:"decidedBy,omitempty"`
	Confidence                string         `json:"confidence,omitempty"`
	PageNumber                int            `json:"pageNumber,omitempty"`
	CreatedAt                 string         `json:"createdAt,omitempty"`
	Metadata                  map[string]any `json:"metadata,omitempty"`
}

type StudyPipelineCurationChecklistItem struct {
	ID                 string `json:"id"`
	Label              string `json:"label"`
	Status             string `json:"status"`
	EvidenceArtifactID string `json:"evidenceArtifactId,omitempty"`
	Reason             string `json:"reason,omitempty"`
}

type StudyPipelineCurationChecklist struct {
	Status                  string                               `json:"status"`
	CheckedBy               string                               `json:"checkedBy,omitempty"`
	CheckedAt               string                               `json:"checkedAt,omitempty"`
	RenderPreviewArtifactID string                               `json:"renderPreviewArtifactId,omitempty"`
	Items                   []StudyPipelineCurationChecklistItem `json:"items"`
}

type StudyPipelineMaterialRecord struct {
	ID             string
	Name           string
	URL            string
	ResourceType   string
	FileType       string
	SectionID      string
	SectionName    string
	Classification string
}

type StudyPipelineTaskLinkRecord struct {
	TaskResourceID     string
	SolutionResourceID string
	Status             string
}

type StudyPipelineRunRecord struct {
	ID                string                          `json:"id"`
	SourceID          string                          `json:"sourceId"`
	CourseID          string                          `json:"courseId"`
	ResourceID        string                          `json:"resourceId,omitempty"`
	FileHash          string                          `json:"fileHash,omitempty"`
	Stage             string                          `json:"stage"`
	Engine            string                          `json:"engine"`
	ConfigHash        string                          `json:"configHash"`
	Ownership         string                          `json:"ownership"`
	CreatedBy         string                          `json:"createdBy,omitempty"`
	Status            string                          `json:"status"`
	ArtifactRoot      string                          `json:"artifactRoot"`
	Error             string                          `json:"error,omitempty"`
	StartedAt         *time.Time                      `json:"startedAt,omitempty"`
	FinishedAt        *time.Time                      `json:"finishedAt,omitempty"`
	CreatedAt         time.Time                       `json:"createdAt"`
	ArtifactRefs      []StudyPipelineArtifactRef      `json:"artifactRefs"`
	CurationChecklist *StudyPipelineCurationChecklist `json:"curationChecklist,omitempty"`
	ElementDecisions  []StudyPipelineElementDecision  `json:"elementDecisions,omitempty"`
}

type ActiveRunSelectionRecord struct {
	SourceID    string    `json:"sourceId"`
	ResourceID  string    `json:"resourceId,omitempty"`
	Stage       string    `json:"stage"`
	ActiveRunID string    `json:"activeRunId"`
	SelectedBy  string    `json:"selectedBy,omitempty"`
	SelectedAt  time.Time `json:"selectedAt"`
	Reason      string    `json:"reason"`
}

func (s *Store) RecordStudyPipeline(ctx context.Context, input StudyPipelineRecordInput) (StudyPipelineRunRecord, error) {
	if s == nil || s.db == nil || strings.TrimSpace(input.CourseID) == "" {
		return StudyPipelineRunRecord{}, nil
	}
	if strings.TrimSpace(input.Stage) == "" {
		return StudyPipelineRunRecord{}, nil
	}
	normalizeStudyPipelineRecordInput(&input)
	artifactRefsJSON, err := json.Marshal(input.ArtifactRefs)
	if err != nil {
		return StudyPipelineRunRecord{}, err
	}
	curationChecklistJSON, err := json.Marshal(input.CurationChecklist)
	if err != nil {
		return StudyPipelineRunRecord{}, err
	}
	elementDecisionsJSON, err := json.Marshal(input.ElementDecisions)
	if err != nil {
		return StudyPipelineRunRecord{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return StudyPipelineRunRecord{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var run StudyPipelineRunRecord
	var artifactRefsRaw string
	var curationChecklistRaw string
	var elementDecisionsRaw string
	err = tx.QueryRowContext(ctx, `
		insert into study_pipeline_runs (
			user_id, course_id, source_id, resource_id, file_hash, stage, engine, config_hash,
			ownership, created_by, status, artifact_root, error, started_at, finished_at, artifact_refs,
			curation_checklist, element_decisions
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16::jsonb, $17::jsonb, $18::jsonb)
		returning id::text, source_id, course_id, coalesce(resource_id, ''), coalesce(file_hash, ''),
			stage, engine, config_hash, ownership, coalesce(created_by::text, ''), status,
			artifact_root, coalesce(error, ''), started_at, finished_at, created_at, artifact_refs::text,
			coalesce(curation_checklist::text, ''), coalesce(element_decisions::text, '[]')
	`, nullableRunOwner(input), input.CourseID, input.SourceID, nullString(input.ResourceID), nullString(input.FileHash),
		input.Stage, input.Engine, input.ConfigHash, input.Ownership, nullableUUID(input.CreatedBy), input.Status,
		input.ArtifactRoot, nullString(input.Error), input.StartedAt, input.FinishedAt, string(artifactRefsJSON),
		string(curationChecklistJSON), string(elementDecisionsJSON)).
		Scan(&run.ID, &run.SourceID, &run.CourseID, &run.ResourceID, &run.FileHash, &run.Stage, &run.Engine,
			&run.ConfigHash, &run.Ownership, &run.CreatedBy, &run.Status, &run.ArtifactRoot, &run.Error,
			&run.StartedAt, &run.FinishedAt, &run.CreatedAt, &artifactRefsRaw, &curationChecklistRaw, &elementDecisionsRaw)
	if err != nil {
		return StudyPipelineRunRecord{}, err
	}
	if err := json.Unmarshal([]byte(artifactRefsRaw), &run.ArtifactRefs); err != nil {
		return StudyPipelineRunRecord{}, err
	}
	if err := decodeStudyPipelineRunExtras(curationChecklistRaw, elementDecisionsRaw, &run); err != nil {
		return StudyPipelineRunRecord{}, err
	}
	for _, material := range input.Materials {
		_, err = tx.ExecContext(ctx, `
			insert into study_resources (
				user_id, course_id, resource_id, name, url, resource_type, file_type,
				section_id, section_name, classification, updated_at
			)
			values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now())
			on conflict (user_id, course_id, resource_id)
			do update set
				name = excluded.name,
				url = excluded.url,
				resource_type = excluded.resource_type,
				file_type = excluded.file_type,
				section_id = excluded.section_id,
				section_name = excluded.section_name,
				classification = excluded.classification,
				updated_at = now()
		`, input.UserID, input.CourseID, material.ID, material.Name, nullString(material.URL),
			nullString(material.ResourceType), nullString(material.FileType), nullString(material.SectionID),
			nullString(material.SectionName), material.Classification)
		if err != nil {
			return StudyPipelineRunRecord{}, err
		}
	}
	for _, link := range input.TaskLinks {
		_, err = tx.ExecContext(ctx, `
			insert into study_task_links (
				user_id, course_id, task_resource_id, solution_resource_id, status, updated_at
			)
			values ($1, $2, $3, $4, $5, now())
			on conflict (user_id, course_id, task_resource_id)
			do update set
				solution_resource_id = excluded.solution_resource_id,
				status = excluded.status,
				updated_at = now()
		`, input.UserID, input.CourseID, link.TaskResourceID, nullString(link.SolutionResourceID), link.Status)
		if err != nil {
			return StudyPipelineRunRecord{}, err
		}
	}
	if input.Summary != nil {
		payload, err := json.Marshal(input.Summary)
		if err != nil {
			return StudyPipelineRunRecord{}, err
		}
		_, err = tx.ExecContext(ctx, `
			insert into study_artifacts (
				user_id, course_id, stage, kind, path, source_path
			)
			values ($1, $2, $3, 'summary', $4, $5)
		`, input.UserID, input.CourseID, input.Stage, input.ArtifactRoot, string(payload))
		if err != nil {
			return StudyPipelineRunRecord{}, err
		}
	}
	if input.Status == "succeeded" {
		if err := upsertActiveStudyPipelineRun(ctx, tx, run.SourceID, run.ResourceID, run.Stage, run.ID, input.CreatedBy, "latest successful run"); err != nil {
			return StudyPipelineRunRecord{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return StudyPipelineRunRecord{}, err
	}
	return run, nil
}

func (s *Store) ListStudyPipelineRuns(ctx context.Context, userID string, courseID string) ([]StudyPipelineRunRecord, []ActiveRunSelectionRecord, error) {
	if s == nil || s.db == nil || strings.TrimSpace(courseID) == "" {
		return nil, nil, nil
	}
	sourceID := studyPipelineSourceID(courseID)
	query := `
		select id::text, source_id, course_id, coalesce(resource_id, ''), coalesce(file_hash, ''),
			stage, engine, config_hash, ownership, coalesce(created_by::text, ''), status,
			artifact_root, coalesce(error, ''), started_at, finished_at, created_at, artifact_refs::text,
			coalesce(curation_checklist::text, ''), coalesce(element_decisions::text, '[]')
		from study_pipeline_runs
		where course_id = $1 and ownership = 'shared'
		order by created_at desc, id desc
	`
	args := []any{courseID}
	if strings.TrimSpace(userID) != "" {
		query = `
			select id::text, source_id, course_id, coalesce(resource_id, ''), coalesce(file_hash, ''),
				stage, engine, config_hash, ownership, coalesce(created_by::text, ''), status,
				artifact_root, coalesce(error, ''), started_at, finished_at, created_at, artifact_refs::text,
				coalesce(curation_checklist::text, ''), coalesce(element_decisions::text, '[]')
			from study_pipeline_runs
			where course_id = $1 and (ownership = 'shared' or user_id = $2::uuid or created_by = $2::uuid)
			order by created_at desc, id desc
		`
		args = append(args, userID)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	runs := []StudyPipelineRunRecord{}
	for rows.Next() {
		run, err := scanStudyPipelineRun(rows)
		if err != nil {
			return nil, nil, err
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	selections, err := s.listActiveStudyPipelineSelections(ctx, sourceID)
	if err != nil {
		return nil, nil, err
	}
	return runs, selections, nil
}

func (s *Store) SelectActiveStudyPipelineRun(ctx context.Context, userID string, courseID string, runID string, reason string) (ActiveRunSelectionRecord, error) {
	if s == nil || s.db == nil {
		return ActiveRunSelectionRecord{}, sql.ErrConnDone
	}
	runID = strings.TrimSpace(runID)
	if strings.TrimSpace(courseID) == "" || runID == "" {
		return ActiveRunSelectionRecord{}, sql.ErrNoRows
	}
	query := `
		select id::text, source_id, course_id, coalesce(resource_id, ''), coalesce(file_hash, ''),
			stage, engine, config_hash, ownership, coalesce(created_by::text, ''), status,
			artifact_root, coalesce(error, ''), started_at, finished_at, created_at, artifact_refs::text,
			coalesce(curation_checklist::text, ''), coalesce(element_decisions::text, '[]')
		from study_pipeline_runs
		where id = $1::uuid and course_id = $2 and ownership = 'shared'
	`
	args := []any{runID, courseID}
	if strings.TrimSpace(userID) != "" {
		query = `
			select id::text, source_id, course_id, coalesce(resource_id, ''), coalesce(file_hash, ''),
				stage, engine, config_hash, ownership, coalesce(created_by::text, ''), status,
				artifact_root, coalesce(error, ''), started_at, finished_at, created_at, artifact_refs::text,
				coalesce(curation_checklist::text, ''), coalesce(element_decisions::text, '[]')
			from study_pipeline_runs
			where id = $1::uuid and course_id = $2 and (ownership = 'shared' or user_id = $3::uuid or created_by = $3::uuid)
		`
		args = append(args, userID)
	}
	row := s.db.QueryRowContext(ctx, query, args...)
	run, err := scanStudyPipelineRun(row)
	if err != nil {
		return ActiveRunSelectionRecord{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ActiveRunSelectionRecord{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := upsertActiveStudyPipelineRun(ctx, tx, run.SourceID, run.ResourceID, run.Stage, run.ID, userID, reason); err != nil {
		return ActiveRunSelectionRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return ActiveRunSelectionRecord{}, err
	}
	return ActiveRunSelectionRecord{
		SourceID:    run.SourceID,
		ResourceID:  run.ResourceID,
		Stage:       run.Stage,
		ActiveRunID: run.ID,
		SelectedBy:  strings.TrimSpace(userID),
		SelectedAt:  time.Now().UTC(),
		Reason:      strings.TrimSpace(reason),
	}, nil
}

func (s *Store) PublishStudyPipelineRun(ctx context.Context, userID string, courseID string, runID string, reason string) (ActiveRunSelectionRecord, StudyPipelineAuditRecord, error) {
	if strings.TrimSpace(reason) == "" {
		reason = "published from pipeline review"
	}
	selection, err := s.SelectActiveStudyPipelineRun(ctx, userID, courseID, runID, reason)
	if err != nil {
		return ActiveRunSelectionRecord{}, StudyPipelineAuditRecord{}, err
	}
	audit, err := insertStudyPipelineAudit(ctx, s.db, studyPipelineAuditInput{
		CourseID:    courseID,
		ActorID:     userID,
		Action:      "run.published",
		TargetKind:  "run",
		TargetID:    runID,
		SourceRunID: runID,
		Message:     reason,
	})
	if err != nil {
		return ActiveRunSelectionRecord{}, StudyPipelineAuditRecord{}, err
	}
	return selection, audit, nil
}

func (s *Store) UnpublishStudyPipelineRun(ctx context.Context, userID string, courseID string, runID string, reason string) (StudyPipelineAuditRecord, error) {
	if s == nil || s.db == nil {
		return StudyPipelineAuditRecord{}, sql.ErrConnDone
	}
	runID = strings.TrimSpace(runID)
	if strings.TrimSpace(courseID) == "" || runID == "" {
		return StudyPipelineAuditRecord{}, sql.ErrNoRows
	}
	if strings.TrimSpace(reason) == "" {
		reason = "unpublished from pipeline review"
	}
	row := s.db.QueryRowContext(ctx, `
		select id::text, source_id, course_id, coalesce(resource_id, ''), coalesce(file_hash, ''),
			stage, engine, config_hash, ownership, coalesce(created_by::text, ''), status,
			artifact_root, coalesce(error, ''), started_at, finished_at, created_at, artifact_refs::text,
			coalesce(curation_checklist::text, ''), coalesce(element_decisions::text, '[]')
		from study_pipeline_runs
		where id = $1::uuid and course_id = $2
	`, runID, strings.TrimSpace(courseID))
	run, err := scanStudyPipelineRun(row)
	if err != nil {
		return StudyPipelineAuditRecord{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return StudyPipelineAuditRecord{}, err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `
		delete from active_run_selections
		where source_id = $1 and resource_id = $2 and stage = $3 and active_run_id = $4::uuid
	`, run.SourceID, run.ResourceID, run.Stage, run.ID)
	if err != nil {
		return StudyPipelineAuditRecord{}, err
	}
	audit, err := insertStudyPipelineAudit(ctx, tx, studyPipelineAuditInput{
		CourseID:    courseID,
		ActorID:     userID,
		Action:      "run.unpublished",
		TargetKind:  "run",
		TargetID:    run.ID,
		SourceRunID: run.ID,
		Message:     reason,
	})
	if err != nil {
		return StudyPipelineAuditRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return StudyPipelineAuditRecord{}, err
	}
	return audit, nil
}

func nullString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	return sql.NullString{String: value, Valid: value != ""}
}

type studyPipelineRunScanner interface {
	Scan(dest ...any) error
}

func scanStudyPipelineRun(row studyPipelineRunScanner) (StudyPipelineRunRecord, error) {
	var run StudyPipelineRunRecord
	var artifactRefsRaw string
	var curationChecklistRaw string
	var elementDecisionsRaw string
	err := row.Scan(&run.ID, &run.SourceID, &run.CourseID, &run.ResourceID, &run.FileHash, &run.Stage, &run.Engine,
		&run.ConfigHash, &run.Ownership, &run.CreatedBy, &run.Status, &run.ArtifactRoot, &run.Error,
		&run.StartedAt, &run.FinishedAt, &run.CreatedAt, &artifactRefsRaw, &curationChecklistRaw, &elementDecisionsRaw)
	if err != nil {
		return StudyPipelineRunRecord{}, err
	}
	if strings.TrimSpace(artifactRefsRaw) != "" {
		if err := json.Unmarshal([]byte(artifactRefsRaw), &run.ArtifactRefs); err != nil {
			return StudyPipelineRunRecord{}, err
		}
	}
	if err := decodeStudyPipelineRunExtras(curationChecklistRaw, elementDecisionsRaw, &run); err != nil {
		return StudyPipelineRunRecord{}, err
	}
	return run, nil
}

func decodeStudyPipelineRunExtras(curationChecklistRaw string, elementDecisionsRaw string, run *StudyPipelineRunRecord) error {
	if strings.TrimSpace(curationChecklistRaw) != "" && strings.TrimSpace(curationChecklistRaw) != "null" {
		var checklist StudyPipelineCurationChecklist
		if err := json.Unmarshal([]byte(curationChecklistRaw), &checklist); err != nil {
			return err
		}
		run.CurationChecklist = &checklist
	}
	if strings.TrimSpace(elementDecisionsRaw) != "" && strings.TrimSpace(elementDecisionsRaw) != "null" {
		if err := json.Unmarshal([]byte(elementDecisionsRaw), &run.ElementDecisions); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) listActiveStudyPipelineSelections(ctx context.Context, sourceID string) ([]ActiveRunSelectionRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		select source_id, resource_id, stage, active_run_id::text, coalesce(selected_by::text, ''), selected_at, reason
		from active_run_selections
		where source_id = $1
		order by selected_at desc
	`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	selections := []ActiveRunSelectionRecord{}
	for rows.Next() {
		var selection ActiveRunSelectionRecord
		if err := rows.Scan(&selection.SourceID, &selection.ResourceID, &selection.Stage, &selection.ActiveRunID, &selection.SelectedBy, &selection.SelectedAt, &selection.Reason); err != nil {
			return nil, err
		}
		selections = append(selections, selection)
	}
	return selections, rows.Err()
}

func upsertActiveStudyPipelineRun(ctx context.Context, tx *sql.Tx, sourceID string, resourceID string, stage string, runID string, selectedBy string, reason string) error {
	if strings.TrimSpace(reason) == "" {
		reason = "selected"
	}
	_, err := tx.ExecContext(ctx, `
		insert into active_run_selections (
			source_id, resource_id, stage, active_run_id, selected_by, selected_at, reason
		)
		values ($1, $2, $3, $4::uuid, $5, now(), $6)
		on conflict (source_id, resource_id, stage)
		do update set
			active_run_id = excluded.active_run_id,
			selected_by = excluded.selected_by,
			selected_at = excluded.selected_at,
			reason = excluded.reason
	`, sourceID, strings.TrimSpace(resourceID), stage, runID, nullableUUID(selectedBy), reason)
	return err
}

func normalizeStudyPipelineRecordInput(input *StudyPipelineRecordInput) {
	input.CourseID = strings.TrimSpace(input.CourseID)
	input.Stage = strings.TrimSpace(input.Stage)
	input.SourceID = strings.TrimSpace(input.SourceID)
	if input.SourceID == "" {
		input.SourceID = studyPipelineSourceID(input.CourseID)
	}
	input.Engine = strings.TrimSpace(input.Engine)
	if input.Engine == "" {
		input.Engine = defaultStudyPipelineEngine(input.Stage)
	}
	input.ConfigHash = strings.TrimSpace(input.ConfigHash)
	if input.ConfigHash == "" {
		input.ConfigHash = "config:" + input.Stage + ":default"
	}
	input.Ownership = strings.TrimSpace(input.Ownership)
	if input.Ownership != "user_owned" {
		input.Ownership = "shared"
	}
	input.CreatedBy = strings.TrimSpace(input.CreatedBy)
	if input.CreatedBy == "" {
		input.CreatedBy = strings.TrimSpace(input.UserID)
	}
	input.Status = strings.TrimSpace(input.Status)
	if input.Status == "" {
		input.Status = "succeeded"
	}
	now := time.Now().UTC()
	if input.StartedAt.IsZero() {
		input.StartedAt = now
	}
	if input.FinishedAt.IsZero() {
		input.FinishedAt = input.StartedAt
	}
	if strings.TrimSpace(input.ArtifactRoot) == "" {
		input.ArtifactRoot = "moodle-study://" + input.CourseID
	}
	if len(input.ArtifactRefs) == 0 && strings.TrimSpace(input.ArtifactRoot) != "" {
		input.ArtifactRefs = []StudyPipelineArtifactRef{{
			ID:   "artifact:summary:" + input.CourseID + ":" + input.Stage,
			Kind: "course_summary",
			URI:  input.ArtifactRoot,
		}}
	}
}

func studyPipelineSourceID(courseID string) string {
	return "source:moodle-course:" + strings.TrimSpace(courseID)
}

func defaultStudyPipelineEngine(stage string) string {
	switch strings.TrimSpace(stage) {
	case "raw", "inventory":
		return "moodle_api"
	case "extracted":
		return "docling"
	case "curated":
		return "codex"
	default:
		return "unknown"
	}
}

func nullableUUID(value string) sql.NullString {
	value = strings.TrimSpace(value)
	return sql.NullString{String: value, Valid: value != ""}
}

func nullableRunOwner(input StudyPipelineRecordInput) sql.NullString {
	if input.Ownership == "user_owned" {
		return nullableUUID(input.CreatedBy)
	}
	return sql.NullString{}
}

func (s *Store) EnsureStudyPipelineSystemUser(ctx context.Context) (string, error) {
	if s == nil || s.db == nil {
		return "", nil
	}
	var userID string
	err := s.db.QueryRowContext(ctx, `
		insert into users (moodle_site_url, moodle_user_id, display_name)
		values ($1, 0, 'moodle-services study pipeline')
		on conflict (moodle_site_url, moodle_user_id)
		do update set
			display_name = excluded.display_name,
			updated_at = now()
		returning id::text
	`, studyPipelineSystemSiteURL).Scan(&userID)
	return userID, err
}
