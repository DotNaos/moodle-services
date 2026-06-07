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
	Workspace string                `json:"workspace"`
	Term      string                `json:"term,omitempty"`
	Summary   StudyPipelineSummary  `json:"summary"`
	Courses   []StudyPipelineCourse `json:"courses"`
}

type StudyPipelineSummary struct {
	Courses        int `json:"courses"`
	Complete       int `json:"complete"`
	Partial        int `json:"partial"`
	Missing        int `json:"missing"`
	RawMaterials   int `json:"rawMaterials"`
	ExtractedFiles int `json:"extractedFiles"`
	CuratedFiles   int `json:"curatedFiles"`
}

type StudyPipelineCourse struct {
	Term         string                      `json:"term"`
	Slug         string                      `json:"slug"`
	Title        string                      `json:"title"`
	Path         string                      `json:"path"`
	Status       string                      `json:"status"`
	UpdatedAt    string                      `json:"updatedAt,omitempty"`
	Raw          StudyPipelineRawStage       `json:"raw"`
	Extracted    StudyPipelineExtractedStage `json:"extracted"`
	Curated      StudyPipelineCuratedStage   `json:"curated"`
	Reader       StudyPipelineReaderStatus   `json:"reader"`
	QualityGates []StudyPipelineQualityGate  `json:"qualityGates"`
	Issues       []string                    `json:"issues,omitempty"`
}

type StudyPipelineRawStage struct {
	Status       string                  `json:"status"`
	MoodleMD     StudyPipelineFileStatus `json:"moodleMd"`
	MaterialsYML StudyPipelineFileStatus `json:"materialsYaml"`
	Materials    StudyPipelineFileCount  `json:"materials"`
}

type StudyPipelineExtractedStage struct {
	Status    string                  `json:"status"`
	Script    StudyPipelineFileStatus `json:"script"`
	Slides    StudyPipelineFileCount  `json:"slides"`
	Tasks     StudyPipelineFileCount  `json:"tasks"`
	Solutions StudyPipelineFileCount  `json:"solutions"`
	Assets    int                     `json:"assets"`
}

type StudyPipelineCuratedStage struct {
	Status         string                  `json:"status"`
	Script         StudyPipelineFileStatus `json:"script"`
	Tasks          StudyPipelineFileCount  `json:"tasks"`
	Solutions      StudyPipelineFileCount  `json:"solutions"`
	SolutionStates map[string]int          `json:"solutionStates,omitempty"`
	StaleFiles     []string                `json:"staleFiles,omitempty"`
}

type StudyPipelineReaderStatus struct {
	Supported bool   `json:"supported"`
	URL       string `json:"url,omitempty"`
}

type StudyPipelineQualityGate struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Passed bool   `json:"passed"`
}

type StudyPipelineFileStatus struct {
	Path      string `json:"path"`
	Exists    bool   `json:"exists"`
	SizeBytes int64  `json:"sizeBytes,omitempty"`
	ModTime   string `json:"modTime,omitempty"`
}

type StudyPipelineFileCount struct {
	Files int   `json:"files"`
	Bytes int64 `json:"bytes,omitempty"`
}
