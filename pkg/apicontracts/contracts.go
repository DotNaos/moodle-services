package apicontracts

import (
	"encoding/json"

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

type CalendarSubscriptionRequest struct {
	URL string `json:"url"`
}

type CalendarSubscriptionResponse struct {
	Configured bool   `json:"configured"`
	URLHint    string `json:"urlHint,omitempty"`
	Saved      bool   `json:"saved,omitempty"`
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
	CourseID          string                                `json:"courseId"`
	Status            string                                `json:"status"`
	Stage             string                                `json:"stage,omitempty"`
	CreatedAt         string                                `json:"createdAt"`
	ArtifactRoot      string                                `json:"artifactRoot,omitempty"`
	Engine            string                                `json:"engine,omitempty"`
	ConfigHash        string                                `json:"configHash,omitempty"`
	Run               *store.StudyPipelineRunRecord         `json:"run,omitempty"`
	ArtifactRefs      []store.StudyPipelineArtifactRef      `json:"artifactRefs,omitempty"`
	CurationChecklist *store.StudyPipelineCurationChecklist `json:"curationChecklist,omitempty"`
	ElementDecisions  []store.StudyPipelineElementDecision  `json:"elementDecisions,omitempty"`
	Summary           StudyPipelineSummary                  `json:"summary"`
	Materials         []StudyPipelineMaterial               `json:"materials"`
	TaskLinks         []StudyPipelineTaskLink               `json:"taskLinks"`
	MissingSolutions  []StudyPipelineMaterial               `json:"missingSolutions"`
}

type StudyPipelineRunsResponse struct {
	CourseID         string                           `json:"courseId"`
	Runs             []store.StudyPipelineRunRecord   `json:"runs"`
	ActiveSelections []store.ActiveRunSelectionRecord `json:"activeSelections"`
}

type StudyPipelineSelectRunRequest struct {
	Reason string `json:"reason,omitempty"`
}

type StudyPipelineModerationRequest struct {
	Reason string `json:"reason,omitempty"`
}

type StudyPipelineStageRequest struct {
	Engine     string `json:"engine,omitempty"`
	ConfigHash string `json:"configHash,omitempty"`
}

type StudyPipelineSelectRunResponse struct {
	Selection store.ActiveRunSelectionRecord `json:"selection"`
}

type StudyPipelinePublishRunResponse struct {
	Selection *store.ActiveRunSelectionRecord `json:"selection,omitempty"`
	Audit     store.StudyPipelineAuditRecord  `json:"audit"`
}

type StudyPipelineFeedbackRequest struct {
	TargetID         string `json:"targetId"`
	TargetKind       string `json:"targetKind"`
	FeedbackType     string `json:"feedbackType"`
	Message          string `json:"message,omitempty"`
	SourceRunID      string `json:"sourceRunId,omitempty"`
	SourceArtifactID string `json:"sourceArtifactId,omitempty"`
}

type StudyPipelineFeedbackResponse struct {
	Feedback store.StudyPipelineFeedbackRecord `json:"feedback"`
}

type StudyPipelineFeedbackModerationResponse struct {
	Feedback store.StudyPipelineFeedbackRecord `json:"feedback"`
	Audit    store.StudyPipelineAuditRecord    `json:"audit"`
}

type StudyPipelineProposalRequest struct {
	TargetID         string `json:"targetId"`
	TargetKind       string `json:"targetKind"`
	Title            string `json:"title,omitempty"`
	ContentPreview   string `json:"contentPreview,omitempty"`
	SourceRunID      string `json:"sourceRunId,omitempty"`
	SourceArtifactID string `json:"sourceArtifactId,omitempty"`
	Model            string `json:"model,omitempty"`
}

type StudyPipelineProposalResponse struct {
	Proposal store.StudyPipelineProposalRecord `json:"proposal"`
}

type StudyPipelineProposalModerationResponse struct {
	Proposal store.StudyPipelineProposalRecord `json:"proposal"`
	Audit    store.StudyPipelineAuditRecord    `json:"audit"`
}

type StudyPipelineReviewResponse struct {
	CourseID  string                              `json:"courseId"`
	Feedback  []store.StudyPipelineFeedbackRecord `json:"feedback"`
	Proposals []store.StudyPipelineProposalRecord `json:"proposals"`
	Audit     []store.StudyPipelineAuditRecord    `json:"audit"`
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

type CourseInventoryResponse struct {
	CourseID        string                     `json:"courseId"`
	GeneratedAt     string                     `json:"generatedAt"`
	ArtifactRoot    string                     `json:"artifactRoot,omitempty"`
	Summary         CourseInventorySummary     `json:"summary"`
	LectureMaterial []CourseInventoryNode      `json:"lectureMaterial"`
	TaskGroups      []CourseInventoryTaskGroup `json:"taskGroups"`
	References      []CourseInventoryNode      `json:"references"`
	Interactions    []CourseInventoryNode      `json:"interactions"`
	Unknown         []CourseInventoryNode      `json:"unknown"`
}

type CourseInventorySummary struct {
	TotalResources        int `json:"totalResources"`
	LectureMaterial       int `json:"lectureMaterial"`
	TaskGroups            int `json:"taskGroups"`
	PairedTaskGroups      int `json:"pairedTaskGroups"`
	MissingSolutionGroups int `json:"missingSolutionGroups"`
	AmbiguousTaskGroups   int `json:"ambiguousTaskGroups"`
	References            int `json:"references"`
	Interactions          int `json:"interactions"`
	Unknown               int `json:"unknown"`
}

type CourseInventoryNode struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	URL          string `json:"url,omitempty"`
	Type         string `json:"type"`
	ResourceType string `json:"resourceType,omitempty"`
	FileType     string `json:"fileType,omitempty"`
	SectionID    string `json:"sectionId,omitempty"`
	SectionName  string `json:"sectionName,omitempty"`
	Bucket       string `json:"bucket"`
	Role         string `json:"role"`
	Reason       string `json:"reason"`
	Confidence   string `json:"confidence"`
}

type CourseInventoryTaskGroup struct {
	ID                 string                `json:"id"`
	Title              string                `json:"title"`
	Sheet              CourseInventoryNode   `json:"sheet"`
	Solution           *CourseInventoryNode  `json:"solution,omitempty"`
	SolutionCandidates []CourseInventoryNode `json:"solutionCandidates,omitempty"`
	PairingStatus      string                `json:"pairingStatus"`
	PairingReason      string                `json:"pairingReason"`
	PairingConfidence  string                `json:"pairingConfidence"`
}

type ExtractedDocumentsResponse struct {
	CourseID     string                       `json:"courseId"`
	RunID        string                       `json:"runId"`
	GeneratedAt  string                       `json:"generatedAt"`
	Engine       string                       `json:"engine"`
	ArtifactRoot string                       `json:"artifactRoot,omitempty"`
	Summary      ExtractedDocumentsSummary    `json:"summary"`
	Documents    []PDFDocument                `json:"documents"`
	Diagnostics  ExtractedDocumentDiagnostics `json:"diagnostics"`
}

type ExtractedDocumentsSummary struct {
	TotalDocuments      int `json:"totalDocuments"`
	TotalPages          int `json:"totalPages"`
	TotalBlocks         int `json:"totalBlocks"`
	PagePreviewAssets   int `json:"pagePreviewAssets"`
	EmbeddedImageAssets int `json:"embeddedImageAssets"`
	PagesMissingText    int `json:"pagesMissingText"`
	VisualOnlyPages     int `json:"visualOnlyPages"`
	UnknownBlocks       int `json:"unknownBlocks"`
}

type PDFDocument struct {
	ID            string                       `json:"id"`
	Resource      StudyPipelineMaterial        `json:"resource"`
	RunID         string                       `json:"runId"`
	Engine        string                       `json:"engine"`
	Status        string                       `json:"status"`
	SourcePath    string                       `json:"sourcePath,omitempty"`
	ExtractedPath string                       `json:"extractedPath,omitempty"`
	Pages         []PDFPage                    `json:"pages"`
	Assets        []DocumentAsset              `json:"assets,omitempty"`
	Diagnostics   ExtractedDocumentDiagnostics `json:"diagnostics"`
}

type PDFPage struct {
	ID             string                       `json:"id"`
	PageNumber     int                          `json:"pageNumber"`
	Text           string                       `json:"text,omitempty"`
	Markdown       string                       `json:"markdown,omitempty"`
	PreviewAssetID string                       `json:"previewAssetId,omitempty"`
	Blocks         []DocumentBlock              `json:"blocks"`
	Diagnostics    ExtractedDocumentDiagnostics `json:"diagnostics"`
}

type DocumentBlock struct {
	ID         string `json:"id"`
	PageNumber int    `json:"pageNumber"`
	Type       string `json:"type"`
	Label      string `json:"label"`
	Text       string `json:"text,omitempty"`
	Markdown   string `json:"markdown,omitempty"`
	AssetID    string `json:"assetId,omitempty"`
	Source     string `json:"source"`
	Confidence string `json:"confidence"`
}

type DocumentAsset struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Path       string `json:"path"`
	PageNumber int    `json:"pageNumber,omitempty"`
	MimeType   string `json:"mimeType,omitempty"`
	Role       string `json:"role,omitempty"`
}

type ExtractedDocumentDiagnostics struct {
	PagesMissingText     []int    `json:"pagesMissingText,omitempty"`
	VisualOnlyPages      []int    `json:"visualOnlyPages,omitempty"`
	ExtractedImageAssets int      `json:"extractedImageAssets,omitempty"`
	UnusedImageAssets    []string `json:"unusedImageAssets,omitempty"`
	UnknownBlocks        []string `json:"unknownBlocks,omitempty"`
	Warnings             []string `json:"warnings,omitempty"`
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
	Kind            string `json:"kind"`
	TargetID        string `json:"targetId"`
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoningEffort,omitempty"`
	CustomPrompt    string `json:"customPrompt,omitempty"`
}

type StudyPipelineRefineResponse struct {
	CourseID       string                  `json:"courseId"`
	Target         StudyPipelineContentRef `json:"target"`
	ContentPreview string                  `json:"contentPreview,omitempty"`
}

type StudyPipelineRefineEvent struct {
	Type            string                   `json:"type"`
	Message         string                   `json:"message,omitempty"`
	Model           string                   `json:"model,omitempty"`
	ReasoningEffort string                   `json:"reasoningEffort,omitempty"`
	Target          *StudyPipelineContentRef `json:"target,omitempty"`
	ContentPreview  string                   `json:"contentPreview,omitempty"`
	Error           string                   `json:"error,omitempty"`
	// Category classifies a Codex stream event as a real "tool" call (shell
	// command, MCP tool, web search) or "status" lifecycle noise. Empty means
	// status. ToolTitle/ToolStatus/ToolID carry the surfaced tool-call details.
	Category   string `json:"category,omitempty"`
	ToolTitle  string `json:"toolTitle,omitempty"`
	ToolStatus string `json:"toolStatus,omitempty"`
	ToolID     string `json:"toolId,omitempty"`
}

type CodexModelCatalogResponse struct {
	Models []CodexModelOption `json:"models"`
}

type CodexModelOption struct {
	ID                     string                 `json:"id"`
	Label                  string                 `json:"label"`
	Description            string                 `json:"description,omitempty"`
	DefaultReasoningEffort string                 `json:"defaultReasoningEffort,omitempty"`
	ReasoningEfforts       []CodexReasoningOption `json:"reasoningEfforts,omitempty"`
	SpeedTiers             []string               `json:"speedTiers,omitempty"`
}

type CodexReasoningOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// CodexWorkspaceFile is one entry in a user's per-user Codex volume, returned
// as a flat list (slash-separated relative paths) for the workspace file tree.
type CodexWorkspaceFile struct {
	Path       string `json:"path"`
	Size       int64  `json:"size"`
	Dir        bool   `json:"dir"`
	ModifiedAt string `json:"modifiedAt,omitempty"`
}

type CodexRunRequest struct {
	Prompt          string          `json:"prompt"`
	Images          []CodexRunImage `json:"images,omitempty"`
	Model           string          `json:"model,omitempty"`
	ReasoningEffort string          `json:"reasoningEffort,omitempty"`
	OutputSchema    json.RawMessage `json:"outputSchema,omitempty"`
	Stream          bool            `json:"stream,omitempty"`
	// AttachmentImages are basenames of uploaded image files (under uploads/)
	// to attach to the Codex prompt via `codex exec -i` for actual vision.
	AttachmentImages []string `json:"attachmentImages,omitempty"`
}

type CodexRunImage struct {
	Name    string `json:"name"`
	DataURL string `json:"dataUrl"`
}

type CodexRunResponse struct {
	ThreadID      *string         `json:"threadId"`
	FinalResponse string          `json:"finalResponse"`
	Actions       []CodexUIAction `json:"actions"`
}

type CodexUIAction struct {
	Type       string   `json:"type"`
	CourseID   *string  `json:"courseId,omitempty"`
	MaterialID *string  `json:"materialId,omitempty"`
	ResourceID *string  `json:"resourceId,omitempty"`
	Page       *float64 `json:"page,omitempty"`
	Reason     *string  `json:"reason,omitempty"`
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
	Done        int `json:"done"`
	Checked     int `json:"checked"`
	Correct     int `json:"correct"`
	Wrong       int `json:"wrong"`
	NeedsReview int `json:"needsReview"`
}

type StudyPipelineTaskStatusRequest struct {
	Status string `json:"status"`
}
