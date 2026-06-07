package studypipeline

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	contract "github.com/DotNaos/moodle-services/pkg/apicontracts"
)

type Options struct {
	Workspace string
	Term      string
	Course    string
}

func Scan(opts Options) (contract.StudyPipelineResponse, error) {
	workspace, err := resolveWorkspace(opts.Workspace)
	if err != nil {
		return contract.StudyPipelineResponse{}, err
	}

	terms, err := resolveTerms(workspace, opts.Term)
	if err != nil {
		return contract.StudyPipelineResponse{}, err
	}

	var courses []contract.StudyPipelineCourse
	for _, term := range terms {
		termCourses, err := scanTerm(workspace, term, strings.TrimSpace(opts.Course))
		if err != nil {
			return contract.StudyPipelineResponse{}, err
		}
		courses = append(courses, termCourses...)
	}

	sort.Slice(courses, func(i, j int) bool {
		if courses[i].Term != courses[j].Term {
			return courses[i].Term < courses[j].Term
		}
		return courses[i].Slug < courses[j].Slug
	})

	return contract.StudyPipelineResponse{
		Workspace: workspace,
		Term:      strings.TrimSpace(opts.Term),
		Summary:   summarize(courses),
		Courses:   courses,
	}, nil
}

func resolveWorkspace(value string) (string, error) {
	workspace := strings.TrimSpace(value)
	if workspace == "" {
		workspace = strings.TrimSpace(os.Getenv("MOODLE_STUDY_WORKSPACE"))
	}
	if workspace == "" {
		workspace = strings.TrimSpace(os.Getenv("SCHOOL_WORKSPACE"))
	}
	if workspace == "" {
		return "", fmt.Errorf("workspace is required; pass ?workspace=/path/to/school or set MOODLE_STUDY_WORKSPACE")
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("workspace %q is not readable: %w", abs, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workspace %q is not a directory", abs)
	}
	return abs, nil
}

func resolveTerms(workspace string, value string) ([]string, error) {
	if term := strings.TrimSpace(value); term != "" {
		if !dirExists(filepath.Join(workspace, "terms", term)) {
			return nil, fmt.Errorf("term %q does not exist under %s", term, filepath.Join(workspace, "terms"))
		}
		return []string{term}, nil
	}

	entries, err := os.ReadDir(filepath.Join(workspace, "terms"))
	if err != nil {
		return nil, fmt.Errorf("read terms: %w", err)
	}
	var terms []string
	for _, entry := range entries {
		if entry.IsDir() {
			terms = append(terms, entry.Name())
		}
	}
	sort.Strings(terms)
	return terms, nil
}

func scanTerm(workspace string, term string, courseFilter string) ([]contract.StudyPipelineCourse, error) {
	coursesRoot := filepath.Join(workspace, "terms", term, "courses")
	entries, err := os.ReadDir(coursesRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read courses for %s: %w", term, err)
	}

	var courses []contract.StudyPipelineCourse
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		slug := entry.Name()
		if courseFilter != "" && slug != courseFilter {
			continue
		}
		course, err := scanCourse(workspace, term, slug, filepath.Join(coursesRoot, slug))
		if err != nil {
			return nil, err
		}
		courses = append(courses, course)
	}
	return courses, nil
}

func scanCourse(workspace string, term string, slug string, coursePath string) (contract.StudyPipelineCourse, error) {
	relPath, _ := filepath.Rel(workspace, coursePath)
	relPath = filepath.ToSlash(relPath)

	raw := scanRaw(coursePath)
	extracted := scanExtracted(coursePath)
	curated := scanCurated(coursePath)
	reader := contract.StudyPipelineReaderStatus{
		Supported: curated.Script.Exists || curated.Tasks.Files > 0,
		URL:       "/?mode=reader&course=" + slug,
	}

	course := contract.StudyPipelineCourse{
		Term:      term,
		Slug:      slug,
		Title:     readCourseTitle(coursePath, slug),
		Path:      relPath,
		Raw:       raw,
		Extracted: extracted,
		Curated:   curated,
		Reader:    reader,
	}
	course.QualityGates = qualityGates(course)
	course.Status = courseStatus(course.QualityGates)
	course.Issues = failedGateLabels(course.QualityGates)
	course.UpdatedAt = latestModTime(coursePath).UTC().Format(time.RFC3339)
	return course, nil
}

func scanRaw(coursePath string) contract.StudyPipelineRawStage {
	materialsRoot := filepath.Join(coursePath, ".raw", "materials")
	required := requiredFiles(coursePath, ".raw/Moodle.md", ".raw/materials.index.yaml")
	materials := countFiles(materialsRoot, nil)
	status := stageStatus(required)
	if status == "complete" && materials.Files == 0 {
		status = "partial"
	}
	return contract.StudyPipelineRawStage{
		Status:       status,
		MoodleMD:     fileStatus(coursePath, ".raw/Moodle.md"),
		MaterialsYML: fileStatus(coursePath, ".raw/materials.index.yaml"),
		Materials:    materials,
	}
}

func scanExtracted(coursePath string) contract.StudyPipelineExtractedStage {
	return contract.StudyPipelineExtractedStage{
		Status:    stageStatus(requiredFiles(coursePath, ".extracted/script/Script.mdx")),
		Script:    fileStatus(coursePath, ".extracted/script/Script.mdx"),
		Slides:    countMDX(filepath.Join(coursePath, ".extracted", "slides")),
		Tasks:     countMDX(filepath.Join(coursePath, ".extracted", "tasks")),
		Solutions: countMDX(filepath.Join(coursePath, ".extracted", "solutions")),
		Assets:    countAssetFiles(filepath.Join(coursePath, ".extracted")),
	}
}

func scanCurated(coursePath string) contract.StudyPipelineCuratedStage {
	tasksRoot := filepath.Join(coursePath, "tasks")
	taskFiles := countMDX(tasksRoot)
	solutions := countMDX(filepath.Join(tasksRoot, "solutions"))
	solutionStates := readTaskSolutionStates(tasksRoot)
	staleFiles := staleCuratedFiles(coursePath)
	status := "missing"
	if dirExists(filepath.Join(coursePath, "script")) || dirExists(tasksRoot) {
		status = "partial"
	}
	if fileExists(filepath.Join(coursePath, "script", "Script.mdx")) && taskFiles.Files > 0 {
		status = "complete"
	}
	if len(staleFiles) > 0 && status == "complete" {
		status = "stale"
	}
	return contract.StudyPipelineCuratedStage{
		Status:         status,
		Script:         fileStatus(coursePath, "script/Script.mdx"),
		Tasks:          taskFiles,
		Solutions:      solutions,
		SolutionStates: solutionStates,
		StaleFiles:     staleFiles,
	}
}

func requiredFiles(coursePath string, files ...string) []contract.StudyPipelineFileStatus {
	statuses := make([]contract.StudyPipelineFileStatus, 0, len(files))
	for _, file := range files {
		statuses = append(statuses, fileStatus(coursePath, file))
	}
	return statuses
}

func fileStatus(coursePath string, rel string) contract.StudyPipelineFileStatus {
	abs := filepath.Join(coursePath, filepath.FromSlash(rel))
	info, err := os.Stat(abs)
	if err != nil || info.IsDir() {
		return contract.StudyPipelineFileStatus{Path: rel, Exists: false}
	}
	return contract.StudyPipelineFileStatus{Path: rel, Exists: true, SizeBytes: info.Size(), ModTime: info.ModTime().UTC().Format(time.RFC3339)}
}

func stageStatus(files []contract.StudyPipelineFileStatus) string {
	missing := 0
	for _, file := range files {
		if !file.Exists {
			missing++
		}
	}
	switch {
	case missing == 0:
		return "complete"
	case missing == len(files):
		return "missing"
	default:
		return "partial"
	}
}

func countMDX(root string) contract.StudyPipelineFileCount {
	return countFiles(root, func(path string, entry fs.DirEntry) bool {
		return !entry.IsDir() && strings.EqualFold(filepath.Ext(path), ".mdx")
	})
}

func countAssetFiles(root string) int {
	return countFiles(root, func(path string, entry fs.DirEntry) bool {
		return !entry.IsDir() && strings.Contains(filepath.ToSlash(path), ".assets/")
	}).Files
}

func countFiles(root string, include func(path string, entry fs.DirEntry) bool) contract.StudyPipelineFileCount {
	var count contract.StudyPipelineFileCount
	if !dirExists(root) {
		return count
	}
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		if include != nil && !include(path, entry) {
			return nil
		}
		count.Files++
		if info, statErr := entry.Info(); statErr == nil {
			count.Bytes += info.Size()
		}
		return nil
	})
	return count
}

func readTaskSolutionStates(tasksRoot string) map[string]int {
	states := map[string]int{}
	entries, err := os.ReadDir(tasksRoot)
	if err != nil {
		return states
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".mdx") || entry.Name() == "Tasks.mdx" {
			continue
		}
		state := frontmatterValue(filepath.Join(tasksRoot, entry.Name()), "solution_status")
		if state == "" {
			state = "unknown"
		}
		states[state]++
	}
	return states
}

func staleCuratedFiles(coursePath string) []string {
	candidates := map[string]string{
		"script/Script.mdx": ".extracted/script/Script.mdx",
	}
	for _, rel := range listDirectMDX(filepath.Join(coursePath, "tasks")) {
		if rel == "Tasks.mdx" {
			continue
		}
		candidates[filepath.ToSlash(filepath.Join("tasks", rel))] = filepath.ToSlash(filepath.Join(".extracted", "tasks", rel))
	}

	var stale []string
	for curatedRel, sourceRel := range candidates {
		curatedInfo, curatedErr := os.Stat(filepath.Join(coursePath, filepath.FromSlash(curatedRel)))
		sourceInfo, sourceErr := os.Stat(filepath.Join(coursePath, filepath.FromSlash(sourceRel)))
		if curatedErr == nil && sourceErr == nil && sourceInfo.ModTime().After(curatedInfo.ModTime()) {
			stale = append(stale, curatedRel)
		}
	}
	sort.Strings(stale)
	return stale
}

func listDirectMDX(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".mdx") {
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)
	return files
}

func qualityGates(course contract.StudyPipelineCourse) []contract.StudyPipelineQualityGate {
	return []contract.StudyPipelineQualityGate{
		gate("raw-index", "Raw Moodle snapshot and material index exist", course.Raw.MoodleMD.Exists && course.Raw.MaterialsYML.Exists),
		gate("raw-materials", "Raw material files are present", course.Raw.Materials.Files > 0),
		gate("extracted-script", "Extracted machine script exists", course.Extracted.Script.Exists),
		gate("extracted-slides", "Extracted slide decks exist", course.Extracted.Slides.Files > 0),
		gate("extracted-tasks", "Extracted task sheets exist", course.Extracted.Tasks.Files > 0),
		gate("curated-script", "Curated script exists", course.Curated.Script.Exists),
		gate("curated-tasks", "Curated task sheets exist", course.Curated.Tasks.Files > 0),
		gate("solutions-linked", "Task solution states are explicit", len(course.Curated.SolutionStates) > 0 && course.Curated.SolutionStates["unknown"] == 0),
		gate("reader-ready", "Reader can link to this course", course.Reader.Supported),
		gate("not-stale", "Curated material is not older than extracted sources", len(course.Curated.StaleFiles) == 0),
	}
}

func gate(id string, label string, passed bool) contract.StudyPipelineQualityGate {
	return contract.StudyPipelineQualityGate{ID: id, Label: label, Passed: passed}
}

func courseStatus(gates []contract.StudyPipelineQualityGate) string {
	failed := 0
	for _, gate := range gates {
		if !gate.Passed {
			failed++
		}
	}
	switch {
	case failed == 0:
		return "complete"
	case failed == len(gates):
		return "missing"
	default:
		return "partial"
	}
}

func failedGateLabels(gates []contract.StudyPipelineQualityGate) []string {
	var labels []string
	for _, gate := range gates {
		if !gate.Passed {
			labels = append(labels, gate.Label)
		}
	}
	return labels
}

func summarize(courses []contract.StudyPipelineCourse) contract.StudyPipelineSummary {
	var summary contract.StudyPipelineSummary
	summary.Courses = len(courses)
	for _, course := range courses {
		switch course.Status {
		case "complete":
			summary.Complete++
		case "missing":
			summary.Missing++
		default:
			summary.Partial++
		}
		summary.RawMaterials += course.Raw.Materials.Files
		summary.ExtractedFiles += course.Extracted.Slides.Files + course.Extracted.Tasks.Files + course.Extracted.Solutions.Files
		summary.CuratedFiles += course.Curated.Tasks.Files + course.Curated.Solutions.Files
		if course.Curated.Script.Exists {
			summary.CuratedFiles++
		}
	}
	return summary
}

func readCourseTitle(coursePath string, fallback string) string {
	for _, rel := range []string{"README.md", "script/Script.mdx"} {
		title := markdownTitle(filepath.Join(coursePath, rel))
		if title != "" {
			return title
		}
	}
	return fallback
}

func markdownTitle(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	return ""
}

func frontmatterValue(path string, key string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	inFrontmatter := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			}
			return ""
		}
		if !inFrontmatter {
			return ""
		}
		prefix := key + ":"
		if strings.HasPrefix(line, prefix) {
			return strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, prefix)), `"'`)
		}
	}
	return ""
}

func latestModTime(root string) time.Time {
	var latest time.Time
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		if info, statErr := entry.Info(); statErr == nil && info.ModTime().After(latest) {
			latest = info.ModTime()
		}
		return nil
	})
	return latest
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
