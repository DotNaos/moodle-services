package ocr

import "testing"

func TestResolveProvidersAllExcludesOlmOCRWithoutGPU(t *testing.T) {
	providers, err := ResolveProviders(EngineAll, false)
	if err != nil {
		t.Fatalf("ResolveProviders: %v", err)
	}
	for _, provider := range providers {
		if provider.ID == "olmocr" {
			t.Fatalf("olmocr should not be enabled by default")
		}
	}
}

func TestResolveProvidersAllIncludesOlmOCRWithGPU(t *testing.T) {
	providers, err := ResolveProviders(EngineAll, true)
	if err != nil {
		t.Fatalf("ResolveProviders: %v", err)
	}
	for _, provider := range providers {
		if provider.ID == "olmocr" {
			return
		}
	}
	t.Fatalf("expected olmocr when gpu mode is requested")
}

func TestProviderByID(t *testing.T) {
	provider, ok := ProviderByID("docling")
	if !ok {
		t.Fatalf("expected docling provider")
	}
	if provider.DockerImage == "" || provider.DefaultTimeoutMs == 0 {
		t.Fatalf("provider metadata is incomplete: %#v", provider)
	}
}

func TestRunnerRequiresPersistentOutputForAll(t *testing.T) {
	_, err := (Runner{}).Run(t.Context(), []byte("%PDF"), "test.pdf", Options{Engine: EngineAll})
	if err == nil {
		t.Fatal("expected engine all without output directory to fail")
	}
}
