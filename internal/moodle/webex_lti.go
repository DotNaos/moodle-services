package moodle

import (
	"net/url"
	"strconv"
	"strings"
)

type WebexLTIActivity struct {
	ID           int
	CourseModule int
	Name         string
}

type mobileLTIResponse struct {
	LTIs []mobileLTI `json:"ltis"`
}

type mobileLTI struct {
	ID           int    `json:"id"`
	CourseModule int    `json:"coursemodule"`
	Name         string `json:"name"`
	Intro        string `json:"intro"`
}

func (c *MobileClient) SiteURL() string {
	return strings.TrimRight(c.Session.SiteURL, "/")
}

func (c *MobileClient) FetchWebexLTIActivities(courseID string) ([]WebexLTIActivity, error) {
	values := url.Values{}
	values.Set("courseids[0]", strings.TrimSpace(courseID))
	var payload mobileLTIResponse
	if err := c.callMobileAPI("mod_lti_get_ltis_by_courses", values, &payload); err != nil {
		return nil, err
	}
	activities := make([]WebexLTIActivity, 0)
	for _, lti := range payload.LTIs {
		haystack := strings.ToLower(lti.Name + " " + lti.Intro)
		if lti.ID == 0 || !strings.Contains(haystack, "webex") {
			continue
		}
		activities = append(activities, WebexLTIActivity{
			ID:           lti.ID,
			CourseModule: lti.CourseModule,
			Name:         strings.TrimSpace(lti.Name),
		})
	}
	return activities, nil
}

func (a WebexLTIActivity) MoodleLaunchPath() string {
	if a.CourseModule == 0 {
		return ""
	}
	return "/mod/lti/launch.php?id=" + url.QueryEscape(strconv.Itoa(a.CourseModule))
}
