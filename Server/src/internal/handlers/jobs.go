package handlers

import (
	"encoding/json"
	"net/http"
	"slices"
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
	Runners     int  `json:"runners"`      // in effect
	AutoRunners bool `json:"auto_runners"` // runners is one per disk (config.yml's 0)
	// Frames per runner by the video's size, largest first (config.TrickplayTiers)
	Tiers []trickplayTier `json:"tiers"`
	// The GPU trickplay can decode on (the hardware encoder's; "" none), and whether it does: the Jobs
	// page can keep trickplay on the CPU. memory and auto_frames_for are for the decoder in use.
	GPU    string `json:"gpu,omitempty"`
	UseGPU bool   `json:"use_gpu"`
	// What automatic gives: the runners (one per disk the libraries are on), for the field's hint.
	AutoRunnersValue int `json:"auto_runners_value"`
	MaxRunners       int `json:"max_runners"`
	// For the estimate beside the settings: runners × (process + frames × frame) for each size, from
	// the 4K figures and each tier's scale, against the server's memory and swap.
	Memory media.TrickplayMemory `json:"memory"`
}

// trickplayTier is one size's frames per runner.
type trickplayTier struct {
	ID         string  `json:"id"`     // 4k, 1080p, 720p, sd
	Name       string  `json:"name"`   // 4K, 1080p, 720p, SD
	Frames     int     `json:"frames"` // in effect, for the runners in effect
	Auto       bool    `json:"auto"`   // config.yml's 0: what fills half the memory
	MaxFrames  int     `json:"max_frames"`
	MaxProduct int     `json:"max_product"` // runners times frames per runner
	Scale      float64 `json:"scale"`       // its memory per frame against 4K's
	// What automatic gives for 1 to max_runners runners (auto_frames_for[r-1]), for the field's hint
	AutoFramesFor []int `json:"auto_frames_for"`
}

func currentTrickplaySettings() trickplaySettings {
	m := media.CurrentTrickplayMemory()
	s := trickplaySettings{Runners: media.TrickplayRunners(), AutoRunners: config.TrickplayRunnersSetting() == 0,
		GPU: media.TrickplayGPUName(), UseGPU: !config.TrickplayCPUOnly(),
		AutoRunnersValue: min(media.LibraryDisks(), config.MaxTrickplayRunners), MaxRunners: config.MaxTrickplayRunners,
		Memory: m}
	for i, t := range config.TrickplayTiers {
		tier := trickplayTier{ID: t.ID, Name: t.Name, Frames: media.TrickplayFramesPerRunner(i),
			Auto: config.TrickplayFramesSetting(i) == 0, MaxFrames: t.MaxFrames, MaxProduct: t.MaxProduct,
			Scale: media.TrickplayTierScale[i]}
		for n := 1; n <= config.MaxTrickplayRunners; n++ {
			tier.AutoFramesFor = append(tier.AutoFramesFor, media.AutoTrickplayFrames(n, m, i))
		}
		s.Tiers = append(s.Tiers, tier)
	}
	return s
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

// SetWaitLimits saves when the jobs wait for a disk or share (admins): POST /api/admin/jobs/limits
// {"disk_percent", "disk_idle_percent", "share_ms", "share_idle_ms"}, 0 for the default, those left
// out as they are. Saved to config.yml and used at once. Answers with the jobs' status.
func (h *MediaHandler) SetWaitLimits(w http.ResponseWriter, r *http.Request) {
	if claims := auth.GetClaims(r.Context()); claims == nil || !claims.IsAdmin {
		respondError(w, http.StatusForbidden, "Admin access required")
		return
	}
	var body struct {
		DiskPercent     *int `json:"disk_percent"`
		DiskIdlePercent *int `json:"disk_idle_percent"`
		ShareMs         *int `json:"share_ms"`
		ShareIdleMs     *int `json:"share_idle_ms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "Give disk_percent, disk_idle_percent, share_ms or share_idle_ms (0: the default)")
		return
	}
	settings := config.GlobalConfig.Jobs
	for _, f := range []struct {
		v  *int
		to *int
	}{{body.DiskPercent, &settings.WaitDiskPercent}, {body.DiskIdlePercent, &settings.WaitDiskIdlePercent},
		{body.ShareMs, &settings.WaitShareMs}, {body.ShareIdleMs, &settings.WaitShareIdleMs}} {
		if f.v != nil {
			*f.to = *f.v
		}
	}
	if err := config.UpdateJobs(settings); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, jobsResponse{Jobs: jobs.All(), Load: media.CurrentLoad(h.streams), Trickplay: currentTrickplaySettings()})
}

// ControlJob runs, pauses or resumes a job, or changes its settings (admins):
// POST /api/admin/jobs/{id}/{run|pause|resume|settings}. Trickplay's settings ({"runners",
// "frames": {"4k", "1080p", "720p", "sd"}}, 0 for automatic, and "use_gpu"; those left out stay as they
// are) are saved to config.yml and apply from its next batch.
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
			Runners *int           `json:"runners"`
			Frames  map[string]int `json:"frames"` // by tier ID
			UseGPU  *bool          `json:"use_gpu"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondError(w, http.StatusBadRequest, "Give runners, frames by size (0: automatic) or use_gpu")
			return
		}
		settings := config.GlobalConfig.Jobs
		if body.Runners != nil {
			settings.TrickplayRunners = *body.Runners
		}
		frames := settings.TierFrames()
		for id, f := range body.Frames {
			i := slices.IndexFunc(config.TrickplayTiers, func(t config.TrickplayTier) bool { return t.ID == id })
			if i < 0 {
				respondError(w, http.StatusBadRequest, "Unknown size "+id+": 4k, 1080p, 720p or sd")
				return
			}
			frames[i] = f
		}
		settings.SetTierFrames(frames)
		if body.UseGPU != nil {
			settings.TrickplayCPUOnly = !*body.UseGPU
		}
		if err := config.UpdateJobs(settings); err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
	default:
		respondError(w, http.StatusBadRequest, "Unknown action")
		return
	}
	respondJSON(w, http.StatusOK, jobsResponse{Jobs: jobs.All(), Load: media.CurrentLoad(h.streams), Trickplay: currentTrickplaySettings()})
}
