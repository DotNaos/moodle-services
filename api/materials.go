package handler

import (
	"net/http"
	"strings"
	"time"

	contract "github.com/DotNaos/moodle-services/pkg/apicontracts"
	svc "github.com/DotNaos/moodle-services/pkg/moodleservices"
	"github.com/DotNaos/moodle-services/pkg/studypipeline"
)

func Materials(w http.ResponseWriter, r *http.Request) {
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
	if r.URL.Query().Get("route") == "study-pipeline" {
		status := "planned"
		if r.Method == http.MethodPost {
			status = "created"
		}
		svc.WriteJSON(w, http.StatusOK, studypipeline.Build(courseID, materials, status, time.Now()))
		return
	}
	if r.Method != http.MethodGet {
		svc.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	svc.WriteJSON(w, http.StatusOK, contract.MaterialsResponse{Materials: materials})
}
