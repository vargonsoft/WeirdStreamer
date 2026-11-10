package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gorilla/mux"
	"github.com/weirdstreamer/server/internal/auth"
	"github.com/weirdstreamer/server/internal/media"
)

// Downloads for offline viewing. Originals are fetched with the stream endpoint (which supports
// ranges); compressed copies, and DVD images (as MP4), are encoded by a download job first.

// SetDownloads enables compressed downloads, keeping their encodes in dir.
func (h *MediaHandler) SetDownloads(m *media.DownloadManager) { h.downloads = m }

type downloadOptions struct {
	media.DownloadSource
	Sizes  []int `json:"sizes"`  // target shares of the original size, percent
	Scales []int `json:"scales"` // resolution shares, percent
}

// DownloadOptions describes a file's download choices: ?path=<base64 path>
func (h *MediaHandler) DownloadOptions(w http.ResponseWriter, r *http.Request) {
	path, ok := h.decodeMediaPath(w, r, r.URL.Query().Get("path"))
	if !ok {
		return
	}
	src, err := media.Source(path)
	if err != nil {
		respondError(w, http.StatusNotFound, "Media not found")
		return
	}
	respondJSON(w, http.StatusOK, downloadOptions{DownloadSource: *src, Sizes: media.DownloadSizes, Scales: media.DownloadScales})
}

func downloadOwner(r *http.Request) int64 {
	if claims := auth.GetClaims(r.Context()); claims != nil {
		return claims.UserID
	}
	return -1
}

// CreateDownload starts an encode: {"path": base64, "size_pct": 90|75|50|25|100, "scale": 100|75|50,
// "keep_video": bool, "keep_audio": bool} (keeping needs size_pct 100 and scale 100: Compatibility)
func (h *MediaHandler) CreateDownload(w http.ResponseWriter, r *http.Request) {
	if h.downloads == nil {
		respondError(w, http.StatusNotFound, "Downloads not enabled")
		return
	}
	var req struct {
		Path      string `json:"path"`
		SizePct   int    `json:"size_pct"`
		Scale     int    `json:"scale"`
		KeepVideo bool   `json:"keep_video"`
		KeepAudio bool   `json:"keep_audio"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	path, ok := h.decodeMediaPath(w, r, req.Path)
	if !ok {
		return
	}
	job, err := h.downloads.Create(downloadOwner(r), path, req.SizePct, req.Scale, req.KeepVideo, req.KeepAudio)
	if err != nil {
		respondError(w, http.StatusBadRequest, withoutDiskPath(err.Error(), path))
		return
	}
	respondJSON(w, http.StatusCreated, clientJob(job))
}

// GetDownload reports a job's progress.
func (h *MediaHandler) GetDownload(w http.ResponseWriter, r *http.Request) {
	if h.downloads == nil {
		respondError(w, http.StatusNotFound, "Downloads not enabled")
		return
	}
	job := h.downloads.Get(downloadOwner(r), mux.Vars(r)["id"])
	if job == nil {
		respondError(w, http.StatusNotFound, "Download not found")
		return
	}
	respondJSON(w, http.StatusOK, clientJob(job))
}

// clientJob is a job for a response: the file by the path clients know it by, in ffmpeg's error too.
func clientJob(job *media.DownloadJob) *media.DownloadJob {
	shown := *job
	shown.Error = withoutDiskPath(job.Error, job.Path)
	shown.Path = clientPath(job.Path)
	return &shown
}

// DownloadFile serves a finished job's file, with range support for resuming.
func (h *MediaHandler) DownloadFile(w http.ResponseWriter, r *http.Request) {
	if h.downloads == nil {
		respondError(w, http.StatusNotFound, "Downloads not enabled")
		return
	}
	id := mux.Vars(r)["id"]
	file, ok := h.downloads.File(downloadOwner(r), id)
	if !ok {
		respondError(w, http.StatusNotFound, "Download not ready")
		return
	}
	f, err := os.Open(file)
	if err != nil {
		respondError(w, http.StatusNotFound, "Download not found")
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to read download")
		return
	}
	job := h.downloads.Get(downloadOwner(r), id)
	name := strings.TrimSuffix(filepath.Base(job.Path), filepath.Ext(job.Path)) + ".mp4"
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(name, `"`, "")+`"`)
	http.ServeContent(w, r, name, st.ModTime(), f)
}

// DeleteDownload cancels a job or removes its finished file.
func (h *MediaHandler) DeleteDownload(w http.ResponseWriter, r *http.Request) {
	if h.downloads == nil || !h.downloads.Delete(downloadOwner(r), mux.Vars(r)["id"]) {
		respondError(w, http.StatusNotFound, "Download not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
