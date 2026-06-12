package moodle

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type MobileSession struct {
	SchoolID     string    `json:"schoolId,omitempty"`
	SiteURL      string    `json:"siteUrl"`
	UserID       int       `json:"userId"`
	Token        string    `json:"token"`
	PrivateToken string    `json:"privateToken,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}

type MobileClient struct {
	Session MobileSession
	School  SchoolConfig
	http    *http.Client
}

func MobileSessionFromToken(token MobileToken) MobileSession {
	return MobileSession{
		SiteURL:      token.SiteURL,
		UserID:       token.UserID,
		Token:        token.Token,
		PrivateToken: token.PrivateToken,
		CreatedAt:    time.Now(),
	}
}

func (s MobileSession) ResolvedSchoolID() string {
	if s.SchoolID != "" {
		return s.SchoolID
	}
	return ActiveSchoolID
}

func LoadMobileSession(path string) (MobileSession, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return MobileSession{}, err
	}
	var session MobileSession
	if err := json.Unmarshal(data, &session); err != nil {
		return MobileSession{}, err
	}
	if session.SiteURL == "" {
		return MobileSession{}, fmt.Errorf("mobile session missing siteUrl")
	}
	if session.UserID == 0 {
		return MobileSession{}, fmt.Errorf("mobile session missing userId")
	}
	if session.Token == "" {
		return MobileSession{}, fmt.Errorf("mobile session missing token")
	}
	return session, nil
}

func SaveMobileSession(path string, session MobileSession) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func NewMobileClient(session MobileSession, schoolID string) (*MobileClient, error) {
	school, err := resolveSchool(schoolID)
	if err != nil {
		return nil, err
	}
	if session.SiteURL == "" || session.Token == "" || session.UserID == 0 {
		return nil, fmt.Errorf("mobile session is incomplete")
	}
	return &MobileClient{
		Session: session,
		School:  school,
		http: &http.Client{
			Timeout: 60 * time.Second,
		},
	}, nil
}

func (c *MobileClient) ValidateSession() error {
	_, err := c.FetchMobileSiteInfo()
	return err
}

func (c *MobileClient) FetchCourses() ([]Course, error) {
	var courses []MobileCourse
	values := url.Values{}
	values.Set("userid", strconv.Itoa(c.Session.UserID))
	if err := c.callMobileAPI("core_enrol_get_users_courses", values, &courses); err != nil {
		return nil, err
	}

	categoryNames := map[int]string{}
	if categories, err := c.FetchCategories(); err == nil {
		categoryNames = CategoryNameByID(categories)
	}

	timelineImages := map[int]string{}
	if hasMissingMobileCourseImages(courses) {
		if timelineCourses, err := c.FetchTimelineCourses(); err == nil {
			timelineImages = mobileCourseImagesByID(timelineCourses, c.Session.Token)
		}
		c.fillMissingCourseImagesFromCourseDetails(courses, timelineImages)
	}

	result := make([]Course, 0, len(courses))
	for _, course := range courses {
		category := categoryNames[course.CategoryID]
		if category == "" && course.CategoryID != 0 {
			category = strconv.Itoa(course.CategoryID)
		}
		courseImage := mobileCourseImage(course, c.Session.Token)
		if courseImage == "" || isMoodleGeneratedCourseSVG(courseImage) {
			courseImage = timelineImages[course.ID]
		}
		result = append(result, Course{
			ID:         course.ID,
			Fullname:   DisplayCourseName(course.FullName, c.School.CourseNamePatterns),
			Shortname:  course.ShortName,
			Category:   category,
			CategoryID: course.CategoryID,
			ViewURL:    strings.TrimRight(c.Session.SiteURL, "/") + "/course/view.php?id=" + strconv.Itoa(course.ID),
			HeroImage:  courseImage,
		})
	}
	return result, nil
}

func (c *MobileClient) FetchTimelineCourses() ([]MobileCourse, error) {
	var response struct {
		Courses []MobileCourse `json:"courses"`
	}
	values := url.Values{}
	values.Set("classification", "all")
	values.Set("sort", "fullname")
	values.Set("limit", "0")
	values.Set("offset", "0")
	values.Set("customfieldname", "")
	values.Set("customfieldvalue", "")
	if err := c.callMobileAPI("core_course_get_enrolled_courses_by_timeline_classification", values, &response); err != nil {
		return nil, err
	}
	return response.Courses, nil
}

func (c *MobileClient) FetchCourseByID(courseID int) (MobileCourse, error) {
	var response struct {
		Courses []MobileCourse `json:"courses"`
	}
	values := url.Values{}
	values.Set("field", "id")
	values.Set("value", strconv.Itoa(courseID))
	if err := c.callMobileAPI("core_course_get_courses_by_field", values, &response); err != nil {
		return MobileCourse{}, err
	}
	if len(response.Courses) == 0 {
		return MobileCourse{}, fmt.Errorf("course %d not found", courseID)
	}
	return response.Courses[0], nil
}

func (c *MobileClient) FetchCategories() ([]Category, error) {
	var raw []mobileCategory
	if err := c.callMobileAPI("core_course_get_categories", nil, &raw); err != nil {
		return nil, err
	}
	categories := make([]Category, 0, len(raw))
	for _, category := range raw {
		categories = append(categories, Category{
			ID:       category.ID,
			Name:     category.Name,
			IDNumber: category.IDNumber,
			ParentID: category.ParentID,
			Path:     category.Path,
			Depth:    category.Depth,
		})
	}
	return categories, nil
}

type mobileCategory struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	IDNumber string `json:"idnumber"`
	ParentID int    `json:"parent"`
	Path     string `json:"path"`
	Depth    int    `json:"depth"`
}

func (c *MobileClient) FetchCourseResources(courseID string) ([]Resource, string, error) {
	var sections []mobileCourseSection
	values := url.Values{}
	values.Set("courseid", courseID)
	if err := c.callMobileAPI("core_course_get_contents", values, &sections); err != nil {
		return nil, "", err
	}

	resources := make([]Resource, 0)
	for _, section := range sections {
		sectionID := strconv.Itoa(section.ID)
		for _, module := range section.Modules {
			resource, ok := mobileModuleToResource(c.Session.SiteURL, c.Session.Token, courseID, sectionID, section.Name, module)
			if ok {
				resources = append(resources, resource)
			}
		}
	}
	return resources, "", nil
}

func (c *MobileClient) FetchMobileSiteInfo() (MobileSiteInfo, error) {
	var info MobileSiteInfo
	if err := c.callMobileAPI("core_webservice_get_site_info", nil, &info); err != nil {
		return MobileSiteInfo{}, err
	}
	return info, nil
}

func (c *MobileClient) DownloadFileToBuffer(fileURL string) (DownloadResult, error) {
	targetURL := addMobileTokenToFileURL(fileURL, c.Session.Token)
	req, err := http.NewRequest(http.MethodGet, targetURL, nil)
	if err != nil {
		return DownloadResult{}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 MoodleMobile")

	resp, err := c.http.Do(req)
	if err != nil {
		return DownloadResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return DownloadResult{}, fmt.Errorf("mobile file download failed: %s (%s)", resp.Status, strings.TrimSpace(string(body)))
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return DownloadResult{}, err
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return DownloadResult{Data: data, ContentType: contentType}, nil
}

func (c *MobileClient) callMobileAPI(function string, values url.Values, target any) error {
	token := MobileToken{
		SiteURL:      c.Session.SiteURL,
		UserID:       c.Session.UserID,
		Token:        c.Session.Token,
		PrivateToken: c.Session.PrivateToken,
	}
	client := &Client{http: c.http}
	return client.CallMobileAPI(token, function, values, target)
}

type mobileCourseSection struct {
	ID      int            `json:"id"`
	Name    string         `json:"name"`
	Visible int            `json:"visible"`
	Modules []mobileModule `json:"modules"`
}

type mobileModule struct {
	ID       int             `json:"id"`
	Name     string          `json:"name"`
	ModName  string          `json:"modname"`
	URL      string          `json:"url"`
	Visible  int             `json:"visible"`
	Contents []mobileContent `json:"contents"`
}

type mobileContent struct {
	Type     string `json:"type"`
	FileName string `json:"filename"`
	FilePath string `json:"filepath"`
	FileSize int    `json:"filesize"`
	FileURL  string `json:"fileurl"`
}

func mobileModuleToResource(siteURL string, token string, courseID string, sectionID string, sectionName string, module mobileModule) (Resource, bool) {
	if module.ModName == "label" || module.ID == 0 {
		return Resource{}, false
	}

	resourceType := "resource"
	id := strconv.Itoa(module.ID)
	if module.ModName == "folder" {
		resourceType = "folder"
		id = "folder-" + id
	}

	fileType := inferMobileFileType(module)
	resourceURL := firstNonEmpty(firstMobileFileURL(module, token), module.URL, strings.TrimRight(siteURL, "/")+"/mod/"+module.ModName+"/view.php?id="+strconv.Itoa(module.ID))
	return Resource{
		ID:          id,
		Name:        strings.TrimSpace(module.Name),
		URL:         resourceURL,
		Type:        resourceType,
		CourseID:    courseID,
		SectionID:   sectionID,
		SectionName: sectionName,
		FileType:    fileType,
	}, true
}

func firstMobileFileURL(module mobileModule, token string) string {
	for _, content := range module.Contents {
		if content.FileURL != "" {
			return addMobileTokenToFileURL(content.FileURL, token)
		}
	}
	return ""
}

func mobileCourseImage(course MobileCourse, token string) string {
	image := strings.TrimSpace(course.CourseImage)
	if image == "" || isMoodleGeneratedCourseSVG(image) {
		for _, overviewFile := range course.OverviewFiles {
			overviewImage := strings.TrimSpace(overviewFile.FileURL)
			if overviewImage != "" {
				image = overviewImage
				break
			}
		}
	}
	if strings.HasPrefix(strings.ToLower(image), "data:") {
		return image
	}
	return addMobileTokenToFileURL(image, token)
}

func mobileCourseImagesByID(courses []MobileCourse, token string) map[int]string {
	images := map[int]string{}
	for _, course := range courses {
		image := mobileCourseImage(course, token)
		if image != "" {
			images[course.ID] = image
		}
	}
	return images
}

func (c *MobileClient) fillMissingCourseImagesFromCourseDetails(courses []MobileCourse, images map[int]string) {
	for _, course := range courses {
		existing := strings.TrimSpace(images[course.ID])
		if existing != "" && !isMoodleGeneratedCourseSVG(existing) {
			continue
		}
		image := mobileCourseImage(course, "")
		if image != "" && !isMoodleGeneratedCourseSVG(image) {
			continue
		}
		detail, err := c.FetchCourseByID(course.ID)
		if err != nil {
			continue
		}
		image = mobileCourseImage(detail, c.Session.Token)
		if image != "" {
			images[course.ID] = image
		}
	}
}

func hasMissingMobileCourseImages(courses []MobileCourse) bool {
	for _, course := range courses {
		image := mobileCourseImage(course, "")
		if image == "" || isMoodleGeneratedCourseSVG(image) {
			return true
		}
	}
	return false
}

func isMoodleGeneratedCourseSVG(image string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(image)), "/course/generated/course.svg")
}

func addMobileTokenToFileURL(fileURL string, token string) string {
	if strings.TrimSpace(fileURL) == "" || strings.TrimSpace(token) == "" {
		return fileURL
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(fileURL)), "data:") {
		return fileURL
	}
	parsed, err := url.Parse(fileURL)
	if err != nil {
		return fileURL
	}
	query := parsed.Query()
	if query.Get("token") == "" {
		query.Set("token", token)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func inferMobileFileType(module mobileModule) string {
	for _, content := range module.Contents {
		if content.FileName == "" {
			continue
		}
		name := strings.ToLower(content.FileName)
		switch {
		case strings.HasSuffix(name, ".pdf"):
			return "pdf"
		case strings.HasSuffix(name, ".doc") || strings.HasSuffix(name, ".docx"):
			return "docx"
		case strings.HasSuffix(name, ".xls") || strings.HasSuffix(name, ".xlsx"):
			return "xlsx"
		case strings.HasSuffix(name, ".ppt") || strings.HasSuffix(name, ".pptx"):
			return "pptx"
		case strings.HasSuffix(name, ".zip"):
			return "zip"
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
