package chatgptapp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

type Handler struct {
	Service Service
	APIKey  string
}

type pdfViewState struct {
	Title             string `json:"title"`
	CourseID          string `json:"courseId,omitempty"`
	ResourceID        string `json:"resourceId,omitempty"`
	Page              int    `json:"page"`
	PageCount         int    `json:"pageCount"`
	ScreenshotDataURL string `json:"screenshotDataURL,omitempty"`
	SelectionDataURL  string `json:"selectionDataURL,omitempty"`
	SelectionPage     int    `json:"selectionPage,omitempty"`
	SelectionX        int    `json:"selectionX,omitempty"`
	SelectionY        int    `json:"selectionY,omitempty"`
	SelectionWidth    int    `json:"selectionWidth,omitempty"`
	SelectionHeight   int    `json:"selectionHeight,omitempty"`
}

var pdfStates = struct {
	sync.Mutex
	byKey map[string]pdfViewState
}{byKey: map[string]pdfViewState{}}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "name": "moodle-chatgpt-app"})
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	var req rpcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeRPC(w, rpcResponse{JSONRPC: "2.0", ID: nil, Error: &rpcError{Code: -32700, Message: "invalid JSON"}})
		return
	}
	if strings.HasPrefix(req.Method, "notifications/") {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	result, err := h.dispatch(req)
	if err != nil {
		writeRPC(w, rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32000, Message: err.Error()}})
		return
	}
	writeRPC(w, rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: result})
}

func (h Handler) dispatch(req rpcRequest) (any, error) {
	switch req.Method {
	case "initialize":
		return map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities": map[string]any{
				"tools":     map[string]any{},
				"resources": map[string]any{},
			},
			"serverInfo": map[string]string{"name": "moodle-chatgpt-app", "version": "0.1.0"},
		}, nil
	case "tools/list":
		return map[string]any{"tools": h.tools()}, nil
	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, fmt.Errorf("invalid tool call")
		}
		return h.callTool(params.Name, params.Arguments)
	case "resources/list":
		return map[string]any{"resources": []any{h.widgetResource()}}, nil
	case "resources/read":
		var params struct {
			URI string `json:"uri"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, fmt.Errorf("invalid resource read")
		}
		if params.URI != widgetURI {
			return nil, fmt.Errorf("unknown resource %s", params.URI)
		}
		return map[string]any{"contents": []any{h.widgetContent()}}, nil
	default:
		return nil, fmt.Errorf("unsupported method %s", req.Method)
	}
}

func (h Handler) callTool(name string, args json.RawMessage) (any, error) {
	switch name {
	case "search":
		var input struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(args, &input)
		results, err := h.Service.Search(input.Query)
		return textJSON(map[string]any{"results": results}), err
	case "fetch":
		var input struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(args, &input)
		doc, err := h.Service.Fetch(input.ID)
		return textJSON(doc), err
	case "list_courses":
		courses, err := h.Service.ListCourses()
		return widgetResult(map[string]any{"courses": courses}, "Showing your Moodle courses."), err
	case "list_course_materials":
		var input struct {
			CourseID string `json:"courseId"`
		}
		_ = json.Unmarshal(args, &input)
		materials, err := h.Service.ListMaterials(input.CourseID)
		return widgetResult(map[string]any{"materials": materials}, "Showing course materials."), err
	case "list_calendar_events":
		var input struct {
			Days int `json:"days"`
		}
		_ = json.Unmarshal(args, &input)
		events, err := h.Service.CalendarEvents(input.Days)
		return widgetResult(map[string]any{"events": events}, "Showing school calendar events."), err
	case "read_material_text":
		var input struct {
			CourseID   string `json:"courseId"`
			ResourceID string `json:"resourceId"`
		}
		_ = json.Unmarshal(args, &input)
		if data, err := h.Service.PDFViewerData(input.CourseID, input.ResourceID); err == nil {
			return h.pdfWidgetResult(data, input.CourseID, input.ResourceID, "Showing the embedded PDF viewer."), nil
		}
		doc, err := h.Service.MaterialText(input.CourseID, input.ResourceID)
		return widgetResult(map[string]any{"document": doc}, "Extracted material text."), err
	case "render_pdf_viewer":
		var input struct {
			CourseID   string  `json:"courseId"`
			ResourceID string  `json:"resourceId"`
			Page       int     `json:"page"`
			Query      string  `json:"query"`
			Zoom       float64 `json:"zoom"`
		}
		_ = json.Unmarshal(args, &input)
		data, err := h.Service.PDFViewerData(input.CourseID, input.ResourceID)
		if err != nil {
			return nil, err
		}
		attachPDFTarget(data, input.Page, input.Query, input.Zoom)
		return h.pdfWidgetResult(data, input.CourseID, input.ResourceID, "Showing the embedded PDF viewer."), nil
	case "open_pdf_location":
		var input struct {
			CourseID   string  `json:"courseId"`
			ResourceID string  `json:"resourceId"`
			Page       int     `json:"page"`
			Query      string  `json:"query"`
			Zoom       float64 `json:"zoom"`
		}
		_ = json.Unmarshal(args, &input)
		data, err := h.Service.PDFViewerData(input.CourseID, input.ResourceID)
		if err != nil {
			return nil, err
		}
		attachPDFTarget(data, input.Page, input.Query, input.Zoom)
		return h.pdfWidgetResult(data, input.CourseID, input.ResourceID, "Moved the embedded PDF viewer to the requested location."), nil
	case "get_pdf_view_state":
		state, ok := h.pdfViewState()
		if !ok {
			return textJSON(map[string]any{"open": false, "status": "No PDF view state has been reported by the widget yet."}), nil
		}
		return textJSON(map[string]any{"open": true, "state": publicPDFState(state)}), nil
	case "capture_pdf_view":
		state, ok := h.pdfViewState()
		if !ok || state.ScreenshotDataURL == "" {
			return textJSON(map[string]any{"captured": false, "status": "No PDF screenshot has been reported by the widget yet. Open the PDF viewer first."}), nil
		}
		mimeType, data := splitDataURL(state.ScreenshotDataURL)
		if data == "" {
			return textJSON(map[string]any{"captured": false, "status": "The reported screenshot was not a valid data URL."}), nil
		}
		return map[string]any{
			"structuredContent": map[string]any{"captured": true, "state": publicPDFState(state)},
			"content": []any{
				map[string]any{"type": "text", "text": fmt.Sprintf("Captured page %d of %d from %s.", state.Page, state.PageCount, state.Title)},
				map[string]any{"type": "image", "data": data, "mimeType": mimeType},
			},
		}, nil
	case "get_pdf_selection":
		state, ok := h.pdfViewState()
		if !ok || state.SelectionDataURL == "" {
			return textJSON(map[string]any{"selected": false, "status": "No PDF area selection has been reported by the widget yet. Use the Ask button in the PDF viewer first."}), nil
		}
		mimeType, data := splitDataURL(state.SelectionDataURL)
		if data == "" {
			return textJSON(map[string]any{"selected": false, "status": "The reported selection was not a valid data URL."}), nil
		}
		return map[string]any{
			"structuredContent": map[string]any{"selected": true, "state": publicPDFState(state), "selection": publicPDFSelection(state)},
			"content": []any{
				map[string]any{"type": "text", "text": fmt.Sprintf("Captured selected area on page %d from %s.", state.SelectionPage, state.Title)},
				map[string]any{"type": "image", "data": data, "mimeType": mimeType},
			},
		}, nil
	case "save_pdf_view_state":
		var input pdfViewState
		_ = json.Unmarshal(args, &input)
		h.savePDFViewState(input)
		return textJSON(map[string]any{"saved": true}), nil
	default:
		return nil, fmt.Errorf("unknown tool %s", name)
	}
}

func (h Handler) tools() []any {
	return []any{
		tool("search", "Search Moodle", "Use this when the user wants to search Moodle courses, course materials, PDFs, or calendar events.", map[string]any{"query": stringSchema("Search query.")}, []string{"query"}, true, false),
		tool("fetch", "Fetch Moodle item", "Use this when the user asks for the full text or details of a search result id.", map[string]any{"id": stringSchema("A result id returned by search.")}, []string{"id"}, true, false),
		tool("list_courses", "List Moodle courses", "Use this when the user wants an overview of their Moodle courses.", map[string]any{}, nil, true, true),
		tool("list_course_materials", "List course materials", "Use this when the user wants the files and resources for a specific Moodle course.", map[string]any{"courseId": stringSchema("Moodle course id.")}, []string{"courseId"}, true, true),
		tool("list_calendar_events", "List school calendar", "Use this when the user asks about their school calendar, schedule, lectures, or upcoming events.", map[string]any{"days": numberSchema("Number of future days to show, from 1 to 120.")}, nil, true, true),
		tool("read_material_text", "Read Moodle material", "Use this when the user wants text extracted from a specific Moodle file or PDF.", map[string]any{"courseId": stringSchema("Moodle course id."), "resourceId": stringSchema("Moodle resource id.")}, []string{"courseId", "resourceId"}, true, true),
		tool("render_pdf_viewer", "Show PDF viewer", "Use this after identifying a PDF resource when the user wants to view or scroll through it in ChatGPT.", pdfToolProperties(), []string{"courseId", "resourceId"}, true, true),
		tool("open_pdf_location", "Scroll PDF viewer", "Use this when a PDF is open and the user wants to jump to a page or to text inside that PDF.", pdfToolProperties(), []string{"courseId", "resourceId"}, true, true),
		tool("get_pdf_view_state", "Get PDF viewer state", "Use this when the user asks whether a PDF is currently open or where the visible PDF view is. The widget reports live state while the PDF is open.", map[string]any{}, nil, true, false),
		tool("capture_pdf_view", "Capture PDF view", "Use this when the user asks for a screenshot or visual capture of the currently visible embedded PDF page.", map[string]any{}, nil, true, false),
		tool("get_pdf_selection", "Get PDF selection", "Use this when the user selected an area in the PDF widget with the Ask button and wants help with that selected screenshot.", map[string]any{}, nil, true, false),
		tool("save_pdf_view_state", "Save PDF view state", "Use this only when called by the widget to report the current PDF page and screenshot back to the MCP server.", pdfStateToolProperties(), []string{"title", "page", "pageCount"}, false, false),
	}
}

func tool(name, title, description string, properties map[string]any, required []string, readOnly bool, widget bool) map[string]any {
	meta := map[string]any{
		"openai/toolInvocation/invoking": "Loading Moodle data...",
		"openai/toolInvocation/invoked":  "Moodle data ready.",
	}
	if widget {
		meta["ui"] = map[string]any{"resourceUri": widgetURI}
		meta["openai/outputTemplate"] = widgetURI
	}
	if name == "save_pdf_view_state" {
		meta["openai/widgetAccessible"] = true
	}
	inputSchema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		inputSchema["required"] = required
	}
	return map[string]any{
		"name":        name,
		"title":       title,
		"description": description,
		"inputSchema": inputSchema,
		"annotations": map[string]any{
			"readOnlyHint":    readOnly,
			"idempotentHint":  true,
			"destructiveHint": false,
			"openWorldHint":   false,
		},
		"_meta": meta,
	}
}

func (h Handler) widgetResource() map[string]any {
	return map[string]any{"uri": widgetURI, "name": "Moodle browser", "mimeType": resourceMimeType}
}

func (h Handler) widgetContent() map[string]any {
	return map[string]any{
		"uri":      widgetURI,
		"mimeType": resourceMimeType,
		"text":     widgetHTML,
		"_meta": map[string]any{
			"ui": map[string]any{
				"prefersBorder": true,
				"domain":        widgetDomain,
				"csp": map[string]any{
					"connectDomains":  []string{widgetDomain},
					"resourceDomains": []string{"https://cdn.jsdelivr.net"},
				},
			},
			"openai/widgetDescription": "Displays Moodle courses, course materials, calendar events, and embedded PDFs.",
		},
	}
}

func widgetResult(data map[string]any, message string) map[string]any {
	return widgetResultWithMeta(data, message, nil)
}

func widgetResultWithMeta(data map[string]any, message string, meta map[string]any) map[string]any {
	if meta == nil {
		meta = map[string]any{}
	}
	meta["openai/outputTemplate"] = widgetURI
	return map[string]any{
		"structuredContent": data,
		"content":           []any{map[string]string{"type": "text", "text": message}},
		"_meta":             meta,
	}
}

func (h Handler) pdfWidgetResult(data map[string]any, courseID string, resourceID string, message string) map[string]any {
	meta := map[string]any{"pdfUrl": h.pdfURL(courseID, resourceID)}
	return widgetResultWithMeta(data, message, meta)
}

func (h Handler) pdfURL(courseID string, resourceID string) string {
	values := url.Values{}
	values.Set("courseId", courseID)
	values.Set("resourceId", resourceID)
	if h.APIKey != "" {
		values.Set("key", h.APIKey)
	}
	return widgetDomain + "/api/pdf?" + values.Encode()
}

func (h Handler) pdfStateKey() string {
	if h.APIKey != "" {
		return h.APIKey
	}
	return "default"
}

func (h Handler) savePDFViewState(state pdfViewState) {
	if state.Page <= 0 || state.PageCount <= 0 {
		return
	}
	if len(state.ScreenshotDataURL) > 2_500_000 {
		state.ScreenshotDataURL = ""
	}
	if len(state.SelectionDataURL) > 2_500_000 {
		state.SelectionDataURL = ""
	}
	pdfStates.Lock()
	defer pdfStates.Unlock()
	pdfStates.byKey[h.pdfStateKey()] = state
}

func (h Handler) pdfViewState() (pdfViewState, bool) {
	pdfStates.Lock()
	defer pdfStates.Unlock()
	state, ok := pdfStates.byKey[h.pdfStateKey()]
	return state, ok
}

func publicPDFState(state pdfViewState) map[string]any {
	return map[string]any{
		"title":      state.Title,
		"courseId":   state.CourseID,
		"resourceId": state.ResourceID,
		"page":       state.Page,
		"pageCount":  state.PageCount,
		"selection":  state.SelectionDataURL != "",
	}
}

func publicPDFSelection(state pdfViewState) map[string]any {
	return map[string]any{
		"page":   state.SelectionPage,
		"x":      state.SelectionX,
		"y":      state.SelectionY,
		"width":  state.SelectionWidth,
		"height": state.SelectionHeight,
	}
}

func attachPDFTarget(data map[string]any, page int, query string, zoom float64) {
	viewer, ok := data["viewer"].(PDFDescriptor)
	if !ok {
		return
	}
	target := map[string]any{}
	if page > 0 {
		target["page"] = page
	}
	if strings.TrimSpace(query) != "" {
		target["query"] = strings.TrimSpace(query)
	}
	if zoom > 0 {
		target["zoom"] = zoom
	}
	data["viewer"] = map[string]any{
		"id":          viewer.ID,
		"title":       viewer.Title,
		"courseId":    viewer.CourseID,
		"resourceId":  viewer.ResourceID,
		"sectionName": viewer.SectionName,
		"fileType":    viewer.FileType,
		"target":      target,
	}
}

func pdfToolProperties() map[string]any {
	return map[string]any{
		"courseId":   stringSchema("Moodle course id."),
		"resourceId": stringSchema("Moodle PDF resource id."),
		"page":       map[string]any{"type": "integer", "minimum": 1, "description": "Optional 1-based PDF page to scroll to."},
		"query":      stringSchema("Optional text to find visually in the PDF and scroll to."),
		"zoom":       map[string]any{"type": "number", "minimum": 0.5, "maximum": 2.75, "description": "Optional zoom factor for the PDF viewer, where 1.0 is normal size."},
	}
}

func pdfStateToolProperties() map[string]any {
	return map[string]any{
		"title":             stringSchema("PDF title."),
		"courseId":          stringSchema("Moodle course id."),
		"resourceId":        stringSchema("Moodle PDF resource id."),
		"page":              map[string]any{"type": "integer", "minimum": 1, "description": "Current visible PDF page."},
		"pageCount":         map[string]any{"type": "integer", "minimum": 1, "description": "Total PDF page count."},
		"screenshotDataURL": stringSchema("Optional data URL screenshot of the current PDF page."),
		"selectionDataURL":  stringSchema("Optional data URL screenshot of a user-selected PDF area."),
		"selectionPage":     map[string]any{"type": "integer", "minimum": 0, "description": "PDF page containing the selected area."},
		"selectionX":        map[string]any{"type": "integer", "minimum": 0, "description": "Selected area x-coordinate in rendered CSS pixels."},
		"selectionY":        map[string]any{"type": "integer", "minimum": 0, "description": "Selected area y-coordinate in rendered CSS pixels."},
		"selectionWidth":    map[string]any{"type": "integer", "minimum": 0, "description": "Selected area width in rendered CSS pixels."},
		"selectionHeight":   map[string]any{"type": "integer", "minimum": 0, "description": "Selected area height in rendered CSS pixels."},
	}
}

func splitDataURL(value string) (string, string) {
	header, data, ok := strings.Cut(value, ",")
	if !ok || data == "" || !strings.HasPrefix(header, "data:") {
		return "", ""
	}
	mimeType := strings.TrimPrefix(strings.TrimSuffix(header, ";base64"), "data:")
	if mimeType == "" {
		mimeType = "image/jpeg"
	}
	return mimeType, data
}

func textJSON(value any) map[string]any {
	data, _ := json.Marshal(value)
	return map[string]any{"content": []any{map[string]string{"type": "text", "text": string(data)}}}
}

func stringSchema(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func numberSchema(description string) map[string]any {
	return map[string]any{"type": "integer", "minimum": 1, "maximum": 120, "description": description}
}

func positiveInt(value string) (int, bool) {
	parsed, err := strconv.Atoi(value)
	return parsed, err == nil && parsed > 0
}

func writeRPC(w http.ResponseWriter, response rpcResponse) {
	writeJSON(w, http.StatusOK, response)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func setCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "authorization, content-type, mcp-session-id, x-moodle-app-key")
	w.Header().Set("Access-Control-Expose-Headers", "Mcp-Session-Id")
}
