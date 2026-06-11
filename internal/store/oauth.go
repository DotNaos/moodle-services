package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/DotNaos/moodle-services/internal/auth"
)

type OAuthClient struct {
	ClientID      string
	ClientName    string
	RedirectURIs  []string
	GrantTypes    []string
	ResponseTypes []string
	Scope         string
	CreatedAt     time.Time
}

type CreateOAuthClientInput struct {
	ClientID      string
	ClientName    string
	RedirectURIs  []string
	GrantTypes    []string
	ResponseTypes []string
	Scope         string
}

type CreateOAuthAuthorizationCodeInput struct {
	Code                string
	ClientID            string
	UserID              string
	RedirectURI         string
	CodeChallenge       string
	CodeChallengeMethod string
	Resource            string
	Scope               string
	ExpiresAt           time.Time
	HashSecret          []byte
}

type OAuthAuthorizationCode struct {
	ClientID            string
	UserID              string
	RedirectURI         string
	CodeChallenge       string
	CodeChallengeMethod string
	Resource            string
	Scope               string
	ExpiresAt           time.Time
}

type CreateOAuthTokenInput struct {
	Token      string
	UserID     string
	ClientID   string
	Resource   string
	Scope      string
	ExpiresAt  time.Time
	HashSecret []byte
}

type OAuthToken struct {
	UserID    string
	ClientID  string
	Resource  string
	Scope     string
	ExpiresAt time.Time
}

func (s *Store) CreateOAuthClient(ctx context.Context, input CreateOAuthClientInput) (OAuthClient, error) {
	redirectURIs, err := json.Marshal(input.RedirectURIs)
	if err != nil {
		return OAuthClient{}, err
	}
	grantTypes, err := json.Marshal(input.GrantTypes)
	if err != nil {
		return OAuthClient{}, err
	}
	responseTypes, err := json.Marshal(input.ResponseTypes)
	if err != nil {
		return OAuthClient{}, err
	}
	var client OAuthClient
	err = s.db.QueryRowContext(ctx, `
		insert into oauth_clients (client_id, client_name, redirect_uris, grant_types, response_types, scope)
		values ($1, $2, $3::jsonb, $4::jsonb, $5::jsonb, $6)
		on conflict (client_id)
		do update set
			client_name = excluded.client_name,
			redirect_uris = excluded.redirect_uris,
			grant_types = excluded.grant_types,
			response_types = excluded.response_types,
			scope = excluded.scope
		returning client_id, client_name, redirect_uris::text, grant_types::text, response_types::text, scope, created_at
	`, input.ClientID, input.ClientName, string(redirectURIs), string(grantTypes), string(responseTypes), input.Scope).
		Scan(&client.ClientID, &client.ClientName, newStringSliceScanner(&client.RedirectURIs), newStringSliceScanner(&client.GrantTypes), newStringSliceScanner(&client.ResponseTypes), &client.Scope, &client.CreatedAt)
	return client, err
}

func (s *Store) OAuthClient(ctx context.Context, clientID string) (OAuthClient, error) {
	var client OAuthClient
	err := s.db.QueryRowContext(ctx, `
		select client_id, client_name, redirect_uris::text, grant_types::text, response_types::text, scope, created_at
		from oauth_clients
		where client_id = $1
	`, clientID).Scan(&client.ClientID, &client.ClientName, newStringSliceScanner(&client.RedirectURIs), newStringSliceScanner(&client.GrantTypes), newStringSliceScanner(&client.ResponseTypes), &client.Scope, &client.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return OAuthClient{}, ErrNotFound
	}
	return client, err
}

func (s *Store) UserForClerkID(ctx context.Context, clerkUserID string) (User, error) {
	var user User
	err := s.db.QueryRowContext(ctx, `
		select id::text, moodle_site_url, moodle_user_id, display_name
		from users
		where clerk_user_id = $1
	`, clerkUserID).Scan(&user.ID, &user.MoodleSiteURL, &user.MoodleUserID, &user.DisplayName)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return user, err
}

func (s *Store) MoodleCredentialsForUserID(ctx context.Context, userID string) (MoodleCredentials, error) {
	var out MoodleCredentials
	out.UserID = userID
	err := s.db.QueryRowContext(ctx, `
		select encrypted_mobile_session_json, coalesce(encrypted_webex_session_json, ''), coalesce(encrypted_webex_credentials, '')
		from moodle_accounts
		where user_id = $1
		order by updated_at desc
		limit 1
	`, userID).Scan(&out.EncryptedMobileSessionJSON, &out.EncryptedWebexSessionJSON, &out.EncryptedWebexCredentials)
	if errors.Is(err, sql.ErrNoRows) {
		return MoodleCredentials{}, fmt.Errorf("no Moodle account is connected for this user")
	}
	if err != nil {
		return MoodleCredentials{}, err
	}
	_ = s.db.QueryRowContext(ctx, `
		select encrypted_url from calendar_subscriptions where user_id = $1 order by updated_at desc limit 1
	`, userID).Scan(&out.EncryptedCalendarURL)
	return out, nil
}

func (s *Store) CreateOAuthAuthorizationCode(ctx context.Context, input CreateOAuthAuthorizationCodeInput) error {
	_, err := s.db.ExecContext(ctx, `
		insert into oauth_authorization_codes (
			code_hash, client_id, user_id, redirect_uri, code_challenge,
			code_challenge_method, resource, scope, expires_at
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, auth.HMACAPIKey(input.Code, input.HashSecret), input.ClientID, input.UserID, input.RedirectURI, input.CodeChallenge, input.CodeChallengeMethod, input.Resource, input.Scope, input.ExpiresAt)
	return err
}

func (s *Store) ConsumeOAuthAuthorizationCode(ctx context.Context, code string, hashSecret []byte) (OAuthAuthorizationCode, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return OAuthAuthorizationCode{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var out OAuthAuthorizationCode
	err = tx.QueryRowContext(ctx, `
		update oauth_authorization_codes
		set used_at = now()
		where code_hash = $1
		  and used_at is null
		  and expires_at > now()
		returning client_id, user_id::text, redirect_uri, code_challenge, code_challenge_method, resource, scope, expires_at
	`, auth.HMACAPIKey(code, hashSecret)).Scan(&out.ClientID, &out.UserID, &out.RedirectURI, &out.CodeChallenge, &out.CodeChallengeMethod, &out.Resource, &out.Scope, &out.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return OAuthAuthorizationCode{}, auth.ErrUnauthorized
	}
	if err != nil {
		return OAuthAuthorizationCode{}, err
	}
	if err := tx.Commit(); err != nil {
		return OAuthAuthorizationCode{}, err
	}
	return out, nil
}

func (s *Store) CreateOAuthAccessToken(ctx context.Context, input CreateOAuthTokenInput) error {
	return s.createOAuthToken(ctx, "oauth_access_tokens", input)
}

func (s *Store) CreateOAuthRefreshToken(ctx context.Context, input CreateOAuthTokenInput) error {
	return s.createOAuthToken(ctx, "oauth_refresh_tokens", input)
}

func (s *Store) OAuthAccessToken(ctx context.Context, token string, hashSecret []byte) (OAuthToken, error) {
	return s.oauthToken(ctx, "oauth_access_tokens", token, hashSecret)
}

func (s *Store) OAuthRefreshToken(ctx context.Context, token string, hashSecret []byte) (OAuthToken, error) {
	return s.oauthToken(ctx, "oauth_refresh_tokens", token, hashSecret)
}

func (s *Store) RevokeOAuthRefreshToken(ctx context.Context, token string, hashSecret []byte) error {
	_, err := s.db.ExecContext(ctx, `
		update oauth_refresh_tokens
		set revoked_at = now()
		where token_hash = $1 and revoked_at is null
	`, auth.HMACAPIKey(token, hashSecret))
	return err
}

func (s *Store) MoodleCredentialsForOAuthAccessToken(ctx context.Context, token string, hashSecret []byte) (MoodleCredentials, error) {
	accessToken, err := s.OAuthAccessToken(ctx, token, hashSecret)
	if err != nil {
		return MoodleCredentials{}, err
	}
	return s.MoodleCredentialsForUserID(ctx, accessToken.UserID)
}

func (s *Store) UserForOAuthAccessToken(ctx context.Context, token string, hashSecret []byte) (User, error) {
	accessToken, err := s.OAuthAccessToken(ctx, token, hashSecret)
	if err != nil {
		return User{}, err
	}
	return s.UserByID(ctx, accessToken.UserID)
}

func (s *Store) createOAuthToken(ctx context.Context, table string, input CreateOAuthTokenInput) error {
	_, err := s.db.ExecContext(ctx, `
		insert into `+table+` (token_hash, user_id, client_id, resource, scope, expires_at)
		values ($1, $2, $3, $4, $5, $6)
	`, auth.HMACAPIKey(input.Token, input.HashSecret), input.UserID, input.ClientID, input.Resource, input.Scope, input.ExpiresAt)
	return err
}

func (s *Store) oauthToken(ctx context.Context, table string, token string, hashSecret []byte) (OAuthToken, error) {
	var out OAuthToken
	err := s.db.QueryRowContext(ctx, `
		update `+table+`
		set last_used_at = now()
		where token_hash = $1
		  and revoked_at is null
		  and expires_at > now()
		returning user_id::text, client_id, resource, scope, expires_at
	`, auth.HMACAPIKey(token, hashSecret)).Scan(&out.UserID, &out.ClientID, &out.Resource, &out.Scope, &out.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return OAuthToken{}, auth.ErrUnauthorized
	}
	return out, err
}

type stringSliceScanner struct {
	target *[]string
}

func newStringSliceScanner(target *[]string) *stringSliceScanner {
	return &stringSliceScanner{target: target}
}

func (s *stringSliceScanner) Scan(src any) error {
	switch value := src.(type) {
	case string:
		return json.Unmarshal([]byte(value), s.target)
	case []byte:
		return json.Unmarshal(value, s.target)
	default:
		return fmt.Errorf("unsupported JSON value %T", src)
	}
}
