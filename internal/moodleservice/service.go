package moodleservice

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DotNaos/moodle-services/internal/moodle"
)

type DataClient interface {
	ValidateSession() error
	FetchCourses() ([]moodle.Course, error)
	FetchCourseResources(courseID string) ([]moodle.Resource, string, error)
	DownloadFileToBuffer(url string) (moodle.DownloadResult, error)
}

type CoursePageClient interface {
	FetchCoursePageReader(courseID string) (string, error)
}

type CategoryClient interface {
	FetchCategories() ([]moodle.Category, error)
}

type WebexCredentials struct {
	Username string
	Password string
}

type Service struct {
	Client           DataClient
	CalendarURL      string
	WebexSessionJSON string
	// WebexCredentialsJSON is the decrypted {"username","password"} blob used to
	// silently re-create an expired Webex browser session (server-side auto-renew).
	WebexCredentialsJSON string
	Now                  func() time.Time
}

type SearchResult struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

type FetchDocument struct {
	ID       string            `json:"id"`
	Title    string            `json:"title"`
	Text     string            `json:"text"`
	URL      string            `json:"url"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type PDFDescriptor struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	CourseID    string `json:"courseId"`
	ResourceID  string `json:"resourceId"`
	SectionName string `json:"sectionName,omitempty"`
	FileType    string `json:"fileType,omitempty"`
}

type PDFFile struct {
	Descriptor  PDFDescriptor
	Data        []byte
	ContentType string
}

func (s Service) ListCourses() ([]moodle.Course, error) {
	if s.Client == nil {
		return nil, fmt.Errorf("moodle client is not configured")
	}
	return s.Client.FetchCourses()
}

func (s Service) ListCategories() ([]moodle.Category, error) {
	if s.Client == nil {
		return nil, fmt.Errorf("moodle client is not configured")
	}
	client, ok := s.Client.(CategoryClient)
	if !ok {
		return nil, fmt.Errorf("moodle client does not support categories")
	}
	return client.FetchCategories()
}

func (s Service) ListMaterials(courseID string) ([]moodle.Resource, error) {
	if strings.TrimSpace(courseID) == "" {
		return nil, fmt.Errorf("courseId is required")
	}
	resources, _, err := s.Client.FetchCourseResources(courseID)
	return resources, err
}

func (s Service) CalendarEvents(days int) ([]moodle.CalendarEvent, error) {
	if strings.TrimSpace(s.CalendarURL) == "" {
		return nil, fmt.Errorf("calendar URL is not configured")
	}
	if days <= 0 || days > 120 {
		days = 30
	}
	now := s.now()
	return moodle.FetchCalendarEvents(s.CalendarURL, now.Add(-24*time.Hour), now.AddDate(0, 0, days))
}

func (s Service) Search(query string) ([]SearchResult, error) {
	query = strings.ToLower(strings.TrimSpace(query))
	courses, err := s.ListCourses()
	if err != nil {
		return nil, err
	}
	results := make([]SearchResult, 0)
	for _, course := range courses {
		courseID := strconv.Itoa(course.ID)
		if matches(query, course.Fullname, course.Shortname, course.Category) {
			results = append(results, SearchResult{ID: "course:" + courseID, Title: course.Fullname, URL: course.ViewURL})
		}
		resources, err := s.ListMaterials(courseID)
		if err != nil {
			continue
		}
		for _, resource := range resources {
			if matches(query, course.Fullname, resource.Name, resource.SectionName, resource.FileType) {
				results = append(results, SearchResult{
					ID:    "material:" + courseID + ":" + resource.ID,
					Title: course.Fullname + " / " + resource.Name,
					URL:   "moodle-material://" + courseID + "/" + resource.ID,
				})
			}
		}
	}
	if events, err := s.CalendarEvents(45); err == nil {
		for _, event := range events {
			if matches(query, event.Summary, event.Description, event.Location) {
				results = append(results, SearchResult{
					ID:    "calendar:" + event.UID,
					Title: event.Start.Format("2006-01-02 15:04") + " " + event.Summary,
					URL:   "moodle-calendar://" + event.UID,
				})
			}
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Title < results[j].Title })
	if len(results) > 25 {
		results = results[:25]
	}
	return results, nil
}

func (s Service) Fetch(id string) (FetchDocument, error) {
	parts := strings.Split(strings.TrimSpace(id), ":")
	if len(parts) == 0 {
		return FetchDocument{}, fmt.Errorf("id is required")
	}
	switch parts[0] {
	case "course":
		if len(parts) != 2 {
			return FetchDocument{}, fmt.Errorf("course id must be course:<courseId>")
		}
		text, err := s.CoursePageText(parts[1])
		return FetchDocument{ID: id, Title: "Course " + parts[1], Text: text, URL: "moodle-course://" + parts[1]}, err
	case "material":
		if len(parts) != 3 {
			return FetchDocument{}, fmt.Errorf("material id must be material:<courseId>:<resourceId>")
		}
		return s.MaterialText(parts[1], parts[2])
	case "calendar":
		return s.fetchCalendar(id)
	default:
		return FetchDocument{}, fmt.Errorf("unknown id type %q", parts[0])
	}
}

func (s Service) CoursePageText(courseID string) (string, error) {
	if client, ok := s.Client.(CoursePageClient); ok {
		return client.FetchCoursePageReader(courseID)
	}
	resources, err := s.ListMaterials(courseID)
	if err != nil {
		return "", err
	}
	lines := []string{"Course materials"}
	for _, resource := range resources {
		lines = append(lines, "- "+resource.Name+" ("+resource.FileType+")")
	}
	return strings.Join(lines, "\n"), nil
}

func (s Service) MaterialText(courseID string, resourceID string) (FetchDocument, error) {
	resource, err := s.findResource(courseID, resourceID)
	if err != nil {
		return FetchDocument{}, err
	}
	download, err := s.Client.DownloadFileToBuffer(resource.URL)
	if err != nil {
		return FetchDocument{}, err
	}
	text := string(download.Data)
	if strings.EqualFold(resource.FileType, "pdf") || strings.Contains(strings.ToLower(download.ContentType), "pdf") {
		extracted, extractErr := moodle.ExtractPDFText(download.Data)
		if extractErr != nil {
			return FetchDocument{}, extractErr
		}
		text = extracted
	}
	return FetchDocument{
		ID:    "material:" + courseID + ":" + resource.ID,
		Title: resource.Name,
		Text:  strings.TrimSpace(text),
		URL:   "moodle-material://" + courseID + "/" + resource.ID,
		Metadata: map[string]string{
			"courseId":    courseID,
			"resourceId":  resource.ID,
			"fileType":    resource.FileType,
			"sectionName": resource.SectionName,
		},
	}, nil
}

func (s Service) PDFViewerData(courseID string, resourceID string) (map[string]any, error) {
	descriptor, err := s.PDFDescriptor(courseID, resourceID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"viewer": descriptor}, nil
}

func (s Service) PDFDescriptor(courseID string, resourceID string) (PDFDescriptor, error) {
	resource, err := s.findResource(courseID, resourceID)
	if err != nil {
		return PDFDescriptor{}, err
	}
	if !strings.EqualFold(resource.FileType, "pdf") {
		return PDFDescriptor{}, fmt.Errorf("resource %s is not marked as a PDF", resourceID)
	}
	return PDFDescriptor{
		ID:          "material:" + courseID + ":" + resource.ID,
		Title:       resource.Name,
		CourseID:    courseID,
		ResourceID:  resource.ID,
		SectionName: resource.SectionName,
		FileType:    resource.FileType,
	}, nil
}

func (s Service) PDFFile(courseID string, resourceID string) (PDFFile, error) {
	descriptor, err := s.PDFDescriptor(courseID, resourceID)
	if err != nil {
		return PDFFile{}, err
	}
	resource, err := s.findResource(courseID, resourceID)
	if err != nil {
		return PDFFile{}, err
	}
	download, err := s.Client.DownloadFileToBuffer(resource.URL)
	if err != nil {
		return PDFFile{}, err
	}
	contentType := download.ContentType
	if contentType == "" || strings.EqualFold(contentType, "application/octet-stream") {
		contentType = "application/pdf"
	}
	return PDFFile{Descriptor: descriptor, Data: download.Data, ContentType: contentType}, nil
}

func (s Service) findResource(courseID string, resourceID string) (moodle.Resource, error) {
	resources, err := s.ListMaterials(courseID)
	if err != nil {
		return moodle.Resource{}, err
	}
	for _, resource := range resources {
		if resource.ID == resourceID {
			return resource, nil
		}
	}
	return moodle.Resource{}, fmt.Errorf("resource %s not found in course %s", resourceID, courseID)
}

func (s Service) fetchCalendar(id string) (FetchDocument, error) {
	events, err := s.CalendarEvents(120)
	if err != nil {
		return FetchDocument{}, err
	}
	uid := strings.TrimPrefix(id, "calendar:")
	for _, event := range events {
		if event.UID == uid {
			text, _ := json.MarshalIndent(event, "", "  ")
			return FetchDocument{ID: id, Title: event.Summary, Text: string(text), URL: "moodle-calendar://" + uid}, nil
		}
	}
	return FetchDocument{}, fmt.Errorf("calendar event %s not found", uid)
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func matches(query string, values ...string) bool {
	if query == "" {
		return true
	}
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), query) {
			return true
		}
	}
	return false
}
