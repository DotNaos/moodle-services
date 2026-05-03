package httpapi

import "encoding/json"

func OpenAPISpecJSON() []byte {
	spec := map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       "Moodle Services API",
			"version":     "0.1.0",
			"description": "Private Moodle Services API for QR login, API keys, Moodle courses, materials, PDFs, calendar data, and ChatGPT MCP support.",
		},
		"servers": []any{
			map[string]any{"url": "https://moodle-services.os-home.net"},
		},
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"bearerAuth": map[string]any{"type": "http", "scheme": "bearer"},
				"apiKey":     map[string]any{"type": "apiKey", "in": "header", "name": "X-Moodle-App-Key"},
			},
			"schemas": schemas(),
		},
		"paths": paths(),
	}
	data, _ := json.MarshalIndent(spec, "", "  ")
	return data
}

func schemas() map[string]any {
	return map[string]any{
		"Error": map[string]any{
			"type":       "object",
			"required":   []string{"error"},
			"properties": map[string]any{"error": map[string]any{"type": "string"}},
		},
		"QRExchangeRequest": map[string]any{
			"type":       "object",
			"required":   []string{"qr"},
			"properties": map[string]any{"qr": map[string]any{"type": "string"}},
		},
		"QRExchangeResponse": map[string]any{
			"type":     "object",
			"required": []string{"user", "apiKey", "apiKeyRecord"},
			"properties": map[string]any{
				"user":         map[string]any{"$ref": "#/components/schemas/User"},
				"apiKey":       map[string]any{"type": "string"},
				"apiKeyRecord": map[string]any{"$ref": "#/components/schemas/APIKey"},
			},
		},
		"CreateAPIKeyRequest": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":           map[string]any{"type": "string"},
				"scopes":         map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"revokeExisting": map[string]any{"type": "boolean"},
			},
		},
		"CreateAPIKeyResponse": map[string]any{
			"type":     "object",
			"required": []string{"apiKey", "apiKeyRecord"},
			"properties": map[string]any{
				"apiKey":          map[string]any{"type": "string"},
				"apiKeyRecord":    map[string]any{"$ref": "#/components/schemas/APIKey"},
				"revokedExisting": map[string]any{"type": "boolean"},
			},
		},
		"User": map[string]any{
			"type":     "object",
			"required": []string{"id", "moodleSiteUrl", "moodleUserId", "displayName"},
			"properties": map[string]any{
				"id":            map[string]any{"type": "string"},
				"moodleSiteUrl": map[string]any{"type": "string"},
				"moodleUserId":  map[string]any{"type": "integer"},
				"displayName":   map[string]any{"type": "string"},
			},
		},
		"APIKey": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":         map[string]any{"type": "string"},
				"name":       map[string]any{"type": "string"},
				"keyPrefix":  map[string]any{"type": "string"},
				"scopes":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"lastUsedAt": map[string]any{"type": "string", "format": "date-time", "nullable": true},
				"revokedAt":  map[string]any{"type": "string", "format": "date-time", "nullable": true},
				"createdAt":  map[string]any{"type": "string", "format": "date-time"},
			},
		},
	}
}

func paths() map[string]any {
	security := []any{map[string]any{"bearerAuth": []any{}}, map[string]any{"apiKey": []any{}}}
	return map[string]any{
		"/api/auth/qr/exchange": map[string]any{
			"post": operation("exchangeQRCode", "Exchange Moodle Mobile QR code", nil, "#/components/schemas/QRExchangeRequest", "#/components/schemas/QRExchangeResponse"),
		},
		"/api/me": map[string]any{
			"get": operation("getMe", "Get current API user", security, "", "#/components/schemas/User"),
		},
		"/api/keys": map[string]any{
			"get":  operation("listAPIKeys", "List API keys", security, "", "#/components/schemas/APIKey"),
			"post": operation("createAPIKey", "Create API key", security, "#/components/schemas/CreateAPIKeyRequest", "#/components/schemas/CreateAPIKeyResponse"),
		},
		"/api/courses": map[string]any{
			"get": operation("listCourses", "List Moodle courses", security, "", ""),
		},
		"/api/courses/{courseId}/materials": map[string]any{
			"get": operationWithParams("listCourseMaterials", "List course materials", security, []any{pathParam("courseId")}),
		},
		"/api/courses/{courseId}/materials/{resourceId}/text": map[string]any{
			"get": operationWithParams("readMaterialText", "Read material text", security, []any{pathParam("courseId"), pathParam("resourceId")}),
		},
		"/api/courses/{courseId}/materials/{resourceId}/pdf": map[string]any{
			"get": operationWithParams("readMaterialPDF", "Read material PDF", security, []any{pathParam("courseId"), pathParam("resourceId")}),
		},
		"/api/search": map[string]any{
			"get": operationWithParams("searchMoodle", "Search Moodle", security, []any{queryParam("q")}),
		},
		"/api/openapi.json": map[string]any{
			"get": operation("getOpenAPISpec", "Get OpenAPI spec", nil, "", ""),
		},
	}
}

func operation(operationID string, summary string, security []any, requestSchema string, responseSchema string) map[string]any {
	op := map[string]any{
		"operationId": operationID,
		"summary":     summary,
		"responses":   responses(responseSchema),
	}
	if security != nil {
		op["security"] = security
	}
	if requestSchema != "" {
		op["requestBody"] = map[string]any{
			"required": true,
			"content":  map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": requestSchema}}},
		}
	}
	return op
}

func operationWithParams(operationID string, summary string, security []any, params []any) map[string]any {
	op := operation(operationID, summary, security, "", "")
	op["parameters"] = params
	return op
}

func responses(schema string) map[string]any {
	okSchema := map[string]any{"type": "object"}
	if schema != "" {
		if schema == "#/components/schemas/APIKey" {
			okSchema = map[string]any{"type": "array", "items": map[string]any{"$ref": schema}}
		} else {
			okSchema = map[string]any{"$ref": schema}
		}
	}
	return map[string]any{
		"200": map[string]any{"description": "OK", "content": map[string]any{"application/json": map[string]any{"schema": okSchema}}},
		"400": errorResponse(),
		"401": errorResponse(),
		"500": errorResponse(),
	}
}

func errorResponse() map[string]any {
	return map[string]any{"description": "Error", "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/Error"}}}}
}

func pathParam(name string) map[string]any {
	return map[string]any{"name": name, "in": "path", "required": true, "schema": map[string]any{"type": "string"}}
}

func queryParam(name string) map[string]any {
	return map[string]any{"name": name, "in": "query", "required": false, "schema": map[string]any{"type": "string"}}
}
