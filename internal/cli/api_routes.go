package cli

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/DotNaos/moodle-services/internal/api"
	"github.com/go-chi/chi/v5"
	"github.com/spf13/cobra"
)

const apiOptionalAnnotation = "apiOptional"

func buildAPICommandRoutes() []api.CommandRoute {
	return []api.CommandRoute{
		{
			APIPath:     "/api/version",
			Method:      http.MethodGet,
			CommandPath: []string{"version"},
			Summary:     "Show version information",
			Description: "Returns the running Moodle Services version metadata.",
		},
		{
			APIPath:     "/api/timetable",
			Method:      http.MethodGet,
			CommandPath: []string{"list", "timetable"},
			Summary:     "List timetable events",
			Description: "Returns timetable events from the configured calendar URL.",
			Arguments: func(r *http.Request, _ api.CommandRequest) ([]string, error) {
				args := []string{}
				query := r.URL.Query()
				if days := strings.TrimSpace(query.Get("days")); days != "" {
					args = append(args, "--days", days)
				}
				if queryBool(query.Get("nextWeek")) || queryBool(query.Get("next-week")) {
					args = append(args, "--next-week")
				}
				if queryBool(query.Get("unique")) {
					args = append(args, "--unique")
				}
				return args, nil
			},
		},
		{
			APIPath:     "/api/current-lecture",
			Method:      http.MethodGet,
			CommandPath: []string{"list", "current-lecture"},
			Summary:     "Resolve current lecture materials",
			Description: "Returns the current or next lecture for today, its matched Moodle course, and ranked materials.",
			Arguments: func(r *http.Request, _ api.CommandRequest) ([]string, error) {
				args := []string{}
				query := r.URL.Query()
				if workspace := strings.TrimSpace(query.Get("workspace")); workspace != "" {
					args = append(args, "--workspace", workspace)
				}
				if at := strings.TrimSpace(query.Get("at")); at != "" {
					args = append(args, "--at", at)
				}
				return args, nil
			},
		},
		{
			APIPath:     "/api/nav",
			Method:      http.MethodGet,
			CommandPath: []string{"nav"},
			Summary:     "Resolve a Moodle navigation path",
			Description: "Resolves a navigation path such as current, today, or current/items/current.",
			Arguments: func(r *http.Request, _ api.CommandRequest) ([]string, error) {
				query := r.URL.Query()
				path := strings.TrimSpace(query.Get("path"))
				if path == "" {
					return nil, fmt.Errorf("path query parameter is required")
				}
				args := []string{path}
				if queryBool(query.Get("print")) {
					args = append(args, "--print")
				}
				if workspace := strings.TrimSpace(query.Get("workspace")); workspace != "" {
					args = append(args, "--workspace", workspace)
				}
				if at := strings.TrimSpace(query.Get("at")); at != "" {
					args = append(args, "--at", at)
				}
				return args, nil
			},
		},
		{
			APIPath:     "/api/courses/{courseID}/page",
			Method:      http.MethodGet,
			CommandPath: []string{"print", "course-page"},
			Summary:     "Print course page outline",
			Description: "Returns the Moodle course page as a reader-friendly text outline.",
			Arguments: func(r *http.Request, _ api.CommandRequest) ([]string, error) {
				courseID := strings.TrimSpace(chi.URLParam(r, "courseID"))
				if courseID == "" {
					return nil, fmt.Errorf("courseID is required")
				}
				return []string{courseID}, nil
			},
		},
		{
			APIPath:     "/api/courses/{courseID}/resources/{resourceID}/text",
			Method:      http.MethodGet,
			CommandPath: []string{"print", "course"},
			Summary:     "Extract resource text",
			Description: "Returns extracted text for a single Moodle file resource. PDFs use the same extraction path as the CLI.",
			Arguments: func(r *http.Request, _ api.CommandRequest) ([]string, error) {
				courseID := strings.TrimSpace(chi.URLParam(r, "courseID"))
				resourceID := strings.TrimSpace(chi.URLParam(r, "resourceID"))
				if courseID == "" {
					return nil, fmt.Errorf("courseID is required")
				}
				if resourceID == "" {
					return nil, fmt.Errorf("resourceID is required")
				}
				args := []string{courseID, resourceID}
				if queryBool(r.URL.Query().Get("raw")) {
					args = append(args, "--raw")
				}
				return args, nil
			},
		},
		{
			APIPath:     "/api/courses/{courseID}/resources/{resourceID}/ocr",
			Method:      http.MethodGet,
			CommandPath: []string{"print", "course"},
			Summary:     "Run PDF text/OCR extraction for a PDF resource",
			Description: "Parses one Moodle PDF resource through a selectable PDF text/OCR engine. Docker is checked only for Docker-backed engines.",
			Arguments: func(r *http.Request, _ api.CommandRequest) ([]string, error) {
				courseID := strings.TrimSpace(chi.URLParam(r, "courseID"))
				resourceID := strings.TrimSpace(chi.URLParam(r, "resourceID"))
				if courseID == "" {
					return nil, fmt.Errorf("courseID is required")
				}
				if resourceID == "" {
					return nil, fmt.Errorf("resourceID is required")
				}
				query := r.URL.Query()
				engine := strings.TrimSpace(query.Get("engine"))
				if engine == "" {
					engine = "docling"
				}
				args := []string{courseID, resourceID, "--engine", engine}
				for _, item := range []struct {
					query string
					flag  string
				}{
					{"out", "--out"},
					{"format", "--format"},
					{"timeout", "--timeout"},
					{"dockerPlatform", "--docker-platform"},
					{"docker-platform", "--docker-platform"},
				} {
					if value := strings.TrimSpace(query.Get(item.query)); value != "" {
						args = append(args, item.flag, value)
					}
				}
				for _, item := range []struct {
					query string
					flag  string
				}{
					{"keepArtifacts", "--keep-artifacts"},
					{"keep-artifacts", "--keep-artifacts"},
					{"gpu", "--gpu"},
					{"formula", "--formula"},
					{"code", "--code"},
					{"verbose", "--verbose"},
				} {
					if queryBool(query.Get(item.query)) {
						args = append(args, item.flag)
					}
				}
				return args, nil
			},
		},
		{
			APIPath:     "/api/mobile/qr/inspect",
			Method:      http.MethodGet,
			CommandPath: []string{"mobile", "qr", "inspect"},
			Summary:     "Inspect Moodle mobile QR link",
			Description: "Explains a Moodle mobile QR login link without redeeming it.",
			Arguments: func(r *http.Request, _ api.CommandRequest) ([]string, error) {
				link := strings.TrimSpace(r.URL.Query().Get("link"))
				if link == "" {
					return nil, fmt.Errorf("link query parameter is required")
				}
				return []string{link}, nil
			},
		},
	}
}

func queryBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func markAPIOptional(cmd *cobra.Command) {
	if cmd == nil {
		return
	}
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[apiOptionalAnnotation] = "true"
}

func isAPIOptional(cmd *cobra.Command) bool {
	if cmd == nil || cmd.Annotations == nil {
		return false
	}
	return cmd.Annotations[apiOptionalAnnotation] == "true"
}
