package studypipeline

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/DotNaos/moodle-services/internal/moodle"
	contract "github.com/DotNaos/moodle-services/pkg/apicontracts"
)

func LoadInventory(courseID string, resources []moodle.Resource, options RunOptions) (contract.CourseInventoryResponse, error) {
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}
	root := strings.TrimSpace(options.Root)
	if root == "" {
		root = ArtifactRootFromEnv()
	}

	inventory := BuildInventory(courseID, resources, now)
	inventory.ArtifactRoot = courseDir(root, courseID)
	if err := writeInventory(root, courseID, inventory); err != nil {
		return contract.CourseInventoryResponse{}, err
	}
	return inventory, nil
}

func BuildInventory(courseID string, resources []moodle.Resource, now time.Time) contract.CourseInventoryResponse {
	if now.IsZero() {
		now = time.Now()
	}
	nodes := make([]contract.CourseInventoryNode, 0, len(resources))
	for _, resource := range resources {
		nodes = append(nodes, inventoryNode(resource))
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		left := nodes[i]
		right := nodes[j]
		if left.SectionName != right.SectionName {
			return left.SectionName < right.SectionName
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		return left.ID < right.ID
	})

	var lectures []contract.CourseInventoryNode
	var references []contract.CourseInventoryNode
	var interactions []contract.CourseInventoryNode
	var unknown []contract.CourseInventoryNode
	var sheets []contract.CourseInventoryNode
	var solutions []contract.CourseInventoryNode
	for _, node := range nodes {
		switch node.Bucket {
		case "task_group":
			if node.Role == "solution" {
				solutions = append(solutions, node)
			} else {
				sheets = append(sheets, node)
			}
		case "lecture_material":
			lectures = append(lectures, node)
		case "reference":
			references = append(references, node)
		case "interaction":
			interactions = append(interactions, node)
		default:
			unknown = append(unknown, node)
		}
	}

	taskGroups := buildInventoryTaskGroups(sheets, solutions)
	summary := contract.CourseInventorySummary{
		TotalResources:  len(nodes),
		LectureMaterial: len(lectures),
		TaskGroups:      len(taskGroups),
		References:      len(references),
		Interactions:    len(interactions),
		Unknown:         len(unknown),
	}
	for _, group := range taskGroups {
		switch group.PairingStatus {
		case "paired":
			summary.PairedTaskGroups++
		case "missing_solution":
			summary.MissingSolutionGroups++
		case "ambiguous_solution":
			summary.AmbiguousTaskGroups++
		}
	}

	return contract.CourseInventoryResponse{
		CourseID:        courseID,
		GeneratedAt:     now.UTC().Format(time.RFC3339),
		Summary:         summary,
		LectureMaterial: lectures,
		TaskGroups:      taskGroups,
		References:      references,
		Interactions:    interactions,
		Unknown:         unknown,
	}
}

func inventoryNode(resource moodle.Resource) contract.CourseInventoryNode {
	material := toMaterial(resource)
	bucket, role, reason, confidence := inventoryBucket(resource, material.Type)
	return contract.CourseInventoryNode{
		ID:           material.ID,
		Name:         material.Name,
		URL:          material.URL,
		Type:         material.Type,
		ResourceType: material.ResourceType,
		FileType:     material.FileType,
		SectionID:    material.SectionID,
		SectionName:  material.SectionName,
		Bucket:       bucket,
		Role:         role,
		Reason:       reason,
		Confidence:   confidence,
	}
}

func inventoryBucket(resource moodle.Resource, materialType string) (bucket string, role string, reason string, confidence string) {
	text := normalize(resource.Name + " " + resource.SectionName + " " + resource.Type + " " + resource.FileType)
	switch materialType {
	case "task":
		return "task_group", "sheet", "classified as task-like material from title, section, or file metadata", "high"
	case "solution":
		return "task_group", "solution", "title or metadata contains solution marker", "high"
	}
	switch {
	case isReferenceLike(text):
		return "reference", "course_reference", "title or metadata describes course information, criteria, or templates", "medium"
	case isInteractionLike(text):
		return "interaction", "course_interaction", "resource looks like forum, assignment, meeting, or tool activity", "medium"
	}
	switch materialType {
	case "script":
		return "lecture_material", "script_source", "title or metadata contains script/reader marker", "high"
	case "slide":
		if strings.EqualFold(resource.FileType, "pdf") && !containsAny(text, "teil", "slide", "slides", "folie", "folien", "vorlesung", "lecture", "praesenz") {
			return "lecture_material", "lecture_source", "pdf resource defaults to lecture material", "medium"
		}
		return "lecture_material", "lecture_source", "title or metadata contains lecture/slide marker", "high"
	}
	return "unknown", "unknown", "no confident inventory bucket matched", "low"
}

func buildInventoryTaskGroups(sheets []contract.CourseInventoryNode, solutions []contract.CourseInventoryNode) []contract.CourseInventoryTaskGroup {
	usedSolutions := map[string]struct{}{}
	groups := make([]contract.CourseInventoryTaskGroup, 0, len(sheets))
	for _, sheet := range sheets {
		candidates := matchingSolutionCandidates(sheet, solutions, usedSolutions)
		group := contract.CourseInventoryTaskGroup{
			ID:                 taskGroupID(sheet),
			Title:              taskGroupTitle(sheet),
			Sheet:              sheet,
			SolutionCandidates: candidates,
			PairingStatus:      "missing_solution",
			PairingReason:      "no solution candidate matched the task sheet number or section",
			PairingConfidence:  "low",
		}
		switch len(candidates) {
		case 0:
		case 1:
			candidate := candidates[0]
			group.Solution = &candidate
			group.PairingStatus = "paired"
			group.PairingReason = "matched by normalized sheet number and/or Moodle section"
			group.PairingConfidence = "high"
			usedSolutions[candidate.ID] = struct{}{}
			group.SolutionCandidates = nil
		default:
			group.PairingStatus = "ambiguous_solution"
			group.PairingReason = "multiple unused solution candidates matched the task sheet"
			group.PairingConfidence = "low"
		}
		groups = append(groups, group)
	}
	return groups
}

func matchingSolutionCandidates(sheet contract.CourseInventoryNode, solutions []contract.CourseInventoryNode, used map[string]struct{}) []contract.CourseInventoryNode {
	sheetNumber := firstNumber(sheet.Name)
	candidates := make([]contract.CourseInventoryNode, 0)
	sectionFallbacks := make([]contract.CourseInventoryNode, 0)
	for _, solution := range solutions {
		if _, ok := used[solution.ID]; ok {
			continue
		}
		sameSection := solution.SectionID != "" && solution.SectionID == sheet.SectionID
		if sheetNumber != "" && sheetNumber == firstNumber(solution.Name) {
			candidates = append(candidates, solution)
			continue
		}
		if sheetNumber == "" && sameSection {
			sectionFallbacks = append(sectionFallbacks, solution)
		}
	}
	if len(candidates) > 0 {
		return candidates
	}
	if len(sectionFallbacks) == 1 {
		return sectionFallbacks
	}
	return nil
}

func isReferenceLike(text string) bool {
	return containsAny(text,
		"modulbeschreibung", "semesterinformation", "semesterbeschreibung",
		"bewertungskriter", "bewertungskriterien", "beurteilungsraster", "bewertungsraster",
		"powerpoint vorlage", "template", "vorlage", "guideline", "guidelines", "merkblatt",
	)
}

func isInteractionLike(text string) bool {
	return containsAny(text,
		"forum", "abgabe", "submission", "assignment submission", "webex", "lti", "meeting", "zoom", "teams",
	)
}

func taskGroupID(sheet contract.CourseInventoryNode) string {
	if number := firstNumber(sheet.Name); number != "" {
		return "task-group-" + number
	}
	return "task-group-" + safeSegment(sheet.ID+"-"+sheet.Name)
}

func taskGroupTitle(sheet contract.CourseInventoryNode) string {
	if number := firstNumber(sheet.Name); number != "" {
		return "Aufgabenblatt " + number
	}
	return sheet.Name
}

func writeInventory(root string, courseID string, inventory contract.CourseInventoryResponse) error {
	dir := filepath.Join(courseDir(root, courseID), "inventory")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return writeJSONFile(filepath.Join(dir, "course-inventory.json"), inventory)
}
