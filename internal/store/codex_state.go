package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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
	if input.Metadata == nil {
		input.Metadata = map[string]any{}
	}
	metadataJSON, err := json.Marshal(input.Metadata)
	if err != nil {
		return CodexStateSnapshot{}, err
	}
	var snapshot CodexStateSnapshot
	err = s.db.QueryRowContext(ctx, `
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
	return snapshot, err
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
