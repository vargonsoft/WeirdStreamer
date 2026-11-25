package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/weirdstreamer/server/internal/auth"
	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/jobs"
	"github.com/weirdstreamer/server/internal/media"
)

// jobsResponse is Settings → Jobs: each background job's status, the load they wait on, and how
// much the server's data folders hold (counted every few minutes).
type jobsResponse struct {
	Jobs           []jobs.Status      `json:"jobs"`
	Load           media.LoadStatus   `json:"load"`
	Storage        []media.StorageUse `json:"storage"`
	StorageCounted *time.Time         `json:"storage_counted,omitempty"`
	Trickplay      trickplaySettings  `json:"trickplay"`
}

// trickplaySettings are the trickplay job's settings on the Jobs page (kept in config.yml),
// and their limits.
type trickplaySettings struct {
	Runners         int `json:"runners"`
	FramesPerRunner int `json:"frames_per_runner"`
	MaxRunners      int `json:"max_runners"`
	MaxFrames       int `json:"max_frames"`
	MaxProduct      int `json:"max_product"` // runners times frames per runner
	// For the estimate beside the settings: runners × (process + frames × frame) for 4K videos, and
	// the frames a 4K video's batch is cut to when that's more than the budget.
	Memory media.TrickplayMemory `json:"memory"`
}

func currentTrickplaySettings() trickplaySettings {
	return trickplaySettings{Runners: config.TrickplayRunners(), FramesPerRunner: config.TrickplayFramesPerRunner(),
		MaxRunners: config.MaxTrickplayRunners, MaxFrames: config.MaxTrickplayFrames, MaxProduct: config.MaxTrickplayProduct,
		Memory: media.CurrentTrickplayMemory()}
}

// ListJobs returns the background jobs' status (admins).
func (h *MediaHandler) ListJobs(w http.ResponseWriter, r *http.Request) {
	if claims := auth.GetClaims(r.Context()); claims == nil || !claims.IsAdmin {
		respondError(w, http.StatusForbidden, "Admin access required")
		return
	}
	resp := jobsResponse{Jobs: jobs.All(), Load: media.CurrentLoad(h.streams), Trickplay: currentTrickplaySettings()}
	var counted time.Time
	if resp.Storage, counted = media.Storage(); !counted.IsZero() {
		resp.StorageCounted = &counted
	}
	respondJSON(w, http.StatusOK, resp)
}

// ControlJob runs, pauses or resumes a job, or changes its settings (admins):
// POST /api/admin/jobs/{id}/{run|pause|resume|settings}. Trickplay's settings ({"runners",
// "frames_per_runner"}) are saved to config.yml and apply from its next batch.
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
	case "settings":
		if j.ID != "trickplay" {
			respondError(w, http.StatusBadRequest, "This job has no settings")
			return
		}
		var body struct {
			Runners         int `json:"runners"`
			FramesPerRunner int `json:"frames_per_runner"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Runners == 0 || body.FramesPerRunner == 0 {
			respondError(w, http.StatusBadRequest, "Give runners and frames_per_runner")
			return
		}
		if err := config.UpdateJobs(body.Runners, body.FramesPerRunner); err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
	default:
		respondError(w, http.StatusBadRequest, "Unknown action")
		return
	}
	respondJSON(w, http.StatusOK, jobsResponse{Jobs: jobs.All(), Load: media.CurrentLoad(h.streams), Trickplay: currentTrickplaySettings()})
}
