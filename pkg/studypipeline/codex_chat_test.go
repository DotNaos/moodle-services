package studypipeline

import (
	"testing"

	contract "github.com/DotNaos/moodle-services/pkg/apicontracts"
)

func collectCodexEvent(t *testing.T, line string) contract.StudyPipelineRefineEvent {
	t.Helper()
	var events []contract.StudyPipelineRefineEvent
	emitCodexLineEvent(line, func(event contract.StudyPipelineRefineEvent) {
		events = append(events, event)
	})
	if len(events) != 1 {
		t.Fatalf("expected exactly 1 emitted event for %q, got %d", line, len(events))
	}
	return events[0]
}

func TestEmitCodexLineEvent_ToolCalls(t *testing.T) {
	cases := []struct {
		name       string
		line       string
		wantTitle  string
		wantStatus string
		wantID     string
	}{
		{
			name:       "command execution running",
			line:       `{"type":"item.started","item":{"id":"i1","type":"command_execution","command":"ls -la /srv","status":"in_progress"}}`,
			wantTitle:  "ls -la /srv",
			wantStatus: "running",
			wantID:     "i1",
		},
		{
			name:       "command execution completed",
			line:       `{"type":"item.completed","item":{"id":"i1","type":"command_execution","command":"grep foo","status":"completed"}}`,
			wantTitle:  "grep foo",
			wantStatus: "completed",
			wantID:     "i1",
		},
		{
			name:       "mcp tool call",
			line:       `{"type":"item.completed","item":{"id":"m1","type":"mcp_tool_call","server":"moodle","tool":"search","status":"completed"}}`,
			wantTitle:  "moodle.search",
			wantStatus: "completed",
			wantID:     "m1",
		},
		{
			name:       "web search",
			line:       `{"type":"item.completed","item":{"id":"w1","type":"web_search","query":"matrix inverse"}}`,
			wantTitle:  "Web search: matrix inverse",
			wantStatus: "completed",
			wantID:     "w1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			event := collectCodexEvent(t, tc.line)
			if event.Category != "tool" {
				t.Fatalf("category = %q, want tool", event.Category)
			}
			if event.ToolTitle != tc.wantTitle {
				t.Errorf("title = %q, want %q", event.ToolTitle, tc.wantTitle)
			}
			if event.ToolStatus != tc.wantStatus {
				t.Errorf("status = %q, want %q", event.ToolStatus, tc.wantStatus)
			}
			if event.ToolID != tc.wantID {
				t.Errorf("id = %q, want %q", event.ToolID, tc.wantID)
			}
		})
	}
}

func TestEmitCodexLineEvent_LifecycleIsStatus(t *testing.T) {
	lines := []string{
		`{"type":"thread.started","thread_id":"t1"}`,
		`{"type":"turn.started"}`,
		`{"type":"turn.completed"}`,
		`{"type":"item.completed","item":{"id":"r1","type":"reasoning","text":"thinking"}}`,
		`{"type":"item.completed","item":{"id":"f1","type":"file_change"}}`,
	}
	for _, line := range lines {
		event := collectCodexEvent(t, line)
		if event.Category != "status" {
			t.Errorf("line %q: category = %q, want status", line, event.Category)
		}
		if event.ToolTitle != "" {
			t.Errorf("line %q: unexpected tool title %q", line, event.ToolTitle)
		}
	}
}
