package moodleservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/DotNaos/moodle-services/internal/moodle"
)

const maxWebexPages = 50

// ErrWebexCredentialsRequired signals that the stored Webex browser session is
// missing or expired. Callers with stored credentials can re-create the session
// (auto-renew) and retry; otherwise the user must sign in again.
var ErrWebexCredentialsRequired = errors.New("Moodle web credentials are required for Webex recordings")

type WebexRecording struct {
	RecordingDate   string `json:"recordingDate"`
	RecordingName   string `json:"recordingName"`
	StreamURL       string `json:"streamUrl"`
	AudioURL        string `json:"audioUrl,omitempty"`
	TranscriptURL   string `json:"transcriptUrl,omitempty"`
	SourceURL       string `json:"sourceUrl,omitempty"`
	RecordingUUID   string `json:"recordingUuid"`
	CoverURL        string `json:"coverUrl,omitempty"`
	SessionTitle    string `json:"sessionTitle"`
	DurationSeconds int    `json:"durationSeconds,omitempty"`
}

type webexLTIClient interface {
	SiteURL() string
	FetchWebexLTIActivities(courseID string) ([]moodle.WebexLTIActivity, error)
}

func (s Service) ListWebexRecordings(ctx context.Context, courseID string) ([]WebexRecording, error) {
	courseID = strings.TrimSpace(courseID)
	if courseID == "" {
		return nil, fmt.Errorf("courseId is required")
	}
	if strings.TrimSpace(s.WebexSessionJSON) == "" {
		return nil, ErrWebexCredentialsRequired
	}
	client, ok := s.Client.(webexLTIClient)
	if !ok {
		return nil, fmt.Errorf("Moodle client does not support Webex LTI discovery")
	}
	activities, err := client.FetchWebexLTIActivities(courseID)
	if err != nil {
		return nil, err
	}
	if len(activities) == 0 {
		return []WebexRecording{}, nil
	}
	launchPath := activities[0].MoodleLaunchPath()
	if launchPath == "" {
		return []WebexRecording{}, nil
	}

	browser := newWebBrowser()
	if err := browser.jar.importJSON(s.WebexSessionJSON); err != nil {
		return nil, fmt.Errorf("restore Webex browser session: %w", err)
	}
	page, err := browser.request(ctx, http.MethodGet, absoluteURL(client.SiteURL(), launchPath), "", nil, nil)
	if err != nil {
		return nil, err
	}
	page, err = browser.followWebexFlow(ctx, page, 20)
	if err != nil {
		return nil, err
	}
	if !isWebexApplication(page.url, page.text) {
		return nil, ErrWebexCredentialsRequired
	}

	csrf := extractCSRFToken(page.text)
	sessions, err := fetchWebexPages(ctx, browser, webexLTIOrigin+"/api/webex/meeting_sessions?start_date=2015-01-01&end_date="+futureWebexEndDate(s.now())+"&with_recordings=true&page=", csrf, "meeting sessions")
	if err != nil {
		return nil, err
	}
	recordings := []WebexRecording{}
	for _, session := range sessions {
		sessionID := stringValue(session, "id", "meetingSessionId")
		if sessionID == "" || !sessionHasRecordings(session) {
			continue
		}
		items, err := fetchWebexPages(ctx, browser, webexLTIOrigin+"/api/webex/meeting_sessions/"+url.QueryEscape(sessionID)+"/recordings?page=", csrf, "recordings")
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			recording, err := recordingFromWebexItem(ctx, browser, item, session, csrf)
			if err != nil || recording.StreamURL == "" {
				continue
			}
			recordings = append(recordings, recording)
		}
	}
	sortRecordings(recordings)
	return recordings, nil
}

func (s Service) CreateWebexBrowserSession(ctx context.Context, courseID string, credentials WebexCredentials) (string, error) {
	if strings.TrimSpace(credentials.Username) == "" || strings.TrimSpace(credentials.Password) == "" {
		return "", fmt.Errorf("username and password are required")
	}
	client, ok := s.Client.(webexLTIClient)
	if !ok {
		return "", fmt.Errorf("Moodle client does not support Webex LTI discovery")
	}

	targetPath := "/my/"
	if strings.TrimSpace(courseID) != "" {
		activities, err := client.FetchWebexLTIActivities(strings.TrimSpace(courseID))
		if err != nil {
			return "", err
		}
		if len(activities) == 0 {
			return "", fmt.Errorf("no Webex activity found for this course")
		}
		if launchPath := activities[0].MoodleLaunchPath(); launchPath != "" {
			targetPath = launchPath
		}
	}

	browser := newWebBrowser()
	page, err := browser.loginFHGR(ctx, client.SiteURL(), targetPath, credentials)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(courseID) != "" {
		page, err = browser.followWebexFlow(ctx, page, 20)
		if err != nil {
			return "", err
		}
		if !isWebexApplication(page.url, page.text) {
			return "", fmt.Errorf("Webex application session could not be opened from %s", safeWebexPage(page.url))
		}
	}
	return browser.jar.exportJSON()
}

func fetchWebexPages(ctx context.Context, browser *webBrowser, prefix string, csrf string, label string) ([]map[string]any, error) {
	out := []map[string]any{}
	for page := 1; page <= maxWebexPages; page++ {
		result, err := browser.request(ctx, http.MethodGet, prefix+strconv.Itoa(page), webexAppURL, webexHeaders(csrf, webexAppURL), nil)
		if err != nil {
			return nil, err
		}
		if result.status < 200 || result.status >= 300 {
			return nil, fmt.Errorf("Webex %s request failed with HTTP %d", label, result.status)
		}
		var payload any
		if err := json.Unmarshal([]byte(result.text), &payload); err != nil {
			return nil, err
		}
		out = append(out, itemsFromPayload(payload)...)
		if !hasNextPage(payload, page) {
			return out, nil
		}
	}
	return out, nil
}

func recordingFromWebexItem(ctx context.Context, browser *webBrowser, item map[string]any, session map[string]any, csrf string) (WebexRecording, error) {
	duration := intValue(item, "duration", "recordingDuration", "durationSeconds")
	if duration > 0 && duration < 60 {
		return WebexRecording{}, nil
	}
	sourceURL := stringValue(item, "recording_url", "recordingUrl")
	uuid := nonEmpty(stringValue(item, "recordUUID", "recordUuid", "record_uuid", "recordingUuid", "recording_uuid", "uuid"), extractRecordUUID(sourceURL))
	password := stringValue(item, "accessPwd", "password", "recordingPassword")
	streamURL := ""
	audioURL := ""
	transcriptURL := ""
	coverURL := ""
	for _, candidate := range uniqueNonEmpty(uuid, stringValue(item, "id", "recordingId", "record_id")) {
		info, err := fetchStreamInfo(ctx, browser, candidate, password, csrf)
		if err == nil {
			streamURL = streamURLFromInfo(info)
			audioURL = audioURLFromInfo(info)
			transcriptURL = transcriptURLFromInfo(info)
			coverURL = coverURLFromInfo(info)
			uuid = candidate
			break
		}
	}
	if streamURL == "" && sourceURL != "" {
		public, err := browser.request(ctx, http.MethodGet, sourceURL, webexAppURL, nil, nil)
		if err == nil {
			public, err = followPublicRecording(ctx, browser, public)
		}
		resolvedUUID := ""
		if err == nil {
			resolvedUUID = nonEmpty(extractRecordUUID(public.url), extractRecordUUID(public.text))
			csrf = nonEmpty(extractCSRFToken(public.text), csrf)
		}
		for _, candidate := range uniqueNonEmpty(resolvedUUID) {
			info, err := fetchStreamInfo(ctx, browser, candidate, password, csrf)
			if err == nil {
				streamURL = streamURLFromInfo(info)
				audioURL = audioURLFromInfo(info)
				transcriptURL = transcriptURLFromInfo(info)
				coverURL = coverURLFromInfo(info)
				uuid = candidate
				break
			}
		}
	}
	name := nonEmpty(stringValue(item, "name", "recordName"), stringValue(session, "name", "title"), "Webex recording")
	return WebexRecording{
		RecordingDate:   deriveRecordingDate(name, stringValue(item, "created_at", "createTime", "gmtCreateTime")),
		RecordingName:   name,
		StreamURL:       streamURL,
		AudioURL:        audioURL,
		TranscriptURL:   transcriptURL,
		SourceURL:       sourceURL,
		RecordingUUID:   nonEmpty(uuid, sourceURL, name),
		CoverURL:        coverURL,
		SessionTitle:    nonEmpty(stringValue(session, "name", "title"), name),
		DurationSeconds: duration,
	}, nil
}

func followPublicRecording(ctx context.Context, browser *webBrowser, page browserPage) (browserPage, error) {
	current := page
	for step := 0; step < 8; step++ {
		next := redirectURL(current)
		if next == "" {
			next = htmlRedirect(current.text, current.url)
		}
		if next == "" {
			return current, nil
		}
		var err error
		current, err = browser.request(ctx, http.MethodGet, next, current.url, nil, nil)
		if err != nil {
			return browserPage{}, err
		}
	}
	return current, nil
}

func fetchStreamInfo(ctx context.Context, browser *webBrowser, uuid string, accessPassword string, csrf string) (map[string]any, error) {
	headers := webexHeaders(csrf, "https://"+webexSite+"/recordingservice/sites/fhgr/recording/playback/"+uuid)
	headers["clientType"] = "web"
	headers["siteFullUrl"] = webexSite
	headers["siteId"] = webexSiteID
	if accessPassword != "" {
		headers["accessPwd"] = accessPassword
	}
	result, err := browser.request(ctx, http.MethodGet, "https://"+webexSite+"/webappng/api/v1/recordings/"+url.PathEscape(uuid)+"/stream?siteurl=fhgr", webexAppURL, headers, nil)
	if err != nil {
		return nil, err
	}
	if result.status < 200 || result.status >= 300 {
		return nil, fmt.Errorf("Webex stream metadata failed with HTTP %d", result.status)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result.text), &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func webexHeaders(csrf string, referer string) map[string]string {
	headers := map[string]string{
		"Accept":  "application/json, text/plain, */*",
		"Referer": referer,
	}
	if csrf != "" {
		headers["x-csrf-token"] = csrf
	}
	return headers
}

func itemsFromPayload(payload any) []map[string]any {
	if list, ok := payload.([]any); ok {
		return mapsFromList(list)
	}
	root, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	for _, key := range []string{"items", "data", "meeting_sessions", "recordings"} {
		if list, ok := root[key].([]any); ok {
			return mapsFromList(list)
		}
	}
	return nil
}

func mapsFromList(list []any) []map[string]any {
	out := []map[string]any{}
	for _, item := range list {
		if typed, ok := item.(map[string]any); ok {
			out = append(out, typed)
		}
	}
	return out
}

func hasNextPage(payload any, page int) bool {
	root, ok := payload.(map[string]any)
	if !ok {
		return false
	}
	for _, key := range []string{"pagination", "paging", "page"} {
		if pagination, ok := root[key].(map[string]any); ok {
			if value, ok := pagination["hasNext"].(bool); ok {
				return value
			}
			if value, ok := pagination["has_next"].(bool); ok {
				return value
			}
			totalPages := numberValue(pagination["total_pages"], pagination["totalPages"])
			currentPage := numberValue(pagination["page"])
			if totalPages > 0 && currentPage > 0 {
				return currentPage < totalPages
			}
		}
	}
	return false
}

func sessionHasRecordings(session map[string]any) bool {
	for _, key := range []string{"has_recordings", "hasRecordings"} {
		if value, ok := session[key].(bool); ok {
			return value
		}
	}
	return true
}

func stringValue(root map[string]any, keys ...string) string {
	for _, key := range keys {
		switch value := root[key].(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		case float64:
			return strconv.FormatFloat(value, 'f', -1, 64)
		}
	}
	return ""
}

func intValue(root map[string]any, keys ...string) int {
	for _, key := range keys {
		if number := numberValue(root[key]); number > 0 {
			return int(number)
		}
	}
	return 0
}

func numberValue(values ...any) float64 {
	for _, value := range values {
		switch typed := value.(type) {
		case float64:
			return typed
		case int:
			return float64(typed)
		case string:
			parsed, err := strconv.ParseFloat(typed, 64)
			if err == nil {
				return parsed
			}
		}
	}
	return 0
}

func streamURLFromInfo(info map[string]any) string {
	return firstPath(info,
		[]string{"downloadRecordingInfo", "downloadInfo", "hlsURL"},
		[]string{"downloadInfo", "hlsURL"},
		[]string{"downloadRecordingInfo", "downloadInfo", "dashURL"},
		[]string{"downloadInfo", "dashURL"},
		[]string{"downloadRecordingInfo", "downloadInfo", "mp4URL"},
		[]string{"downloadInfo", "mp4URL"},
	)
}

func audioURLFromInfo(info map[string]any) string {
	return nonEmpty(firstPath(info,
		[]string{"downloadRecordingInfo", "downloadInfo", "mp3URL"},
		[]string{"downloadInfo", "mp3URL"},
		[]string{"downloadRecordingInfo", "downloadInfo", "audioURL"},
		[]string{"downloadInfo", "audioURL"},
		[]string{"downloadRecordingInfo", "downloadInfo", "m4aURL"},
		[]string{"downloadInfo", "m4aURL"},
		[]string{"downloadRecordingInfo", "downloadInfo", "audioDownloadURL"},
		[]string{"downloadInfo", "audioDownloadURL"},
	), findURLByHint(info, []string{"mp3", "m4a", "audio"}, []string{".mp3", ".m4a", "audio"}))
}

func transcriptURLFromInfo(info map[string]any) string {
	return nonEmpty(firstPath(info,
		[]string{"downloadRecordingInfo", "downloadInfo", "transcriptURL"},
		[]string{"downloadInfo", "transcriptURL"},
		[]string{"downloadRecordingInfo", "downloadInfo", "vttURL"},
		[]string{"downloadInfo", "vttURL"},
		[]string{"downloadRecordingInfo", "downloadInfo", "captionURL"},
		[]string{"downloadInfo", "captionURL"},
		[]string{"downloadRecordingInfo", "downloadInfo", "closedCaptionURL"},
		[]string{"downloadInfo", "closedCaptionURL"},
	), findURLByHint(info, []string{"transcript", "caption", "vtt", "subtitle"}, []string{".vtt", ".srt", "transcript", "caption"}))
}

func coverURLFromInfo(info map[string]any) string {
	return firstPath(info,
		[]string{"downloadRecordingInfo", "downloadInfo", "playerCoverURL"},
		[]string{"downloadInfo", "playerCoverURL"},
		[]string{"downloadInfo", "coverUrl"},
		[]string{"downloadInfo", "thumbnailUrl"},
	)
}

func firstPath(root map[string]any, paths ...[]string) string {
	for _, path := range paths {
		current := any(root)
		for _, part := range path {
			next, ok := current.(map[string]any)
			if !ok {
				current = nil
				break
			}
			current = next[part]
		}
		if value, ok := current.(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func findURLByHint(root any, keyHints []string, valueHints []string) string {
	return findURLByHintAt(root, "", keyHints, valueHints, 0)
}

func findURLByHintAt(root any, key string, keyHints []string, valueHints []string, depth int) string {
	if depth > 8 {
		return ""
	}
	switch typed := root.(type) {
	case map[string]any:
		for childKey, value := range typed {
			if found := findURLByHintAt(value, childKey, keyHints, valueHints, depth+1); found != "" {
				return found
			}
		}
	case []any:
		for _, value := range typed {
			if found := findURLByHintAt(value, key, keyHints, valueHints, depth+1); found != "" {
				return found
			}
		}
	case string:
		value := strings.TrimSpace(typed)
		if !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") {
			return ""
		}
		lowerKey := strings.ToLower(key)
		lowerValue := strings.ToLower(value)
		for _, hint := range keyHints {
			if strings.Contains(lowerKey, strings.ToLower(hint)) {
				return value
			}
		}
		for _, hint := range valueHints {
			if strings.Contains(lowerValue, strings.ToLower(hint)) {
				return value
			}
		}
	}
	return ""
}

var (
	csrfRe   = regexp.MustCompile(`(?is)<meta[^>]+name=["'](?:csrf-token|csrfToken|_csrf)["'][^>]+content=["']([^"']+)["']`)
	uuidExpr = regexp.MustCompile(`(?i)(?:recording/playback/|playback/|recording/)([a-f0-9-]{32,36})`)
	dateExpr = regexp.MustCompile(`(\d{4}-\d{2}-\d{2}|\d{8})`)
)

func extractCSRFToken(htmlText string) string {
	if match := csrfRe.FindStringSubmatch(htmlText); len(match) > 1 {
		return strings.TrimSpace(match[1])
	}
	return ""
}

func extractRecordUUID(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	if parsed, err := url.Parse(rawURL); err == nil {
		if rcid := parsed.Query().Get("RCID"); rcid != "" {
			return strings.ReplaceAll(rcid, "-", "")
		}
	}
	if match := uuidExpr.FindStringSubmatch(rawURL); len(match) > 1 {
		return strings.ReplaceAll(match[1], "-", "")
	}
	return ""
}

func deriveRecordingDate(values ...string) string {
	for _, value := range values {
		if match := dateExpr.FindStringSubmatch(value); len(match) > 1 {
			date := match[1]
			if len(date) == 8 {
				return date[:4] + "-" + date[4:6] + "-" + date[6:8]
			}
			return date
		}
	}
	return ""
}

func futureWebexEndDate(now time.Time) string {
	return strconv.Itoa(now.Year()+3) + "-12-31"
}

func sortRecordings(recordings []WebexRecording) {
	for i := 0; i < len(recordings); i++ {
		for j := i + 1; j < len(recordings); j++ {
			if recordings[j].RecordingDate > recordings[i].RecordingDate {
				recordings[i], recordings[j] = recordings[j], recordings[i]
			}
		}
	}
}

func safeWebexPage(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "[invalid-url]"
	}
	return parsed.Scheme + "://" + parsed.Host + parsed.Path
}

func uniqueNonEmpty(values ...string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
