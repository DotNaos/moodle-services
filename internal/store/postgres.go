package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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
	return &Store{db: db}, nil
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

func (s *Store) MoodleCredentialsForAPIKey(ctx context.Context, key string, hashSecret []byte) (MoodleCredentials, error) {
	user, err := s.UserForAPIKey(ctx, key, hashSecret)
	if err != nil {
		return MoodleCredentials{}, err
	}
	var out MoodleCredentials
	out.UserID = user.ID
	err = s.db.QueryRowContext(ctx, `
		select encrypted_mobile_session_json
		from moodle_accounts
		where user_id = $1
		order by updated_at desc
		limit 1
	`, user.ID).Scan(&out.EncryptedMobileSessionJSON)
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
