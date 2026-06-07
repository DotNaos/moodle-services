package ocr

import (
	"fmt"
	"strings"
)

const (
	EngineAll     = "all"
	RuntimeDocker = "docker"
	RuntimeLocal  = "local"
)

var defaultProviders = []Provider{
	{
		ID:                  "pdftotext",
		DisplayName:         "Poppler pdftotext",
		Runtime:             RuntimeLocal,
		SupportsCPU:         true,
		SupportsGPU:         false,
		ExtractsImages:      false,
		DefaultTimeoutMs:    60 * 1000,
		ExpectedOutputFiles: []string{"output.md", "text.txt"},
		EnabledByDefault:    true,
	},
	{
		ID:                  "docling",
		DisplayName:         "Docling",
		Runtime:             RuntimeDocker,
		DockerImage:         "moodle-ocr-docling:local",
		BuildContext:        "docker/ocr/docling",
		Dockerfile:          "docker/ocr/docling/Dockerfile",
		SupportsCPU:         true,
		SupportsGPU:         false,
		ExtractsImages:      true,
		DefaultTimeoutMs:    10 * 60 * 1000,
		ExpectedOutputFiles: []string{"output.md", "output.html", "output.json"},
		EnabledByDefault:    true,
	},
	{
		ID:                  "marker",
		DisplayName:         "Marker",
		Runtime:             RuntimeDocker,
		DockerImage:         "moodle-ocr-marker:local",
		BuildContext:        "docker/ocr/marker",
		Dockerfile:          "docker/ocr/marker/Dockerfile",
		SupportsCPU:         true,
		SupportsGPU:         true,
		ExtractsImages:      true,
		DefaultTimeoutMs:    15 * 60 * 1000,
		ExpectedOutputFiles: []string{"output.md"},
		EnabledByDefault:    true,
	},
	{
		ID:                  "paddleocr",
		DisplayName:         "PaddleOCR PP-StructureV3",
		Runtime:             RuntimeDocker,
		DockerImage:         "moodle-ocr-paddleocr:local",
		BuildContext:        "docker/ocr/paddleocr",
		Dockerfile:          "docker/ocr/paddleocr/Dockerfile",
		SupportsCPU:         true,
		SupportsGPU:         true,
		ExtractsImages:      true,
		DefaultTimeoutMs:    20 * 60 * 1000,
		ExpectedOutputFiles: []string{"output.md", "output.json"},
		EnabledByDefault:    true,
	},
	{
		ID:                  "mineru",
		DisplayName:         "MinerU",
		Runtime:             RuntimeDocker,
		DockerImage:         "moodle-ocr-mineru:local",
		BuildContext:        "docker/ocr/mineru",
		Dockerfile:          "docker/ocr/mineru/Dockerfile",
		SupportsCPU:         true,
		SupportsGPU:         true,
		ExtractsImages:      true,
		DefaultTimeoutMs:    20 * 60 * 1000,
		ExpectedOutputFiles: []string{"output.md", "output.json"},
		EnabledByDefault:    true,
	},
	{
		ID:                  "olmocr",
		DisplayName:         "olmOCR",
		Runtime:             RuntimeDocker,
		DockerImage:         "moodle-ocr-olmocr:local",
		BuildContext:        "docker/ocr/olmocr",
		Dockerfile:          "docker/ocr/olmocr/Dockerfile",
		SupportsCPU:         true,
		SupportsGPU:         true,
		ExtractsImages:      true,
		DefaultTimeoutMs:    30 * 60 * 1000,
		ExpectedOutputFiles: []string{"output.md"},
		EnabledByDefault:    false,
	},
}

func Providers() []Provider {
	providers := make([]Provider, len(defaultProviders))
	copy(providers, defaultProviders)
	return providers
}

func ProviderByID(id string) (Provider, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, provider := range defaultProviders {
		if provider.ID == id {
			return provider, true
		}
	}
	return Provider{}, false
}

func ResolveProviders(engine string, gpu bool) ([]Provider, error) {
	engine = strings.ToLower(strings.TrimSpace(engine))
	if engine == "" {
		return nil, fmt.Errorf("ocr engine is required")
	}
	if engine != EngineAll {
		provider, ok := ProviderByID(engine)
		if !ok {
			return nil, fmt.Errorf("unknown ocr engine %q", engine)
		}
		if gpu && !provider.SupportsGPU {
			return nil, fmt.Errorf("ocr engine %q does not support gpu mode", engine)
		}
		return []Provider{provider}, nil
	}

	var providers []Provider
	for _, provider := range defaultProviders {
		if provider.EnabledByDefault || (gpu && provider.SupportsGPU) {
			providers = append(providers, provider)
		}
	}
	return providers, nil
}

func SupportedEngineList() string {
	ids := make([]string, 0, len(defaultProviders)+1)
	for _, provider := range defaultProviders {
		ids = append(ids, provider.ID)
	}
	ids = append(ids, EngineAll)
	return strings.Join(ids, "|")
}
