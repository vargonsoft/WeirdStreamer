package handlers

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/weirdstreamer/server/internal/auth"
	"github.com/weirdstreamer/server/internal/jobs"
	"github.com/weirdstreamer/server/internal/media"
)

// jobsResponse is Settings → Jobs: each background job's status, and the load they wait on.
type jobsResponse struct {
	Jobs []jobs.Status    `json:"jobs"`
	Load media.LoadStatus `json:"load"`
}

// ListJobs returns the background jobs' status (admins).
func (h *MediaHandler) ListJobs(w http.ResponseWriter, r *http.Request) {
	if claims := auth.GetClaims(r.Context()); claims == nil || !claims.IsAdmin {
		respondError(w, http.StatusForbidden, "Admin access required")
		return
	}
	respondJSON(w, http.StatusOK, jobsResponse{Jobs: jobs.All(), Load: media.CurrentLoad(h.streams)})
}

// ControlJob runs, pauses or resumes a job (admins): POST /api/admin/jobs/{id}/{run|pause|resume}.
func (h *MediaHandler) ControlJob(w http.ResponseWriter, r *http.Request) {
	if claims := auth.GetClaims(r.Context()); claims == nil || !claims.IsAdmin {
		respondError(w, http.StatusForbidden, "Admin access required")
		return
	}
	vars := mux.Vars(r)
	j := jobs.Get(vars["id"])
	if j == nil {
		respondError(w, http.StatusNotFound, "No such job")
		return
	}
	switch vars["action"] {
	case "run":
		if !j.RunNow() {
			respondError(w, http.StatusBadRequest, "This job can't be started by hand")
			return
		}
	case "pause":
		j.Pause()
	case "resume":
		j.Resume()
	default:
		respondError(w, http.StatusBadRequest, "Unknown action")
		return
	}
	respondJSON(w, http.StatusOK, jobsResponse{Jobs: jobs.All(), Load: media.CurrentLoad(h.streams)})
}
