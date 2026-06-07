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

type CredentialLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
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

type CalendarEventsResponse struct {
	Events []moodle.CalendarEvent `json:"events"`
}

type MaterialTextResponse struct {
	Document moodleservice.FetchDocument `json:"document"`
}

type SearchResponse struct {
	Results []moodleservice.SearchResult `json:"results"`
}

type WebexCredentialsRequest struct {
	CourseID string `json:"courseId,omitempty"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type WebexCredentialsResponse struct {
	SavedSession bool `json:"savedSession"`
}

type WebexRecordingsResponse struct {
	Recordings []moodleservice.WebexRecording `json:"recordings"`
}

type StudyPipelineResponse struct {
	CourseID         string                  `json:"courseId"`
	Status           string                  `json:"status"`
	CreatedAt        string                  `json:"createdAt"`
	Summary          StudyPipelineSummary    `json:"summary"`
	Materials        []StudyPipelineMaterial `json:"materials"`
	TaskLinks        []StudyPipelineTaskLink `json:"taskLinks"`
	MissingSolutions []StudyPipelineMaterial `json:"missingSolutions"`
}

type StudyPipelineSummary struct {
	TotalResources   int `json:"totalResources"`
	Slides           int `json:"slides"`
	Scripts          int `json:"scripts"`
	Tasks            int `json:"tasks"`
	Solutions        int `json:"solutions"`
	Other            int `json:"other"`
	LinkedSolutions  int `json:"linkedSolutions"`
	MissingSolutions int `json:"missingSolutions"`
}

type StudyPipelineMaterial struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	URL          string `json:"url,omitempty"`
	Type         string `json:"type"`
	ResourceType string `json:"resourceType,omitempty"`
	FileType     string `json:"fileType,omitempty"`
	SectionID    string `json:"sectionId,omitempty"`
	SectionName  string `json:"sectionName,omitempty"`
}

type StudyPipelineTaskLink struct {
	Task     StudyPipelineMaterial  `json:"task"`
	Solution *StudyPipelineMaterial `json:"solution,omitempty"`
	Status   string                 `json:"status"`
}
