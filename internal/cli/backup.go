package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DotNaos/moodle-services/internal/moodle"
	"github.com/spf13/cobra"
)

var backupWorkspace string
var backupSemesterFlag string
var backupUpload bool

type backupCommandResult struct {
	Action  string                 `json:"action" yaml:"action"`
	Results []backupSemesterResult `json:"results" yaml:"results"`
}

type backupSemesterResult struct {
	Semester string   `json:"semester" yaml:"semester"`
	RunID    string   `json:"runId" yaml:"run_id"`
	Status   string   `json:"status" yaml:"status"`
	Courses  int      `json:"courses" yaml:"courses"`
	Failures []string `json:"failures,omitempty" yaml:"failures,omitempty"`
}

var backupCmd = &cobra.Command{
	Use:    "backup",
	Short:  "Back up Moodle course material",
	Long:   "Back up Moodle course material and generate lightweight course indexes.",
	Hidden: true,
}

func init() {
	exportCmd.AddCommand(newFHGRExportCommand(false))
	backupCmd.AddCommand(newFHGRExportCommand(true))
}

func newFHGRExportCommand(hidden bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fhgr",
		Short: "Export FHGR Moodle material for a school workspace",
		Long:  "Export FHGR Moodle material for a school workspace.\n\nThe command reads school.yaml, always processes current_term, and processes old semesters only when export.index.yaml does not show a completed export.",
		Example: "  moodle export fhgr --workspace /Users/oli/school --upload\n" +
			"  moodle export fhgr --workspace /Users/oli/school --semester FS26 --upload",
		Args:   cobra.NoArgs,
		Hidden: hidden,
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := runMoodleBackupFHGR(cmd.Context(), backupWorkspace, backupSemesterFlag, backupUpload)
			if err != nil {
				return err
			}
			return writeCommandOutput(cmd, result, func(w io.Writer) error {
				for _, item := range result.Results {
					if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%d courses\n", item.Semester, item.RunID, item.Status, item.Courses); err != nil {
						return err
					}
					for _, failure := range item.Failures {
						if _, err := fmt.Fprintf(w, "  failure: %s\n", failure); err != nil {
							return err
						}
					}
				}
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&backupWorkspace, "workspace", ".", "School workspace root containing school.yaml")
	cmd.Flags().StringVar(&backupSemesterFlag, "semester", "", "Process one semester only")
	cmd.Flags().BoolVar(&backupUpload, "upload", false, "Upload raw and processed output to Google Drive")
	return cmd
}

func runMoodleBackupFHGR(ctx context.Context, workspace string, semester string, upload bool) (backupCommandResult, error) {
	root, err := filepath.Abs(workspace)
	if err != nil {
		return backupCommandResult{}, err
	}
	cfg, err := loadSchoolBackupConfig(root)
	if err != nil {
		return backupCommandResult{}, err
	}
	index, err := loadBackupIndex(root)
	if err != nil {
		return backupCommandResult{}, err
	}
	selected, err := semestersToProcess(root, cfg, index, strings.TrimSpace(semester))
	if err != nil {
		return backupCommandResult{}, err
	}
	client, err := ensureAuthenticatedClient()
	if err != nil {
		return backupCommandResult{}, err
	}
	uploader, err := buildBackupDriveUploader(ctx, upload)
	if err != nil {
		return backupCommandResult{}, err
	}

	result := backupCommandResult{Action: "export-fhgr", Results: make([]backupSemesterResult, 0, len(selected))}
	for _, term := range selected {
		semesterResult, err := backupSemesterRun(ctx, client, root, cfg, term, uploader, &index)
		if err != nil {
			return backupCommandResult{}, err
		}
		result.Results = append(result.Results, semesterResult)
	}
	if err := writeBackupIndex(root, index); err != nil {
		return backupCommandResult{}, err
	}
	for _, item := range result.Results {
		if item.Status != backupStatusComplete {
			return result, markErrorEmitted(fmt.Errorf("one or more backup runs are incomplete"))
		}
	}
	return result, nil
}

func buildBackupDriveUploader(ctx context.Context, upload bool) (backupDriveUploader, error) {
	if !upload {
		return newDryRunBackupDriveUploader(), nil
	}
	return newGoogleBackupDriveUploader(ctx)
}

func backupSemesterRun(ctx context.Context, client *moodle.Client, root string, cfg schoolBackupConfig, semester string, uploader backupDriveUploader, index *backupIndex) (backupSemesterResult, error) {
	run := buildBackupRunContext(root, semester, time.Now())
	courses, err := backupCoursesForSemester(client, root, cfg, semester)
	if err != nil {
		return backupSemesterResult{}, err
	}
	semesterFolder, err := uploader.EnsureFolderPath(ctx, []string{semester})
	if err != nil {
		return backupSemesterResult{}, err
	}
	runFolder, err := uploader.CreateRunFolder(ctx, []string{semester, run.RunID})
	if err != nil {
		return backupSemesterResult{}, err
	}
	rawFolder, err := uploader.EnsureFolderPath(ctx, []string{semester, run.RunID, "raw"})
	if err != nil {
		return backupSemesterResult{}, err
	}
	processedFolder, err := uploader.EnsureFolderPath(ctx, []string{semester, run.RunID, "processed"})
	if err != nil {
		return backupSemesterResult{}, err
	}

	failures := []string{}
	if len(courses) == 0 {
		failures = append(failures, semester+": no Moodle courses found")
	}
	manifests := make([]backupCourseManifest, 0, len(courses))
	tempDir, err := os.MkdirTemp("", "moodle-backup-"+semester+"-")
	if err != nil {
		return backupSemesterResult{}, err
	}
	defer os.RemoveAll(tempDir)

	for _, course := range courses {
		manifest, err := backupCourseRun(ctx, client, run, course, rawFolder, runFolder, uploader, tempDir)
		if err != nil {
			failures = append(failures, course.Slug+": "+err.Error())
			continue
		}
		manifests = append(manifests, manifest)
	}
	status := backupStatusComplete
	if len(failures) > 0 {
		status = backupStatusIncomplete
	}
	completedAt := time.Now().UTC().Truncate(time.Second)
	runYAML, err := yamlString(backupRunYAML{
		Semester:         semester,
		Run:              run.RunID,
		GitHubRunID:      run.GitHubRunID,
		GitHubRunAttempt: run.GitHubRunAttempt,
		Status:           status,
		StartedAt:        isoUTC(run.StartedAt),
		CompletedAt:      isoUTC(completedAt),
	})
	if err != nil {
		return backupSemesterResult{}, err
	}
	if _, err := uploader.UploadText(ctx, runYAML, runFolder.ID, "run.yaml", false); err != nil {
		return backupSemesterResult{}, err
	}
	if _, err := uploader.UploadText(ctx, renderBackupReport(run, status, manifests, failures), runFolder.ID, "report.md", false); err != nil {
		return backupSemesterResult{}, err
	}
	if err := uploadProcessedBackupFiles(ctx, uploader, processedFolder.ID, courses); err != nil {
		return backupSemesterResult{}, err
	}
	if status == backupStatusComplete {
		latest, err := yamlString(backupLatestYAML{Semester: semester, LatestRun: run.RunID, Status: status, UpdatedAt: isoUTC(completedAt)})
		if err != nil {
			return backupSemesterResult{}, err
		}
		if _, err := uploader.UploadText(ctx, latest, semesterFolder.ID, "latest.yaml", true); err != nil {
			return backupSemesterResult{}, err
		}
	}
	updateBackupIndex(index, semester, run, status, completedAt, semesterFolder, manifests)
	return backupSemesterResult{Semester: semester, RunID: run.RunID, Status: status, Courses: len(manifests), Failures: failures}, nil
}

func backupCourseRun(ctx context.Context, client *moodle.Client, run backupRunContext, course backupCourse, rawFolder backupDriveFile, runFolder backupDriveFile, uploader backupDriveUploader, tempDir string) (backupCourseManifest, error) {
	if err := ensureBackupCourseFiles(course); err != nil {
		return backupCourseManifest{}, err
	}
	courseID := fmt.Sprintf("%d", course.ID)
	resources, _, err := client.FetchCourseResources(courseID)
	if err != nil {
		return backupCourseManifest{}, err
	}
	readerText, err := client.FetchCoursePageReader(courseID)
	if err != nil {
		return backupCourseManifest{}, err
	}
	if err := writeBackupCourseSnapshot(course, readerText, resources); err != nil {
		return backupCourseManifest{}, err
	}
	zipPath := filepath.Join(tempDir, course.Slug+".zip")
	if _, err := exportCourseZip(client, resources, zipPath); err != nil {
		return backupCourseManifest{}, err
	}
	sha, err := sha256File(zipPath)
	if err != nil {
		return backupCourseManifest{}, err
	}
	rawUpload, err := uploader.UploadFile(ctx, zipPath, rawFolder.ID, filepath.Base(zipPath))
	if err != nil {
		return backupCourseManifest{}, err
	}
	textResults := extractBackupResourceTexts(client, course, resources)
	if _, err := writeBackupMaterialIndex(course, run, zipPath, sha, rawUpload, resources, textResults); err != nil {
		return backupCourseManifest{}, err
	}
	return backupCourseManifest{
		Semester:            run.Semester,
		CourseID:            courseID,
		CourseSlug:          course.Slug,
		CourseName:          course.Title,
		RunID:               run.RunID,
		GoogleDriveFolderID: runFolder.ID,
		GoogleDriveFileIDs:  []string{rawUpload.ID},
		GoogleDriveLink:     rawUpload.WebViewLink,
		RawZipFilename:      filepath.Base(zipPath),
		SHA256:              sha,
		BackupStatus:        backupStatusComplete,
		BackedUpAt:          isoUTC(time.Now()),
		SourceMoodleMetadata: map[string]string{
			"fullname":  course.Fullname,
			"shortname": course.Shortname,
			"category":  course.Category,
			"view_url":  course.ViewURL,
		},
	}, nil
}

func uploadProcessedBackupFiles(ctx context.Context, uploader backupDriveUploader, folderID string, courses []backupCourse) error {
	for _, course := range courses {
		for _, name := range []string{"MOODLE.md", "moodle-course.yaml", "materials.index.yaml"} {
			path := filepath.Join(course.Dir, name)
			if _, err := os.Stat(path); err == nil {
				if _, err := uploader.UploadFile(ctx, path, folderID, driveUploadName(course.Slug, path)); err != nil {
					return err
				}
			}
		}
		textDir := filepath.Join(course.Dir, "materials-text")
		entries, err := os.ReadDir(textDir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
				continue
			}
			path := filepath.Join(textDir, entry.Name())
			if ok, err := isBackupMarkdownFile(path); err != nil {
				return err
			} else if !ok {
				continue
			}
			if _, err := uploader.UploadFile(ctx, path, folderID, course.Slug+"--materials-text--"+entry.Name()); err != nil {
				return err
			}
		}
	}
	return nil
}

func isBackupMarkdownFile(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	return looksLikeBackupText(data), nil
}

func updateBackupIndex(index *backupIndex, semester string, run backupRunContext, status string, completedAt time.Time, semesterFolder backupDriveFile, manifests []backupCourseManifest) {
	index.GeneratedAt = isoUTC(completedAt)
	index.GoogleDriveRoot = backupDriveRootName
	if index.Semesters == nil {
		index.Semesters = map[string]backupSemesterRef{}
	}
	courses := map[string]backupCourseManifest{}
	for _, manifest := range manifests {
		courses[manifest.CourseSlug] = manifest
	}
	index.Semesters[semester] = backupSemesterRef{
		LatestRun:           run.RunID,
		Status:              status,
		UpdatedAt:           isoUTC(completedAt),
		GoogleDriveFolderID: semesterFolder.ID,
		GoogleDriveLink:     semesterFolder.WebViewLink,
		Courses:             courses,
	}
}
