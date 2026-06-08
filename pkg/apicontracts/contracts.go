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
	Stage            string                  `json:"stage,omitempty"`
	CreatedAt        string                  `json:"createdAt"`
	ArtifactRoot     string                  `json:"artifactRoot,omitempty"`
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

type StudyPipelineTaskViewResponse struct {
	CourseID       string                    `json:"courseId"`
	GeneratedAt    string                    `json:"generatedAt"`
	Source         string                    `json:"source"`
	ScriptMarkdown string                    `json:"scriptMarkdown"`
	ScriptSections []StudyPipelineContentRef `json:"scriptSections,omitempty"`
	Sheets         []StudyPipelineTaskSheet  `json:"sheets"`
	Resources      []StudyPipelineViewSource `json:"resources"`
	Progress       StudyPipelineProgress     `json:"progress"`
}

type StudyPipelineTaskSheet struct {
	ResourceID         string                  `json:"resourceId"`
	Title              string                  `json:"title"`
	Kind               string                  `json:"kind"`
	SolutionResourceID string                  `json:"solutionResourceId,omitempty"`
	SolutionTitle      string                  `json:"solutionTitle,omitempty"`
	SolutionMarkdown   string                  `json:"solutionMarkdown,omitempty"`
	Tasks              []StudyPipelineTaskItem `json:"tasks"`
}

type StudyPipelineTaskItem struct {
	TaskID           string                  `json:"taskId"`
	SourceResourceID string                  `json:"sourceResourceId"`
	Title            string                  `json:"title"`
	PromptMarkdown   string                  `json:"promptMarkdown"`
	ContentState     StudyPipelineContentRef `json:"contentState"`
	Parts            []StudyPipelineTaskPart `json:"parts"`
	LatestAttempt    *StudyPipelineAttempt   `json:"latestAttempt,omitempty"`
	Status           string                  `json:"status"`
}

type StudyPipelineContentRef struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	StatusLabel string `json:"statusLabel"`
	Model       string `json:"model,omitempty"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
	SourcePath  string `json:"sourcePath,omitempty"`
}

type StudyPipelineRefineRequest struct {
	Kind     string `json:"kind"`
	TargetID string `json:"targetId"`
}

type StudyPipelineRefineResponse struct {
	CourseID       string                  `json:"courseId"`
	Target         StudyPipelineContentRef `json:"target"`
	ContentPreview string                  `json:"contentPreview,omitempty"`
}

type StudyPipelineTaskPart struct {
	ID             string `json:"id"`
	Label          string `json:"label,omitempty"`
	PromptMarkdown string `json:"promptMarkdown"`
}

type StudyPipelineAttempt struct {
	UserAnswer string               `json:"userAnswer"`
	Verdict    StudyPipelineVerdict `json:"verdict"`
}

type StudyPipelineVerdict struct {
	IsCorrect         bool     `json:"isCorrect"`
	FeedbackMarkdown  string   `json:"feedbackMarkdown"`
	Mistakes          []string `json:"mistakes"`
	SuggestedNextStep string   `json:"suggestedNextStep,omitempty"`
}

type StudyPipelineViewSource struct {
	ResourceID string `json:"resourceId"`
	Title      string `json:"title"`
	Kind       string `json:"kind"`
}

type StudyPipelineProgress struct {
	Open        int `json:"open"`
	Checked     int `json:"checked"`
	Correct     int `json:"correct"`
	Wrong       int `json:"wrong"`
	NeedsReview int `json:"needsReview"`
}
