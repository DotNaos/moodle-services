package ocr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"
)

type DockerExecutor struct {
	DockerBinary string
}

func (e DockerExecutor) Run(ctx context.Context, provider Provider, inputPDF string, outputDir string, opts Options) (RunResult, error) {
	docker := strings.TrimSpace(e.DockerBinary)
	if docker == "" {
		docker = "docker"
	}
	if _, err := exec.LookPath(docker); err != nil {
		return RunResult{}, fmt.Errorf("docker executable not found: %w", err)
	}
	if !opts.GPU && !provider.SupportsCPU {
		return RunResult{}, fmt.Errorf("ocr engine %q requires gpu mode", provider.ID)
	}
	if opts.GPU && !provider.SupportsGPU {
		return RunResult{}, fmt.Errorf("ocr engine %q does not support gpu mode", provider.ID)
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return RunResult{}, err
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

	args, err := DockerRunArgs(provider, inputPDF, outputDir, opts)
	if err != nil {
		return RunResult{}, err
	}
	progressf(opts, "starting %s container", provider.ID)
	progressf(opts, "docker %s", strings.Join(args, " "))
	cmd := exec.CommandContext(runCtx, docker, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = logStreamWriter(opts, &stdout, "stdout")
	cmd.Stderr = logStreamWriter(opts, &stderr, "stderr")

	started := time.Now()
	err = cmd.Run()
	durationMs := time.Since(started).Milliseconds()
	timedOut := runCtx.Err() != nil
	exitCode := exitCodeFromError(err, timedOut)
	progressf(opts, "finished %s status=%d duration=%dms", provider.ID, exitCode, durationMs)

	_ = os.WriteFile(filepath.Join(outputDir, "logs", "stdout.txt"), stdout.Bytes(), 0o644)
	_ = os.WriteFile(filepath.Join(outputDir, "logs", "stderr.txt"), stderr.Bytes(), 0o644)

	result := ParseOutput(provider, outputDir, exitCode, timedOut, durationMs)
	if err != nil && !timedOut {
		result.Warnings = uniqueStrings(append(result.Warnings, compactProcessWarning(stderr.String())))
	}
	return result, nil
}

func compactProcessWarning(value string) string {
	lines := strings.Split(strings.TrimSpace(value), "\n")
	kept := make([]string, 0, 6)
	for i := len(lines) - 1; i >= 0 && len(kept) < 6; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		kept = append(kept, line)
	}
	for i, j := 0, len(kept)-1; i < j; i, j = i+1, j-1 {
		kept[i], kept[j] = kept[j], kept[i]
	}
	warning := strings.Join(kept, " ")
	if len([]rune(warning)) > 500 {
		runes := []rune(warning)
		warning = string(runes[:500]) + "..."
	}
	return warning
}

func logStreamWriter(opts Options, buffer *bytes.Buffer, stream string) io.Writer {
	if !opts.Verbose || opts.LogWriter == nil {
		return buffer
	}
	return io.MultiWriter(buffer, &prefixedLineWriter{writer: opts.LogWriter, prefix: "[ocr][" + stream + "] "})
}

type prefixedLineWriter struct {
	writer      io.Writer
	prefix      string
	lineStarted bool
}

func (w *prefixedLineWriter) Write(p []byte) (int, error) {
	for _, b := range p {
		if b == '\r' {
			if w.lineStarted {
				if _, err := w.writer.Write([]byte{'\n'}); err != nil {
					return 0, err
				}
				w.lineStarted = false
			}
			continue
		}
		if !w.lineStarted {
			if _, err := io.WriteString(w.writer, w.prefix); err != nil {
				return 0, err
			}
			w.lineStarted = true
		}
		if _, err := w.writer.Write([]byte{b}); err != nil {
			return 0, err
		}
		if b == '\n' {
			w.lineStarted = false
		}
	}
	return len(p), nil
}

func DockerRunArgs(provider Provider, inputPDF string, outputDir string, opts Options) ([]string, error) {
	if strings.TrimSpace(provider.DockerImage) == "" {
		return nil, fmt.Errorf("ocr engine %q has no docker image", provider.ID)
	}
	absInput, err := filepath.Abs(inputPDF)
	if err != nil {
		return nil, err
	}
	absOutput, err := filepath.Abs(outputDir)
	if err != nil {
		return nil, err
	}
	inputDir := filepath.Dir(absInput)
	inputName := filepath.Base(absInput)
	mountInputDir := dockerHostMountPath(inputDir)
	mountOutputDir := dockerHostMountPath(absOutput)

	args := []string{"run", "--rm"}
	if platform := resolveDockerPlatform(opts.DockerPlatform); platform != "" {
		args = append(args, "--platform", platform)
	}
	if opts.GPU {
		args = append(args, "--gpus", "all")
	}
	args = append(args,
		"-e", boolEnv("OCR_FORMULA", opts.Formula),
		"-e", boolEnv("OCR_CODE", opts.Code),
	)
	if provider.ID == "paddleocr" {
		args = append(args,
			"-e", "OMP_NUM_THREADS=1",
			"-e", "OPENBLAS_NUM_THREADS=1",
			"-e", "CPU_NUM=1",
			"-e", "FLAGS_use_mkldnn=0",
		)
	}
	if cacheDir, err := resolveOCRCacheDir(); err == nil && cacheDir != "" {
		args = append(args,
			"-e", "HF_HOME=/cache/huggingface",
			"-e", "XDG_CACHE_HOME=/cache/xdg",
			"-e", "MODELSCOPE_CACHE=/cache/modelscope",
			"-e", "PADDLE_PDX_CACHE_HOME=/cache/paddlex",
			"-e", "PADDLE_PDX_DISABLE_MODEL_SOURCE_CHECK=True",
			"-v", dockerHostMountPath(cacheDir)+":/cache",
		)
	}
	if provider.ID == "olmocr" {
		for _, envName := range []string{"OLMOCR_SERVER", "OLMOCR_MODEL", "OLMOCR_API_KEY"} {
			if strings.TrimSpace(os.Getenv(envName)) != "" {
				args = append(args, "-e", envName)
			}
		}
	}
	args = append(args,
		"-v", mountInputDir+":/input:ro",
		"-v", mountOutputDir+":/output",
		provider.DockerImage,
		"/input/"+inputName,
		"/output",
	)
	return args, nil
}

func resolveOCRCacheDir() (string, error) {
	if dir := strings.TrimSpace(os.Getenv("MOODLE_OCR_CACHE_DIR")); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return "", err
		}
		return dir, nil
	}
	if home := strings.TrimSpace(os.Getenv("MOODLE_HOME")); home != "" {
		dir := filepath.Join(home, "ocr", "cache")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return "", err
		}
		return dir, nil
	}
	if dir, err := os.UserCacheDir(); err == nil && strings.TrimSpace(dir) != "" {
		dir = filepath.Join(dir, "moodle-services", "ocr")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return "", err
		}
		return dir, nil
	}
	return "", nil
}

func dockerHostMountPath(containerPath string) string {
	hostDataDir := strings.TrimSpace(os.Getenv("MOODLE_DOCKER_HOST_DATA_DIR"))
	if hostDataDir == "" {
		return containerPath
	}
	containerDataDir := strings.TrimSpace(os.Getenv("MOODLE_DOCKER_CONTAINER_DATA_DIR"))
	if containerDataDir == "" {
		containerDataDir = strings.TrimSpace(os.Getenv("MOODLE_HOME"))
	}
	if containerDataDir == "" {
		containerDataDir = "/data"
	}
	containerMatchPath := cleanDockerContainerPath(containerPath)
	containerDataDir = cleanDockerContainerPath(containerDataDir)
	if containerMatchPath == containerDataDir {
		return cleanDockerHostPath(hostDataDir)
	}
	prefix := strings.TrimRight(containerDataDir, "/") + "/"
	if strings.HasPrefix(containerMatchPath, prefix) {
		rel := strings.TrimPrefix(containerMatchPath, prefix)
		return joinDockerHostPath(hostDataDir, rel)
	}
	return containerPath
}

func cleanDockerContainerPath(value string) string {
	cleaned := path.Clean(strings.ReplaceAll(value, "\\", "/"))
	if len(cleaned) >= 3 && cleaned[1] == ':' && cleaned[2] == '/' {
		cleaned = cleaned[2:]
	}
	if cleaned == "" {
		return "."
	}
	return cleaned
}

func cleanDockerHostPath(value string) string {
	if usesSlashPath(value) {
		return path.Clean(value)
	}
	return filepath.Clean(value)
}

func joinDockerHostPath(hostDataDir string, rel string) string {
	if usesSlashPath(hostDataDir) {
		return path.Join(hostDataDir, rel)
	}
	return filepath.Join(hostDataDir, filepath.FromSlash(rel))
}

func usesSlashPath(value string) bool {
	return strings.Contains(value, "/") && !strings.Contains(value, "\\")
}

func resolveDockerPlatform(value string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(os.Getenv("OCR_DOCKER_PLATFORM"))
}

func boolEnv(name string, enabled bool) string {
	if enabled {
		return name + "=1"
	}
	return name + "=0"
}

func exitCodeFromError(err error, timedOut bool) int {
	if timedOut {
		return -1
	}
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return 1
}
