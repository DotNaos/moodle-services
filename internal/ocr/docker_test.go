package ocr

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDockerRunArgsBuildsIsolatedMounts(t *testing.T) {
	t.Setenv("OCR_DOCKER_PLATFORM", "linux/amd64")
	input := filepath.Join(t.TempDir(), "input.pdf")
	if err := os.WriteFile(input, []byte("%PDF"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	provider, _ := ProviderByID("docling")

	args, err := DockerRunArgs(provider, input, out, Options{Formula: true})
	if err != nil {
		t.Fatalf("DockerRunArgs: %v", err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"run --rm",
		"--platform linux/amd64",
		"-v " + filepath.Dir(input) + ":/input:ro",
		"-v " + out + ":/output",
		"-e OCR_FORMULA=1",
		provider.DockerImage,
		"/input/input.pdf /output",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected %q in docker args %q", want, joined)
		}
	}
}

func TestDockerRunArgsAddsGPU(t *testing.T) {
	provider, _ := ProviderByID("olmocr")
	args, err := DockerRunArgs(provider, "/tmp/input.pdf", "/tmp/out", Options{GPU: true})
	if err != nil {
		t.Fatalf("DockerRunArgs: %v", err)
	}
	if !strings.Contains(strings.Join(args, " "), "--gpus all") {
		t.Fatalf("expected gpu args, got %v", args)
	}
}

func TestDockerRunArgsTranslatesContainerDataPathForHostDaemon(t *testing.T) {
	t.Setenv("MOODLE_DOCKER_CONTAINER_DATA_DIR", "/data")
	t.Setenv("MOODLE_DOCKER_HOST_DATA_DIR", "/home/codex/.moodle")
	provider, _ := ProviderByID("docling")

	args, err := DockerRunArgs(provider, "/data/ocr/runtime/input.pdf", "/data/ocr/out", Options{})
	if err != nil {
		t.Fatalf("DockerRunArgs: %v", err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"-v /home/codex/.moodle/ocr/runtime:/input:ro",
		"-v /home/codex/.moodle/ocr/out:/output",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected %q in docker args %q", want, joined)
		}
	}
}

func TestDockerHostMountPathMatchesWindowsAbsoluteContainerPath(t *testing.T) {
	t.Setenv("MOODLE_DOCKER_CONTAINER_DATA_DIR", "/data")
	t.Setenv("MOODLE_DOCKER_HOST_DATA_DIR", "/home/codex/.moodle")

	got := dockerHostMountPath(`D:\data\ocr\runtime`)
	want := "/home/codex/.moodle/ocr/runtime"
	if got != want {
		t.Fatalf("dockerHostMountPath = %q, want %q", got, want)
	}
}

func TestDockerRunArgsAddsSharedOCRCache(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "cache")
	t.Setenv("MOODLE_OCR_CACHE_DIR", cache)
	provider, _ := ProviderByID("docling")

	args, err := DockerRunArgs(provider, "/tmp/input.pdf", "/tmp/out", Options{})
	if err != nil {
		t.Fatalf("DockerRunArgs: %v", err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"-v " + cache + ":/cache",
		"-e HF_HOME=/cache/huggingface",
		"-e XDG_CACHE_HOME=/cache/xdg",
		"-e MODELSCOPE_CACHE=/cache/modelscope",
		"-e PADDLE_PDX_CACHE_HOME=/cache/paddlex",
		"-e PADDLE_PDX_DISABLE_MODEL_SOURCE_CHECK=True",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected %q in docker args %q", want, joined)
		}
	}
}

func TestLogStreamWriterPrefixesVerboseOutput(t *testing.T) {
	var log bytes.Buffer
	var saved bytes.Buffer
	writer := logStreamWriter(Options{Verbose: true, LogWriter: &log}, &saved, "stderr")

	if _, err := writer.Write([]byte("first\nsecond\rprogress\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := saved.String(); got != "first\nsecond\rprogress\n" {
		t.Fatalf("saved log = %q", got)
	}
	if got := log.String(); got != "[ocr][stderr] first\n[ocr][stderr] second\n[ocr][stderr] progress\n" {
		t.Fatalf("streamed log = %q", got)
	}
}

func TestCompactProcessWarningKeepsTailAndTruncates(t *testing.T) {
	warning := compactProcessWarning(strings.Repeat("noise\n", 20) + strings.Repeat("x", 800))
	if len([]rune(warning)) > 503 {
		t.Fatalf("warning was not truncated: %d", len([]rune(warning)))
	}
	if !strings.Contains(warning, "xxx") {
		t.Fatalf("expected tail content in warning, got %q", warning)
	}
}
