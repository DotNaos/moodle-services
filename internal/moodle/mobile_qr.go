package moodle

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
)

const moodleMobileScheme = "moodlemobile://"
const mobileQRTokenFunction = "tool_mobile_get_tokens_for_qr_login"

var profileQRImagePattern = regexp.MustCompile(`(?is)<div[^>]+id=["']qrcode["'][^>]*>.*?<img[^>]+src=["']data:image/png;base64,([^"']+)["']`)

type MobileQRLink struct {
	Raw         string
	SiteURL     string
	QRLoginKey  string
	UserID      int
	IsAutoLogin bool
}

type MobileToken struct {
	SiteURL      string
	UserID       int
	Token        string
	PrivateToken string
	QRLoginKey   string
}

type MobileSiteInfo struct {
	SiteName string `json:"sitename"`
	UserName string `json:"username"`
	UserID   int    `json:"userid"`
	SiteURL  string `json:"siteurl"`
}

type MobileCourse struct {
	ID            int                    `json:"id"`
	FullName      string                 `json:"fullname"`
	ShortName     string                 `json:"shortname"`
	Visible       any                    `json:"visible"`
	CategoryID    int                    `json:"category"`
	CourseImage   string                 `json:"courseimage"`
	OverviewFiles []MobileCourseOverview `json:"overviewfiles"`
}

type MobileCourseOverview struct {
	FileName     string `json:"filename"`
	FilePath     string `json:"filepath"`
	FileSize     int    `json:"filesize"`
	FileURL      string `json:"fileurl"`
	TimeModified int64  `json:"timemodified"`
	MimeType     string `json:"mimetype"`
}

func ParseMobileQRLink(raw string) (MobileQRLink, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return MobileQRLink{}, fmt.Errorf("link is empty")
	}
	if !strings.HasPrefix(strings.ToLower(raw), moodleMobileScheme) {
		return MobileQRLink{}, fmt.Errorf("link must start with %s", moodleMobileScheme)
	}

	trimmed := raw[len(moodleMobileScheme):]
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return MobileQRLink{}, fmt.Errorf("parse link: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return MobileQRLink{}, fmt.Errorf("unsupported site URL scheme %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return MobileQRLink{}, fmt.Errorf("link is missing a Moodle host")
	}

	query := parsed.Query()
	link := MobileQRLink{
		Raw:         raw,
		SiteURL:     (&url.URL{Scheme: parsed.Scheme, Host: parsed.Host}).String(),
		QRLoginKey:  query.Get("qrlogin"),
		IsAutoLogin: query.Get("qrlogin") != "",
	}

	if userIDRaw := query.Get("userid"); userIDRaw != "" {
		userID, err := strconv.Atoi(userIDRaw)
		if err != nil {
			return MobileQRLink{}, fmt.Errorf("invalid userid %q", userIDRaw)
		}
		link.UserID = userID
	}

	return link, nil
}

func ExtractMobileQRLinkFromProfileHTML(html string) (MobileQRLink, error) {
	matches := profileQRImagePattern.FindStringSubmatch(html)
	if len(matches) < 2 {
		return MobileQRLink{}, fmt.Errorf("mobile QR image not found in profile page")
	}
	return DecodeMobileQRPNGBase64(matches[1])
}

func DecodeMobileQRPNGBase64(encoded string) (MobileQRLink, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return MobileQRLink{}, fmt.Errorf("decode QR image: %w", err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return MobileQRLink{}, fmt.Errorf("decode QR PNG: %w", err)
	}
	bitmap, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		return MobileQRLink{}, fmt.Errorf("prepare QR bitmap: %w", err)
	}
	result, err := qrcode.NewQRCodeReader().Decode(bitmap, nil)
	if err != nil {
		return MobileQRLink{}, fmt.Errorf("decode QR contents: %w", err)
	}
	return ParseMobileQRLink(result.String())
}

func (c *Client) FetchMobileQRLink() (MobileQRLink, error) {
	body, err := c.FetchPage("/user/profile.php")
	if err != nil {
		return MobileQRLink{}, err
	}
	return ExtractMobileQRLinkFromProfileHTML(body)
}

func (c *Client) ExchangeMobileQRToken(link MobileQRLink) (MobileToken, error) {
	httpClient := c.http
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return exchangeMobileQRToken(httpClient, link)
}

func ExchangeMobileQRToken(link MobileQRLink) (MobileToken, error) {
	return exchangeMobileQRToken(http.DefaultClient, link)
}

func exchangeMobileQRToken(httpClient *http.Client, link MobileQRLink) (MobileToken, error) {
	if !link.IsAutoLogin {
		return MobileToken{}, fmt.Errorf("QR link does not contain an automatic login key")
	}
	if link.UserID == 0 {
		return MobileToken{}, fmt.Errorf("QR link is missing user id")
	}

	payload := []map[string]any{
		{
			"index":      0,
			"methodname": mobileQRTokenFunction,
			"args": map[string]string{
				"qrloginkey": link.QRLoginKey,
				"userid":     strconv.Itoa(link.UserID),
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return MobileToken{}, err
	}
	req, err := http.NewRequest(http.MethodPost, link.MobileTokenEndpoint(), bytes.NewReader(body))
	if err != nil {
		return MobileToken{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("User-Agent", "Mozilla/5.0 MoodleMobile")
	resp, err := httpClient.Do(req)
	if err != nil {
		return MobileToken{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return MobileToken{}, fmt.Errorf("QR token exchange failed: %s (%s)", resp.Status, strings.TrimSpace(string(body)))
	}

	var result []struct {
		Error bool `json:"error"`
		Data  struct {
			Token        string `json:"token"`
			PrivateToken string `json:"privatetoken"`
		} `json:"data"`
		Exception struct {
			Message   string `json:"message"`
			ErrorCode string `json:"errorcode"`
		} `json:"exception"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return MobileToken{}, fmt.Errorf("decode QR token response: %w", err)
	}
	if len(result) == 0 {
		return MobileToken{}, fmt.Errorf("QR token exchange returned an empty response")
	}
	if result[0].Error {
		msg := strings.TrimSpace(result[0].Exception.Message)
		if msg == "" {
			msg = result[0].Exception.ErrorCode
		}
		return MobileToken{}, fmt.Errorf("QR token exchange failed: %s", msg)
	}
	if result[0].Data.Token == "" {
		return MobileToken{}, fmt.Errorf("QR token exchange returned no token")
	}

	return MobileToken{
		SiteURL:      link.SiteURL,
		UserID:       link.UserID,
		Token:        result[0].Data.Token,
		PrivateToken: result[0].Data.PrivateToken,
		QRLoginKey:   link.QRLoginKey,
	}, nil
}

func (c *Client) CallMobileAPI(token MobileToken, function string, values url.Values, target any) error {
	if token.Token == "" {
		return fmt.Errorf("mobile token is empty")
	}
	if values == nil {
		values = url.Values{}
	}
	values.Set("wstoken", token.Token)
	values.Set("wsfunction", function)

	req, err := http.NewRequest(http.MethodPost, token.RESTEndpoint(), strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 MoodleMobile")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("mobile API request failed: %s (%s)", resp.Status, strings.TrimSpace(string(body)))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 20*1024*1024))
	if err != nil {
		return err
	}
	var apiErr mobileAPIError
	if err := json.Unmarshal(data, &apiErr); err == nil && apiErr.Exception != "" {
		return fmt.Errorf("mobile API error: %s", apiErr.Message)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode mobile API response: %w", err)
	}
	return nil
}

func (c *Client) FetchMobileSiteInfo(token MobileToken) (MobileSiteInfo, error) {
	var info MobileSiteInfo
	if err := c.CallMobileAPI(token, "core_webservice_get_site_info", nil, &info); err != nil {
		return MobileSiteInfo{}, err
	}
	return info, nil
}

func (c *Client) FetchMobileUserCourses(token MobileToken) ([]MobileCourse, error) {
	var courses []MobileCourse
	values := url.Values{}
	values.Set("userid", strconv.Itoa(token.UserID))
	if err := c.CallMobileAPI(token, "core_enrol_get_users_courses", values, &courses); err != nil {
		return nil, err
	}
	return courses, nil
}

type mobileAPIError struct {
	Exception string `json:"exception"`
	Message   string `json:"message"`
}

func (l MobileQRLink) TokensEndpoint() string {
	return l.SiteURL + "/webservice/rest/server.php?moodlewsrestformat=json"
}

func (l MobileQRLink) MobileTokenEndpoint() string {
	return l.SiteURL + "/lib/ajax/service-nologin.php?info=" + mobileQRTokenFunction + "&lang=de_ch"
}

func (l MobileQRLink) PublicConfigEndpoint() string {
	return l.SiteURL + "/lib/ajax/service-nologin.php?info=tool_mobile_get_public_config&lang=en"
}

func (t MobileToken) RESTEndpoint() string {
	return t.SiteURL + "/webservice/rest/server.php?moodlewsrestformat=json"
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
