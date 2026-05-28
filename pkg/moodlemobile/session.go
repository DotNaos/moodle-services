package moodlemobile

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

type Token struct {
	SiteURL      string
	UserID       int
	Token        string
	PrivateToken string
}

type Session struct {
	SchoolID     string    `json:"schoolId,omitempty"`
	SiteURL      string    `json:"siteUrl"`
	UserID       int       `json:"userId"`
	Token        string    `json:"token"`
	PrivateToken string    `json:"privateToken,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}

type SiteInfo struct {
	SiteName string `json:"sitename"`
	UserName string `json:"username"`
	UserID   int    `json:"userid"`
	SiteURL  string `json:"siteurl"`
}

type Client struct {
	Session Session
	http    *http.Client
}

func SessionFromToken(token Token) Session {
	return Session{
		SiteURL:      NormalizeSiteURL(token.SiteURL),
		UserID:       token.UserID,
		Token:        token.Token,
		PrivateToken: token.PrivateToken,
		CreatedAt:    time.Now(),
	}
}

func LoadSession(path string) (Session, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Session{}, err
	}
	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return Session{}, err
	}
	if err := ValidateSession(session); err != nil {
		return Session{}, err
	}
	return session, nil
}

func SaveSession(path string, session Session) error {
	if err := ValidateSession(session); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func ValidateSession(session Session) error {
	if strings.TrimSpace(session.SiteURL) == "" {
		return fmt.Errorf("mobile session missing siteUrl")
	}
	if session.UserID == 0 {
		return fmt.Errorf("mobile session missing userId")
	}
	if strings.TrimSpace(session.Token) == "" {
		return fmt.Errorf("mobile session missing token")
	}
	return nil
}

func NewClient(session Session) (*Client, error) {
	if err := ValidateSession(session); err != nil {
		return nil, err
	}
	return &Client{
		Session: session,
		http:    &http.Client{Timeout: 60 * time.Second},
	}, nil
}

func (c *Client) ValidateSession() error {
	_, err := c.FetchSiteInfo()
	return err
}

func FetchSiteInfo(token Token) (SiteInfo, error) {
	var info SiteInfo
	if err := callToken(token, "core_webservice_get_site_info", nil, &info); err != nil {
		return SiteInfo{}, err
	}
	return info, nil
}

func (c *Client) FetchSiteInfo() (SiteInfo, error) {
	var info SiteInfo
	if err := c.call("core_webservice_get_site_info", nil, &info); err != nil {
		return SiteInfo{}, err
	}
	return info, nil
}

func (c *Client) FetchCourses() ([]Course, error) {
	var courses []mobileCourse
	values := url.Values{}
	values.Set("userid", strconv.Itoa(c.Session.UserID))
	if err := c.call("core_enrol_get_users_courses", values, &courses); err != nil {
		return nil, err
	}

	result := make([]Course, 0, len(courses))
	for _, course := range courses {
		result = append(result, Course{
			ID:        course.ID,
			FullName:  course.FullName,
			ShortName: course.ShortName,
			Category:  strconv.Itoa(course.CategoryID),
			HeroImage: course.CourseImage,
			ViewURL:   strings.TrimRight(c.Session.SiteURL, "/") + "/course/view.php?id=" + strconv.Itoa(course.ID),
		})
	}
	return result, nil
}

func (c *Client) call(function string, values url.Values, target any) error {
	token := Token{
		SiteURL:      c.Session.SiteURL,
		UserID:       c.Session.UserID,
		Token:        c.Session.Token,
		PrivateToken: c.Session.PrivateToken,
	}
	return callToken(token, function, values, target)
}

func callToken(token Token, function string, values url.Values, target any) error {
	endpoint := token.RESTEndpoint()
	form := url.Values{}
	form.Set("wstoken", token.Token)
	form.Set("wsfunction", function)
	form.Set("moodlewsrestformat", "json")
	for key, value := range values {
		for _, item := range value {
			form.Add(key, item)
		}
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 MoodleMobile")
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("mobile api %s failed: %s (%s)", function, resp.Status, strings.TrimSpace(string(body)))
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if looksLikeMobileException(data) {
		return fmt.Errorf("mobile api %s failed: %s", function, strings.TrimSpace(string(data)))
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode mobile api %s response: %w", function, err)
	}
	return nil
}

func (t Token) RESTEndpoint() string {
	return strings.TrimRight(NormalizeSiteURL(t.SiteURL), "/") + "/webservice/rest/server.php?moodlewsrestformat=json"
}

func RedactSecret(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 10 {
		return strings.Repeat("*", len(value))
	}
	return value[:6] + strings.Repeat("*", len(value)-10) + value[len(value)-4:]
}

func looksLikeMobileException(data []byte) bool {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return false
	}
	_, hasException := raw["exception"]
	_, hasErrorCode := raw["errorcode"]
	return hasException || hasErrorCode
}
