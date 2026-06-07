package ocr

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Executor interface {
	Run(ctx context.Context, provider Provider, inputPDF string, outputDir string, opts Options) (RunResult, error)
}

type Runner struct {
	Executor Executor
}

func (r Runner) Run(ctx context.Context, pdfData []byte, filename string, opts Options) (any, error) {
	providers, err := ResolveProviders(opts.Engine, opts.GPU)
	if err != nil {
		return nil, err
	}
	progressf(opts, "resolved engines: %s", providerIDs(providers))
	executor := r.Executor
	if executor == nil {
		executor = DispatchExecutor{}
	}
	if strings.EqualFold(opts.Engine, EngineAll) && strings.TrimSpace(opts.OutputDir) == "" && !opts.KeepArtifacts {
		return nil, fmt.Errorf("ocr engine all requires --out or --keep-artifacts")
	}

	workParent, err := runtimeWorkParent()
	if err != nil {
		return nil, err
	}
	workDir, err := os.MkdirTemp(workParent, "moodle-services-ocr-input-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(workDir)

	filename = sanitizeInputFilename(filename)
	inputPath := filepath.Join(workDir, filename)
	progressf(opts, "writing input PDF: %s", inputPath)
	if err := os.WriteFile(inputPath, pdfData, 0o600); err != nil {
		return nil, err
	}

	baseOutputDir, cleanup, err := resolveBaseOutputDir(opts.OutputDir)
	if err != nil {
		return nil, err
	}
	progressf(opts, "output directory: %s", baseOutputDir)
	if cleanup != nil && !opts.KeepArtifacts {
		defer cleanup()
	}

	if strings.EqualFold(opts.Engine, EngineAll) {
		return r.runAll(ctx, providers, inputPath, baseOutputDir, opts)
	}
	outputDir := baseOutputDir
	result, err := executor.Run(ctx, providers[0], inputPath, outputDir, opts)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r Runner) runAll(ctx context.Context, providers []Provider, inputPath string, baseOutputDir string, opts Options) (RunAllResult, error) {
	executor := r.Executor
	if executor == nil {
		executor = DispatchExecutor{}
	}
	basename := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	root := filepath.Join(baseOutputDir, basename)
	results := make([]RunResult, 0, len(providers))
	for _, provider := range providers {
		outputDir := filepath.Join(root, provider.ID)
		progressf(opts, "running engine %s -> %s", provider.ID, outputDir)
		result, err := executor.Run(ctx, provider, inputPath, outputDir, opts)
		if err != nil {
			return RunAllResult{}, err
		}
		results = append(results, result)
	}
	comparison := BuildComparison(root, results)
	if err := WriteComparisonMarkdown(comparison); err != nil {
		return RunAllResult{}, err
	}
	return RunAllResult{Action: "ocr", Results: results, Comparison: comparison}, nil
}

func progressf(opts Options, format string, args ...any) {
	if !opts.Verbose || opts.LogWriter == nil {
		return
	}
	_, _ = fmt.Fprintf(opts.LogWriter, "[ocr] "+format+"\n", args...)
}

func providerIDs(providers []Provider) string {
	ids := make([]string, 0, len(providers))
	for _, provider := range providers {
		ids = append(ids, provider.ID)
	}
	return strings.Join(ids, ",")
}

func resolveBaseOutputDir(outputDir string) (string, func(), error) {
	outputDir = strings.TrimSpace(outputDir)
	if outputDir != "" {
		return outputDir, nil, os.MkdirAll(outputDir, 0o755)
	}
	workParent, err := runtimeWorkParent()
	if err != nil {
		return "", nil, err
	}
	tempDir, err := os.MkdirTemp(workParent, "moodle-services-ocr-output-*")
	if err != nil {
		return "", nil, err
	}
	return tempDir, func() { _ = os.RemoveAll(tempDir) }, nil
}

func runtimeWorkParent() (string, error) {
	if dir := strings.TrimSpace(os.Getenv("MOODLE_OCR_WORK_DIR")); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return "", err
		}
		return dir, nil
	}
	if home := strings.TrimSpace(os.Getenv("MOODLE_HOME")); home != "" {
		dir := filepath.Join(home, "ocr", "runtime")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return "", err
		}
		return dir, nil
	}
	return "", nil
}

func sanitizeInputFilename(filename string) string {
	filename = strings.TrimSpace(filepath.Base(filename))
	if filename == "" || filename == "." || filename == string(filepath.Separator) {
		return "input.pdf"
	}
	if !strings.EqualFold(filepath.Ext(filename), ".pdf") {
		filename += ".pdf"
	}
	return filename
}

func BuildComparison(root string, results []RunResult) Comparison {
	rows := make([]ComparisonRow, 0, len(results))
	for _, result := range results {
		rows = append(rows, ComparisonRow{
			Engine:             result.Engine,
			Status:             result.Status,
			DurationMs:         result.DurationMs,
			OutputFiles:        result.OutputFiles,
			Warnings:           result.Warnings,
			MarkdownCharacters: len([]rune(result.Markdown)),
			ImageCount:         len(result.Images),
		})
	}
	return Comparison{Path: filepath.Join(root, "comparison.md"), Results: rows}
}

func WriteComparisonMarkdown(comparison Comparison) error {
	if strings.TrimSpace(comparison.Path) == "" {
		return nil
	}
	var builder strings.Builder
	builder.WriteString("# OCR comparison\n\n")
	builder.WriteString("| Engine | Status | Duration | Markdown chars | Images | Warnings | Output files |\n")
	builder.WriteString("| --- | --- | ---: | ---: | ---: | --- | --- |\n")
	for _, row := range comparison.Results {
		builder.WriteString(fmt.Sprintf(
			"| %s | %s | %d ms | %d | %d | %s | %s |\n",
			escapeTable(row.Engine),
			escapeTable(row.Status),
			row.DurationMs,
			row.MarkdownCharacters,
			row.ImageCount,
			escapeTable(formatComparisonWarnings(row.Warnings)),
			escapeTable(formatComparisonFiles(row.OutputFiles)),
		))
	}
	if err := os.MkdirAll(filepath.Dir(comparison.Path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(comparison.Path, []byte(builder.String()), 0o644)
}

func escapeTable(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.ReplaceAll(value, "|", "\\|")
}

func formatComparisonWarnings(warnings []string) string {
	if len(warnings) == 0 {
		return ""
	}
	value := strings.Join(warnings, "; ")
	value = strings.Join(strings.Fields(value), " ")
	const limit = 500
	if len([]rune(value)) > limit {
		runes := []rune(value)
		return string(runes[:limit]) + "..."
	}
	return value
}

func formatComparisonFiles(files []string) string {
	if len(files) == 0 {
		return ""
	}
	const maxFiles = 8
	kept := files
	if len(files) > maxFiles {
		kept = files[:maxFiles]
	}
	value := strings.Join(kept, ", ")
	if len(files) > maxFiles {
		value = fmt.Sprintf("%d files total, showing first %d: %s", len(files), maxFiles, value)
	}
	const limit = 500
	if len([]rune(value)) > limit {
		runes := []rune(value)
		return string(runes[:limit]) + "..."
	}
	return value
}
