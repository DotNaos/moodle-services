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
	UserID       string
	CourseID     string
	Stage        string
	ArtifactRoot string
	Summary      any
	Materials    []StudyPipelineMaterialRecord
	TaskLinks    []StudyPipelineTaskLinkRecord
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

func (s *Store) RecordStudyPipeline(ctx context.Context, input StudyPipelineRecordInput) error {
	if s == nil || s.db == nil || strings.TrimSpace(input.UserID) == "" || strings.TrimSpace(input.CourseID) == "" {
		return nil
	}
	if strings.TrimSpace(input.Stage) == "" {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	finishedAt := time.Now().UTC()
	_, err = tx.ExecContext(ctx, `
		insert into study_pipeline_runs (
			user_id, course_id, stage, status, artifact_root, started_at, finished_at
		)
		values ($1, $2, $3, 'succeeded', $4, $5, $5)
	`, input.UserID, input.CourseID, input.Stage, input.ArtifactRoot, finishedAt)
	if err != nil {
		return err
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
			return err
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
			return err
		}
	}
	payload, err := json.Marshal(input.Summary)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		insert into study_artifacts (
			user_id, course_id, stage, kind, path, source_path
		)
		values ($1, $2, $3, 'summary', $4, $5)
	`, input.UserID, input.CourseID, input.Stage, input.ArtifactRoot, string(payload))
	if err != nil {
		return err
	}
	return tx.Commit()
}

func nullString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	return sql.NullString{String: value, Valid: value != ""}
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
