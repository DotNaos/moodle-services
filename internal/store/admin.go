package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

type AdminUser struct {
	ID                                 string `json:"id"`
	MoodleSiteURL                      string `json:"moodleSiteUrl"`
	MoodleUserID                       int    `json:"moodleUserId"`
	DisplayName                        string `json:"displayName"`
	ClerkUserID                        string `json:"clerkUserId,omitempty"`
	IsAdmin                            bool   `json:"isAdmin"`
	CodexStateQuotaBytes               int64  `json:"codexStateQuotaBytes"`
	CodexStateQuotaOverrideBytes       *int64 `json:"codexStateQuotaOverrideBytes,omitempty"`
	CodexStateUsageBytes               int64  `json:"codexStateUsageBytes"`
	CodexStateSnapshotCount            int    `json:"codexStateSnapshotCount"`
	CodexStateQuotaConfiguredByDefault bool   `json:"codexStateQuotaConfiguredByDefault"`
}

func (s *Store) UserIsAdmin(ctx context.Context, clerkUserID string, configuredAdmin bool) (bool, error) {
	if configuredAdmin {
		return true, nil
	}
	err := s.db.QueryRowContext(ctx, `
		select is_admin
		from users
		where clerk_user_id = $1
	`, strings.TrimSpace(clerkUserID)).Scan(&configuredAdmin)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrNotFound
	}
	return configuredAdmin, err
}

func (s *Store) ListAdminUsers(ctx context.Context, defaultQuotaBytes int64, adminQuotaBytes int64) ([]AdminUser, error) {
	rows, err := s.db.QueryContext(ctx, adminUserSelectSQL("", `
		order by lower(nullif(u.display_name, '')) nulls last, u.created_at desc
	`))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := []AdminUser{}
	for rows.Next() {
		user, err := scanAdminUser(rows, defaultQuotaBytes, adminQuotaBytes)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *Store) UpdateAdminUser(ctx context.Context, userID string, quotaBytes *int64, resetQuota bool, isAdmin *bool, defaultQuotaBytes int64, adminQuotaBytes int64) (AdminUser, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AdminUser{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if resetQuota {
		if _, err := tx.ExecContext(ctx, `
			update users
			set codex_state_quota_bytes = null, updated_at = now()
			where id = $1
		`, userID); err != nil {
			return AdminUser{}, err
		}
	} else if quotaBytes != nil {
		if _, err := tx.ExecContext(ctx, `
			update users
			set codex_state_quota_bytes = $2, updated_at = now()
			where id = $1
		`, userID, *quotaBytes); err != nil {
			return AdminUser{}, err
		}
	}
	if isAdmin != nil {
		if _, err := tx.ExecContext(ctx, `
			update users
			set is_admin = $2, updated_at = now()
			where id = $1
		`, userID, *isAdmin); err != nil {
			return AdminUser{}, err
		}
	}
	var updated AdminUser
	err = tx.QueryRowContext(ctx, adminUserSelectSQL("where u.id = $1", ""), userID).Scan(adminUserScanDest(&updated, defaultQuotaBytes, adminQuotaBytes)...)
	if errors.Is(err, sql.ErrNoRows) {
		return AdminUser{}, ErrNotFound
	}
	if err != nil {
		return AdminUser{}, err
	}
	if err := tx.Commit(); err != nil {
		return AdminUser{}, err
	}
	return updated, nil
}

func adminUserSelectSQL(filter string, tail string) string {
	return `
		select
			u.id::text,
			u.moodle_site_url,
			u.moodle_user_id,
			u.display_name,
			coalesce(u.clerk_user_id, ''),
			u.is_admin,
			u.codex_state_quota_bytes,
			coalesce(sum(s.zip_size_bytes), 0)::bigint,
			count(s.id)::int
		from users u
		left join codex_state_snapshots s on s.user_id = u.id
		` + filter + `
		group by u.id
		` + tail + `
	`
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanAdminUser(scanner rowScanner, defaultQuotaBytes int64, adminQuotaBytes int64) (AdminUser, error) {
	var user AdminUser
	err := scanner.Scan(adminUserScanDest(&user, defaultQuotaBytes, adminQuotaBytes)...)
	return user, err
}

func adminUserScanDest(user *AdminUser, defaultQuotaBytes int64, adminQuotaBytes int64) []any {
	var quotaOverride sql.NullInt64
	return []any{
		&user.ID,
		&user.MoodleSiteURL,
		&user.MoodleUserID,
		&user.DisplayName,
		&user.ClerkUserID,
		&user.IsAdmin,
		quotaScanner{target: &quotaOverride, user: user, defaultQuotaBytes: defaultQuotaBytes, adminQuotaBytes: adminQuotaBytes},
		&user.CodexStateUsageBytes,
		&user.CodexStateSnapshotCount,
	}
}

type quotaScanner struct {
	target            *sql.NullInt64
	user              *AdminUser
	defaultQuotaBytes int64
	adminQuotaBytes   int64
}

// Scan resolves the effective quota: an explicit per-user override wins;
// admins otherwise get the admin default; everyone else the user default.
// Relies on user.IsAdmin being scanned before this column.
func (s quotaScanner) Scan(value any) error {
	if err := s.target.Scan(value); err != nil {
		return err
	}
	if s.target.Valid && s.target.Int64 > 0 {
		value := s.target.Int64
		s.user.CodexStateQuotaBytes = value
		s.user.CodexStateQuotaOverrideBytes = &value
		return nil
	}
	if s.user.IsAdmin && s.adminQuotaBytes > 0 {
		s.user.CodexStateQuotaBytes = s.adminQuotaBytes
		s.user.CodexStateQuotaConfiguredByDefault = true
		return nil
	}
	s.user.CodexStateQuotaBytes = s.defaultQuotaBytes
	s.user.CodexStateQuotaConfiguredByDefault = true
	return nil
}
