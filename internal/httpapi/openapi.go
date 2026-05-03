package httpapi

import (
	"encoding/json"

	"github.com/DotNaos/moodle-services/internal/moodle"
	"github.com/DotNaos/moodle-services/internal/moodleservice"
	"github.com/DotNaos/moodle-services/internal/store"
	contract "github.com/DotNaos/moodle-services/pkg/apicontracts"
	"github.com/invopop/jsonschema"
)

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
		"Error":                schemaFor[contract.ErrorResponse](),
		"QRExchangeRequest":    schemaFor[contract.QRExchangeRequest](),
		"QRExchangeResponse":   schemaFor[contract.QRExchangeResponse](),
		"CreateAPIKeyRequest":  schemaFor[contract.CreateAPIKeyRequest](),
		"CreateAPIKeyResponse": schemaFor[contract.CreateAPIKeyResponse](),
		"ListAPIKeysResponse":  schemaFor[contract.ListAPIKeysResponse](),
		"RevokeAPIKeyResponse": schemaFor[contract.RevokeAPIKeyResponse](),
		"User":                 schemaFor[store.User](),
		"APIKey":               schemaFor[store.APIKeyRecord](),
		"Course":               schemaFor[moodle.Course](),
		"CoursesResponse":      schemaFor[contract.CoursesResponse](),
		"Category":             schemaFor[moodle.Category](),
		"CategoriesResponse":   schemaFor[contract.CategoriesResponse](),
		"Resource":             schemaFor[moodle.Resource](),
		"MaterialsResponse":    schemaFor[contract.MaterialsResponse](),
		"FetchDocument":        schemaFor[moodleservice.FetchDocument](),
		"MaterialTextResponse": schemaFor[contract.MaterialTextResponse](),
		"SearchResult":         schemaFor[moodleservice.SearchResult](),
		"SearchResponse":       schemaFor[contract.SearchResponse](),
	}
}

func schemaFor[T any]() map[string]any {
	reflector := jsonschema.Reflector{
		Anonymous:                  true,
		DoNotReference:             true,
		ExpandedStruct:             true,
		AllowAdditionalProperties:  false,
		RequiredFromJSONSchemaTags: false,
	}
	schema := reflector.Reflect(new(T))
	data, _ := json.Marshal(schema)
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	delete(out, "$schema")
	delete(out, "$id")
	delete(out, "$defs")
	return out
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
			"get":    operation("listAPIKeys", "List API keys", security, "", "#/components/schemas/ListAPIKeysResponse"),
			"post":   operation("createAPIKey", "Create API key", security, "#/components/schemas/CreateAPIKeyRequest", "#/components/schemas/CreateAPIKeyResponse"),
			"delete": operationWithParams("revokeAPIKey", "Revoke API key", security, []any{queryParamRequired("id")}, "#/components/schemas/RevokeAPIKeyResponse"),
		},
		"/api/courses": map[string]any{
			"get": operation("listCourses", "List Moodle courses", security, "", "#/components/schemas/CoursesResponse"),
		},
		"/api/categories": map[string]any{
			"get": operation("listCategories", "List Moodle categories", security, "", "#/components/schemas/CategoriesResponse"),
		},
		"/api/courses/{courseId}/materials": map[string]any{
			"get": operationWithParams("listCourseMaterials", "List course materials", security, []any{pathParam("courseId")}, "#/components/schemas/MaterialsResponse"),
		},
		"/api/courses/{courseId}/materials/{resourceId}/text": map[string]any{
			"get": operationWithParams("readMaterialText", "Read material text", security, []any{pathParam("courseId"), pathParam("resourceId")}, "#/components/schemas/MaterialTextResponse"),
		},
		"/api/courses/{courseId}/materials/{resourceId}/pdf": map[string]any{
			"get": operationWithParams("readMaterialPDF", "Read material PDF", security, []any{pathParam("courseId"), pathParam("resourceId")}),
		},
		"/api/search": map[string]any{
			"get": operationWithParams("searchMoodle", "Search Moodle", security, []any{queryParam("q")}, "#/components/schemas/SearchResponse"),
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

func operationWithParams(operationID string, summary string, security []any, params []any, responseSchema ...string) map[string]any {
	schema := ""
	if len(responseSchema) > 0 {
		schema = responseSchema[0]
	}
	op := operation(operationID, summary, security, "", schema)
	op["parameters"] = params
	return op
}

func responses(schema string) map[string]any {
	okSchema := map[string]any{"type": "object"}
	if schema != "" {
		okSchema = map[string]any{"$ref": schema}
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

func queryParamRequired(name string) map[string]any {
	param := queryParam(name)
	param["required"] = true
	return param
}
