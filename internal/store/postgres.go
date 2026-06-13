package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DotNaos/moodle-services/internal/auth"

	_ "github.com/jackc/pgx/v5/stdlib"
)

var ErrNotFound = sql.ErrNoRows

type Store struct {
	db *sql.DB
}

type User struct {
	ID            string `json:"id"`
	MoodleSiteURL string `json:"moodleSiteUrl"`
	MoodleUserID  int    `json:"moodleUserId"`
	DisplayName   string `json:"displayName"`
}

type MoodleCredentials struct {
	UserID                     string
	EncryptedMobileSessionJSON string
	EncryptedCalendarURL       string
	EncryptedWebexSessionJSON  string
	EncryptedWebexCredentials  string
	LegacyMobileSessionJSON    string
	LegacyCalendarURL          string
}

type UpsertMoodleAccountInput struct {
	SiteURL                    string
	MoodleUserID               int
	DisplayName                string
	ClerkUserID                string
	SchoolID                   string
	EncryptedMobileSessionJSON string
}

type UpsertWebexSessionInput struct {
	UserID                    string
	EncryptedWebexSessionJSON string
	// EncryptedWebexCredentials, when non-empty, also persists the encrypted
	// {username,password} blob used for silent session auto-renew.
	EncryptedWebexCredentials string
}

type APIKeyRecord struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	KeyPrefix  string     `json:"keyPrefix"`
	Scopes     []string   `json:"scopes"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	RevokedAt  *time.Time `json:"revokedAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
}

func Open(databaseURL string) (*Store, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(0)
	db.SetConnMaxLifetime(2 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := ensureCompatibilitySchema(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func ensureCompatibilitySchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		alter table moodle_accounts
		  add column if not exists encrypted_webex_session_json text,
		  add column if not exists webex_session_updated_at timestamptz,
		  add column if not exists encrypted_webex_credentials text,
		  add column if not exists webex_credentials_updated_at timestamptz
	`)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `
		alter table study_pipeline_runs
		  add column if not exists source_id text,
		  add column if not exists resource_id text,
		  add column if not exists file_hash text,
		  add column if not exists engine text not null default 'unknown',
		  add column if not exists config_hash text not null default 'config:default',
		  add column if not exists ownership text not null default 'shared',
		  add column if not exists created_by uuid references users(id) on delete set null,
		  add column if not exists artifact_refs jsonb not null default '[]'::jsonb;

		update study_pipeline_runs
		set source_id = 'source:moodle-course:' || course_id
		where source_id is null or source_id = '';

		alter table study_pipeline_runs
		  alter column source_id set not null;

		alter table study_pipeline_runs
		  drop constraint if exists study_pipeline_runs_stage_check,
		  add constraint study_pipeline_runs_stage_check
		    check (stage in ('inventory', 'raw', 'extracted', 'curated'));

		alter table study_pipeline_runs
		  drop constraint if exists study_pipeline_runs_status_check,
		  add constraint study_pipeline_runs_status_check
		    check (status in ('queued', 'running', 'succeeded', 'failed', 'stale'));

		alter table study_pipeline_runs
		  drop constraint if exists study_pipeline_runs_ownership_check,
		  add constraint study_pipeline_runs_ownership_check
		    check (ownership in ('shared', 'user_owned'));

		create index if not exists study_pipeline_runs_source_stage_idx
		  on study_pipeline_runs (source_id, coalesce(resource_id, ''), stage, created_at desc);

		create table if not exists active_run_selections (
		  source_id text not null,
		  resource_id text not null default '',
		  stage text not null check (stage in ('inventory', 'raw', 'extracted', 'curated')),
		  active_run_id uuid not null references study_pipeline_runs(id) on delete cascade,
		  selected_by uuid references users(id) on delete set null,
		  selected_at timestamptz not null default now(),
		  reason text not null default '',
		  primary key (source_id, resource_id, stage)
		);

		create table if not exists study_pipeline_feedback (
		  id uuid primary key default gen_random_uuid(),
		  course_id text not null,
		  target_id text not null,
		  target_kind text not null,
		  feedback_type text not null,
		  message text not null default '',
		  source_run_id uuid references study_pipeline_runs(id) on delete set null,
		  source_artifact_id text,
		  status text not null default 'open' check (status in ('open', 'triaged', 'resolved', 'dismissed')),
		  created_by uuid references users(id) on delete set null,
		  created_at timestamptz not null default now(),
		  updated_at timestamptz not null default now()
		);

		create index if not exists study_pipeline_feedback_course_idx
		  on study_pipeline_feedback (course_id, created_at desc);

		create table if not exists study_pipeline_proposals (
		  id uuid primary key default gen_random_uuid(),
		  course_id text not null,
		  target_id text not null,
		  target_kind text not null,
		  title text not null,
		  content_preview text not null default '',
		  source_run_id uuid references study_pipeline_runs(id) on delete set null,
		  source_artifact_id text,
		  model text,
		  status text not null default 'private' check (status in ('private', 'submitted_for_review', 'promoted', 'dismissed')),
		  created_by uuid references users(id) on delete set null,
		  submitted_at timestamptz,
		  created_at timestamptz not null default now(),
		  updated_at timestamptz not null default now()
		);

		create index if not exists study_pipeline_proposals_course_idx
		  on study_pipeline_proposals (course_id, created_at desc);

		create table if not exists study_pipeline_audit_events (
		  id uuid primary key default gen_random_uuid(),
		  course_id text not null,
		  actor_id uuid references users(id) on delete set null,
		  action text not null,
		  target_kind text not null,
		  target_id text not null,
		  source_run_id uuid references study_pipeline_runs(id) on delete set null,
		  source_artifact_id text,
		  message text not null default '',
		  created_at timestamptz not null default now()
		);

		create index if not exists study_pipeline_audit_events_course_idx
		  on study_pipeline_audit_events (course_id, created_at desc);
	`)
	return err
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store is not configured")
	}
	return s.db.PingContext(ctx)
}

func (s *Store) UpsertMoodleAccount(ctx context.Context, input UpsertMoodleAccountInput) (User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var user User
	if input.ClerkUserID != "" {
		err = tx.QueryRowContext(ctx, `
			select id::text, moodle_site_url, moodle_user_id, display_name
			from users
			where clerk_user_id = $1
		`, input.ClerkUserID).Scan(&user.ID, &user.MoodleSiteURL, &user.MoodleUserID, &user.DisplayName)
		if errors.Is(err, sql.ErrNoRows) {
			err = tx.QueryRowContext(ctx, `
				insert into users (moodle_site_url, moodle_user_id, display_name, clerk_user_id)
				values ($1, $2, $3, $4)
				on conflict (moodle_site_url, moodle_user_id)
				do update set
					display_name = excluded.display_name,
					clerk_user_id = excluded.clerk_user_id,
					updated_at = now()
				returning id::text, moodle_site_url, moodle_user_id, display_name
			`, input.SiteURL, input.MoodleUserID, input.DisplayName, input.ClerkUserID).Scan(&user.ID, &user.MoodleSiteURL, &user.MoodleUserID, &user.DisplayName)
		} else if err == nil {
			err = tx.QueryRowContext(ctx, `
				update users
				set
					moodle_site_url = $1,
					moodle_user_id = $2,
					display_name = $3,
					updated_at = now()
				where id = $4
				returning id::text, moodle_site_url, moodle_user_id, display_name
			`, input.SiteURL, input.MoodleUserID, input.DisplayName, user.ID).Scan(&user.ID, &user.MoodleSiteURL, &user.MoodleUserID, &user.DisplayName)
		}
	} else {
		err = tx.QueryRowContext(ctx, `
			insert into users (moodle_site_url, moodle_user_id, display_name)
			values ($1, $2, $3)
			on conflict (moodle_site_url, moodle_user_id)
			do update set display_name = excluded.display_name, updated_at = now()
			returning id::text, moodle_site_url, moodle_user_id, display_name
		`, input.SiteURL, input.MoodleUserID, input.DisplayName).Scan(&user.ID, &user.MoodleSiteURL, &user.MoodleUserID, &user.DisplayName)
	}
	if err != nil {
		return User{}, err
	}
	if input.ClerkUserID != "" {
		_, err = tx.ExecContext(ctx, `
			update users
			set clerk_user_id = $1, updated_at = now()
			where id = $2 and clerk_user_id is distinct from $1
		`, input.ClerkUserID, user.ID)
		if err != nil {
			return User{}, err
		}
	}
	if input.ClerkUserID != "" {
		err = tx.QueryRowContext(ctx, `
			select id::text, moodle_site_url, moodle_user_id, display_name
			from users
			where id = $1
		`, user.ID).Scan(&user.ID, &user.MoodleSiteURL, &user.MoodleUserID, &user.DisplayName)
		if err != nil {
			return User{}, err
		}
	}
	_, err = tx.ExecContext(ctx, `
		insert into moodle_accounts (user_id, school_id, site_url, encrypted_mobile_session_json, token_last_validated_at)
		values ($1, $2, $3, $4, now())
		on conflict (user_id, site_url)
		do update set
			school_id = excluded.school_id,
			encrypted_mobile_session_json = excluded.encrypted_mobile_session_json,
			token_last_validated_at = now(),
			updated_at = now()
	`, user.ID, input.SchoolID, input.SiteURL, input.EncryptedMobileSessionJSON)
	if err != nil {
		return User{}, err
	}
	if err := tx.Commit(); err != nil {
		return User{}, err
	}
	return user, nil
}

func (s *Store) CreateAPIKey(ctx context.Context, userID string, name string, key string, hashSecret []byte, scopes []string) (APIKeyRecord, error) {
	scopesJSON, err := json.Marshal(scopes)
	if err != nil {
		return APIKeyRecord{}, err
	}
	var record APIKeyRecord
	err = s.db.QueryRowContext(ctx, `
		insert into api_keys (user_id, name, key_prefix, key_hash, scopes)
		values ($1, $2, $3, $4, $5::jsonb)
		returning id::text, name, key_prefix, scopes::text, last_used_at, revoked_at, created_at
	`, userID, name, auth.KeyPrefix(key), auth.HMACAPIKey(key, hashSecret), string(scopesJSON)).
		Scan(&record.ID, &record.Name, &record.KeyPrefix, newScopesScanner(&record.Scopes), &record.LastUsedAt, &record.RevokedAt, &record.CreatedAt)
	return record, err
}

func (s *Store) RevokeActiveAPIKeysForUser(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, `
		update api_keys
		set revoked_at = now()
		where user_id = $1 and revoked_at is null
	`, userID)
	return err
}

func (s *Store) UserForAPIKey(ctx context.Context, key string, hashSecret []byte) (User, error) {
	var user User
	err := s.db.QueryRowContext(ctx, `
		update api_keys set last_used_at = now()
		where key_hash = $1 and revoked_at is null
		returning user_id::text
	`, auth.HMACAPIKey(key, hashSecret)).Scan(&user.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, auth.ErrUnauthorized
	}
	if err != nil {
		return User{}, err
	}
	err = s.db.QueryRowContext(ctx, `
		select id::text, moodle_site_url, moodle_user_id, display_name
		from users
		where id = $1
	`, user.ID).Scan(&user.ID, &user.MoodleSiteURL, &user.MoodleUserID, &user.DisplayName)
	return user, err
}

func (s *Store) UserByID(ctx context.Context, userID string) (User, error) {
	var user User
	err := s.db.QueryRowContext(ctx, `
		select id::text, moodle_site_url, moodle_user_id, display_name
		from users
		where id = $1
	`, userID).Scan(&user.ID, &user.MoodleSiteURL, &user.MoodleUserID, &user.DisplayName)
	return user, err
}

func (s *Store) MoodleCredentialsForAPIKey(ctx context.Context, key string, hashSecret []byte) (MoodleCredentials, error) {
	user, err := s.UserForAPIKey(ctx, key, hashSecret)
	if err != nil {
		return MoodleCredentials{}, err
	}
	var out MoodleCredentials
	out.UserID = user.ID
	err = s.db.QueryRowContext(ctx, `
		select encrypted_mobile_session_json, coalesce(encrypted_webex_session_json, ''), coalesce(encrypted_webex_credentials, '')
		from moodle_accounts
		where user_id = $1
		order by updated_at desc
		limit 1
	`, user.ID).Scan(&out.EncryptedMobileSessionJSON, &out.EncryptedWebexSessionJSON, &out.EncryptedWebexCredentials)
	if errors.Is(err, sql.ErrNoRows) {
		return MoodleCredentials{}, fmt.Errorf("no Moodle account is connected for this user")
	}
	if err != nil {
		return MoodleCredentials{}, err
	}
	_ = s.db.QueryRowContext(ctx, `
		select encrypted_url from calendar_subscriptions where user_id = $1 order by updated_at desc limit 1
	`, user.ID).Scan(&out.EncryptedCalendarURL)
	return out, nil
}

func (s *Store) UpsertCalendarSubscription(ctx context.Context, userID string, encryptedURL string) error {
	if strings.TrimSpace(userID) == "" {
		return fmt.Errorf("user id is required")
	}
	if strings.TrimSpace(encryptedURL) == "" {
		return fmt.Errorf("encrypted calendar url is required")
	}
	_, err := s.db.ExecContext(ctx, `
		insert into calendar_subscriptions (user_id, encrypted_url, updated_at)
		values ($1, $2, now())
	`, userID, encryptedURL)
	return err
}

func (s *Store) CalendarSubscriptionConfigured(ctx context.Context, userID string) (bool, error) {
	var configured bool
	err := s.db.QueryRowContext(ctx, `
		select exists(
			select 1
			from calendar_subscriptions
			where user_id = $1 and trim(encrypted_url) <> ''
		)
	`, userID).Scan(&configured)
	return configured, err
}

func (s *Store) UpsertWebexSession(ctx context.Context, input UpsertWebexSessionInput) error {
	if input.UserID == "" {
		return fmt.Errorf("user id is required")
	}
	if input.EncryptedWebexSessionJSON == "" {
		return fmt.Errorf("encrypted webex session json is required")
	}
	result, err := s.db.ExecContext(ctx, `
		update moodle_accounts
		set
			encrypted_webex_session_json = $2,
			webex_session_updated_at = now(),
			encrypted_webex_credentials = case when $3 <> '' then $3 else encrypted_webex_credentials end,
			webex_credentials_updated_at = case when $3 <> '' then now() else webex_credentials_updated_at end,
			updated_at = now()
		where id = (
			select id
			from moodle_accounts
			where user_id = $1
			order by updated_at desc
			limit 1
		)
	`, input.UserID, input.EncryptedWebexSessionJSON, input.EncryptedWebexCredentials)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("no Moodle account is connected for this user")
	}
	return nil
}

func (s *Store) ListAPIKeys(ctx context.Context, userID string) ([]APIKeyRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		select id::text, name, key_prefix, scopes::text, last_used_at, revoked_at, created_at
		from api_keys
		where user_id = $1
		order by created_at desc
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []APIKeyRecord
	for rows.Next() {
		var record APIKeyRecord
		if err := rows.Scan(&record.ID, &record.Name, &record.KeyPrefix, newScopesScanner(&record.Scopes), &record.LastUsedAt, &record.RevokedAt, &record.CreatedAt); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Store) RevokeAPIKey(ctx context.Context, userID string, keyID string) error {
	result, err := s.db.ExecContext(ctx, `
		update api_keys set revoked_at = now()
		where id = $1 and user_id = $2 and revoked_at is null
	`, keyID, userID)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

type scopesScanner struct {
	target *[]string
}

func newScopesScanner(target *[]string) *scopesScanner {
	return &scopesScanner{target: target}
}

func (s *scopesScanner) Scan(value any) error {
	var raw []byte
	switch typed := value.(type) {
	case string:
		raw = []byte(typed)
	case []byte:
		raw = typed
	default:
		return fmt.Errorf("unsupported scopes value %T", value)
	}
	return json.Unmarshal(raw, s.target)
}
