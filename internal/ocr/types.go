package ocr

import (
	"io"
	"time"
)

type Provider struct {
	ID                  string   `json:"id"`
	DisplayName         string   `json:"displayName"`
	Runtime             string   `json:"runtime,omitempty"`
	DockerImage         string   `json:"dockerImage"`
	BuildContext        string   `json:"buildContext,omitempty"`
	Dockerfile          string   `json:"dockerfile,omitempty"`
	SupportsCPU         bool     `json:"supportsCpu"`
	SupportsGPU         bool     `json:"supportsGpu"`
	ExtractsImages      bool     `json:"extractsImages"`
	DefaultTimeoutMs    int      `json:"defaultTimeoutMs"`
	ExpectedOutputFiles []string `json:"expectedOutputFiles"`
	EnabledByDefault    bool     `json:"enabledByDefault"`
}

type Options struct {
	Engine         string
	OutputDir      string
	KeepArtifacts  bool
	Timeout        time.Duration
	DockerPlatform string
	GPU            bool
	Formula        bool
	Code           bool
	Verbose        bool
	LogWriter      io.Writer
}

type ImageArtifact struct {
	Path     string `json:"path"`
	MimeType string `json:"mimeType,omitempty"`
}

type RunResult struct {
	Engine       string          `json:"engine"`
	Status       string          `json:"status"`
	Markdown     string          `json:"markdown,omitempty"`
	HTML         string          `json:"html,omitempty"`
	Text         string          `json:"text,omitempty"`
	JSON         any             `json:"json,omitempty"`
	Images       []ImageArtifact `json:"images"`
	ArtifactsDir string          `json:"artifactsDir"`
	Warnings     []string        `json:"warnings"`
	DurationMs   int64           `json:"durationMs"`
	OutputFiles  []string        `json:"outputFiles,omitempty"`
	ExitCode     int             `json:"exitCode,omitempty"`
}

type ComparisonRow struct {
	Engine             string   `json:"engine"`
	Status             string   `json:"status"`
	DurationMs         int64    `json:"durationMs"`
	OutputFiles        []string `json:"outputFiles"`
	Warnings           []string `json:"warnings"`
	MarkdownCharacters int      `json:"markdownCharacters"`
	ImageCount         int      `json:"imageCount"`
}

type Comparison struct {
	Path    string          `json:"path,omitempty"`
	Results []ComparisonRow `json:"results"`
}

type RunAllResult struct {
	Action     string      `json:"action"`
	CourseID   string      `json:"courseId,omitempty"`
	ResourceID string      `json:"resourceId,omitempty"`
	URL        string      `json:"url,omitempty"`
	FileType   string      `json:"fileType,omitempty"`
	Results    []RunResult `json:"results"`
	Comparison Comparison  `json:"comparison"`
}

type SingleRunResult struct {
	Action     string    `json:"action"`
	CourseID   string    `json:"courseId,omitempty"`
	ResourceID string    `json:"resourceId,omitempty"`
	URL        string    `json:"url,omitempty"`
	FileType   string    `json:"fileType,omitempty"`
	Result     RunResult `json:"result"`
}
