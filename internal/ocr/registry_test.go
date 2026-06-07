package ocr

import "testing"

func TestResolveProvidersAllExcludesOlmOCRWithoutGPU(t *testing.T) {
	providers, err := ResolveProviders(EngineAll, false)
	if err != nil {
		t.Fatalf("ResolveProviders: %v", err)
	}
	foundPdftotext := false
	for _, provider := range providers {
		if provider.ID == "olmocr" {
			t.Fatalf("olmocr should not be enabled by default")
		}
		if provider.ID == "pdftotext" {
			foundPdftotext = true
		}
	}
	if !foundPdftotext {
		t.Fatalf("pdftotext should be enabled by default")
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

func TestProviderByIDPdftotext(t *testing.T) {
	provider, ok := ProviderByID("pdftotext")
	if !ok {
		t.Fatalf("expected pdftotext provider")
	}
	if provider.Runtime != RuntimeLocal {
		t.Fatalf("expected local runtime, got %#v", provider)
	}
	if provider.DockerImage != "" {
		t.Fatalf("pdftotext should not require a docker image: %#v", provider)
	}
}

func TestRunnerRequiresPersistentOutputForAll(t *testing.T) {
	_, err := (Runner{}).Run(t.Context(), []byte("%PDF"), "test.pdf", Options{Engine: EngineAll})
	if err == nil {
		t.Fatal("expected engine all without output directory to fail")
	}
}
