package studypipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DotNaos/moodle-services/internal/moodle"
	"github.com/DotNaos/moodle-services/internal/store"
	contract "github.com/DotNaos/moodle-services/pkg/apicontracts"
)

type curatedAccountabilityReport struct {
	RunID            string
	ArtifactRefs     []store.StudyPipelineArtifactRef
	Checklist        *store.StudyPipelineCurationChecklist
	ElementDecisions []store.StudyPipelineElementDecision
}

type elementAccountabilityManifest struct {
	CourseID         string                               `json:"courseId"`
	RunID            string                               `json:"runId"`
	GeneratedAt      string                               `json:"generatedAt"`
	ExtractedRunID   string                               `json:"extractedRunId,omitempty"`
	TotalElements    int                                  `json:"totalElements"`
	Used             int                                  `json:"used"`
	Ignored          int                                  `json:"ignored"`
	Unsupported      int                                  `json:"unsupported"`
	Failed           int                                  `json:"failed"`
	NeedsReview      int                                  `json:"needsReview"`
	ElementDecisions []store.StudyPipelineElementDecision `json:"elementDecisions"`
}

func buildCuratedAccountability(root string, courseID string, moodleResources []moodle.Resource, resources []contract.StudyPipelineMaterial, now time.Time) (curatedAccountabilityReport, error) {
	if now.IsZero() {
		now = time.Now()
	}
	extracted, ok, err := readLatestExtractedDocuments(root, courseID)
	if err != nil {
		return curatedAccountabilityReport{}, err
	}
	if !ok {
		var err error
		extracted, err = LoadExtractedDocuments(courseID, moodleResources, RunOptions{Root: root, Now: now})
		if err != nil {
			return curatedAccountabilityReport{}, err
		}
	}

	runID := "curated-" + now.UTC().Format("20060102T150405Z")
	runDir := filepath.Join(courseDir(root, courseID), "curated", "accountability", runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return curatedAccountabilityReport{}, err
	}

	outputs := curatedOutputsByResource(root, courseID, resources)
	pageRenderRefs := pageRenderArtifactRefs(root, courseID, runID, extracted.Documents)
	pageRenderByDocumentPage := pageRenderArtifactIDsByDocumentPage(extracted.Documents)
	decisions := make([]store.StudyPipelineElementDecision, 0)
	for _, document := range extracted.Documents {
		output := outputs[document.Resource.ID]
		decisions = append(decisions, blockElementDecisions(document, output, pageRenderByDocumentPage, now)...)
		decisions = append(decisions, assetElementDecisions(document, output, pageRenderByDocumentPage, now)...)
	}

	manifest := elementAccountabilityManifest{
		CourseID:         courseID,
		RunID:            runID,
		GeneratedAt:      now.UTC().Format(time.RFC3339),
		ExtractedRunID:   extracted.RunID,
		TotalElements:    len(decisions),
		ElementDecisions: decisions,
	}
	for _, decision := range decisions {
		switch decision.Outcome {
		case "used_in_output":
			manifest.Used++
		case "ignored":
			manifest.Ignored++
		case "unsupported":
			manifest.Unsupported++
		case "failed":
			manifest.Failed++
		case "needs_review":
			manifest.NeedsReview++
		}
	}

	manifestPath := filepath.Join(runDir, "element-accountability.json")
	checklistPath := filepath.Join(runDir, "checklist.json")
	previewPath := filepath.Join(runDir, "rendered-preview.md")
	if err := writeJSONFile(manifestPath, manifest); err != nil {
		return curatedAccountabilityReport{}, err
	}
	if err := os.WriteFile(previewPath, []byte(curatedPreviewMarkdown(root, courseID, resources)), 0o644); err != nil {
		return curatedAccountabilityReport{}, err
	}

	manifestArtifactID := "artifact:element-accountability:" + courseID + ":" + runID
	checklistArtifactID := "artifact:curation-checklist:" + courseID + ":" + runID
	previewArtifactID := "artifact:rendered-preview:" + courseID + ":" + runID
	checklist := buildCurationChecklist(now, manifest, manifestArtifactID, firstPageRenderArtifactID(pageRenderRefs), previewArtifactID)
	if err := writeJSONFile(checklistPath, checklist); err != nil {
		return curatedAccountabilityReport{}, err
	}

	artifactRefs := make([]store.StudyPipelineArtifactRef, 0, len(pageRenderRefs)+3)
	artifactRefs = append(artifactRefs, pageRenderRefs...)
	artifactRefs = append(artifactRefs,
		store.StudyPipelineArtifactRef{
			ID:         manifestArtifactID,
			Kind:       "element_accountability_manifest",
			StorageKey: storageKeyForPath(root, manifestPath),
			URI:        filepath.ToSlash(manifestPath),
			Metadata: map[string]any{
				"totalElements": manifest.TotalElements,
				"needsReview":   manifest.NeedsReview,
				"failed":        manifest.Failed,
			},
		},
		store.StudyPipelineArtifactRef{
			ID:         checklistArtifactID,
			Kind:       "curation_checklist",
			StorageKey: storageKeyForPath(root, checklistPath),
			URI:        filepath.ToSlash(checklistPath),
			Metadata: map[string]any{
				"status": checklist.Status,
			},
		},
		store.StudyPipelineArtifactRef{
			ID:         previewArtifactID,
			Kind:       "rendered_preview",
			StorageKey: storageKeyForPath(root, previewPath),
			URI:        filepath.ToSlash(previewPath),
		},
	)

	return curatedAccountabilityReport{
		RunID:            runID,
		ArtifactRefs:     artifactRefs,
		Checklist:        checklist,
		ElementDecisions: decisions,
	}, nil
}

func curatedOutputsByResource(root string, courseID string, materials []contract.StudyPipelineMaterial) map[string]string {
	outputs := map[string]string{}
	for _, material := range materials {
		switch material.Type {
		case "task":
			path := filepath.Join(courseDir(root, courseID), "curated", "tasks", safeSegment(taskID(material))+".mdx")
			outputs[material.ID] = readTextFile(path)
		case "solution":
			path := filepath.Join(courseDir(root, courseID), "curated", "tasks", "solutions", safeSegment(taskID(material))+".mdx")
			outputs[material.ID] = readTextFile(path)
		case "slide", "script":
			outputs[material.ID] = readTextFile(filepath.Join(courseDir(root, courseID), "curated", "script", "Script.mdx"))
		}
	}
	return outputs
}

func blockElementDecisions(document contract.PDFDocument, output string, pageRenderByDocumentPage map[string]string, now time.Time) []store.StudyPipelineElementDecision {
	decisions := []store.StudyPipelineElementDecision{}
	for _, page := range document.Pages {
		for _, block := range page.Blocks {
			outcome := "used_in_output"
			reason := "The extracted text block is represented in the curated output."
			confidence := block.Confidence
			if confidence == "" {
				confidence = "medium"
			}
			if !textBlockRepresented(output, block) {
				outcome = "needs_review"
				reason = "The extracted text block was not found in the curated output."
				confidence = "medium"
			}
			if block.Type == "unknown" {
				outcome = "unsupported"
				reason = "The extracted block type is unknown and must be reviewed before richer rendering is possible."
				confidence = "low"
			}
			decisions = append(decisions, store.StudyPipelineElementDecision{
				ID:                        "element-decision:" + block.ID,
				SourceElementID:           block.ID,
				SourceArtifactID:          "artifact:document-block:" + block.ID,
				SourcePageImageArtifactID: pageRenderByDocumentPage[documentPageKey(document.ID, page.PageNumber)],
				OutputArtifactID:          outputArtifactID(document.Resource),
				ElementKind:               pipelineElementKind(block.Type),
				Outcome:                   outcome,
				Reason:                    reason,
				DecidedBy:                 "system",
				Confidence:                confidence,
				PageNumber:                page.PageNumber,
				CreatedAt:                 now.UTC().Format(time.RFC3339),
				Metadata: map[string]any{
					"resourceId": document.Resource.ID,
					"label":      block.Label,
					"source":     block.Source,
				},
			})
		}
	}
	return decisions
}

func assetElementDecisions(document contract.PDFDocument, output string, pageRenderByDocumentPage map[string]string, now time.Time) []store.StudyPipelineElementDecision {
	decisions := []store.StudyPipelineElementDecision{}
	for _, asset := range document.Assets {
		if asset.Kind == "page_preview" {
			continue
		}
		outcome := "needs_review"
		reason := "The extracted visual asset is not referenced in the curated output and must be either placed or intentionally ignored."
		confidence := "medium"
		if assetReferenced(output, asset) {
			outcome = "used_in_output"
			reason = "The extracted visual asset is referenced in the curated output."
			confidence = "high"
		} else if looksDecorativeAsset(document, asset) {
			outcome = "ignored"
			reason = "The visual asset appears to be decorative or template-originated and is not part of the task content."
			confidence = "low"
		}
		decisions = append(decisions, store.StudyPipelineElementDecision{
			ID:                        "element-decision:" + document.ID + ":" + asset.ID,
			SourceElementID:           document.ID + ":" + asset.ID,
			SourceArtifactID:          "artifact:" + asset.Kind + ":" + document.ID + ":" + asset.ID,
			SourceAssetID:             asset.ID,
			SourcePageImageArtifactID: pageRenderByDocumentPage[documentPageKey(document.ID, asset.PageNumber)],
			OutputArtifactID:          outputArtifactID(document.Resource),
			ElementKind:               pipelineElementKind(asset.Kind),
			Outcome:                   outcome,
			Reason:                    reason,
			DecidedBy:                 "system",
			Confidence:                confidence,
			PageNumber:                asset.PageNumber,
			CreatedAt:                 now.UTC().Format(time.RFC3339),
			Metadata: map[string]any{
				"resourceId": document.Resource.ID,
				"path":       asset.Path,
				"role":       asset.Role,
				"mimeType":   asset.MimeType,
			},
		})
	}
	return decisions
}

func buildCurationChecklist(now time.Time, manifest elementAccountabilityManifest, manifestArtifactID string, pageRenderArtifactID string, previewArtifactID string) *store.StudyPipelineCurationChecklist {
	status := "complete"
	accountabilityStatus := "checked"
	accountabilityReason := ""
	layoutStatus := "checked"
	layoutReason := ""
	if manifest.NeedsReview > 0 || manifest.Failed > 0 {
		status = "incomplete"
		accountabilityStatus = "failed"
		accountabilityReason = fmt.Sprintf("%d detected element(s) still need review and %d failed.", manifest.NeedsReview, manifest.Failed)
		layoutStatus = "missing"
		layoutReason = "Layout reconstruction is blocked until all visual and text elements have final outcomes."
	}
	pageStatus := "checked"
	pageReason := ""
	if pageRenderArtifactID == "" {
		status = "incomplete"
		pageStatus = "missing"
		pageReason = "No rendered PDF page image was available as visual evidence."
	}
	extractedStatus := "checked"
	if manifest.TotalElements == 0 {
		status = "incomplete"
		extractedStatus = "missing"
	}
	return &store.StudyPipelineCurationChecklist{
		Status:                  status,
		CheckedBy:               "system",
		CheckedAt:               now.UTC().Format(time.RFC3339),
		RenderPreviewArtifactID: previewArtifactID,
		Items: []store.StudyPipelineCurationChecklistItem{
			{ID: "page_images_reviewed", Label: "Rendered PDF page images were available for review", Status: pageStatus, EvidenceArtifactID: pageRenderArtifactID, Reason: pageReason},
			{ID: "extracted_elements_reviewed", Label: "Extracted PDF elements were inspected", Status: extractedStatus, EvidenceArtifactID: manifestArtifactID},
			{ID: "element_accountability_complete", Label: "Every detected PDF element has a final outcome", Status: accountabilityStatus, EvidenceArtifactID: manifestArtifactID, Reason: accountabilityReason},
			{ID: "layout_reconstructed", Label: "Task layout was reconstructed from the PDF evidence", Status: layoutStatus, EvidenceArtifactID: previewArtifactID, Reason: layoutReason},
			{ID: "rendered_preview_reviewed", Label: "Rendered website preview was generated", Status: "checked", EvidenceArtifactID: previewArtifactID},
			{ID: "source_mapping_complete", Label: "Output source mapping is complete", Status: "checked", EvidenceArtifactID: manifestArtifactID},
		},
	}
}

func pageRenderArtifactRefs(root string, courseID string, runID string, documents []contract.PDFDocument) []store.StudyPipelineArtifactRef {
	refs := []store.StudyPipelineArtifactRef{}
	for _, document := range documents {
		for _, asset := range document.Assets {
			if asset.Kind != "page_preview" {
				continue
			}
			refs = append(refs, store.StudyPipelineArtifactRef{
				ID:         pageRenderArtifactID(document.ID, asset.PageNumber),
				Kind:       "page_render",
				URI:        asset.Path,
				StorageKey: storageKeyForPath(root, asset.Path),
				PageNumber: asset.PageNumber,
				Metadata: map[string]any{
					"documentId": document.ID,
					"resourceId": document.Resource.ID,
					"runId":      runID,
				},
			})
		}
	}
	return refs
}

func pageRenderArtifactIDsByDocumentPage(documents []contract.PDFDocument) map[string]string {
	out := map[string]string{}
	for _, document := range documents {
		for _, asset := range document.Assets {
			if asset.Kind == "page_preview" {
				out[documentPageKey(document.ID, asset.PageNumber)] = pageRenderArtifactID(document.ID, asset.PageNumber)
			}
		}
	}
	return out
}

func curatedPreviewMarkdown(root string, courseID string, materials []contract.StudyPipelineMaterial) string {
	var out strings.Builder
	script := readTextFile(filepath.Join(courseDir(root, courseID), "curated", "script", "Script.mdx"))
	if strings.TrimSpace(script) != "" {
		out.WriteString("# Script Preview\n\n")
		out.WriteString(stripFrontmatter(script))
		out.WriteString("\n\n")
	}
	out.WriteString("# Task Preview\n\n")
	for _, material := range materials {
		if material.Type != "task" {
			continue
		}
		content := readTextFile(filepath.Join(courseDir(root, courseID), "curated", "tasks", safeSegment(taskID(material))+".mdx"))
		if strings.TrimSpace(content) == "" {
			continue
		}
		out.WriteString(stripFrontmatter(content))
		out.WriteString("\n\n")
	}
	return strings.TrimSpace(out.String()) + "\n"
}

func textBlockRepresented(output string, block contract.DocumentBlock) bool {
	needle := normalizeComparable(firstNonEmpty(block.Text, block.Markdown))
	if needle == "" {
		return true
	}
	if len(needle) > 120 {
		needle = needle[:120]
	}
	return strings.Contains(normalizeComparable(output), needle)
}

func assetReferenced(output string, asset contract.DocumentAsset) bool {
	output = strings.ToLower(output)
	path := strings.ToLower(asset.Path)
	return strings.Contains(output, strings.ToLower(asset.ID)) ||
		(path != "" && strings.Contains(output, filepath.Base(path))) ||
		(path != "" && strings.Contains(output, path))
}

func looksDecorativeAsset(document contract.PDFDocument, asset contract.DocumentAsset) bool {
	name := normalize(document.Resource.Name + " " + asset.ID + " " + asset.Path + " " + asset.Role)
	return containsAny(name, "logo", "fhgr", "banner", "header", "footer", "template", "icon")
}

func pipelineElementKind(kind string) string {
	switch strings.TrimSpace(kind) {
	case "heading", "paragraph", "list", "code":
		return "text"
	case "table":
		return "table"
	case "formula":
		return "formula"
	case "caption":
		return "caption"
	case "page_header":
		return "header"
	case "page_footer":
		return "footer"
	case "embedded_image", "image":
		return "image"
	case "figure":
		return "figure"
	case "chart":
		return "chart"
	case "diagram":
		return "diagram"
	default:
		return "unknown"
	}
}

func outputArtifactID(material contract.StudyPipelineMaterial) string {
	switch material.Type {
	case "task":
		return "artifact:task-draft:" + material.ID
	case "solution":
		return "artifact:solution-draft:" + material.ID
	case "slide", "script":
		return "artifact:script-draft:" + material.ID
	default:
		return "artifact:curated-output:" + material.ID
	}
}

func pageRenderArtifactID(documentID string, pageNumber int) string {
	return fmt.Sprintf("artifact:page-render:%s:p%d", documentID, pageNumber)
}

func firstPageRenderArtifactID(refs []store.StudyPipelineArtifactRef) string {
	for _, ref := range refs {
		if ref.Kind == "page_render" {
			return ref.ID
		}
	}
	return ""
}

func documentPageKey(documentID string, pageNumber int) string {
	return fmt.Sprintf("%s:%d", documentID, pageNumber)
}

func storageKeyForPath(root string, path string) string {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return filepath.ToSlash(path)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	if rel, err := filepath.Rel(absRoot, absPath); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}

func readTextFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func normalizeComparable(value string) string {
	value = strings.ToLower(stripMarkdownMarkers(value))
	value = strings.Join(strings.Fields(value), " ")
	return value
}
