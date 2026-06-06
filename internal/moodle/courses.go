package moodle

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strings"
)

var (
	trailingSemesterPattern   = regexp.MustCompile(`(?i)\s+\b(?:FS|HS)\d{2}\b\s*$`)
	trailingCourseCodePattern = regexp.MustCompile(`(?i)\s+\(([a-z]{2,}[a-z0-9]*-[a-z0-9-]+)\)\s*$`)
	courseListItemPattern     = regexp.MustCompile(`(?is)<li[^>]*\bdata-course-id=["']?(\d+)["']?[^>]*>(.*?)</li>`)
	courseListImagePattern    = regexp.MustCompile(`(?is)background-image\s*:\s*url\((.*?)\)`)
)

type Course struct {
	ID         int    `json:"id"`
	Fullname   string `json:"fullname"`
	Shortname  string `json:"shortname"`
	Category   string `json:"category"`
	CategoryID int    `json:"categoryId,omitempty"`
	ViewURL    string `json:"viewUrl"`
	HeroImage  string `json:"heroImage"`
}

type moodleAPICourse struct {
	ID             int    `json:"id"`
	Fullname       string `json:"fullname"`
	Shortname      string `json:"shortname"`
	CourseCategory string `json:"coursecategory"`
	ViewURL        string `json:"viewurl"`
	CourseImage    string `json:"courseimage"`
}

type moodleAPIData struct {
	Courses []moodleAPICourse `json:"courses"`
}

type moodleAPIResponse struct {
	Error     bool           `json:"error"`
	Exception string         `json:"exception"`
	Data      *moodleAPIData `json:"data"`
}

type moodleAPIRequest struct {
	Index      int                    `json:"index"`
	MethodName string                 `json:"methodname"`
	Args       map[string]interface{} `json:"args"`
}

func (c *Client) FetchCourses() ([]Course, error) {
	sesskey, err := c.GetSesskey()
	if err != nil {
		return nil, err
	}

	apiURL := fmt.Sprintf("%s/lib/ajax/service.php?sesskey=%s&info=core_course_get_enrolled_courses_by_timeline_classification", strings.TrimRight(c.BaseURL, "/"), sesskey)

	payload := []moodleAPIRequest{
		{
			Index:      0,
			MethodName: "core_course_get_enrolled_courses_by_timeline_classification",
			Args: map[string]interface{}{
				"offset":           0,
				"limit":            0,
				"classification":   "all",
				"sort":             "fullname",
				"customfieldname":  "",
				"customfieldvalue": "",
				"requiredfields": []string{
					"id",
					"fullname",
					"shortname",
					"courseimage",
					"showcoursecategory",
					"showshortname",
					"visible",
					"enddate",
				},
			},
		},
	}

	resp, err := c.PostJSON(apiURL, payload, nil)
	if err != nil {
		return nil, err
	}
	if err := ensureOK(resp, 2048); err != nil {
		return nil, err
	}

	var response []moodleAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}
	if len(response) == 0 {
		return nil, fmt.Errorf("empty api response")
	}

	result := response[0]
	if result.Error || result.Data == nil {
		return nil, fmt.Errorf("moodle api error: %s", result.Exception)
	}

	filtered := result.Data.Courses
	if c.School.CategoryFilter != nil {
		filtered = make([]moodleAPICourse, 0, len(result.Data.Courses))
		for _, course := range result.Data.Courses {
			if ShouldIncludeCategory(course.CourseCategory, c.School) {
				filtered = append(filtered, course)
			}
		}
	}

	courseListImages := map[int]string{}
	if hasMissingCourseImages(filtered) {
		if images, err := c.FetchCourseListImages(); err == nil {
			courseListImages = images
		}
	}

	courses := make([]Course, 0, len(filtered))
	for _, course := range filtered {
		heroImage := strings.TrimSpace(course.CourseImage)
		if heroImage == "" {
			heroImage = courseListImages[course.ID]
		}

		courses = append(courses, Course{
			ID:        course.ID,
			Fullname:  DisplayCourseName(course.Fullname, c.School.CourseNamePatterns),
			Shortname: course.Shortname,
			Category:  course.CourseCategory,
			ViewURL:   course.ViewURL,
			HeroImage: heroImage,
		})
	}

	return courses, nil
}

func (c *Client) FetchCourseListImages() (map[int]string, error) {
	body, err := c.FetchPage("/my/courses.php")
	if err != nil {
		return nil, err
	}
	return ExtractCourseListImages(body), nil
}

func ExtractCourseListImages(body string) map[int]string {
	images := map[int]string{}
	for _, match := range courseListItemPattern.FindAllStringSubmatch(body, -1) {
		if len(match) < 3 {
			continue
		}
		id := parseCourseID(match[1])
		if id == 0 {
			continue
		}
		image := extractCourseListItemImage(match[2])
		if image != "" {
			images[id] = image
		}
	}
	return images
}

func hasMissingCourseImages(courses []moodleAPICourse) bool {
	for _, course := range courses {
		if strings.TrimSpace(course.CourseImage) == "" {
			return true
		}
	}
	return false
}

func extractCourseListItemImage(itemHTML string) string {
	match := courseListImagePattern.FindStringSubmatch(itemHTML)
	if len(match) < 2 {
		return ""
	}

	image := strings.TrimSpace(html.UnescapeString(match[1]))
	image = strings.Trim(image, `"'`)
	return image
}

func parseCourseID(value string) int {
	var id int
	if _, err := fmt.Sscanf(value, "%d", &id); err != nil {
		return 0
	}
	return id
}

func (c *Client) GetSesskey() (string, error) {
	if c.sesskey != "" {
		return c.sesskey, nil
	}
	html, err := c.FetchPage("/my/")
	if err != nil {
		return "", err
	}

	rePrimary := regexp.MustCompile(`"sesskey":"([^"]+)"`)
	match := rePrimary.FindStringSubmatch(html)
	if len(match) > 1 {
		c.sesskey = match[1]
		return c.sesskey, nil
	}

	reFallback := regexp.MustCompile(`sesskey=([a-zA-Z0-9]+)`) // fallback
	match = reFallback.FindStringSubmatch(html)
	if len(match) > 1 {
		c.sesskey = match[1]
		return c.sesskey, nil
	}

	return "", fmt.Errorf("could not extract sesskey from Moodle page")
}

func DisplayCourseName(name string, patterns []*regexp.Regexp) string {
	cleaned := cleanCourseName(name, patterns)
	for {
		next := trailingSemesterPattern.ReplaceAllString(cleaned, "")
		next = trailingCourseCodePattern.ReplaceAllString(next, "")
		next = html.UnescapeString(next)
		next = strings.TrimSpace(strings.TrimRight(next, "-·"))
		if next == "" {
			return name
		}
		if next == cleaned {
			return next
		}
		cleaned = next
	}
}

func cleanCourseName(name string, patterns []*regexp.Regexp) string {
	cleaned := name
	for _, pattern := range patterns {
		cleaned = pattern.ReplaceAllString(cleaned, "")
	}
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		return name
	}
	return cleaned
}
