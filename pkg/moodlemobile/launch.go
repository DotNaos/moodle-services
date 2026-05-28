package moodlemobile

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
)

const DefaultLaunchService = "moodle_mobile_app"
const DefaultLaunchScheme = "studyreplay"

type LaunchURL struct {
	SiteURL   string `json:"siteUrl"`
	Service   string `json:"service"`
	Passport  string `json:"passport"`
	URLScheme string `json:"urlScheme"`
	URL       string `json:"url"`
	SiteID    string `json:"siteId"`
}

type LaunchToken struct {
	Raw          string `json:"-"`
	Scheme       string `json:"scheme"`
	SiteID       string `json:"siteId"`
	Token        string `json:"token"`
	PrivateToken string `json:"privateToken,omitempty"`
}

func BuildLaunchURL(siteURL string, service string, passport string, urlScheme string) (LaunchURL, error) {
	siteURL = NormalizeSiteURL(siteURL)
	if siteURL == "" {
		return LaunchURL{}, fmt.Errorf("site URL is required")
	}
	if service == "" {
		service = DefaultLaunchService
	}
	if passport == "" {
		var err error
		passport, err = NewPassport()
		if err != nil {
			return LaunchURL{}, err
		}
	}
	if urlScheme == "" {
		urlScheme = DefaultLaunchScheme
	}
	if err := validateURLScheme(urlScheme); err != nil {
		return LaunchURL{}, err
	}

	parsed, err := url.Parse(siteURL)
	if err != nil {
		return LaunchURL{}, fmt.Errorf("parse site URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return LaunchURL{}, fmt.Errorf("unsupported site URL scheme %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return LaunchURL{}, fmt.Errorf("site URL is missing a host")
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

	return LaunchURL{
		SiteURL:   siteURL,
		Service:   service,
		Passport:  passport,
		URLScheme: urlScheme,
		URL:       launch.String(),
		SiteID:    ExpectedSiteID(siteURL, passport),
	}, nil
}

func NewPassport() (string, error) {
	var data [24]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", fmt.Errorf("generate launch passport: %w", err)
	}
	return hex.EncodeToString(data[:]), nil
}

func ExpectedSiteID(siteURL string, passport string) string {
	sum := md5.Sum([]byte(NormalizeSiteURL(siteURL) + passport))
	return hex.EncodeToString(sum[:])
}

func ParseCallback(raw string) (LaunchToken, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return LaunchToken{}, fmt.Errorf("callback URL is empty")
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return LaunchToken{}, fmt.Errorf("parse callback URL: %w", err)
	}
	if parsed.Scheme == "" {
		return LaunchToken{}, fmt.Errorf("callback URL is missing a scheme")
	}

	encoded := extractPayload(raw, parsed)
	if encoded == "" {
		return LaunchToken{}, fmt.Errorf("callback URL does not contain a token payload")
	}

	decoded, err := decodePayload(encoded)
	if err != nil {
		return LaunchToken{}, err
	}
	parts := strings.Split(decoded, ":::")
	if len(parts) < 2 {
		return LaunchToken{}, fmt.Errorf("mobile launch payload has %d parts, expected at least 2", len(parts))
	}
	siteID := strings.TrimSpace(parts[0])
	token := strings.TrimSpace(parts[1])
	if siteID == "" {
		return LaunchToken{}, fmt.Errorf("mobile launch payload missing site id")
	}
	if token == "" {
		return LaunchToken{}, fmt.Errorf("mobile launch payload missing token")
	}
	privateToken := ""
	if len(parts) >= 3 {
		privateToken = strings.TrimSpace(parts[2])
	}

	return LaunchToken{
		Raw:          raw,
		Scheme:       parsed.Scheme,
		SiteID:       siteID,
		Token:        token,
		PrivateToken: privateToken,
	}, nil
}

func TokenFromLaunch(siteURL string, launch LaunchToken) Token {
	return Token{
		SiteURL:      NormalizeSiteURL(siteURL),
		Token:        launch.Token,
		PrivateToken: launch.PrivateToken,
	}
}

func NormalizeSiteURL(siteURL string) string {
	siteURL = strings.TrimSpace(siteURL)
	if siteURL == "" {
		return ""
	}
	parsed, err := url.Parse(siteURL)
	if err != nil {
		return strings.TrimRight(siteURL, "/")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return strings.TrimRight(parsed.String(), "/")
}

func extractPayload(raw string, parsed *url.URL) string {
	if token := parsed.Query().Get("token"); token != "" {
		return token
	}
	if parsed.Host == "token" {
		return strings.TrimPrefix(parsed.Opaque, "=")
	}
	if strings.HasPrefix(parsed.Host, "token=") {
		return strings.TrimPrefix(parsed.Host, "token=")
	}
	if strings.Contains(raw, "://token=") {
		return strings.SplitN(raw, "://token=", 2)[1]
	}
	if strings.Contains(raw, ":token=") {
		return strings.SplitN(raw, ":token=", 2)[1]
	}
	return ""
}

func decodePayload(encoded string) (string, error) {
	encoded = strings.TrimSpace(encoded)
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawURLEncoding, base64.URLEncoding} {
		decoded, err := encoding.DecodeString(encoded)
		if err == nil {
			return string(decoded), nil
		}
	}
	return "", fmt.Errorf("decode mobile launch payload: invalid base64")
}

func validateURLScheme(scheme string) error {
	if scheme == "" {
		return fmt.Errorf("mobile launch URL scheme is required")
	}
	for _, r := range scheme {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '+' || r == '-' || r == '.' {
			continue
		}
		return fmt.Errorf("mobile launch URL scheme %q contains unsupported character %q", scheme, r)
	}
	return nil
}
