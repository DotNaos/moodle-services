package ocr

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDockerExecutorIntegration(t *testing.T) {
	if os.Getenv("RUN_DOCKER_OCR_TESTS") != "1" {
		t.Skip("set RUN_DOCKER_OCR_TESTS=1 to run Docker OCR integration tests")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skipf("docker not found: %v", err)
	}

	image := fmt.Sprintf("moodle-ocr-test:%d", os.Getpid())
	contextDir := t.TempDir()
	writeTestFile(t, filepath.Join(contextDir, "Dockerfile"), `FROM alpine:3.20
COPY run.sh /usr/local/bin/run-ocr
RUN chmod +x /usr/local/bin/run-ocr
ENTRYPOINT ["run-ocr"]
`)
	writeTestFile(t, filepath.Join(contextDir, "run.sh"), `#!/bin/sh
set -eu
input="$1"
output="$2"
mkdir -p "$output/images" "$output/logs" "$output/artifacts"
printf '# Docker OCR smoke\n\nInput: %s\nThis markdown output is intentionally long enough for warning checks.\n' "$input" > "$output/output.md"
printf '{"ok":true}\n' > "$output/output.json"
printf 'text output\n' > "$output/text.txt"
printf 'png' > "$output/images/page.png"
`)

	build := exec.Command("docker", "build", "-t", image, contextDir)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("docker build failed: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("docker", "rmi", "-f", image).Run()
	})

	input := filepath.Join(t.TempDir(), "fixture.pdf")
	writeTestFile(t, input, "%PDF-1.4\n%%EOF\n")
	output := t.TempDir()
	result, err := (DockerExecutor{}).Run(context.Background(), Provider{
		ID:               "smoke",
		DockerImage:      image,
		SupportsCPU:      true,
		DefaultTimeoutMs: 30_000,
	}, input, output, Options{Timeout: 30 * time.Second})
	if err != nil {
		t.Fatalf("DockerExecutor.Run: %v", err)
	}
	if result.Status != StatusSuccess {
		t.Fatalf("expected success, got %#v", result)
	}
	if !strings.Contains(result.Markdown, "Docker OCR smoke") {
		t.Fatalf("expected markdown output, got %q", result.Markdown)
	}
	if len(result.Images) != 1 {
		t.Fatalf("expected one image, got %#v", result.Images)
	}
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}
