package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	contract "github.com/DotNaos/moodle-services/pkg/apicontracts"
	svc "github.com/DotNaos/moodle-services/pkg/moodleservices"
)

const webexRecordingCacheMaxAge = 24 * time.Hour

// loadWebexRecordingsWithRenew fetches recordings and, if the stored Webex
// session is missing/expired, silently re-creates it from the stored encrypted
// credentials and retries once. If no credentials are stored or renewal fails,
// the original ErrWebexCredentialsRequired is surfaced so the UI shows the
// sign-in form.
func loadWebexRecordingsWithRenew(
	ctx context.Context,
	service *svc.Service,
	st *svc.Store,
	userID string,
	courseID string,
) ([]svc.WebexRecording, error) {
	recordings, err := service.ListWebexRecordings(ctx, courseID)
	if err == nil || !errors.Is(err, svc.ErrWebexCredentialsRequired) {
		return recordings, err
	}

	credentials, ok := parseWebexCredentials(service.WebexCredentialsJSON)
	if !ok {
		return nil, err
	}
	sessionJSON, renewErr := service.CreateWebexBrowserSession(ctx, courseID, credentials)
	if renewErr != nil {
		return nil, err
	}

	cfg := svc.LoadServerEnv()
	if box, boxErr := svc.EncryptionBox(cfg); boxErr == nil {
		if encrypted, encErr := box.EncryptString(sessionJSON); encErr == nil {
			_ = st.UpsertWebexSession(ctx, svc.UpsertWebexSessionInput{
				UserID:                    userID,
				EncryptedWebexSessionJSON: encrypted,
			})
		}
	}
	service.WebexSessionJSON = sessionJSON
	return service.ListWebexRecordings(ctx, courseID)
}

func parseWebexCredentials(raw string) (svc.WebexCredentials, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return svc.WebexCredentials{}, false
	}
	var parsed struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return svc.WebexCredentials{}, false
	}
	if strings.TrimSpace(parsed.Username) == "" || strings.TrimSpace(parsed.Password) == "" {
		return svc.WebexCredentials{}, false
	}
	return svc.WebexCredentials{Username: parsed.Username, Password: parsed.Password}, true
}

func Courses(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("route") == "calendar-subscription" {
		handleCalendarSubscription(w, r)
		return
	}
	if !svc.AllowMethods(w, r, http.MethodGet) {
		return
	}
	service, closeFn, err := svc.ServiceForRequest(r, svc.LoadServerEnv())
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	defer closeFn()
	if r.URL.Query().Get("route") == "categories" {
		categories, err := service.ListCategories()
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		svc.WriteJSON(w, http.StatusOK, contract.CategoriesResponse{Categories: categories})
		return
	}
	if r.URL.Query().Get("route") == "recordings" {
		courseID := strings.TrimSpace(r.URL.Query().Get("courseId"))
		if courseID == "" {
			svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "courseId query parameter is required"})
			return
		}
		refresh := r.URL.Query().Get("refresh") == "1" || r.URL.Query().Get("cache") == "bypass"
		st, user, _, err := svc.AuthenticatedUser(r, svc.LoadServerEnv())
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		defer st.Close()
		if !refresh {
			cached, ok, err := st.CachedWebexRecordings(r.Context(), user.ID, courseID, webexRecordingCacheMaxAge)
			if err != nil {
				svc.WriteError(w, err)
				return
			}
			if ok {
				var recordings []svc.WebexRecording
				if err := json.Unmarshal(cached.RecordingsJSON, &recordings); err != nil {
					svc.WriteError(w, err)
					return
				}
				svc.WriteJSON(w, http.StatusOK, contract.WebexRecordingsResponse{Recordings: recordings})
				return
			}
		}
		recordings, err := loadWebexRecordingsWithRenew(r.Context(), &service, st, user.ID, courseID)
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		if encoded, err := json.Marshal(recordings); err == nil {
			_ = st.UpsertWebexRecordings(r.Context(), user.ID, courseID, encoded)
		}
		svc.WriteJSON(w, http.StatusOK, contract.WebexRecordingsResponse{Recordings: recordings})
		return
	}
	if r.URL.Query().Get("route") == "calendar" {
		days := 30
		if rawDays := strings.TrimSpace(r.URL.Query().Get("days")); rawDays != "" {
			parsedDays, err := strconv.Atoi(rawDays)
			if err != nil || parsedDays <= 0 {
				svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "days query parameter must be a positive number"})
				return
			}
			days = parsedDays
		}
		events, err := service.CalendarEvents(days)
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		svc.WriteJSON(w, http.StatusOK, contract.CalendarEventsResponse{Events: events})
		return
	}
	courses, err := service.ListCourses()
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	svc.WriteJSON(w, http.StatusOK, contract.CoursesResponse{Courses: courses})
}

func handleCalendarSubscription(w http.ResponseWriter, r *http.Request) {
	if !svc.AllowMethods(w, r, http.MethodGet, http.MethodPost) {
		return
	}
	cfg := svc.LoadServerEnv()
	st, user, _, err := svc.AuthenticatedUser(r, cfg)
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	defer st.Close()

	switch r.Method {
	case http.MethodGet:
		configured, err := st.CalendarSubscriptionConfigured(r.Context(), user.ID)
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		svc.WriteJSON(w, http.StatusOK, contract.CalendarSubscriptionResponse{Configured: configured})
	case http.MethodPost:
		var input contract.CalendarSubscriptionRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
			return
		}
		nextURL := strings.TrimSpace(input.URL)
		if !isLikelyCalendarURL(nextURL) {
			svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "url must be a valid http(s) calendar link"})
			return
		}
		box, err := svc.EncryptionBox(cfg)
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		encryptedURL, err := box.EncryptString(nextURL)
		if err != nil {
			svc.WriteError(w, err)
			return
		}
		if err := st.UpsertCalendarSubscription(r.Context(), user.ID, encryptedURL); err != nil {
			svc.WriteError(w, err)
			return
		}
		svc.WriteJSON(w, http.StatusOK, contract.CalendarSubscriptionResponse{
			Configured: true,
			Saved:      true,
			URLHint:    calendarURLHint(nextURL),
		})
	}
}

func isLikelyCalendarURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

func calendarURLHint(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" {
		return ""
	}
	file := parsed.Path
	if idx := strings.LastIndex(file, "/"); idx >= 0 {
		file = file[idx+1:]
	}
	if file == "" {
		return parsed.Host
	}
	return parsed.Host + "/…/" + file
}
