package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/DotNaos/moodle-services/internal/config"
	"github.com/DotNaos/moodle-services/internal/moodle"
	"github.com/spf13/cobra"
)

var printRaw bool
var printPDFVision bool
var printPDFVisionModel string
var printPDFVisionMaxPages int
var printPDFVisionCodexCommand string
var printCurrentLectureWorkspace string
var printCurrentLectureAt string

type printCommandResult struct {
	Action     string `json:"action" yaml:"action"`
	CourseID   string `json:"courseId,omitempty" yaml:"courseId,omitempty"`
	ResourceID string `json:"resourceId,omitempty" yaml:"resourceId,omitempty"`
	URL        string `json:"url,omitempty" yaml:"url,omitempty"`
	FileType   string `json:"fileType,omitempty" yaml:"fileType,omitempty"`
	Text       string `json:"text" yaml:"text"`
}

var printCmd = &cobra.Command{
	Use:              "print [course] [resource]",
	Short:            "Print Moodle content to stdout",
	Long:             "Print Moodle content to stdout.\n\nUse a single course selector such as `moodle print 12345` or `moodle print 0` to print the course outline, or two selectors such as `moodle print current current` to print a file.",
	TraverseChildren: true,
	Example:          "  moodle print 12345\n  moodle print 0\n  moodle print current current\n  moodle print current-course\n  moodle print current-resource\n  moodle print 0 0\n  moodle print course 12345 67890\n  moodle print course-page 12345\n  moodle print course-page current",
	Args: func(cmd *cobra.Command, args []string) error {
		args = expandSingleCurrentAlias(args)
		if len(args) == 0 {
			return nil
		}
		if len(args) > 2 {
			return fmt.Errorf("expected either a subcommand, 1 argument <course>, or 2 arguments <course> <resource>")
		}
		return nil
	},
	ValidArgsFunction: completePrintCourseFile,
	RunE: func(cmd *cobra.Command, args []string) error {
		args = expandSingleCurrentAlias(args)
		if len(args) == 0 {
			return helpOrMachineError(cmd, "expected either a subcommand, 1 argument <course>, or 2 arguments <course> <resource>")
		}
		var (
			result printCommandResult
			err    error
		)
		if len(args) == 1 {
			result, err = runPrintCoursePageSelection(args[0])
		} else {
			result, err = runPrintSelection(args[0], args[1])
		}
		if err != nil {
			return err
		}
		return writeCommandOutput(cmd, result, func(w io.Writer) error {
			_, err := fmt.Fprintln(w, result.Text)
			return err
		})
	},
}

var printCourseCmd = &cobra.Command{
	Use:               "course <course-id|name|current|0> <resource-id|name|current|0>",
	Short:             "Print file contents to stdout (PDFs use OCR fallback)",
	Long:              "Print a single file's contents to stdout.\n\nThe course and file can be specified by ID, name, `current`, `0`, or a positive index.\nPDFs are converted to text and automatically fall back to OCR when native extraction looks poor.\nUse --raw to skip cleanup.",
	Example:           "  moodle print course 12345 67890\n  moodle print course current current\n  moodle print course 0 1",
	Args:              cobra.ExactArgs(2),
	ValidArgsFunction: completePrintCourseFile,
	RunE: func(cmd *cobra.Command, args []string) error {
		result, err := runPrintSelection(args[0], args[1])
		if err != nil {
			return err
		}
		return writeCommandOutput(cmd, result, func(w io.Writer) error {
			_, err := fmt.Fprintln(w, result.Text)
			return err
		})
	},
}

var printCoursePageCmd = &cobra.Command{
	Use:               "course-page <course-id|name|current|0>",
	Short:             "Print the course outline to stdout",
	Long:              "Print the course page as a reader-friendly outline.\n\nThe course can be specified by ID, name, `current`, `0`, or a positive index.",
	Example:           "  moodle print course-page 12345\n  moodle print course-page current\n  moodle print 12345\n  moodle print 0",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeCourseIDs,
	RunE: func(cmd *cobra.Command, args []string) error {
		result, err := runPrintCoursePageSelection(args[0])
		if err != nil {
			return err
		}
		return writeCommandOutput(cmd, result, func(w io.Writer) error {
			_, err := fmt.Fprintln(w, result.Text)
			return err
		})
	},
}

var printCurrentLectureCmd = &cobra.Command{
	Use:   "current-lecture",
	Short: "Print the best matching material for the current lecture",
	Long: "Resolve the current lecture from the timetable and print the best matching material to stdout.\n\n" +
		"This uses the same current-lecture selection as `moodle list current-lecture` and `moodle open current-lecture`.",
	Example: "  moodle print current-lecture\n" +
		"  moodle print current-lecture --workspace /Users/oli/school\n" +
		"  moodle print current-lecture --at 2026-03-20T11:15:00+01:00",
	Args: cobra.NoArgs,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		now, err := resolveLectureTimeAt(printCurrentLectureAt)
		if err != nil {
			return err
		}
		cfg, err := config.LoadConfig(opts.ConfigPath)
		if err != nil {
			return err
		}
		if cfg.CalendarURL == "" {
			return fmt.Errorf("calendar URL not set. Run: moodle config set --calendar-url <url>")
		}
		client, err := ensureAuthenticatedClient()
		if err != nil {
			return err
		}
		result, err := buildCurrentLectureResult(client, cfg.CalendarURL, now, printCurrentLectureWorkspace)
		if err != nil {
			return err
		}
		if result.Material == nil || strings.TrimSpace(result.Material.URL) == "" {
			if result.Event == nil {
				return fmt.Errorf("no current or upcoming lecture found for today")
			}
			return fmt.Errorf("current lecture matched, but no printable material was found")
		}
		text, err := renderDownloadedResource(client, result.Material.URL, result.Material.FileType, printRaw)
		if err != nil {
			return err
		}
		output := printCommandResult{
			Action:     "print",
			URL:        result.Material.URL,
			ResourceID: result.Material.ID,
			FileType:   result.Material.FileType,
			Text:       text,
		}
		if result.Course != nil {
			output.CourseID = fmt.Sprintf("%d", result.Course.ID)
		}
		return writeCommandOutput(cmd, output, func(w io.Writer) error {
			_, err := fmt.Fprintln(w, text)
			return err
		})
	},
}

func init() {
	printCmd.PersistentFlags().BoolVar(&printRaw, "raw", false, "Print raw PDF text without cleanup")
	printCmd.PersistentFlags().BoolVar(&printPDFVision, "pdf-vision", false, "Use Codex vision extraction for PDF pages")
	printCmd.PersistentFlags().StringVar(&printPDFVisionModel, "pdf-vision-model", "", "Codex model for --pdf-vision (default: gpt-5.4-mini or MOODLE_PDF_VISION_MODEL)")
	printCmd.PersistentFlags().IntVar(&printPDFVisionMaxPages, "pdf-vision-max-pages", 0, "Maximum PDF pages to process with --pdf-vision (0 means all pages)")
	printCmd.PersistentFlags().StringVar(&printPDFVisionCodexCommand, "pdf-vision-codex-command", "", "Codex app-server command for --pdf-vision (default: Codex.app app-server, then codex on PATH)")
	printCurrentLectureCmd.Flags().StringVar(&printCurrentLectureWorkspace, "workspace", "", "Optional workspace root for local file matching")
	printCurrentLectureCmd.Flags().StringVar(&printCurrentLectureAt, "at", "", "Override current time for testing (RFC3339)")
	printCmd.AddCommand(
		printCourseCmd,
		printCoursePageCmd,
		printCurrentLectureCmd,
	)
}

func runPrintCoursePageSelection(courseArg string) (printCommandResult, error) {
	client, err := ensureAuthenticatedClient()
	if err != nil {
		return printCommandResult{}, err
	}
	return runPrintCoursePageWithClient(client, courseArg)
}

func runPrintCoursePageWithClient(client *moodle.Client, courseArg string) (printCommandResult, error) {
	courseID, err := resolveCourseIDWithOptions(client, courseArg, selectorOptions{})
	if err != nil {
		return printCommandResult{}, err
	}
	text, err := client.FetchCoursePageReader(courseID)
	if err != nil {
		return printCommandResult{}, err
	}
	return printCommandResult{
		Action:   "print-course-page",
		CourseID: courseID,
		Text:     text,
	}, nil
}

func runPrintSelection(courseArg string, resourceArg string) (printCommandResult, error) {
	client, err := ensureAuthenticatedClient()
	if err != nil {
		return printCommandResult{}, err
	}

	courseID, err := resolveCourseIDWithOptions(client, courseArg, selectorOptions{})
	if err != nil {
		return printCommandResult{}, err
	}
	resources, _, err := client.FetchCourseResources(courseID)
	if err != nil {
		return printCommandResult{}, err
	}
	target, err := resolveResourceWithOptions(client, courseID, resources, resourceArg, selectorOptions{})
	if err != nil {
		return printCommandResult{}, err
	}
	if target.Type != "resource" {
		return printCommandResult{}, fmt.Errorf("resource %s is not a file", target.ID)
	}

	text, err := renderDownloadedResource(client, target.URL, target.FileType, printRaw)
	if err != nil {
		return printCommandResult{}, err
	}
	return printCommandResult{
		Action:     "print",
		CourseID:   courseID,
		ResourceID: target.ID,
		URL:        target.URL,
		FileType:   target.FileType,
		Text:       text,
	}, nil
}

func renderDownloadedResource(client *moodle.Client, url string, fileType string, raw bool) (string, error) {
	result, err := client.DownloadFileToBuffer(url)
	if err != nil {
		return "", err
	}
	if fileType == "pdf" || strings.Contains(strings.ToLower(result.ContentType), "pdf") {
		text, err := moodle.ExtractPDFTextWithOptions(result.Data, moodle.PDFTextExtractionOptions{
			UseVision:          printPDFVision,
			VisionModel:        printPDFVisionModel,
			VisionMaxPages:     printPDFVisionMaxPages,
			VisionCodexCommand: printPDFVisionCodexCommand,
		})
		if err != nil {
			return "", err
		}
		if !raw {
			text = cleanExtractedTextWithTimeout(text, 2*time.Second)
		}
		return text, nil
	}
	return string(result.Data), nil
}

func cleanExtractedTextWithTimeout(input string, timeout time.Duration) string {
	type cleaningResult struct {
		text string
	}
	done := make(chan cleaningResult, 1)
	go func() {
		done <- cleaningResult{text: moodle.CleanExtractedText(input)}
	}()
	select {
	case result := <-done:
		return result.text
	case <-time.After(timeout):
		return strings.TrimSpace(input)
	}
}
