package moodle

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
)

const codexAppServerCommandEnv = "MOODLE_CODEX_APP_SERVER_COMMAND"

type codexAppServerClient struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	messages chan codexRPCMessage
	readErr  chan error
	nextID   atomic.Int64
	close    sync.Once
	stderr   safeBuffer
}

type codexRPCMessage struct {
	ID     int64           `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *codexRPCError  `json:"error,omitempty"`
}

type codexRPCError struct {
	Message string `json:"message"`
}

func newCodexAppServerClient(ctx context.Context, commandOverride string) (*codexAppServerClient, error) {
	command, args, err := resolveCodexAppServerCommand(commandOverride)
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, command, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	client := &codexAppServerClient{
		cmd:      cmd,
		stdin:    stdin,
		messages: make(chan codexRPCMessage, 64),
		readErr:  make(chan error, 1),
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	go client.readStdout(stdout)
	go func() {
		_, _ = io.Copy(&client.stderr, stderr)
	}()

	return client, nil
}

func resolveCodexAppServerCommand(commandOverride string) (string, []string, error) {
	command := strings.TrimSpace(commandOverride)
	if command == "" {
		command = strings.TrimSpace(os.Getenv(codexAppServerCommandEnv))
	}
	if command != "" {
		parts := strings.Fields(command)
		if len(parts) == 0 {
			return "", nil, fmt.Errorf("%s is empty", codexAppServerCommandEnv)
		}
		if len(parts) == 1 {
			return parts[0], []string{"app-server", "--listen", "stdio://"}, nil
		}
		return parts[0], parts[1:], nil
	}

	appPath := "/Applications/Codex.app/Contents/Resources/codex"
	if _, err := os.Stat(appPath); err == nil {
		return appPath, []string{"app-server", "--listen", "stdio://"}, nil
	}

	if path, err := exec.LookPath("codex"); err == nil {
		return path, []string{"app-server", "--listen", "stdio://"}, nil
	}

	return "", nil, errors.New("codex app-server command not found; install Codex CLI or set MOODLE_CODEX_APP_SERVER_COMMAND")
}

func (c *codexAppServerClient) Initialize(ctx context.Context) error {
	_, err := c.call(ctx, "initialize", map[string]any{
		"clientInfo": map[string]any{
			"name":    "moodle-services",
			"title":   "Moodle Services",
			"version": "0.1.0",
		},
		"capabilities": map[string]any{
			"experimentalApi":    false,
			"requestAttestation": false,
			"optOutNotificationMethods": []string{
				"mcpServer/startupStatus/updated",
			},
		},
	})
	if err != nil {
		return err
	}
	return c.notify("initialized", nil)
}

func (c *codexAppServerClient) StartOCRThread(ctx context.Context, model string) (string, error) {
	cwd, _ := os.Getwd()
	result, err := c.call(ctx, "thread/start", map[string]any{
		"model":          model,
		"cwd":            cwd,
		"approvalPolicy": "never",
		"sandbox":        "read-only",
		"ephemeral":      true,
		"baseInstructions": "You are an OCR extraction engine for Moodle PDFs. " +
			"Extract text from PDF page images. Return only the extracted page text in plain Markdown. " +
			"Do not explain, do not mention the image, and do not add commentary.",
	})
	if err != nil {
		return "", err
	}

	var parsed struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := json.Unmarshal(result, &parsed); err != nil {
		return "", err
	}
	if strings.TrimSpace(parsed.Thread.ID) == "" {
		return "", errors.New("codex thread/start response did not include a thread id")
	}
	return parsed.Thread.ID, nil
}

func (c *codexAppServerClient) ExtractTextFromImage(ctx context.Context, threadID string, imagePath string) (string, error) {
	result, err := c.call(ctx, "turn/start", map[string]any{
		"threadId": threadID,
		"input": []map[string]any{
			{
				"type":          "text",
				"text":          codexOCRPrompt(),
				"text_elements": []any{},
			},
			{
				"type":   "localImage",
				"path":   imagePath,
				"detail": "high",
			},
		},
	})
	if err != nil {
		return "", err
	}

	var parsed struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if err := json.Unmarshal(result, &parsed); err != nil {
		return "", err
	}
	if strings.TrimSpace(parsed.Turn.ID) == "" {
		return "", errors.New("codex turn/start response did not include a turn id")
	}

	collector := codexTurnTextCollector{threadID: threadID, turnID: parsed.Turn.ID}
	for {
		message, err := c.nextMessage(ctx)
		if err != nil {
			return "", err
		}
		done, err := collector.handle(message)
		if err != nil {
			return "", err
		}
		if done {
			return collector.text()
		}
	}
}

func codexOCRPrompt() string {
	return "Extract the visible text from this PDF page for a study CLI. Preserve reading order. " +
		"Keep code blocks and shell commands exactly. Include text visible inside screenshots. " +
		"Return plain Markdown only, with no explanation before or after the extracted text."
}

func (c *codexAppServerClient) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := c.nextID.Add(1)
	if err := c.write(codexRPCMessage{ID: id, Method: method, Params: mustMarshalRaw(params)}); err != nil {
		return nil, err
	}
	for {
		message, err := c.nextMessage(ctx)
		if err != nil {
			return nil, err
		}
		if message.ID != id {
			continue
		}
		if message.Error != nil {
			return nil, errors.New(message.Error.Message)
		}
		return message.Result, nil
	}
}

func (c *codexAppServerClient) notify(method string, params any) error {
	return c.write(codexRPCMessage{Method: method, Params: mustMarshalRaw(params)})
}

func (c *codexAppServerClient) write(message codexRPCMessage) error {
	body, err := json.Marshal(message)
	if err != nil {
		return err
	}
	body = append(body, '\n')
	_, err = c.stdin.Write(body)
	return err
}

func (c *codexAppServerClient) nextMessage(ctx context.Context) (codexRPCMessage, error) {
	select {
	case <-ctx.Done():
		return codexRPCMessage{}, ctx.Err()
	case err := <-c.readErr:
		if stderr := strings.TrimSpace(c.stderr.String()); stderr != "" {
			return codexRPCMessage{}, fmt.Errorf("%w: %s", err, stderr)
		}
		return codexRPCMessage{}, err
	case message := <-c.messages:
		return message, nil
	}
}

func (c *codexAppServerClient) readStdout(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var message codexRPCMessage
		if err := json.Unmarshal(line, &message); err != nil {
			continue
		}
		c.messages <- message
	}
	if err := scanner.Err(); err != nil {
		c.readErr <- err
		return
	}
	c.readErr <- io.EOF
}

func (c *codexAppServerClient) Close() error {
	var err error
	c.close.Do(func() {
		if c.stdin != nil {
			err = c.stdin.Close()
		}
		if c.cmd != nil && c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
			_ = c.cmd.Wait()
		}
	})
	return err
}

func mustMarshalRaw(value any) json.RawMessage {
	if value == nil {
		return nil
	}
	body, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return body
}

type codexTurnTextCollector struct {
	threadID      string
	turnID        string
	delta         strings.Builder
	completedText string
}

func (c *codexTurnTextCollector) handle(message codexRPCMessage) (bool, error) {
	switch message.Method {
	case "item/agentMessage/delta":
		var params struct {
			ThreadID string `json:"threadId"`
			TurnID   string `json:"turnId"`
			Delta    string `json:"delta"`
		}
		if err := json.Unmarshal(message.Params, &params); err != nil {
			return false, err
		}
		if params.ThreadID == c.threadID && params.TurnID == c.turnID {
			c.delta.WriteString(params.Delta)
		}
	case "item/completed":
		var params struct {
			ThreadID string `json:"threadId"`
			TurnID   string `json:"turnId"`
			Item     struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"item"`
		}
		if err := json.Unmarshal(message.Params, &params); err != nil {
			return false, err
		}
		if params.ThreadID == c.threadID && params.TurnID == c.turnID && params.Item.Type == "agentMessage" {
			c.completedText = params.Item.Text
		}
	case "turn/completed":
		var params struct {
			ThreadID string `json:"threadId"`
			Turn     struct {
				ID string `json:"id"`
			} `json:"turn"`
		}
		if err := json.Unmarshal(message.Params, &params); err != nil {
			return false, err
		}
		return params.ThreadID == c.threadID && params.Turn.ID == c.turnID, nil
	case "error":
		var params struct {
			ThreadID string `json:"threadId"`
			TurnID   string `json:"turnId"`
			Error    struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(message.Params, &params); err != nil {
			return false, err
		}
		if params.ThreadID == c.threadID && (params.TurnID == "" || params.TurnID == c.turnID) {
			return false, errors.New(params.Error.Message)
		}
	}
	return false, nil
}

func (c *codexTurnTextCollector) text() (string, error) {
	text := strings.TrimSpace(c.completedText)
	if text == "" {
		text = strings.TrimSpace(c.delta.String())
	}
	if text == "" {
		return "", errors.New("codex turn completed without extracted text")
	}
	return text, nil
}

type safeBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.b.Len() < 64*1024 {
		_, _ = b.b.Write(p)
	}
	return len(p), nil
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}
