package apicontracts

import (
	"github.com/DotNaos/moodle-services/internal/moodle"
	"github.com/DotNaos/moodle-services/internal/moodleservice"
	"github.com/DotNaos/moodle-services/internal/store"
)

type ErrorResponse struct {
	Error            string `json:"error"`
	Code             string `json:"code,omitempty"`
	ErrorDescription string `json:"error_description,omitempty"`
}

type QRExchangeRequest struct {
	QR   string `json:"qr"`
	Name string `json:"name,omitempty"`
}

type QRExchangeResponse struct {
	User         store.User         `json:"user"`
	APIKey       string             `json:"apiKey"`
	APIKeyRecord store.APIKeyRecord `json:"apiKeyRecord"`
}

type CreateAPIKeyRequest struct {
	Name           string   `json:"name,omitempty"`
	Scopes         []string `json:"scopes,omitempty"`
	RevokeExisting bool     `json:"revokeExisting,omitempty"`
}

type CreateAPIKeyResponse struct {
	APIKey          string             `json:"apiKey"`
	APIKeyRecord    store.APIKeyRecord `json:"apiKeyRecord"`
	RevokedExisting bool               `json:"revokedExisting"`
}

type ListAPIKeysResponse struct {
	Keys []store.APIKeyRecord `json:"keys"`
}

type RevokeAPIKeyResponse struct {
	Revoked bool `json:"revoked"`
}

type CreateCodexStateSnapshotRequest struct {
	Kind      string         `json:"kind"`
	ZipBase64 string         `json:"zipBase64"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type CodexStateSnapshotResponse struct {
	Snapshot  store.CodexStateSnapshot `json:"snapshot"`
	ZipBase64 string                   `json:"zipBase64,omitempty"`
}

type CoursesResponse struct {
	Courses []moodle.Course `json:"courses"`
}

type CategoriesResponse struct {
	Categories []moodle.Category `json:"categories"`
}

type MaterialsResponse struct {
	Materials []moodle.Resource `json:"materials"`
}

type MaterialTextResponse struct {
	Document moodleservice.FetchDocument `json:"document"`
}

type SearchResponse struct {
	Results []moodleservice.SearchResult `json:"results"`
}
