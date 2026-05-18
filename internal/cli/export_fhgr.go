package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	moodleconfig "github.com/DotNaos/moodle-services/internal/config"
	"github.com/DotNaos/moodle-services/internal/moodle"
	"github.com/spf13/cobra"
)

var exportWorkspace string
var exportSemesterFlag string
var exportUpload bool
var exportArchiveOutput string
var exportArchiveProfile string

type exportFHGRCommandResult struct {
	Action  string                 `json:"action" yaml:"action"`
	Results []exportSemesterResult `json:"results" yaml:"results"`
}

type exportSemesterResult struct {
	Semester       string   `json:"semester" yaml:"semester"`
	RunID          string   `json:"runId" yaml:"run_id"`
	Status         string   `json:"status" yaml:"status"`
	Courses        int      `json:"courses" yaml:"courses"`
	CalendarEvents int      `json:"calendarEvents" yaml:"calendar_events"`
	ArchivePath    string   `json:"archivePath,omitempty" yaml:"archive_path,omitempty"`
	Failures       []string `json:"failures,omitempty" yaml:"failures,omitempty"`
}

type exportSemesterRunOptions struct {
	Upload         bool
	ArchiveOutput  string
	ArchiveProfile string
}

func init() {
	exportCmd.AddCommand(newFHGRExportCommand(false))
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
			result, err := runMoodleExportFHGR(cmd.Context(), exportWorkspace, exportSemesterFlag, exportUpload, exportArchiveOutput)
			if err != nil {
				return err
			}
			return writeCommandOutput(cmd, result, func(w io.Writer) error {
				for _, item := range result.Results {
					if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%d courses\t%d calendar events\n", item.Semester, item.RunID, item.Status, item.Courses, item.CalendarEvents); err != nil {
						return err
					}
					if item.ArchivePath != "" {
						if _, err := fmt.Fprintf(w, "  archive: %s\n", item.ArchivePath); err != nil {
							return err
						}
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
	cmd.Flags().StringVar(&exportWorkspace, "workspace", ".", "School workspace root containing school.yaml")
	cmd.Flags().StringVar(&exportSemesterFlag, "semester", "", "Process one semester only")
	cmd.Flags().BoolVar(&exportUpload, "upload", false, "Upload raw and processed output to Google Drive")
	cmd.Flags().StringVar(&exportArchiveOutput, "archive-output", "", "Optional local zip archive path or output directory")
	cmd.Flags().StringVar(&exportArchiveProfile, "archive-profile", "full", "Archive layout: full or goodnotes")
	return cmd
}

func runMoodleExportFHGR(ctx context.Context, workspace string, semester string, upload bool, archiveOutput string) (exportFHGRCommandResult, error) {
	root, err := filepath.Abs(workspace)
	if err != nil {
		return exportFHGRCommandResult{}, err
	}
	cfg, err := loadSchoolExportConfig(root)
	if err != nil {
		return exportFHGRCommandResult{}, err
	}
	index, err := loadExportIndex(root)
	if err != nil {
		return exportFHGRCommandResult{}, err
	}
	selected, err := semestersToProcess(root, cfg, index, strings.TrimSpace(semester))
	if err != nil {
		return exportFHGRCommandResult{}, err
	}
	if strings.TrimSpace(archiveOutput) != "" && filepath.Ext(archiveOutput) == ".zip" && len(selected) > 1 {
		return exportFHGRCommandResult{}, fmt.Errorf("--archive-output must be a directory when exporting multiple semesters")
	}
	archiveProfile, err := normalizeExportArchiveProfile(exportArchiveProfile)
	if err != nil {
		return exportFHGRCommandResult{}, err
	}
	client, err := ensureAuthenticatedClient()
	if err != nil {
		return exportFHGRCommandResult{}, err
	}
	uploader, err := buildExportDriveUploader(ctx, upload)
	if err != nil {
		return exportFHGRCommandResult{}, err
	}

	result := exportFHGRCommandResult{Action: "export-fhgr", Results: make([]exportSemesterResult, 0, len(selected))}
	for _, term := range selected {
		semesterResult, err := exportSemesterRun(ctx, client, root, cfg, term, uploader, &index, exportSemesterRunOptions{
			Upload:         upload,
			ArchiveOutput:  archiveOutput,
			ArchiveProfile: archiveProfile,
		})
		if err != nil {
			return exportFHGRCommandResult{}, err
		}
		result.Results = append(result.Results, semesterResult)
	}
	if err := writeExportIndex(root, index); err != nil {
		return exportFHGRCommandResult{}, err
	}
	for _, item := range result.Results {
		if item.Status != exportStatusComplete {
			return result, markErrorEmitted(fmt.Errorf("one or more export runs are incomplete"))
		}
	}
	return result, nil
}

func buildExportDriveUploader(ctx context.Context, upload bool) (exportDriveUploader, error) {
	if !upload {
		return newDryRunExportDriveUploader(), nil
	}
	return newGoogleExportDriveUploader(ctx)
}

func exportSemesterRun(ctx context.Context, client *moodle.Client, root string, cfg schoolExportConfig, semester string, uploader exportDriveUploader, index *exportIndex, options exportSemesterRunOptions) (exportSemesterResult, error) {
	run := buildExportRunContext(root, semester, time.Now())
	courses, err := exportCoursesForSemester(client, root, cfg, semester)
	if err != nil {
		return exportSemesterResult{}, err
	}
	semesterFolder, err := uploader.EnsureFolderPath(ctx, []string{semester})
	if err != nil {
		return exportSemesterResult{}, err
	}
	runFolder, err := uploader.CreateRunFolder(ctx, []string{semester, "runs", run.RunID})
	if err != nil {
		return exportSemesterResult{}, err
	}
	rawFolder, err := uploader.EnsureFolderPath(ctx, []string{semester, "runs", run.RunID, "raw-zips"})
	if err != nil {
		return exportSemesterResult{}, err
	}
	processedFolder, err := uploader.EnsureFolderPath(ctx, []string{semester, "runs", run.RunID, "processed"})
	if err != nil {
		return exportSemesterResult{}, err
	}
	moodleCfg, err := moodleconfig.LoadConfig(opts.ConfigPath)
	if err != nil {
		return exportSemesterResult{}, err
	}

	failures := []string{}
	if len(courses) == 0 {
		failures = append(failures, semester+": no Moodle courses found")
	}
	manifests := make([]exportCourseManifest, 0, len(courses))
	tempDir, err := os.MkdirTemp("", "moodle-export-"+semester+"-")
	if err != nil {
		return exportSemesterResult{}, err
	}
	defer os.RemoveAll(tempDir)

	allRecords := make([]exportMaterialRecord, 0)
	for _, course := range courses {
		manifest, records, err := exportCourseRun(ctx, client, run, course, rawFolder, runFolder, uploader, tempDir, options)
		if err != nil {
			failures = append(failures, course.Slug+": "+err.Error())
			continue
		}
		manifests = append(manifests, manifest)
		allRecords = append(allRecords, records...)
	}
	calendarResult, err := exportCalendarRun(ctx, uploader, run, cfg, courses, moodleCfg.CalendarURL, tempDir)
	if err != nil {
		failures = append(failures, "calendar: "+err.Error())
	}
	status := exportStatusComplete
	if len(failures) > 0 {
		status = exportStatusIncomplete
	}
	completedAt := time.Now().UTC().Truncate(time.Second)
	runYAML, err := yamlString(exportRunYAML{
		Semester:         semester,
		Run:              run.RunID,
		GitHubRunID:      run.GitHubRunID,
		GitHubRunAttempt: run.GitHubRunAttempt,
		Status:           status,
		StartedAt:        isoUTC(run.StartedAt),
		CompletedAt:      isoUTC(completedAt),
	})
	if err != nil {
		return exportSemesterResult{}, err
	}
	if _, err := uploader.UploadText(ctx, runYAML, runFolder.ID, "run.yaml", false); err != nil {
		return exportSemesterResult{}, err
	}
	if _, err := uploader.UploadText(ctx, renderExportReport(run, status, manifests, calendarResult.Index.EventCount, failures), runFolder.ID, "report.md", false); err != nil {
		return exportSemesterResult{}, err
	}
	if err := uploadProcessedExportFiles(ctx, uploader, processedFolder.ID, courses); err != nil {
		return exportSemesterResult{}, err
	}
	if err := uploadExportNavigation(ctx, uploader, run, courses, allRecords, calendarResult.Index); err != nil {
		return exportSemesterResult{}, err
	}
	archivePath := ""
	if strings.TrimSpace(options.ArchiveOutput) != "" {
		resolvedArchivePath, err := resolveExportArchivePath(options.ArchiveOutput, run)
		if err != nil {
			return exportSemesterResult{}, err
		}
		if err := writeExportArchive(run, courses, allRecords, manifests, calendarResult.Index, tempDir, resolvedArchivePath, options.ArchiveProfile); err != nil {
			return exportSemesterResult{}, err
		}
		archivePath = resolvedArchivePath
	}
	if status == exportStatusComplete {
		latest, err := yamlString(exportLatestYAML{Semester: semester, LatestRun: run.RunID, Status: status, UpdatedAt: isoUTC(completedAt)})
		if err != nil {
			return exportSemesterResult{}, err
		}
		if _, err := uploader.UploadText(ctx, latest, semesterFolder.ID, "latest.yaml", true); err != nil {
			return exportSemesterResult{}, err
		}
	}
	updateExportIndex(index, semester, run, status, completedAt, semesterFolder, manifests, calendarResult.Index)
	return exportSemesterResult{Semester: semester, RunID: run.RunID, Status: status, Courses: len(manifests), CalendarEvents: calendarResult.Index.EventCount, ArchivePath: archivePath, Failures: failures}, nil
}

func exportCourseRun(ctx context.Context, client *moodle.Client, run exportRunContext, course exportCourse, rawFolder exportDriveFile, runFolder exportDriveFile, uploader exportDriveUploader, tempDir string, options exportSemesterRunOptions) (exportCourseManifest, []exportMaterialRecord, error) {
	if err := ensureExportCourseFiles(course); err != nil {
		return exportCourseManifest{}, nil, err
	}
	courseID := fmt.Sprintf("%d", course.ID)
	resources, contextID, err := client.FetchCourseResources(courseID)
	if err != nil {
		return exportCourseManifest{}, nil, err
	}
	readerText, err := client.FetchCoursePageReader(courseID)
	if err != nil {
		return exportCourseManifest{}, nil, err
	}
	if err := writeExportCourseSnapshot(course, readerText, resources); err != nil {
		return exportCourseManifest{}, nil, err
	}
	zipPath := filepath.Join(tempDir, course.Slug+".zip")
	if _, err := exportCourseZip(client, resources, contextID, zipPath); err != nil {
		return exportCourseManifest{}, nil, err
	}
	sha, err := sha256File(zipPath)
	if err != nil {
		return exportCourseManifest{}, nil, err
	}
	rawUpload, err := uploader.UploadFile(ctx, zipPath, rawFolder.ID, filepath.Base(zipPath), false)
	if err != nil {
		return exportCourseManifest{}, nil, err
	}
	textResults := []exportTextExtraction{}
	records := []exportMaterialRecord{}
	if !goodNotesArchiveOnly(options) {
		textResults = extractExportResourceTexts(client, course, resources)
		var err error
		records, err = exportCourseDriveArtifacts(ctx, client, uploader, run, course, resources, textResults, tempDir)
		if err != nil {
			return exportCourseManifest{}, nil, err
		}
	}
	if _, err := writeExportMaterialIndex(course, run, zipPath, sha, rawUpload, resources, textResults); err != nil {
		return exportCourseManifest{}, nil, err
	}
	return exportCourseManifest{
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
		ExportStatus:        exportStatusComplete,
		BackedUpAt:          isoUTC(time.Now()),
		SourceMoodleMetadata: map[string]string{
			"fullname":  course.Fullname,
			"shortname": course.Shortname,
			"category":  course.Category,
			"view_url":  course.ViewURL,
		},
	}, records, nil
}

func goodNotesArchiveOnly(options exportSemesterRunOptions) bool {
	return !options.Upload && strings.TrimSpace(options.ArchiveOutput) != "" && strings.EqualFold(options.ArchiveProfile, "goodnotes")
}

func uploadProcessedExportFiles(ctx context.Context, uploader exportDriveUploader, folderID string, courses []exportCourse) error {
	for _, course := range courses {
		for _, name := range []string{"MOODLE.md", "moodle-course.yaml", "materials.index.yaml"} {
			path := filepath.Join(course.Dir, name)
			if _, err := os.Stat(path); err == nil {
				if _, err := uploader.UploadFile(ctx, path, folderID, driveUploadName(course.Slug, path), false); err != nil {
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
			if ok, err := isExportMarkdownFile(path); err != nil {
				return err
			} else if !ok {
				continue
			}
			if _, err := uploader.UploadFile(ctx, path, folderID, course.Slug+"--materials-text--"+entry.Name(), false); err != nil {
				return err
			}
		}
	}
	return nil
}

func isExportMarkdownFile(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	return looksLikeExportText(data), nil
}

func updateExportIndex(index *exportIndex, semester string, run exportRunContext, status string, completedAt time.Time, semesterFolder exportDriveFile, manifests []exportCourseManifest, calendar exportCalendarIndex) {
	index.GeneratedAt = isoUTC(completedAt)
	index.GoogleDriveRoot = exportDriveRootName
	if index.Semesters == nil {
		index.Semesters = map[string]exportSemesterRef{}
	}
	courses := map[string]exportCourseManifest{}
	for _, manifest := range manifests {
		courses[manifest.CourseSlug] = manifest
	}
	index.Semesters[semester] = exportSemesterRef{
		LatestRun:           run.RunID,
		Status:              status,
		UpdatedAt:           isoUTC(completedAt),
		GoogleDriveFolderID: semesterFolder.ID,
		GoogleDriveLink:     semesterFolder.WebViewLink,
		Calendar:            completedCalendarIndex(calendar),
		Courses:             courses,
	}
}

func completedCalendarIndex(calendar exportCalendarIndex) *exportCalendarIndex {
	if strings.TrimSpace(calendar.Semester) == "" {
		return nil
	}
	return &calendar
}
