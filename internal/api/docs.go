package api

import (
	"html/template"
	"net/http"
	"strings"

	ver "github.com/DotNaos/moodle-services/internal/version"
)

const (
	openAPIPath = "/openapi.json"
	scalarPath  = "/scalar"
	docsPath    = "/docs"
)

var scalarPageTemplate = template.Must(template.New("scalar").Parse(`<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Moodle Services API Reference</title>
    <style>
      html, body, #app {
        height: 100%;
      }

      body {
        margin: 0;
      }
    </style>
  </head>
  <body>
    <div id="app"></div>
    <noscript>This API reference needs JavaScript enabled.</noscript>
    <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
    <script>
      Scalar.createApiReference('#app', {
        url: {{ .OpenAPIPath }},
        title: 'Moodle Services API',
      })
    </script>
  </body>
</html>
`))

func openAPIHandler(opts ServerOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, openAPIDocument(r, opts))
	}
}

func scalarHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := scalarPageTemplate.Execute(w, map[string]string{
			"OpenAPIPath": openAPIPath,
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func openAPIDocument(r *http.Request, opts ServerOptions) map[string]any {
	paths := map[string]any{
		"/healthz": map[string]any{
			"get": map[string]any{
				"summary":     "Check server health",
				"description": "Validates that the saved Moodle session is still usable.",
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Healthy server",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"$ref": "#/components/schemas/HealthStatus",
								},
							},
						},
					},
					"401": errorResponse("Saved session expired"),
					"500": errorResponse("Server bootstrap error"),
					"502": errorResponse("Moodle validation failed"),
				},
			},
		},
		"/api/courses": map[string]any{
			"get": map[string]any{
				"summary":     "List courses",
				"description": "Returns the courses visible to the authenticated Moodle user.",
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Courses list",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type":  "array",
									"items": map[string]any{"$ref": "#/components/schemas/Course"},
								},
							},
						},
					},
					"500": errorResponse("Server bootstrap error"),
					"502": errorResponse("Moodle fetch failed"),
				},
			},
		},
		"/api/categories": map[string]any{
			"get": map[string]any{
				"summary":     "List course categories",
				"description": "Returns normalized Moodle course categories visible through the mobile web service.",
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Categories list",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type":  "array",
									"items": map[string]any{"$ref": "#/components/schemas/Category"},
								},
							},
						},
					},
					"500": errorResponse("Server bootstrap error"),
					"501": errorResponse("Client does not support categories"),
					"502": errorResponse("Moodle fetch failed"),
				},
			},
		},
		"/api/courses/{courseID}/resources": map[string]any{
			"get": map[string]any{
				"summary":     "List course resources",
				"description": "Returns Moodle files and folders for one course.",
				"parameters": []map[string]any{
					{
						"name":        "courseID",
						"in":          "path",
						"required":    true,
						"description": "Moodle course id.",
						"schema": map[string]any{
							"type": "string",
						},
					},
				},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Course resources",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type":  "array",
									"items": map[string]any{"$ref": "#/components/schemas/Resource"},
								},
							},
						},
					},
					"400": errorResponse("Missing course id"),
					"500": errorResponse("Server bootstrap error"),
					"502": errorResponse("Moodle fetch failed"),
				},
			},
		},
		"/api/courses/{courseID}/study-pipeline": map[string]any{
			"get": map[string]any{
				"summary":     "Get course study material plan",
				"description": "Returns a study material plan built from the selected Moodle course resources.",
				"parameters": []map[string]any{
					{
						"name":        "courseID",
						"in":          "path",
						"required":    true,
						"description": "Moodle course id.",
						"schema":      map[string]any{"type": "string"},
					},
				},
				"responses": map[string]any{
					"200": jsonResponse("Study material plan", "#/components/schemas/StudyPipelineResponse"),
					"400": errorResponse("Missing course id"),
					"500": errorResponse("Server bootstrap error"),
					"502": errorResponse("Moodle fetch failed"),
				},
			},
			"post": map[string]any{
				"summary":     "Create course study material plan",
				"description": "Creates a fresh study material plan from the selected Moodle course resources.",
				"parameters": []map[string]any{
					{
						"name":        "courseID",
						"in":          "path",
						"required":    true,
						"description": "Moodle course id.",
						"schema":      map[string]any{"type": "string"},
					},
				},
				"responses": map[string]any{
					"200": jsonResponse("Created study material plan", "#/components/schemas/StudyPipelineResponse"),
					"400": errorResponse("Missing course id"),
					"500": errorResponse("Server bootstrap error"),
					"502": errorResponse("Moodle fetch failed"),
				},
			},
		},
		"/api/courses/{courseID}/study-pipeline/inventory": map[string]any{
			"get": map[string]any{
				"summary":     "Inspect course inventory mapping",
				"description": "Returns the first course inventory mapping with lecture material, task groups, references, interactions, and unknown resources.",
				"parameters": []map[string]any{
					{
						"name":        "courseID",
						"in":          "path",
						"required":    true,
						"description": "Moodle course id.",
						"schema":      map[string]any{"type": "string"},
					},
				},
				"responses": map[string]any{
					"200": jsonResponse("Course inventory mapping", "#/components/schemas/CourseInventoryResponse"),
					"400": errorResponse("Missing course id"),
					"500": errorResponse("Server bootstrap error"),
					"502": errorResponse("Moodle fetch failed"),
				},
			},
		},
		"/api/courses/{courseID}/study-pipeline/extracted-documents": map[string]any{
			"get": map[string]any{
				"summary":     "Inspect extracted document structure",
				"description": "Returns the machine-extracted document structure with pages, blocks, assets, and diagnostics before Codex curation.",
				"parameters": []map[string]any{
					{
						"name":        "courseID",
						"in":          "path",
						"required":    true,
						"description": "Moodle course id.",
						"schema":      map[string]any{"type": "string"},
					},
				},
				"responses": map[string]any{
					"200": jsonResponse("Extracted document structure", "#/components/schemas/ExtractedDocumentsResponse"),
					"400": errorResponse("Missing course id"),
					"500": errorResponse("Server bootstrap error or extraction failed"),
					"502": errorResponse("Moodle fetch failed"),
				},
			},
		},
	}

	for _, route := range opts.CommandRoutes {
		paths[route.APIPath] = openAPICommandPath(route)
	}

	return map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":       "Moodle Services API",
			"version":     ver.Version(),
			"description": "Local HTTP API exposed by `moodle serve`.",
		},
		"servers": []map[string]string{
			{"url": requestBaseURL(r)},
		},
		"paths": paths,
		"components": map[string]any{
			"schemas": map[string]any{
				"CommandRequest": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"arguments": map[string]any{
							"type": "array",
							"items": map[string]any{
								"type": "string",
							},
							"description": "Remaining CLI arguments and flags. This is only used by internal command-backed routes that explicitly accept a request body.",
							"example":     []string{"current"},
						},
					},
				},
				"CommandResponse": map[string]any{
					"type":                 "object",
					"additionalProperties": true,
					"description":          "Machine-readable JSON payload returned by the matching CLI command.",
				},
				"HealthStatus": map[string]any{
					"type": "object",
					"required": []string{
						"status",
					},
					"properties": map[string]any{
						"status": map[string]any{
							"type":    "string",
							"example": "ok",
						},
					},
				},
				"Course": map[string]any{
					"type": "object",
					"required": []string{
						"id",
						"fullname",
						"shortname",
						"category",
						"viewUrl",
						"heroImage",
					},
					"properties": map[string]any{
						"id": map[string]any{
							"type":    "integer",
							"example": 18236,
						},
						"fullname": map[string]any{
							"type":    "string",
							"example": "Software Engineering",
						},
						"shortname": map[string]any{
							"type":    "string",
							"example": "SE",
						},
						"category": map[string]any{
							"type":    "string",
							"example": "FS26",
						},
						"categoryId": map[string]any{
							"type":    "integer",
							"example": 1885,
						},
						"viewUrl": map[string]any{
							"type":    "string",
							"format":  "uri",
							"example": "https://moodle.example.edu/course/view.php?id=18236",
						},
						"heroImage": map[string]any{
							"type":    "string",
							"format":  "uri",
							"example": "https://moodle.example.edu/pluginfile.php/123/course/overviewfiles/banner.jpg",
						},
					},
				},
				"Category": map[string]any{
					"type":     "object",
					"required": []string{"id", "name"},
					"properties": map[string]any{
						"id": map[string]any{
							"type":    "integer",
							"example": 1885,
						},
						"name": map[string]any{
							"type":    "string",
							"example": "FS26",
						},
						"idNumber": map[string]any{
							"type":    "string",
							"example": "",
						},
						"parentId": map[string]any{
							"type":    "integer",
							"example": 1157,
						},
						"path": map[string]any{
							"type":    "string",
							"example": "/7/1157/1885",
						},
						"depth": map[string]any{
							"type":    "integer",
							"example": 3,
						},
					},
				},
				"Resource": map[string]any{
					"type": "object",
					"required": []string{
						"id",
						"name",
						"url",
						"type",
						"courseId",
					},
					"properties": map[string]any{
						"id": map[string]any{
							"type":    "string",
							"example": "42",
						},
						"name": map[string]any{
							"type":    "string",
							"example": "Lecture Slides",
						},
						"url": map[string]any{
							"type":    "string",
							"format":  "uri",
							"example": "https://moodle.example.edu/mod/resource/view.php?id=42&redirect=1",
						},
						"type": map[string]any{
							"type": "string",
							"enum": []string{"resource", "folder"},
						},
						"courseId": map[string]any{
							"type":    "string",
							"example": "18236",
						},
						"sectionId": map[string]any{
							"type":    "string",
							"example": "7",
						},
						"sectionName": map[string]any{
							"type":    "string",
							"example": "Week 3",
						},
						"fileType": map[string]any{
							"type":    "string",
							"example": "pdf",
						},
						"uploadedAt": map[string]any{
							"type":    "string",
							"format":  "date-time",
							"example": "2026-04-09T08:15:00Z",
						},
					},
				},
				"StudyPipelineResponse": map[string]any{
					"type":     "object",
					"required": []string{"courseId", "status", "createdAt", "summary", "materials", "taskLinks", "missingSolutions"},
					"properties": map[string]any{
						"courseId":  map[string]any{"type": "string", "example": "22584"},
						"status":    map[string]any{"type": "string", "enum": []string{"planned", "created"}},
						"createdAt": map[string]any{"type": "string", "format": "date-time"},
						"summary":   map[string]any{"$ref": "#/components/schemas/StudyPipelineSummary"},
						"materials": map[string]any{
							"type":  "array",
							"items": map[string]any{"$ref": "#/components/schemas/StudyPipelineMaterial"},
						},
						"taskLinks": map[string]any{
							"type":  "array",
							"items": map[string]any{"$ref": "#/components/schemas/StudyPipelineTaskLink"},
						},
						"missingSolutions": map[string]any{
							"type":  "array",
							"items": map[string]any{"$ref": "#/components/schemas/StudyPipelineMaterial"},
						},
					},
				},
				"StudyPipelineSummary": map[string]any{
					"type":     "object",
					"required": []string{"totalResources", "slides", "scripts", "tasks", "solutions", "other", "linkedSolutions", "missingSolutions"},
					"properties": map[string]any{
						"totalResources":   map[string]any{"type": "integer"},
						"slides":           map[string]any{"type": "integer"},
						"scripts":          map[string]any{"type": "integer"},
						"tasks":            map[string]any{"type": "integer"},
						"solutions":        map[string]any{"type": "integer"},
						"other":            map[string]any{"type": "integer"},
						"linkedSolutions":  map[string]any{"type": "integer"},
						"missingSolutions": map[string]any{"type": "integer"},
					},
				},
				"StudyPipelineMaterial": map[string]any{
					"type":     "object",
					"required": []string{"id", "name", "type"},
					"properties": map[string]any{
						"id":           map[string]any{"type": "string"},
						"name":         map[string]any{"type": "string"},
						"url":          map[string]any{"type": "string"},
						"type":         map[string]any{"type": "string", "enum": []string{"slide", "script", "task", "solution", "other"}},
						"resourceType": map[string]any{"type": "string"},
						"fileType":     map[string]any{"type": "string"},
						"sectionId":    map[string]any{"type": "string"},
						"sectionName":  map[string]any{"type": "string"},
					},
				},
				"StudyPipelineTaskLink": map[string]any{
					"type":     "object",
					"required": []string{"task", "status"},
					"properties": map[string]any{
						"task":     map[string]any{"$ref": "#/components/schemas/StudyPipelineMaterial"},
						"solution": map[string]any{"$ref": "#/components/schemas/StudyPipelineMaterial"},
						"status":   map[string]any{"type": "string", "enum": []string{"linked", "missing-solution"}},
					},
				},
				"CourseInventoryResponse": map[string]any{
					"type":     "object",
					"required": []string{"courseId", "generatedAt", "summary", "lectureMaterial", "taskGroups", "references", "interactions", "unknown"},
					"properties": map[string]any{
						"courseId":     map[string]any{"type": "string", "example": "22584"},
						"generatedAt":  map[string]any{"type": "string", "format": "date-time"},
						"artifactRoot": map[string]any{"type": "string"},
						"summary":      map[string]any{"$ref": "#/components/schemas/CourseInventorySummary"},
						"lectureMaterial": map[string]any{
							"type":  "array",
							"items": map[string]any{"$ref": "#/components/schemas/CourseInventoryNode"},
						},
						"taskGroups": map[string]any{
							"type":  "array",
							"items": map[string]any{"$ref": "#/components/schemas/CourseInventoryTaskGroup"},
						},
						"references": map[string]any{
							"type":  "array",
							"items": map[string]any{"$ref": "#/components/schemas/CourseInventoryNode"},
						},
						"interactions": map[string]any{
							"type":  "array",
							"items": map[string]any{"$ref": "#/components/schemas/CourseInventoryNode"},
						},
						"unknown": map[string]any{
							"type":  "array",
							"items": map[string]any{"$ref": "#/components/schemas/CourseInventoryNode"},
						},
					},
				},
				"CourseInventorySummary": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"totalResources":        map[string]any{"type": "integer"},
						"lectureMaterial":       map[string]any{"type": "integer"},
						"taskGroups":            map[string]any{"type": "integer"},
						"pairedTaskGroups":      map[string]any{"type": "integer"},
						"missingSolutionGroups": map[string]any{"type": "integer"},
						"ambiguousTaskGroups":   map[string]any{"type": "integer"},
						"references":            map[string]any{"type": "integer"},
						"interactions":          map[string]any{"type": "integer"},
						"unknown":               map[string]any{"type": "integer"},
					},
				},
				"CourseInventoryNode": map[string]any{
					"type":     "object",
					"required": []string{"id", "name", "type", "bucket", "role", "reason", "confidence"},
					"properties": map[string]any{
						"id":           map[string]any{"type": "string"},
						"name":         map[string]any{"type": "string"},
						"url":          map[string]any{"type": "string"},
						"type":         map[string]any{"type": "string"},
						"resourceType": map[string]any{"type": "string"},
						"fileType":     map[string]any{"type": "string"},
						"sectionId":    map[string]any{"type": "string"},
						"sectionName":  map[string]any{"type": "string"},
						"bucket":       map[string]any{"type": "string", "enum": []string{"lecture_material", "task_group", "reference", "interaction", "unknown"}},
						"role":         map[string]any{"type": "string"},
						"reason":       map[string]any{"type": "string"},
						"confidence":   map[string]any{"type": "string", "enum": []string{"high", "medium", "low"}},
					},
				},
				"CourseInventoryTaskGroup": map[string]any{
					"type":     "object",
					"required": []string{"id", "title", "sheet", "pairingStatus", "pairingReason", "pairingConfidence"},
					"properties": map[string]any{
						"id":                 map[string]any{"type": "string"},
						"title":              map[string]any{"type": "string"},
						"sheet":              map[string]any{"$ref": "#/components/schemas/CourseInventoryNode"},
						"solution":           map[string]any{"$ref": "#/components/schemas/CourseInventoryNode"},
						"solutionCandidates": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/CourseInventoryNode"}},
						"pairingStatus":      map[string]any{"type": "string", "enum": []string{"paired", "missing_solution", "ambiguous_solution"}},
						"pairingReason":      map[string]any{"type": "string"},
						"pairingConfidence":  map[string]any{"type": "string", "enum": []string{"high", "medium", "low"}},
					},
				},
				"ExtractedDocumentsResponse": map[string]any{
					"type":     "object",
					"required": []string{"courseId", "runId", "generatedAt", "engine", "summary", "documents", "diagnostics"},
					"properties": map[string]any{
						"courseId":     map[string]any{"type": "string", "example": "22584"},
						"runId":        map[string]any{"type": "string", "example": "baseline-20260612T112400Z"},
						"generatedAt":  map[string]any{"type": "string", "format": "date-time"},
						"engine":       map[string]any{"type": "string"},
						"artifactRoot": map[string]any{"type": "string"},
						"summary":      map[string]any{"$ref": "#/components/schemas/ExtractedDocumentsSummary"},
						"documents":    map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/PDFDocument"}},
						"diagnostics":  map[string]any{"$ref": "#/components/schemas/ExtractedDocumentDiagnostics"},
					},
				},
				"ExtractedDocumentsSummary": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"totalDocuments":      map[string]any{"type": "integer"},
						"totalPages":          map[string]any{"type": "integer"},
						"totalBlocks":         map[string]any{"type": "integer"},
						"pagePreviewAssets":   map[string]any{"type": "integer"},
						"embeddedImageAssets": map[string]any{"type": "integer"},
						"pagesMissingText":    map[string]any{"type": "integer"},
						"visualOnlyPages":     map[string]any{"type": "integer"},
						"unknownBlocks":       map[string]any{"type": "integer"},
					},
				},
				"PDFDocument": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":            map[string]any{"type": "string"},
						"resource":      map[string]any{"$ref": "#/components/schemas/StudyPipelineMaterial"},
						"runId":         map[string]any{"type": "string"},
						"engine":        map[string]any{"type": "string"},
						"status":        map[string]any{"type": "string"},
						"sourcePath":    map[string]any{"type": "string"},
						"extractedPath": map[string]any{"type": "string"},
						"pages":         map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/PDFPage"}},
						"assets":        map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/DocumentAsset"}},
						"diagnostics":   map[string]any{"$ref": "#/components/schemas/ExtractedDocumentDiagnostics"},
					},
				},
				"PDFPage": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":             map[string]any{"type": "string"},
						"pageNumber":     map[string]any{"type": "integer"},
						"text":           map[string]any{"type": "string"},
						"markdown":       map[string]any{"type": "string"},
						"previewAssetId": map[string]any{"type": "string"},
						"blocks":         map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/DocumentBlock"}},
						"diagnostics":    map[string]any{"$ref": "#/components/schemas/ExtractedDocumentDiagnostics"},
					},
				},
				"DocumentBlock": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":         map[string]any{"type": "string"},
						"pageNumber": map[string]any{"type": "integer"},
						"type":       map[string]any{"type": "string", "enum": []string{"heading", "paragraph", "list", "table", "image", "formula", "code", "page_header", "page_footer", "caption", "unknown"}},
						"label":      map[string]any{"type": "string"},
						"text":       map[string]any{"type": "string"},
						"markdown":   map[string]any{"type": "string"},
						"assetId":    map[string]any{"type": "string"},
						"source":     map[string]any{"type": "string"},
						"confidence": map[string]any{"type": "string", "enum": []string{"high", "medium", "low"}},
					},
				},
				"DocumentAsset": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":         map[string]any{"type": "string"},
						"kind":       map[string]any{"type": "string", "enum": []string{"page_preview", "embedded_image"}},
						"path":       map[string]any{"type": "string"},
						"pageNumber": map[string]any{"type": "integer"},
						"mimeType":   map[string]any{"type": "string"},
						"role":       map[string]any{"type": "string"},
					},
				},
				"ExtractedDocumentDiagnostics": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"pagesMissingText":     map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
						"visualOnlyPages":      map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
						"extractedImageAssets": map[string]any{"type": "integer"},
						"unusedImageAssets":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"unknownBlocks":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"warnings":             map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					},
				},
				"Error": map[string]any{
					"type": "object",
					"required": []string{
						"error",
					},
					"properties": map[string]any{
						"error": map[string]any{
							"type":    "string",
							"example": "session expired",
						},
					},
				},
			},
		},
	}
}

func openAPICommandPath(route CommandRoute) map[string]any {
	description := strings.TrimSpace(route.Description)
	if description == "" {
		description = strings.TrimSpace(route.Summary)
	}
	if route.CommandPath != nil {
		if description != "" {
			description += "\n\n"
		}
		description += "Backed by `moodle --json " + strings.Join(route.CommandPath, " ") + "` and returns the command's machine-readable output."
	}

	responses := map[string]any{
		"200": map[string]any{
			"description": "Command completed successfully",
		},
		"400": errorResponse("Command rejected the provided arguments"),
		"500": errorResponse("Command failed"),
	}

	if route.Stream {
		responses["200"] = map[string]any{
			"description": "Streaming command output",
			"content": map[string]any{
				"application/x-ndjson": map[string]any{
					"schema": map[string]any{
						"type": "string",
					},
				},
			},
		}
	} else {
		responses["200"] = map[string]any{
			"description": "Command completed successfully",
			"content": map[string]any{
				"application/json": map[string]any{
					"schema": map[string]any{
						"$ref": "#/components/schemas/CommandResponse",
					},
				},
			},
		}
	}

	operation := map[string]any{
		"summary":     route.Summary,
		"description": description,
		"responses":   responses,
	}

	if strings.EqualFold(route.Method, http.MethodPost) {
		operation["requestBody"] = map[string]any{
			"required": false,
			"content": map[string]any{
				"application/json": map[string]any{
					"schema": map[string]any{
						"$ref": "#/components/schemas/CommandRequest",
					},
				},
			},
		}
	}

	method := strings.ToLower(strings.TrimSpace(route.Method))
	if method == "" {
		method = "get"
	}

	return map[string]any{
		method: operation,
	}
}

func errorResponse(description string) map[string]any {
	return map[string]any{
		"description": description,
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": map[string]any{
					"$ref": "#/components/schemas/Error",
				},
			},
		},
	}
}

func jsonResponse(description string, schemaRef string) map[string]any {
	return map[string]any{
		"description": description,
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": map[string]any{
					"$ref": schemaRef,
				},
			},
		},
	}
}

func queryParameter(name string, description string) map[string]any {
	return map[string]any{
		"name":        name,
		"in":          "query",
		"required":    false,
		"description": description,
		"schema": map[string]any{
			"type": "string",
		},
	}
}

func requestBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwarded != "" {
		scheme = strings.Split(forwarded, ",")[0]
	}

	host := strings.TrimSpace(r.Host)
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); forwarded != "" {
		host = strings.Split(forwarded, ",")[0]
	}
	if host == "" {
		host = "127.0.0.1:8080"
	}

	return scheme + "://" + strings.TrimSpace(host)
}
