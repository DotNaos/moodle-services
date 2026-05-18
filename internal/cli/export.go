package cli

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/DotNaos/moodle-services/internal/moodle"
	"github.com/spf13/cobra"
)

var exportOutputDir string

type exportCommandResult struct {
	Action        string `json:"action" yaml:"action"`
	CourseID      string `json:"courseId" yaml:"courseId"`
	CourseName    string `json:"courseName" yaml:"courseName"`
	ArchivePath   string `json:"archivePath" yaml:"archivePath"`
	ExportedFiles int    `json:"exportedFiles" yaml:"exportedFiles"`
}

var exportCmd = &cobra.Command{
	Use:               "export course <course-id|name|current|0>",
	Short:             "Export an entire course as a zip archive",
	Long:              "Export all files from a course into a zip archive.\n\nFiles are organized by section name inside the archive. The course can be specified by ID, name, `current`, `0`, or a positive index.",
	Example:           "  moodle export course 12345\n  moodle export course current -o ./course.zip\n  moodle export course 0 -o ./course.zip",
	ValidArgsFunction: completeExportCourse,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) != 2 {
			return fmt.Errorf("expected 'course' and course id/name")
		}
		if args[0] != "course" {
			return fmt.Errorf("expected 'course' subcommand")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := ensureAuthenticatedClient()
		if err != nil {
			return err
		}

		courseID, err := resolveCourseIDWithOptions(client, args[1], selectorOptions{})
		if err != nil {
			return err
		}
		courses, err := client.FetchCourses()
		if err != nil {
			return err
		}

		resources, contextID, err := client.FetchCourseResources(courseID)
		if err != nil {
			return err
		}

		courseName := resolveCourseName(courses, courseID)
		zipPath, err := resolveExportPath(courseName, exportOutputDir)
		if err != nil {
			return err
		}

		count, err := exportCourseZip(client, resources, contextID, zipPath)
		if err != nil {
			return err
		}
		result := exportCommandResult{
			Action:        "export",
			CourseID:      courseID,
			CourseName:    courseName,
			ArchivePath:   zipPath,
			ExportedFiles: count,
		}
		return writeCommandOutput(cmd, result, func(w io.Writer) error {
			return nil
		})
	},
}

func init() {
	exportCmd.Flags().StringVarP(&exportOutputDir, "output-dir", "o", "", "Output directory (or zip file path)")
}

func resolveCourseName(courses []moodle.Course, courseID string) string {
	for _, course := range courses {
		if fmt.Sprintf("%d", course.ID) == courseID {
			if strings.TrimSpace(course.Fullname) != "" {
				return course.Fullname
			}
		}
	}
	return "course-" + courseID
}

func resolveExportPath(courseName string, outputPath string) (string, error) {
	if outputPath == "" {
		filename := sanitizeFilename(courseName) + ".zip"
		return filepath.Join(opts.ExportDir, filename), nil
	}
	if filepath.Ext(outputPath) == "" {
		filename := sanitizeFilename(courseName) + ".zip"
		return filepath.Join(outputPath, filename), nil
	}
	return outputPath, nil
}

func exportCourseZip(client *moodle.Client, resources []moodle.Resource, contextID string, path string) (int, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return 0, err
	}
	if strings.TrimSpace(contextID) != "" && countExportFileResources(resources) > 1 {
		return downloadCourseContentZip(client, contextID, path)
	}
	file, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	zipWriter := zip.NewWriter(file)
	defer zipWriter.Close()
	count := 0

	for _, res := range resources {
		if res.Type != "resource" {
			continue
		}
		result, err := client.DownloadFileToBuffer(res.URL)
		if err != nil {
			return 0, err
		}

		section := res.SectionName
		if strings.TrimSpace(section) == "" {
			section = "General"
		}
		section = sanitizeFilename(section)
		entryName := filepath.ToSlash(filepath.Join(section, buildResourceFilename(res)))

		entry, err := zipWriter.Create(entryName)
		if err != nil {
			return 0, err
		}
		if _, err := entry.Write(result.Data); err != nil {
			return 0, err
		}
		count++
	}
	return count, nil
}

func countExportFileResources(resources []moodle.Resource) int {
	count := 0
	for _, res := range resources {
		if res.Type == "resource" {
			count++
		}
	}
	return count
}

func downloadCourseContentZip(client *moodle.Client, contextID string, path string) (int, error) {
	sesskey, err := client.GetSesskey()
	if err != nil {
		return 0, err
	}
	result, err := client.DownloadFileToBuffer("/course/downloadcontent.php?contextid=" + strings.TrimSpace(contextID) + "&download=1&sesskey=" + sesskey)
	if err != nil {
		return 0, err
	}
	if err := os.WriteFile(path, result.Data, 0o644); err != nil {
		return 0, err
	}
	entries, err := readExportZipEntries(path)
	if err != nil {
		return 0, fmt.Errorf("downloaded course content is not a readable zip: %w", err)
	}
	if len(entries) == 0 {
		return 0, fmt.Errorf("downloaded course content zip is empty")
	}
	return len(entries), nil
}
