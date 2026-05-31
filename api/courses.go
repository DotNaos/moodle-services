package handler

import (
	"net/http"
	"strconv"
	"strings"

	contract "github.com/DotNaos/moodle-services/pkg/apicontracts"
	svc "github.com/DotNaos/moodle-services/pkg/moodleservices"
)

func Courses(w http.ResponseWriter, r *http.Request) {
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
		recordings, err := service.ListWebexRecordings(r.Context(), courseID)
		if err != nil {
			svc.WriteError(w, err)
			return
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
