package moodle

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
)

const DefaultMobileLaunchService = "moodle_mobile_app"
const DefaultMobileLaunchScheme = "studyreplay"

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

func BuildMobileLaunchURL(siteURL string, service string, passport string, urlScheme string) (MobileLaunchURL, error) {
	siteURL = normalizeMobileLaunchSiteURL(siteURL)
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
	sum := md5.Sum([]byte(normalizeMobileLaunchSiteURL(siteURL) + passport))
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
		SiteURL:      normalizeMobileLaunchSiteURL(siteURL),
		Token:        launch.Token,
		PrivateToken: launch.PrivateToken,
	}
}

func extractMobileLaunchPayload(raw string, parsed *url.URL) string {
	if token := parsed.Query().Get("token"); token != "" {
		return token
	}
	if parsed.Host == "token" {
		return strings.TrimPrefix(parsed.Opaque, "=")
	}
	if parsed.Host != "" && strings.HasPrefix(parsed.Host, "token=") {
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

func decodeMobileLaunchPayload(encoded string) (string, error) {
	encoded = strings.TrimSpace(encoded)
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err == nil {
		return string(decoded), nil
	}
	decoded, err = base64.RawURLEncoding.DecodeString(encoded)
	if err == nil {
		return string(decoded), nil
	}
	decoded, err = base64.URLEncoding.DecodeString(encoded)
	if err == nil {
		return string(decoded), nil
	}
	return "", fmt.Errorf("decode mobile launch payload: invalid base64")
}

func normalizeMobileLaunchSiteURL(siteURL string) string {
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
	if parsed.Path == "" {
		parsed.Path = ""
	}
	return strings.TrimRight(parsed.String(), "/")
}

func validateMobileLaunchScheme(scheme string) error {
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
