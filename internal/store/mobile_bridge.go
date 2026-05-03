package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/DotNaos/moodle-services/internal/auth"
)

type MobileBridgeRequest struct {
	ChallengeHash   string
	ClerkUserID     string
	Origin          string
	Endpoint        string
	AppName         string
	State           string
	ExpiresAt       time.Time
	CompletedAt     *time.Time
	ConsumedAt      *time.Time
	UserID          string
	EncryptedAPIKey string
}

type CreateMobileBridgeRequestInput struct {
	Challenge   string
	ClerkUserID string
	Origin      string
	Endpoint    string
	AppName     string
	State       string
	ExpiresAt   time.Time
	HashSecret  []byte
}

func (s *Store) CreateMobileBridgeRequest(ctx context.Context, input CreateMobileBridgeRequestInput) (MobileBridgeRequest, error) {
	var out MobileBridgeRequest
	err := s.db.QueryRowContext(ctx, `
		insert into mobile_bridge_requests (
			challenge_hash, clerk_user_id, origin, endpoint, app_name, state, expires_at
		)
		values ($1, $2, $3, $4, $5, $6, $7)
		returning challenge_hash, clerk_user_id, origin, endpoint, app_name, state,
			expires_at, completed_at, consumed_at, coalesce(user_id::text, ''),
			coalesce(encrypted_api_key, '')
	`, auth.HMACAPIKey(input.Challenge, input.HashSecret), input.ClerkUserID, input.Origin, input.Endpoint, input.AppName, input.State, input.ExpiresAt).
		Scan(&out.ChallengeHash, &out.ClerkUserID, &out.Origin, &out.Endpoint, &out.AppName, &out.State, &out.ExpiresAt, &out.CompletedAt, &out.ConsumedAt, &out.UserID, &out.EncryptedAPIKey)
	return out, err
}

func (s *Store) MobileBridgeRequest(ctx context.Context, challenge string, hashSecret []byte) (MobileBridgeRequest, error) {
	var out MobileBridgeRequest
	err := s.db.QueryRowContext(ctx, `
		select challenge_hash, clerk_user_id, origin, endpoint, app_name, state,
			expires_at, completed_at, consumed_at, coalesce(user_id::text, ''),
			coalesce(encrypted_api_key, '')
		from mobile_bridge_requests
		where challenge_hash = $1
	`, auth.HMACAPIKey(challenge, hashSecret)).
		Scan(&out.ChallengeHash, &out.ClerkUserID, &out.Origin, &out.Endpoint, &out.AppName, &out.State, &out.ExpiresAt, &out.CompletedAt, &out.ConsumedAt, &out.UserID, &out.EncryptedAPIKey)
	if errors.Is(err, sql.ErrNoRows) {
		return MobileBridgeRequest{}, ErrNotFound
	}
	return out, err
}

func (s *Store) CompleteMobileBridgeRequest(ctx context.Context, challenge string, hashSecret []byte, userID string, encryptedAPIKey string) error {
	result, err := s.db.ExecContext(ctx, `
		update mobile_bridge_requests
		set completed_at = now(),
			user_id = $2,
			encrypted_api_key = $3,
			updated_at = now()
		where challenge_hash = $1
		  and completed_at is null
		  and expires_at > now()
	`, auth.HMACAPIKey(challenge, hashSecret), userID, encryptedAPIKey)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return ErrNotFound
	}
	return nil
}
