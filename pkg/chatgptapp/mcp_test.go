package chatgptapp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DotNaos/moodle-services/internal/moodle"
)

type stubClient struct {
	courses   []moodle.Course
	materials map[string][]moodle.Resource
	files     map[string]moodle.DownloadResult
}

func (s stubClient) ValidateSession() error { return nil }

func (s stubClient) FetchCourses() ([]moodle.Course, error) {
	return s.courses, nil
}

func (s stubClient) FetchCourseResources(courseID string) ([]moodle.Resource, string, error) {
	return s.materials[courseID], "", nil
}

func (s stubClient) DownloadFileToBuffer(url string) (moodle.DownloadResult, error) {
	return s.files[url], nil
}

func TestMCPInitializeAndToolsList(t *testing.T) {
	handler := testHandler()

	initResp := callRPC(t, handler, "initialize", nil)
	result := initResp["result"].(map[string]any)
	if result["protocolVersion"] == "" {
		t.Fatalf("missing protocol version: %#v", result)
	}

	toolsResp := callRPC(t, handler, "tools/list", nil)
	tools := toolsResp["result"].(map[string]any)["tools"].([]any)
	if len(tools) < 11 {
		t.Fatalf("expected app tools, got %#v", tools)
	}
	for _, raw := range tools {
		tool := raw.(map[string]any)
		annotations := tool["annotations"].(map[string]any)
		if tool["name"] != "save_pdf_view_state" && annotations["readOnlyHint"] != true {
			t.Fatalf("tool should be read-only: %#v", tool)
		}
		if annotations["destructiveHint"] != false || annotations["openWorldHint"] != false {
			t.Fatalf("tool should include complete safe annotations: %#v", tool)
		}
		schema := tool["inputSchema"].(map[string]any)
		if schema["required"] == nil {
			if _, exists := schema["required"]; exists {
				t.Fatalf("required must be omitted instead of null: %#v", schema)
			}
		}
	}
}

func TestMCPListCoursesUsesWidgetTemplate(t *testing.T) {
	handler := testHandler()
	resp := callTool(t, handler, "list_courses", map[string]any{})

	result := resp["result"].(map[string]any)
	content := result["structuredContent"].(map[string]any)
	if len(content["courses"].([]any)) != 1 {
		t.Fatalf("expected one course, got %#v", content)
	}
	if !strings.Contains(result["_meta"].(map[string]any)["openai/outputTemplate"].(string), "moodle-browser") {
		t.Fatalf("missing output template: %#v", result["_meta"])
	}
}

func TestMCPSearchDoesNotExposeMoodleFileURL(t *testing.T) {
	handler := testHandler()
	resp := callTool(t, handler, "search", map[string]any{"query": "slides"})

	result := resp["result"].(map[string]any)
	rawText := result["content"].([]any)[0].(map[string]any)["text"].(string)
	if strings.Contains(rawText, "moodle.test/slides.pdf") {
		t.Fatalf("search result exposed the raw Moodle file URL: %s", rawText)
	}
	if !strings.Contains(rawText, "moodle-material://42/100") {
		t.Fatalf("search result should include internal material URL: %s", rawText)
	}
}

func TestMCPWidgetResourceIncludesDomain(t *testing.T) {
	handler := testHandler()
	resp := callRPC(t, handler, "resources/read", map[string]any{"uri": widgetURI})

	result := resp["result"].(map[string]any)
	contents := result["contents"].([]any)
	content := contents[0].(map[string]any)
	meta := content["_meta"].(map[string]any)
	ui := meta["ui"].(map[string]any)
	if ui["domain"] != widgetDomain {
		t.Fatalf("expected widget domain %q, got %#v", widgetDomain, ui)
	}
	csp := ui["csp"].(map[string]any)
	if !strings.Contains(fmt.Sprint(csp["connectDomains"]), widgetDomain) {
		t.Fatalf("expected widget API connect domain, got %#v", csp)
	}
	if !strings.Contains(fmt.Sprint(csp["resourceDomains"]), "cdn.jsdelivr.net") {
		t.Fatalf("expected PDF.js worker resource domain, got %#v", csp)
	}
}

func TestMCPPDFViewStateAndCapture(t *testing.T) {
	handler := testHandler()
	handler.APIKey = "capture-test"
	callTool(t, handler, "save_pdf_view_state", map[string]any{
		"title":             "Slides",
		"courseId":          "42",
		"resourceId":        "100",
		"page":              2,
		"pageCount":         16,
		"screenshotDataURL": "data:image/jpeg;base64,ZmFrZQ==",
		"selectionDataURL":  "data:image/jpeg;base64,c2VsZWN0aW9u",
		"selectionPage":     2,
		"selectionX":        10,
		"selectionY":        20,
		"selectionWidth":    300,
		"selectionHeight":   140,
	})

	stateResp := callTool(t, handler, "get_pdf_view_state", map[string]any{})
	stateText := stateResp["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(stateText, `"open":true`) || !strings.Contains(stateText, `"page":2`) {
		t.Fatalf("expected saved view state, got %s", stateText)
	}

	captureResp := callTool(t, handler, "capture_pdf_view", map[string]any{})
	content := captureResp["result"].(map[string]any)["content"].([]any)
	image := content[1].(map[string]any)
	if image["type"] != "image" || image["mimeType"] != "image/jpeg" {
		t.Fatalf("expected image content, got %#v", image)
	}

	selectionResp := callTool(t, handler, "get_pdf_selection", map[string]any{})
	selectionContent := selectionResp["result"].(map[string]any)["content"].([]any)
	selectionImage := selectionContent[1].(map[string]any)
	if selectionImage["type"] != "image" || selectionImage["data"] != "c2VsZWN0aW9u" {
		t.Fatalf("expected selected area image, got %#v", selectionImage)
	}
}

func TestMCPRenderPDFViewerReturnsEmbeddedViewerMetadata(t *testing.T) {
	handler := testHandler()
	handler.APIKey = "secret"
	resp := callTool(t, handler, "render_pdf_viewer", map[string]any{"courseId": "42", "resourceId": "100", "page": 3, "zoom": 1.4})

	result := resp["result"].(map[string]any)
	content := result["structuredContent"].(map[string]any)
	viewer := content["viewer"].(map[string]any)
	if viewer["title"] != "Slides" {
		t.Fatalf("expected PDF viewer descriptor, got %#v", viewer)
	}
	target := viewer["target"].(map[string]any)
	if target["page"] != float64(3) && target["page"] != 3 {
		t.Fatalf("expected page target, got %#v", target)
	}
	if target["zoom"] != float64(1.4) {
		t.Fatalf("expected zoom target, got %#v", target)
	}
	meta := result["_meta"].(map[string]any)
	if !strings.Contains(meta["pdfUrl"].(string), "/api/pdf?") || !strings.Contains(meta["pdfUrl"].(string), "key=secret") {
		t.Fatalf("expected widget-only pdf url, got %#v", meta)
	}
}

func TestMCPReadMaterialTextForPDFReturnsEmbeddedViewer(t *testing.T) {
	handler := testHandler()
	handler.APIKey = "secret"
	resp := callTool(t, handler, "read_material_text", map[string]any{"courseId": "42", "resourceId": "100"})

	result := resp["result"].(map[string]any)
	content := result["structuredContent"].(map[string]any)
	viewer := content["viewer"].(map[string]any)
	if viewer["title"] != "Slides" || viewer["fileType"] != "pdf" {
		t.Fatalf("expected PDF viewer descriptor, got %#v", viewer)
	}
	if _, ok := content["document"]; ok {
		t.Fatalf("PDF material should render through the viewer instead of document text: %#v", content)
	}
	meta := result["_meta"].(map[string]any)
	if !strings.Contains(meta["pdfUrl"].(string), "/api/pdf?") || !strings.Contains(meta["pdfUrl"].(string), "key=secret") {
		t.Fatalf("expected widget-only pdf url, got %#v", meta)
	}
}

func testHandler() Handler {
	client := stubClient{
		courses: []moodle.Course{{ID: 42, Fullname: "Machine Learning", Shortname: "ML"}},
		materials: map[string][]moodle.Resource{
			"42": {{ID: "100", Name: "Slides", URL: "https://moodle.test/slides.pdf", FileType: "pdf", CourseID: "42"}},
		},
		files: map[string]moodle.DownloadResult{
			"https://moodle.test/slides.pdf": {Data: []byte("Page one\n\nPage two"), ContentType: "text/plain"},
		},
	}
	return Handler{Service: Service{
		Client: client,
		Now:    func() time.Time { return time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC) },
	}}
}

func callTool(t *testing.T, handler Handler, name string, args map[string]any) map[string]any {
	t.Helper()
	return callRPC(t, handler, "tools/call", map[string]any{"name": name, "arguments": args})
}

func callRPC(t *testing.T, handler Handler, method string, params any) map[string]any {
	t.Helper()
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/mcp", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["error"] != nil {
		t.Fatalf("unexpected rpc error: %#v", payload["error"])
	}
	return payload
}
