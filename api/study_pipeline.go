package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/DotNaos/moodle-services/internal/studypipeline"
	svc "github.com/DotNaos/moodle-services/pkg/moodleservices"
)

func StudyPipeline(w http.ResponseWriter, r *http.Request) {
	if !svc.AllowMethods(w, r, http.MethodGet, http.MethodPost) {
		return
	}
	courseID := strings.TrimSpace(r.URL.Query().Get("courseId"))
	if courseID == "" {
		svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "courseId query parameter is required"})
		return
	}

	service, closeFn, err := svc.ServiceForRequest(r, svc.LoadServerEnv())
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	defer closeFn()

	materials, err := service.ListMaterials(courseID)
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	status := "planned"
	if r.Method == http.MethodPost {
		status = "created"
	}
	svc.WriteJSON(w, http.StatusOK, studypipeline.Build(courseID, materials, status, time.Now()))
}
