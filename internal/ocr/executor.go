package ocr

import (
	"context"
	"fmt"
)

type DispatchExecutor struct {
	Docker DockerExecutor
	Local  LocalExecutor
}

func (e DispatchExecutor) Run(ctx context.Context, provider Provider, inputPDF string, outputDir string, opts Options) (RunResult, error) {
	switch provider.Runtime {
	case RuntimeLocal:
		return e.Local.Run(ctx, provider, inputPDF, outputDir, opts)
	case "", RuntimeDocker:
		return e.Docker.Run(ctx, provider, inputPDF, outputDir, opts)
	default:
		return RunResult{}, fmt.Errorf("ocr engine %q has unsupported runtime %q", provider.ID, provider.Runtime)
	}
}
