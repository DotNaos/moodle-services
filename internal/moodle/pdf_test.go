package moodle

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestShouldAttemptOCR(t *testing.T) {
	if !shouldAttemptOCR("", errors.New("native extraction failed")) {
		t.Fatalf("expected OCR attempt when native extraction failed")
	}
	if !shouldAttemptOCR("", nil) {
		t.Fatalf("expected OCR attempt for empty native text")
	}
	if !shouldAttemptOCR("short text only", nil) {
		t.Fatalf("expected OCR attempt for very short text")
	}

	goodText := "This is a readable sentence. " +
		"This is another readable sentence with enough words to exceed the threshold significantly. " +
		"The extractor should keep this without OCR because the content quality is high and consistent. " +
		"Adding more words here keeps the text long enough to avoid triggering OCR heuristics."
	if shouldAttemptOCR(goodText, nil) {
		t.Fatalf("did not expect OCR attempt for readable text")
	}
}

func TestShouldPreferOCR(t *testing.T) {
	if !shouldPreferOCR("", errors.New("native failed"), "valid ocr output", nil) {
		t.Fatalf("expected OCR preference when native extraction failed")
	}
	if shouldPreferOCR("native text", nil, "", nil) {
		t.Fatalf("did not expect OCR preference for empty OCR output")
	}

	nativePoor := "l l l l l l l l"
	ocrGood := "This output contains clear readable words and punctuation."
	if !shouldPreferOCR(nativePoor, nil, ocrGood, nil) {
		t.Fatalf("expected OCR preference for clearly better OCR output")
	}

	nativeGood := "This sentence is clear and already readable without OCR."
	ocrWorse := "Th1s se nte nce i$ noisy"
	if shouldPreferOCR(nativeGood, nil, ocrWorse, nil) {
		t.Fatalf("did not expect OCR preference for worse OCR output")
	}
}

func TestSelectTesseractLanguage(t *testing.T) {
	out := "List of available languages in \"/opt/homebrew/share/tessdata\" (2):\ndeu\neng\n"
	if got := selectTesseractLanguage(out, "", nil); got != "deu+eng" {
		t.Fatalf("expected deu+eng, got %q", got)
	}

	out = "List of available languages\neng\n"
	if got := selectTesseractLanguage(out, "", nil); got != "eng" {
		t.Fatalf("expected eng, got %q", got)
	}

	if got := selectTesseractLanguage("", "", errors.New("failed")); got != "" {
		t.Fatalf("expected empty language on error, got %q", got)
	}
}

func TestPageIndexFromPath(t *testing.T) {
	if got := pageIndexFromPath("/tmp/page-12.png"); got != 12 {
		t.Fatalf("expected 12, got %d", got)
	}
	if got := pageIndexFromPath("/tmp/page.png"); got <= 1000 {
		t.Fatalf("expected large fallback index, got %d", got)
	}
}

func TestCodexTurnTextCollectorUsesCompletedAgentMessage(t *testing.T) {
	collector := codexTurnTextCollector{threadID: "thread-1", turnID: "turn-1"}
	done, err := collector.handle(codexTestMessage("item/agentMessage/delta", map[string]any{
		"threadId": "thread-1",
		"turnId":   "turn-1",
		"delta":    "partial ",
	}))
	if err != nil {
		t.Fatalf("handle delta: %v", err)
	}
	if done {
		t.Fatalf("did not expect delta to complete the turn")
	}

	done, err = collector.handle(codexTestMessage("item/completed", map[string]any{
		"threadId": "thread-1",
		"turnId":   "turn-1",
		"item": map[string]any{
			"type": "agentMessage",
			"text": "final OCR text",
		},
	}))
	if err != nil {
		t.Fatalf("handle completed item: %v", err)
	}
	if done {
		t.Fatalf("did not expect item completion to complete the turn")
	}

	done, err = collector.handle(codexTestMessage("turn/completed", map[string]any{
		"threadId": "thread-1",
		"turn": map[string]any{
			"id": "turn-1",
		},
	}))
	if err != nil {
		t.Fatalf("handle turn completed: %v", err)
	}
	if !done {
		t.Fatalf("expected matching turn to complete")
	}

	got, err := collector.text()
	if err != nil {
		t.Fatalf("collector text: %v", err)
	}
	if got != "final OCR text" {
		t.Fatalf("expected completed text, got %q", got)
	}
}

func TestCodexTurnTextCollectorFallsBackToDelta(t *testing.T) {
	collector := codexTurnTextCollector{threadID: "thread-1", turnID: "turn-1"}
	for _, delta := range []string{"first", " second"} {
		_, err := collector.handle(codexTestMessage("item/agentMessage/delta", map[string]any{
			"threadId": "thread-1",
			"turnId":   "turn-1",
			"delta":    delta,
		}))
		if err != nil {
			t.Fatalf("handle delta: %v", err)
		}
	}
	got, err := collector.text()
	if err != nil {
		t.Fatalf("collector text: %v", err)
	}
	if got != "first second" {
		t.Fatalf("expected delta text, got %q", got)
	}
}

func TestCodexTurnTextCollectorError(t *testing.T) {
	collector := codexTurnTextCollector{threadID: "thread-1", turnID: "turn-1"}
	_, err := collector.handle(codexTestMessage("error", map[string]any{
		"threadId": "thread-1",
		"turnId":   "turn-1",
		"error": map[string]any{
			"message": "bad model",
		},
	}))
	if err == nil {
		t.Fatalf("expected codex error notification to fail")
	}
}

func codexTestMessage(method string, params any) codexRPCMessage {
	body, err := json.Marshal(params)
	if err != nil {
		panic(err)
	}
	return codexRPCMessage{Method: method, Params: body}
}
