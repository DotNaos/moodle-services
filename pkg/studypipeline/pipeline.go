package studypipeline

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/DotNaos/moodle-services/internal/moodle"
	contract "github.com/DotNaos/moodle-services/pkg/apicontracts"
)

var firstNumberRe = regexp.MustCompile(`\d+`)

// Build creates a course-local study-material plan from Moodle resources.
// It is intentionally storage-free: callers can use the response as the
// manifest for later raw/extracted/curated jobs.
func Build(courseID string, resources []moodle.Resource, status string, now time.Time) contract.StudyPipelineResponse {
	if status == "" {
		status = "planned"
	}
	materials := make([]contract.StudyPipelineMaterial, 0, len(resources))
	for _, resource := range resources {
		materials = append(materials, toMaterial(resource))
	}
	sort.SliceStable(materials, func(i, j int) bool {
		left := materials[i]
		right := materials[j]
		if left.SectionName != right.SectionName {
			return left.SectionName < right.SectionName
		}
		return left.Name < right.Name
	})

	taskLinks, missingSolutions := linkTasks(materials)
	summary := summarize(materials, taskLinks, missingSolutions)

	return contract.StudyPipelineResponse{
		CourseID:         courseID,
		Status:           status,
		CreatedAt:        now.UTC().Format(time.RFC3339),
		Summary:          summary,
		Materials:        materials,
		TaskLinks:        taskLinks,
		MissingSolutions: missingSolutions,
	}
}

func toMaterial(resource moodle.Resource) contract.StudyPipelineMaterial {
	name := strings.TrimSpace(resource.Name)
	return contract.StudyPipelineMaterial{
		ID:           strings.TrimSpace(resource.ID),
		Name:         name,
		URL:          strings.TrimSpace(resource.URL),
		Type:         classify(resource),
		ResourceType: strings.TrimSpace(resource.Type),
		FileType:     strings.TrimSpace(resource.FileType),
		SectionID:    strings.TrimSpace(resource.SectionID),
		SectionName:  strings.TrimSpace(resource.SectionName),
	}
}

func classify(resource moodle.Resource) string {
	text := normalize(resource.Name + " " + resource.SectionName + " " + resource.FileType)
	switch {
	case containsAny(text, "loesung", "solution", "solutions", "musterloesung", "musterloesungen"):
		return "solution"
	case isTaskLike(text):
		return "task"
	case containsAny(text, "script", "skript", "reader"):
		return "script"
	case containsAny(text, "slide", "slides", "folie", "folien", "vorlesung", "lecture", "praesenz"):
		return "slide"
	case strings.EqualFold(resource.FileType, "pdf"):
		return "slide"
	default:
		return "other"
	}
}

func isTaskLike(text string) bool {
	if containsAny(text,
		"bewertungskriter", "assessment criteria", "beurteilungsraster", "bewertungsraster",
		"semesterinformation", "semesterbeschreibung", "modulbeschreibung", "powerpoint vorlage",
		"template", "vorlage", "guideline", "guidelines", "merkblatt",
	) {
		return false
	}
	return containsAny(text,
		"aufgabe", "aufgaben", "auftrag", "arbeitsauftrag",
		"uebung", "uebungen", "uebungsblatt", "klassenaufgabe",
		"exercise", "exercises", "task", "tasks", "assignment", "homework", "worksheet", "activity", "activities",
		"assessment 1", "assessment 2", "assessment 3", "assessment 4",
		"probepruefung", "probeklausur", "probe klausur", "exam overview", "mid-term exam",
		"fallbeispiel", "case study", "checkliste",
	)
}

func linkTasks(materials []contract.StudyPipelineMaterial) ([]contract.StudyPipelineTaskLink, []contract.StudyPipelineMaterial) {
	solutions := make([]contract.StudyPipelineMaterial, 0)
	for _, material := range materials {
		if material.Type == "solution" {
			solutions = append(solutions, material)
		}
	}

	usedSolutions := map[string]struct{}{}
	links := make([]contract.StudyPipelineTaskLink, 0)
	missing := make([]contract.StudyPipelineMaterial, 0)

	for _, task := range materials {
		if task.Type != "task" {
			continue
		}
		solution := bestSolution(task, solutions, usedSolutions)
		if solution == nil {
			missing = append(missing, task)
			links = append(links, contract.StudyPipelineTaskLink{
				Task:   task,
				Status: "missing-solution",
			})
			continue
		}
		usedSolutions[solution.ID] = struct{}{}
		links = append(links, contract.StudyPipelineTaskLink{
			Task:     task,
			Solution: solution,
			Status:   "linked",
		})
	}

	return links, missing
}

func bestSolution(task contract.StudyPipelineMaterial, solutions []contract.StudyPipelineMaterial, used map[string]struct{}) *contract.StudyPipelineMaterial {
	taskKey := firstNumber(task.Name)
	var fallback *contract.StudyPipelineMaterial
	for i := range solutions {
		solution := &solutions[i]
		if _, ok := used[solution.ID]; ok {
			continue
		}
		sameSection := solution.SectionID != "" && solution.SectionID == task.SectionID
		if taskKey == "" && fallback == nil && sameSection {
			fallback = solution
		}
		if taskKey != "" && taskKey == firstNumber(solution.Name) {
			if sameSection || fallback == nil {
				return solution
			}
		}
	}
	return fallback
}

func summarize(materials []contract.StudyPipelineMaterial, links []contract.StudyPipelineTaskLink, missing []contract.StudyPipelineMaterial) contract.StudyPipelineSummary {
	var summary contract.StudyPipelineSummary
	summary.TotalResources = len(materials)
	summary.MissingSolutions = len(missing)
	for _, material := range materials {
		switch material.Type {
		case "slide":
			summary.Slides++
		case "script":
			summary.Scripts++
		case "task":
			summary.Tasks++
		case "solution":
			summary.Solutions++
		default:
			summary.Other++
		}
	}
	for _, link := range links {
		if link.Status == "linked" {
			summary.LinkedSolutions++
		}
	}
	return summary
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func normalize(value string) string {
	replacer := strings.NewReplacer(
		"ä", "ae",
		"ö", "oe",
		"ü", "ue",
		"Ä", "ae",
		"Ö", "oe",
		"Ü", "ue",
		"ß", "ss",
	)
	return strings.ToLower(replacer.Replace(strings.TrimSpace(value)))
}

func firstNumber(value string) string {
	match := firstNumberRe.FindString(value)
	return strings.TrimLeft(match, "0")
}
