package cli

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/DotNaos/moodle-services/internal/moodle"
	"gopkg.in/yaml.v3"
)

const (
	exportDriveRootName    = "fhgr-moodle-export"
	exportIndexFile        = "export.index.yaml"
	exportStatusComplete   = "complete"
	exportStatusIncomplete = "incomplete"
)

type schoolExportConfig struct {
	CurrentTerm string `yaml:"current_term"`
	Timezone    string `yaml:"timezone"`
	Moodle      struct {
		SyncEnabled         bool              `yaml:"sync_enabled"`
		CourseSlugOverrides map[string]string `yaml:"course_slug_overrides"`
	} `yaml:"moodle"`
}

type exportIndex struct {
	GeneratedAt     string                       `yaml:"generated_at,omitempty"`
	GoogleDriveRoot string                       `yaml:"google_drive_root,omitempty"`
	Semesters       map[string]exportSemesterRef `yaml:"semesters"`
}

type exportSemesterRef struct {
	LatestRun           string                          `yaml:"latest_run,omitempty"`
	Status              string                          `yaml:"status,omitempty"`
	UpdatedAt           string                          `yaml:"updated_at,omitempty"`
	GoogleDriveFolderID string                          `yaml:"google_drive_folder_id,omitempty"`
	GoogleDriveLink     string                          `yaml:"google_drive_link,omitempty"`
	Calendar            *exportCalendarIndex            `yaml:"calendar,omitempty"`
	Courses             map[string]exportCourseManifest `yaml:"courses,omitempty"`
}

type exportCourse struct {
	ID        int
	Fullname  string
	Shortname string
	Category  string
	ViewURL   string
	Slug      string
	Title     string
	Dir       string
}

type exportRunContext struct {
	Semester         string
	RunID            string
	GitHubRunID      string
	GitHubRunAttempt string
	StartedAt        time.Time
	Workspace        string
}

func loadSchoolExportConfig(root string) (schoolExportConfig, error) {
	data, err := os.ReadFile(filepath.Join(root, "school.yaml"))
	if err != nil {
		return schoolExportConfig{}, err
	}
	var cfg schoolExportConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return schoolExportConfig{}, err
	}
	if strings.TrimSpace(cfg.CurrentTerm) == "" {
		return schoolExportConfig{}, fmt.Errorf("school.yaml requires current_term")
	}
	if cfg.Moodle.CourseSlugOverrides == nil {
		cfg.Moodle.CourseSlugOverrides = map[string]string{}
	}
	return cfg, nil
}

func loadExportIndex(root string) (exportIndex, error) {
	path := filepath.Join(root, exportIndexFile)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return exportIndex{Semesters: map[string]exportSemesterRef{}}, nil
	}
	if err != nil {
		return exportIndex{}, err
	}
	var index exportIndex
	if err := yaml.Unmarshal(data, &index); err != nil {
		return exportIndex{}, err
	}
	if index.Semesters == nil {
		index.Semesters = map[string]exportSemesterRef{}
	}
	return index, nil
}

func writeExportIndex(root string, index exportIndex) error {
	if index.Semesters == nil {
		index.Semesters = map[string]exportSemesterRef{}
	}
	data, err := yaml.Marshal(index)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, exportIndexFile), data, 0o644)
}

func buildExportRunContext(root string, semester string, now time.Time) exportRunContext {
	started := now.UTC().Truncate(time.Second)
	runID := envOrDefault("GITHUB_RUN_ID", "local")
	attempt := envOrDefault("GITHUB_RUN_ATTEMPT", "1")
	return exportRunContext{
		Semester:         semester,
		RunID:            started.Format("2006-01-02-150405") + "-" + runID,
		GitHubRunID:      runID,
		GitHubRunAttempt: attempt,
		StartedAt:        started,
		Workspace:        root,
	}
}

func envOrDefault(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

var semesterDirPattern = regexp.MustCompile(`^(FS|HS)\d{2}$`)

func semestersToProcess(root string, cfg schoolExportConfig, index exportIndex, explicit string) ([]string, error) {
	if explicit != "" {
		return []string{explicit}, nil
	}
	selected := []string{cfg.CurrentTerm}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !semesterDirPattern.MatchString(entry.Name()) || entry.Name() == cfg.CurrentTerm {
			continue
		}
		ref, ok := index.Semesters[entry.Name()]
		if !ok || ref.Status != exportStatusComplete {
			selected = append(selected, entry.Name())
		}
	}
	sort.Strings(selected[1:])
	return selected, nil
}

func exportCoursesForSemester(client *moodle.Client, root string, cfg schoolExportConfig, semester string) ([]exportCourse, error) {
	courses, err := client.FetchCourses()
	if err != nil {
		return nil, err
	}
	result := make([]exportCourse, 0)
	for _, course := range courses {
		if !courseMatchesSemester(course, semester) {
			continue
		}
		slug := exportCourseSlug(course, cfg)
		title := normalizeExportCourseTitle(course.Fullname)
		if title == "" {
			title = humanizeSlug(slug)
		}
		result = append(result, exportCourse{
			ID:        course.ID,
			Fullname:  course.Fullname,
			Shortname: course.Shortname,
			Category:  course.Category,
			ViewURL:   course.ViewURL,
			Slug:      slug,
			Title:     title,
			Dir:       filepath.Join(root, semester, slug),
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].Title) < strings.ToLower(result[j].Title)
	})
	return result, nil
}

func courseMatchesSemester(course moodle.Course, semester string) bool {
	return strings.Contains(course.Fullname, semester) || strings.Contains(course.Shortname, semester) || course.Category == semester
}

func exportCourseSlug(course moodle.Course, cfg schoolExportConfig) string {
	candidates := []string{
		fmt.Sprintf("%d", course.ID),
		course.Fullname,
		course.Shortname,
	}
	for _, candidate := range candidates {
		if value := cfg.Moodle.CourseSlugOverrides[candidate]; strings.TrimSpace(value) != "" {
			return value
		}
	}
	return slugifyExportName(normalizeExportCourseTitle(course.Fullname))
}

var exportTermSuffixPattern = regexp.MustCompile(`(?i)\s+(FS|HS)\d{2}\s*$`)
var exportParenSuffixPattern = regexp.MustCompile(`\s+\([^)]*\)\s*$`)
var exportSlugReplacePattern = regexp.MustCompile(`[^a-z0-9]+`)

func normalizeExportCourseTitle(value string) string {
	cleaned := html.UnescapeString(strings.TrimSpace(value))
	cleaned = exportTermSuffixPattern.ReplaceAllString(cleaned, "")
	cleaned = exportParenSuffixPattern.ReplaceAllString(cleaned, "")
	return strings.TrimSpace(cleaned)
}

func slugifyExportName(value string) string {
	lower := strings.ToLower(html.UnescapeString(value))
	lower = exportSlugReplacePattern.ReplaceAllString(lower, "-")
	lower = strings.Trim(lower, "-")
	if lower == "" {
		return "course"
	}
	return lower
}

func humanizeSlug(slug string) string {
	parts := strings.Split(strings.ReplaceAll(slug, "_", "-"), "-")
	for i, part := range parts {
		if part == "" {
			continue
		}
		switch strings.ToLower(part) {
		case "nlp", "rpa", "hpc":
			parts[i] = strings.ToUpper(part)
		default:
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, " ")
}
