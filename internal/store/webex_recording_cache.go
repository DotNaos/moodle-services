package store

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type WebexRecordingCache struct {
	CourseID       string
	FetchedAt      time.Time
	RecordingsJSON []byte
	UserID         string
}

func (s *Store) CachedWebexRecordings(ctx context.Context, userID string, courseID string, maxAge time.Duration) (WebexRecordingCache, bool, error) {
	if userID == "" {
		return WebexRecordingCache{}, false, fmt.Errorf("user id is required")
	}
	if courseID == "" {
		return WebexRecordingCache{}, false, fmt.Errorf("course id is required")
	}
	if err := s.ensureWebexRecordingCache(ctx); err != nil {
		return WebexRecordingCache{}, false, err
	}
	var out WebexRecordingCache
	var raw string
	err := s.db.QueryRowContext(ctx, `
		select user_id::text, course_id, recordings_json::text, fetched_at
		from webex_recording_cache
		where user_id = $1 and course_id = $2
	`, userID, courseID).Scan(&out.UserID, &out.CourseID, &raw, &out.FetchedAt)
	if errors.Is(err, ErrNotFound) {
		return WebexRecordingCache{}, false, nil
	}
	if err != nil {
		return WebexRecordingCache{}, false, err
	}
	if maxAge > 0 && time.Since(out.FetchedAt) > maxAge {
		return WebexRecordingCache{}, false, nil
	}
	out.RecordingsJSON = []byte(raw)
	return out, true, nil
}

func (s *Store) UpsertWebexRecordings(ctx context.Context, userID string, courseID string, recordingsJSON []byte) error {
	if userID == "" {
		return fmt.Errorf("user id is required")
	}
	if courseID == "" {
		return fmt.Errorf("course id is required")
	}
	if len(recordingsJSON) == 0 {
		return fmt.Errorf("recordings json is required")
	}
	if err := s.ensureWebexRecordingCache(ctx); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		insert into webex_recording_cache (user_id, course_id, recordings_json, fetched_at)
		values ($1, $2, $3::jsonb, now())
		on conflict (user_id, course_id)
		do update set recordings_json = excluded.recordings_json, fetched_at = now()
	`, userID, courseID, string(recordingsJSON))
	return err
}

func (s *Store) ensureWebexRecordingCache(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		create table if not exists webex_recording_cache (
			user_id uuid not null references users(id) on delete cascade,
			course_id text not null,
			recordings_json jsonb not null,
			fetched_at timestamptz not null default now(),
			primary key (user_id, course_id)
		)
	`)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		create index if not exists webex_recording_cache_fetched_at_idx
		on webex_recording_cache (fetched_at)
	`)
	return err
}
