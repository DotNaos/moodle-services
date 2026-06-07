package ocr

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type LocalExecutor struct {
	PdftotextBinary string
}

func (e LocalExecutor) Run(ctx context.Context, provider Provider, inputPDF string, outputDir string, opts Options) (RunResult, error) {
	if provider.ID != "pdftotext" {
		return RunResult{}, fmt.Errorf("local ocr engine %q is not supported", provider.ID)
	}
	if !opts.GPU && !provider.SupportsCPU {
		return RunResult{}, fmt.Errorf("ocr engine %q requires gpu mode", provider.ID)
	}
	if opts.GPU && !provider.SupportsGPU {
		return RunResult{}, fmt.Errorf("ocr engine %q does not support gpu mode", provider.ID)
	}
	binary := strings.TrimSpace(e.PdftotextBinary)
	if binary == "" {
		binary = "pdftotext"
	}
	if _, err := exec.LookPath(binary); err != nil {
		return RunResult{}, fmt.Errorf("pdftotext executable not found: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(outputDir, "logs"), 0o755); err != nil {
		return RunResult{}, err
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = time.Duration(provider.DefaultTimeoutMs) * time.Millisecond
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	textPath := filepath.Join(outputDir, "text.txt")
	progressf(opts, "starting pdftotext")
	progressf(opts, "pdftotext -layout %s %s", inputPDF, textPath)
	cmd := exec.CommandContext(runCtx, binary, "-layout", inputPDF, textPath)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = logStreamWriter(opts, &stdout, "stdout")
	cmd.Stderr = logStreamWriter(opts, &stderr, "stderr")

	started := time.Now()
	err := cmd.Run()
	durationMs := time.Since(started).Milliseconds()
	timedOut := runCtx.Err() != nil
	exitCode := exitCodeFromError(err, timedOut)
	progressf(opts, "finished pdftotext status=%d duration=%dms", exitCode, durationMs)

	_ = os.WriteFile(filepath.Join(outputDir, "logs", "stdout.txt"), stdout.Bytes(), 0o644)
	_ = os.WriteFile(filepath.Join(outputDir, "logs", "stderr.txt"), stderr.Bytes(), 0o644)
	if exitCode == 0 {
		if data, readErr := os.ReadFile(textPath); readErr == nil {
			_ = os.WriteFile(filepath.Join(outputDir, "output.md"), normalizePdftotextMarkdown(data), 0o644)
		}
	}

	result := ParseOutput(provider, outputDir, exitCode, timedOut, durationMs)
	if err != nil && !timedOut {
		result.Warnings = uniqueStrings(append(result.Warnings, compactProcessWarning(stderr.String())))
	}
	return result, nil
}

func normalizePdftotextMarkdown(data []byte) []byte {
	text := strings.TrimSpace(string(data))
	if text == "" {
		return []byte{}
	}
	return []byte(text + "\n")
}
