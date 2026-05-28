package moodle

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/playwright-community/playwright-go"
)

const DefaultMobileLaunchService = "moodle_mobile_app"
const DefaultMobileLaunchScheme = "moodlecli"

type MobileLaunchURL struct {
	SiteURL   string
	Service   string
	Passport  string
	URLScheme string
	URL       string
	SiteID    string
}

type MobileLaunchToken struct {
	Raw          string
	Scheme       string
	SiteID       string
	Token        string
	PrivateToken string
}

type MobileLaunchOptions struct {
	SiteURL   string
	Cookies   string
	Service   string
	Passport  string
	URLScheme string
	Headless  bool
	Timeout   time.Duration
}

type MobileLaunchResult struct {
	LaunchURL MobileLaunchURL
	Callback  MobileLaunchToken
}

func BuildMobileLaunchURL(siteURL string, service string, passport string, urlScheme string) (MobileLaunchURL, error) {
	siteURL = normalizeSiteURL(siteURL)
	if siteURL == "" {
		return MobileLaunchURL{}, fmt.Errorf("site URL is required")
	}
	if service == "" {
		service = DefaultMobileLaunchService
	}
	if passport == "" {
		var err error
		passport, err = NewMobileLaunchPassport()
		if err != nil {
			return MobileLaunchURL{}, err
		}
	}
	if urlScheme == "" {
		urlScheme = DefaultMobileLaunchScheme
	}
	if err := validateMobileLaunchScheme(urlScheme); err != nil {
		return MobileLaunchURL{}, err
	}

	parsed, err := url.Parse(siteURL)
	if err != nil {
		return MobileLaunchURL{}, fmt.Errorf("parse site URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return MobileLaunchURL{}, fmt.Errorf("unsupported site URL scheme %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return MobileLaunchURL{}, fmt.Errorf("site URL is missing a host")
	}

	launch := *parsed
	launch.Path = strings.TrimRight(launch.Path, "/") + "/admin/tool/mobile/launch.php"
	query := launch.Query()
	query.Set("service", service)
	query.Set("passport", passport)
	query.Set("urlscheme", urlScheme)
	query.Set("confirmed", "0")
	query.Set("oauthsso", "0")
	launch.RawQuery = query.Encode()

	return MobileLaunchURL{
		SiteURL:   siteURL,
		Service:   service,
		Passport:  passport,
		URLScheme: urlScheme,
		URL:       launch.String(),
		SiteID:    ExpectedMobileLaunchSiteID(siteURL, passport),
	}, nil
}

func NewMobileLaunchPassport() (string, error) {
	var data [24]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", fmt.Errorf("generate launch passport: %w", err)
	}
	return hex.EncodeToString(data[:]), nil
}

func ExpectedMobileLaunchSiteID(siteURL string, passport string) string {
	sum := md5.Sum([]byte(normalizeSiteURL(siteURL) + passport))
	return hex.EncodeToString(sum[:])
}

func ParseMobileLaunchCallback(raw string) (MobileLaunchToken, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return MobileLaunchToken{}, fmt.Errorf("callback URL is empty")
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return MobileLaunchToken{}, fmt.Errorf("parse callback URL: %w", err)
	}
	if parsed.Scheme == "" {
		return MobileLaunchToken{}, fmt.Errorf("callback URL is missing a scheme")
	}

	encoded := extractMobileLaunchPayload(raw, parsed)
	if encoded == "" {
		return MobileLaunchToken{}, fmt.Errorf("callback URL does not contain a token payload")
	}

	decoded, err := decodeMobileLaunchPayload(encoded)
	if err != nil {
		return MobileLaunchToken{}, err
	}
	parts := strings.Split(decoded, ":::")
	if len(parts) < 2 {
		return MobileLaunchToken{}, fmt.Errorf("mobile launch payload has %d parts, expected at least 2", len(parts))
	}
	siteID := strings.TrimSpace(parts[0])
	token := strings.TrimSpace(parts[1])
	if siteID == "" {
		return MobileLaunchToken{}, fmt.Errorf("mobile launch payload missing site id")
	}
	if token == "" {
		return MobileLaunchToken{}, fmt.Errorf("mobile launch payload missing token")
	}
	privateToken := ""
	if len(parts) >= 3 {
		privateToken = strings.TrimSpace(parts[2])
	}

	return MobileLaunchToken{
		Raw:          raw,
		Scheme:       parsed.Scheme,
		SiteID:       siteID,
		Token:        token,
		PrivateToken: privateToken,
	}, nil
}

func MobileTokenFromLaunch(siteURL string, launch MobileLaunchToken) MobileToken {
	return MobileToken{
		SiteURL:      normalizeSiteURL(siteURL),
		Token:        launch.Token,
		PrivateToken: launch.PrivateToken,
	}
}

func LaunchMobileLoginWithSession(options MobileLaunchOptions) (MobileLaunchResult, error) {
	launchURL, err := BuildMobileLaunchURL(options.SiteURL, options.Service, options.Passport, options.URLScheme)
	if err != nil {
		return MobileLaunchResult{}, err
	}
	if strings.TrimSpace(options.Cookies) == "" {
		return MobileLaunchResult{}, fmt.Errorf("web session cookies are required")
	}
	timeout := options.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	pw, err := runPlaywrightWithAutoInstall()
	if err != nil {
		return MobileLaunchResult{}, err
	}
	defer pw.Stop()

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{Headless: playwright.Bool(options.Headless)})
	if err != nil {
		return MobileLaunchResult{}, err
	}
	defer browser.Close()

	context, err := browser.NewContext(playwright.BrowserNewContextOptions{
		UserAgent: playwright.String("Mozilla/5.0 MoodleMobile"),
	})
	if err != nil {
		return MobileLaunchResult{}, err
	}
	if err := addCookieHeaderToContext(context, launchURL.SiteURL, options.Cookies); err != nil {
		return MobileLaunchResult{}, err
	}

	page, err := context.NewPage()
	if err != nil {
		return MobileLaunchResult{}, err
	}

	tokenURL := make(chan string, 1)
	var once sync.Once
	capture := func(raw string) {
		if raw == "" || !strings.HasPrefix(strings.ToLower(raw), strings.ToLower(launchURL.URLScheme)+"://") {
			return
		}
		if !strings.Contains(raw, "token=") {
			return
		}
		once.Do(func() {
			tokenURL <- raw
		})
	}
	page.OnRequest(func(request playwright.Request) {
		capture(request.URL())
	})
	page.OnRequestFailed(func(request playwright.Request) {
		capture(request.URL())
	})
	page.OnFrameNavigated(func(frame playwright.Frame) {
		capture(frame.URL())
	})

	go func() {
		_, _ = page.Goto(launchURL.URL, playwright.PageGotoOptions{
			WaitUntil: playwright.WaitUntilStateDomcontentloaded,
			Timeout:   playwright.Float(float64(timeout.Milliseconds())),
		})
		capture(page.URL())
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case raw := <-tokenURL:
		callback, err := ParseMobileLaunchCallback(raw)
		if err != nil {
			return MobileLaunchResult{}, err
		}
		if callback.SiteID != launchURL.SiteID {
			return MobileLaunchResult{}, fmt.Errorf("mobile launch callback site id mismatch")
		}
		return MobileLaunchResult{LaunchURL: launchURL, Callback: callback}, nil
	case <-timer.C:
		return MobileLaunchResult{}, fmt.Errorf("mobile launch login timed out before Moodle returned a token")
	}
}

func FetchMobileSiteInfo(token MobileToken) (MobileSiteInfo, error) {
	client := &Client{http: &http.Client{Timeout: 60 * time.Second}}
	return client.FetchMobileSiteInfo(token)
}

func FetchMobileUserCourses(token MobileToken) ([]MobileCourse, error) {
	client := &Client{http: &http.Client{Timeout: 60 * time.Second}}
	return client.FetchMobileUserCourses(token)
}

func extractMobileLaunchPayload(raw string, parsed *url.URL) string {
	lower := strings.ToLower(raw)
	if idx := strings.Index(lower, "://token="); idx >= 0 {
		payload := raw[idx+len("://token="):]
		if cut := strings.IndexAny(payload, "?#"); cut >= 0 {
			payload = payload[:cut]
		}
		return payload
	}
	if value := parsed.Query().Get("token"); value != "" {
		return value
	}
	if strings.HasPrefix(strings.ToLower(parsed.Host), "token=") {
		return parsed.Host[len("token="):] + strings.TrimPrefix(parsed.EscapedPath(), "/")
	}
	return ""
}

func decodeMobileLaunchPayload(encoded string) (string, error) {
	encoded = strings.TrimSpace(encoded)
	if decoded, err := base64.StdEncoding.DecodeString(encoded); err == nil {
		return string(decoded), nil
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(encoded); err == nil {
		return string(decoded), nil
	}
	return "", fmt.Errorf("decode mobile launch payload: invalid base64")
}

func addCookieHeaderToContext(context playwright.BrowserContext, siteURL string, cookieHeader string) error {
	cookies := parseCookieHeader(siteURL, cookieHeader)
	if len(cookies) == 0 {
		return fmt.Errorf("web session cookie header contains no cookies")
	}
	return context.AddCookies(cookies)
}

func parseCookieHeader(siteURL string, cookieHeader string) []playwright.OptionalCookie {
	parts := strings.Split(cookieHeader, ";")
	out := make([]playwright.OptionalCookie, 0, len(parts))
	for _, part := range parts {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		if name == "" {
			continue
		}
		out = append(out, playwright.OptionalCookie{
			Name:  name,
			Value: value,
			URL:   playwright.String(siteURL),
		})
	}
	return out
}

func normalizeSiteURL(siteURL string) string {
	return strings.TrimRight(strings.TrimSpace(siteURL), "/")
}

func validateMobileLaunchScheme(scheme string) error {
	if scheme == "" {
		return fmt.Errorf("URL scheme is required")
	}
	for i, r := range scheme {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case i > 0 && r >= '0' && r <= '9':
		case i > 0 && (r == '-' || r == '+' || r == '.'):
		default:
			return fmt.Errorf("invalid URL scheme %q", scheme)
		}
	}
	return nil
}
