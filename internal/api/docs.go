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
