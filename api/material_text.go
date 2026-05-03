package handler

import (
	"net/http"
	"strings"

	contract "github.com/DotNaos/moodle-services/pkg/apicontracts"
	svc "github.com/DotNaos/moodle-services/pkg/moodleservices"
)

func MaterialText(w http.ResponseWriter, r *http.Request) {
	if !svc.AllowMethods(w, r, http.MethodGet) {
		return
	}
	courseID := strings.TrimSpace(r.URL.Query().Get("courseId"))
	resourceID := strings.TrimSpace(r.URL.Query().Get("resourceId"))
	if courseID == "" || resourceID == "" {
		svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "courseId and resourceId query parameters are required"})
		return
	}
	service, closeFn, err := svc.ServiceForRequest(r, svc.LoadServerEnv())
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	defer closeFn()
	doc, err := service.MaterialText(courseID, resourceID)
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	svc.WriteJSON(w, http.StatusOK, contract.MaterialTextResponse{Document: doc})
}
