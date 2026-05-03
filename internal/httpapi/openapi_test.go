package httpapi

import (
	"encoding/json"
	"testing"
)

func TestOpenAPISpecIncludesRequiredContract(t *testing.T) {
	var spec map[string]any
	if err := json.Unmarshal(OpenAPISpecJSON(), &spec); err != nil {
		t.Fatalf("OpenAPI JSON should decode: %v", err)
	}
	if spec["openapi"] != "3.1.0" {
		t.Fatalf("expected OpenAPI 3.1.0, got %#v", spec["openapi"])
	}
	paths := spec["paths"].(map[string]any)
	for _, path := range []string{"/api/auth/qr/exchange", "/api/keys", "/api/courses", "/api/categories", "/api/openapi.json"} {
		if paths[path] == nil {
			t.Fatalf("missing OpenAPI path %s", path)
		}
	}
	schemas := spec["components"].(map[string]any)["schemas"].(map[string]any)
	assertSchemaProperty(t, schemas, "Course", "categoryId")
	assertSchemaProperty(t, schemas, "Course", "heroImage")
	assertSchemaProperty(t, schemas, "CoursesResponse", "courses")
	assertSchemaProperty(t, schemas, "CategoriesResponse", "categories")
	assertSchemaProperty(t, schemas, "CreateAPIKeyResponse", "apiKeyRecord")
	keysPath := paths["/api/keys"].(map[string]any)
	if keysPath["delete"] == nil {
		t.Fatalf("missing DELETE /api/keys operation")
	}
	assertComponentRefsResolve(t, schemas, spec)
}

func assertSchemaProperty(t *testing.T, schemas map[string]any, schemaName string, propertyName string) {
	t.Helper()
	schema, ok := schemas[schemaName].(map[string]any)
	if !ok {
		t.Fatalf("missing schema %s: %#v", schemaName, schemas[schemaName])
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema %s has no properties: %#v", schemaName, schema)
	}
	if properties[propertyName] == nil {
		t.Fatalf("schema %s missing property %s: %#v", schemaName, propertyName, properties)
	}
}

func assertComponentRefsResolve(t *testing.T, schemas map[string]any, value any) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == "$ref" {
				ref, ok := child.(string)
				if !ok {
					t.Fatalf("schema ref should be a string: %#v", child)
				}
				const prefix = "#/components/schemas/"
				if len(ref) > len(prefix) && ref[:len(prefix)] == prefix {
					name := ref[len(prefix):]
					if schemas[name] == nil {
						t.Fatalf("schema ref %s points to missing component %s", ref, name)
					}
				}
				continue
			}
			assertComponentRefsResolve(t, schemas, child)
		}
	case []any:
		for _, child := range typed {
			assertComponentRefsResolve(t, schemas, child)
		}
	}
}
