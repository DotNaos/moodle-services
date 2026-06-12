package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type CodexStateSnapshot struct {
	ID             string         `json:"id"`
	Kind           string         `json:"kind"`
	StorageBackend string         `json:"storageBackend"`
	StorageAccount string         `json:"storageAccount,omitempty"`
	ObjectKey      string         `json:"objectKey,omitempty"`
	ZipSHA256      string         `json:"zipSha256"`
	ZipSizeBytes   int            `json:"zipSizeBytes"`
	Metadata       map[string]any `json:"metadata"`
	CreatedAt      time.Time      `json:"createdAt"`
}

type CreateCodexStateSnapshotInput struct {
	UserID         string
	Kind           string
	StorageBackend string
	StorageAccount string
	ObjectKey      string
	EncryptedZip   string
	ZipSHA256      string
	ZipSizeBytes   int
	Metadata       map[string]any
	UserQuotaBytes int64
}

type CodexStateSnapshotData struct {
	Snapshot      CodexStateSnapshot
	EncryptedZip  string
	StorageObject string
}

func (s *Store) CreateCodexStateSnapshot(ctx context.Context, input CreateCodexStateSnapshotInput) (CodexStateSnapshot, error) {
	if input.StorageBackend == "" {
		input.StorageBackend = "postgres"
	}
	if input.UserQuotaBytes > 0 && int64(input.ZipSizeBytes) > input.UserQuotaBytes {
		return CodexStateSnapshot{}, fmt.Errorf("codex state snapshot exceeds user quota")
	}
	if input.Metadata == nil {
		input.Metadata = map[string]any{}
	}
	metadataJSON, err := json.Marshal(input.Metadata)
	if err != nil {
		return CodexStateSnapshot{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CodexStateSnapshot{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var snapshot CodexStateSnapshot
	err = tx.QueryRowContext(ctx, `
		insert into codex_state_snapshots (
			user_id, kind, storage_backend, storage_account_id, object_key, encrypted_zip, zip_sha256, zip_size_bytes, metadata
		)
		values ($1, $2, $3, nullif($4, '')::uuid, nullif($5, ''), nullif($6, ''), $7, $8, $9::jsonb)
		returning id::text, kind, storage_backend, coalesce(storage_account_id::text, ''), coalesce(object_key, ''), zip_sha256, zip_size_bytes, metadata::text, created_at
	`, input.UserID, input.Kind, input.StorageBackend, input.StorageAccount, input.ObjectKey, input.EncryptedZip, input.ZipSHA256, input.ZipSizeBytes, string(metadataJSON)).
		Scan(
			&snapshot.ID,
			&snapshot.Kind,
			&snapshot.StorageBackend,
			&snapshot.StorageAccount,
			&snapshot.ObjectKey,
			&snapshot.ZipSHA256,
			&snapshot.ZipSizeBytes,
			newJSONMapScanner(&snapshot.Metadata),
			&snapshot.CreatedAt,
		)
	if err != nil {
		return CodexStateSnapshot{}, err
	}
	if input.UserQuotaBytes > 0 {
		if _, err := tx.ExecContext(ctx, `
			with ordered as (
				select
					id,
					sum(zip_size_bytes) over (order by created_at desc, id desc) as running_bytes
				from codex_state_snapshots
				where user_id = $1
			)
			delete from codex_state_snapshots
			where user_id = $1
				and id in (
					select id
					from ordered
					where running_bytes > $2
				)
		`, input.UserID, input.UserQuotaBytes); err != nil {
			return CodexStateSnapshot{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return CodexStateSnapshot{}, err
	}
	return snapshot, nil
}

func (s *Store) EffectiveCodexStateQuotaBytes(ctx context.Context, userID string, defaultQuotaBytes int64, adminQuotaBytes int64) (int64, error) {
	var override sql.NullInt64
	var isAdmin bool
	err := s.db.QueryRowContext(ctx, `
		select codex_state_quota_bytes, is_admin
		from users
		where id = $1
	`, userID).Scan(&override, &isAdmin)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	// Explicit per-user override wins; admins otherwise get the admin default.
	if override.Valid && override.Int64 > 0 {
		return override.Int64, nil
	}
	if isAdmin && adminQuotaBytes > 0 {
		return adminQuotaBytes, nil
	}
	return defaultQuotaBytes, nil
}

func (s *Store) LatestCodexStateSnapshot(ctx context.Context, userID string, kind string) (CodexStateSnapshotData, error) {
	var out CodexStateSnapshotData
	err := s.db.QueryRowContext(ctx, `
		select id::text, kind, storage_backend, coalesce(storage_account_id::text, ''), coalesce(object_key, ''), zip_sha256, zip_size_bytes, metadata::text, created_at, coalesce(encrypted_zip, '')
		from codex_state_snapshots
		where user_id = $1 and kind = $2
		order by created_at desc
		limit 1
	`, userID, kind).Scan(
		&out.Snapshot.ID,
		&out.Snapshot.Kind,
		&out.Snapshot.StorageBackend,
		&out.Snapshot.StorageAccount,
		&out.Snapshot.ObjectKey,
		&out.Snapshot.ZipSHA256,
		&out.Snapshot.ZipSizeBytes,
		newJSONMapScanner(&out.Snapshot.Metadata),
		&out.Snapshot.CreatedAt,
		&out.EncryptedZip,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return CodexStateSnapshotData{}, ErrNotFound
	}
	return out, err
}

type jsonMapScanner struct {
	target *map[string]any
}

func newJSONMapScanner(target *map[string]any) *jsonMapScanner {
	return &jsonMapScanner{target: target}
}

func (s *jsonMapScanner) Scan(value any) error {
	var raw []byte
	switch typed := value.(type) {
	case string:
		raw = []byte(typed)
	case []byte:
		raw = typed
	default:
		*s.target = map[string]any{}
		return nil
	}
	if len(raw) == 0 {
		*s.target = map[string]any{}
		return nil
	}
	return json.Unmarshal(raw, s.target)
}
