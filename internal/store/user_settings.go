package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// UserSettings returns the opaque per-user settings JSON blob (the app owns the
// schema). A missing row yields an empty object.
func (s *Store) UserSettings(ctx context.Context, userID string) (json.RawMessage, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("user id is required")
	}
	var raw []byte
	err := s.db.QueryRowContext(ctx, `select settings from user_settings where user_id = $1`, userID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return json.RawMessage("{}"), nil
	}
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return json.RawMessage("{}"), nil
	}
	return json.RawMessage(raw), nil
}

func (s *Store) UpsertUserSettings(ctx context.Context, userID string, settings json.RawMessage) error {
	if strings.TrimSpace(userID) == "" {
		return fmt.Errorf("user id is required")
	}
	if len(settings) == 0 || !json.Valid(settings) {
		return fmt.Errorf("settings must be valid JSON")
	}
	_, err := s.db.ExecContext(ctx, `
		insert into user_settings (user_id, settings, updated_at)
		values ($1, $2, now())
		on conflict (user_id) do update set settings = excluded.settings, updated_at = now()
	`, userID, []byte(settings))
	return err
}
