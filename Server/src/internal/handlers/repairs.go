package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/weirdstreamer/server/internal/auth"
	"github.com/weirdstreamer/server/internal/media"
)

// Damaged videos (media/repair.go): Settings' list for admins, with Replace, and the trash of
// replaced originals, each with Restore, and Empty trash.

// StartRepairs finds damaged videos and makes their fixed copies in the background, until ctx ends.
func (h *MediaHandler) StartRepairs(ctx context.Context) {
	h.repairs = media.NewRepairJob(h.db, h.streams, func(path string) bool {
		_, found := libraryForPath(path)
		return found
	})
	go h.repairs.Run(ctx)
}

func isAdmin(w http.ResponseWriter, r *http.Request) bool {
	if claims := auth.GetClaims(r.Context()); claims == nil || !claims.IsAdmin {
		respondError(w, http.StatusForbidden, "Admins only")
		return false
	}
	return true
}

type damagedVideo struct {
	Path      string  `json:"path"`            // the client path, for playing it (original=1 for the damaged file)
	Name      string  `json:"name"`            // the file's name
	Library   string  `json:"library"`         // its library's name
	Folder    string  `json:"folder"`          // the folder within the library ("" at its top)
	Reason    string  `json:"reason"`          // what's wrong
	State     string  `json:"state"`           // "fixed", or "failed" (no fixed copy)
	Error     string  `json:"error,omitempty"` // why it couldn't be fixed
	Size      int64   `json:"size"`            // the original's
	FixedSize int64   `json:"fixed_size,omitempty"`
	Claimed   float64 `json:"claimed,omitempty"`       // the length the original says it has
	Duration  float64 `json:"duration,omitempty"`      // the fixed copy's real length
	Thumbnail string  `json:"thumbnail_url,omitempty"` // its thumbnail, when the library has one
}

// trashItem is a replaced original in the trash.
type trashItem struct {
	ID          string `json:"id"`                    // for restoring it
	Name        string `json:"name"`                  // the file's name
	Library     string `json:"library,omitempty"`     // its library's name
	Folder      string `json:"folder"`                // the folder it was in ("" at its library's top)
	Size        int64  `json:"size"`                  //
	TrashedAt   string `json:"trashed_at"`            // RFC 3339
	DeletesAt   string `json:"deletes_at"`            // when the trash deletes it
	Reason      string `json:"reason,omitempty"`      // what was wrong with it
	Replacement string `json:"replacement,omitempty"` // the name of the file that replaced it
	Restorable  bool   `json:"restorable"`            // the trash knows where it goes back
	Thumbnail   string `json:"thumbnail_url,omitempty"`
}

type trashReport struct {
	Items []trashItem `json:"items"`
	Files int         `json:"files"`
	Bytes int64       `json:"bytes"`
	Days  int         `json:"days"` // how long it keeps a file
}

const rfc3339 = "2006-01-02T15:04:05Z"

func trashState() trashReport {
	items, err := media.TrashItems()
	if err != nil {
		log.Printf("trash: %v", err)
	}
	out := trashReport{Items: []trashItem{}, Days: media.TrashDays}
	for _, t := range items {
		item := trashItem{ID: t.ID, Name: t.Name, Size: t.Size, Reason: t.Reason,
			TrashedAt: t.At.UTC().Format(rfc3339), DeletesAt: t.Deletes().UTC().Format(rfc3339)}
		if t.Path != "" {
			if lib, ok := libraryForPath(t.Path); ok {
				item.Library, item.Restorable = lib.Name, true
				if rel, err := filepath.Rel(lib.Path, filepath.Dir(t.Path)); err == nil && rel != "." {
					item.Folder = filepath.ToSlash(rel)
				}
			}
			item.Replacement = filepath.Base(t.Replacement)
		}
		if t.Thumbnail != "" {
			item.Thumbnail = "/api/admin/trash/thumbnail?id=" + t.ID
		}
		out.Items = append(out.Items, item)
		out.Files++
		out.Bytes += t.Size
	}
	return out
}

// TrashList lists the replaced originals in the trash (GET /api/admin/trash).
func (h *MediaHandler) TrashList(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(w, r) {
		return
	}
	respondJSON(w, http.StatusOK, trashState())
}

// TrashThumbnail serves the copy of a trashed original's thumbnail kept in the trash (GET
// /api/admin/trash/thumbnail?id=), to admins only.
func (h *MediaHandler) TrashThumbnail(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(w, r) {
		return
	}
	item, err := media.GetTrashItem(r.URL.Query().Get("id"))
	if err != nil || item.Thumbnail == "" {
		respondError(w, http.StatusNotFound, "No thumbnail")
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, item.Thumbnail)
}

// RestoreTrash undoes a replace (POST /api/admin/trash/restore with {"id"}): the original goes
// back to its place in the library with its progress and the rest, and the file that replaced it
// back to the fixed copies (listed as fixed again). Refused while another file is in its place, or
// someone else is playing the file that replaced it. Answers {"path": the original's client path}.
func (h *MediaHandler) RestoreTrash(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(w, r) {
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}
	item, err := media.GetTrashItem(req.ID)
	if err != nil {
		respondError(w, http.StatusNotFound, "It isn't in the trash any more")
		return
	}
	me := ""
	if claims := auth.GetClaims(r.Context()); claims != nil {
		me = claims.Username
	}
	if item.Replacement != "" && playing(h.streams, item.Replacement, me) {
		respondError(w, http.StatusConflict, "Someone is playing "+filepath.Base(item.Replacement)+": try again when they've stopped")
		return
	}
	item, err = media.RestoreFromTrash(req.ID)
	switch {
	case errors.Is(err, media.ErrNoTrashItem):
		respondError(w, http.StatusNotFound, "It isn't in the trash any more")
		return
	case errors.Is(err, media.ErrNotRestorable), errors.Is(err, media.ErrNoLibrary):
		respondError(w, http.StatusConflict, "Can't restore it: "+err.Error())
		return
	case errors.Is(err, media.ErrPathTaken):
		respondError(w, http.StatusConflict, "Can't restore it: a file named "+item.Name+" is in its place")
		return
	case errors.Is(err, media.ErrReplacementChanged):
		respondError(w, http.StatusConflict, "Can't restore it: "+filepath.Base(item.Replacement)+" has changed since it replaced it")
		return
	case err != nil:
		log.Printf("[REPAIRS] Restoring %s: %v", item.Path, err)
		respondError(w, http.StatusInternalServerError, "Couldn't restore it: "+withoutDiskPath(withoutDiskPath(err.Error(), item.Path), item.Replacement))
		return
	}
	if item.Replacement != "" && item.Replacement != item.Path {
		if err := h.db.MoveFile(item.Replacement, clientPath(item.Replacement), clientPath(item.Path)); err != nil {
			log.Printf("[REPAIRS] %s: moving its progress and the rest back: %v", item.Path, err)
		}
		h.thumbnailGenerator.Forget(item.Replacement)
	}
	go h.scan() // the original joins the library again (and the replacement leaves it)
	respondJSON(w, http.StatusOK, map[string]any{"path": clientPath(item.Path)})
}

// DamagedVideos lists the damaged videos (GET /api/admin/damaged; the trash is GET /api/admin/trash).
func (h *MediaHandler) DamagedVideos(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(w, r) {
		return
	}
	list := []damagedVideo{}
	for _, v := range media.DamagedVideos(func(p string) bool { _, ok := libraryForPath(p); return ok }) {
		lib, _ := libraryForPath(v.Path)
		rel, _ := filepath.Rel(lib.Path, filepath.Dir(v.Path))
		if rel == "." {
			rel = ""
		}
		list = append(list, damagedVideo{Path: clientPath(v.Path), Name: filepath.Base(v.Path), Library: lib.Name,
			Folder: rel, Reason: v.Reason, State: v.State, Error: withoutDiskPath(v.Error, v.Path),
			Size: v.Size, FixedSize: v.FixedSize, Claimed: v.Claimed, Duration: v.Duration,
			Thumbnail: h.thumbnailOf(v.Path)})
	}
	respondJSON(w, http.StatusOK, map[string]any{"videos": list})
}

// ReplaceDamaged puts a damaged video's fixed copy in its place, as .mkv, its progress and the rest
// following it, and moves the original to the trash (POST /api/admin/damaged/replace with
// {"path": base64 client path}). Answers {"path": the new file's client path}.
func (h *MediaHandler) ReplaceDamaged(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(w, r) {
		return
	}
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}
	path, ok := h.decodeMediaPath(w, r, req.Path)
	if !ok {
		return
	}
	me := ""
	if claims := auth.GetClaims(r.Context()); claims != nil {
		me = claims.Username
	}
	if playing(h.streams, path, me) {
		respondError(w, http.StatusConflict, "Someone is playing it: try again when they've stopped")
		return
	}
	newPath, err := media.ReplaceWithFixed(path)
	switch {
	case errors.Is(err, media.ErrNotFixed):
		respondError(w, http.StatusConflict, "It has no fixed copy (yet)")
		return
	case errors.Is(err, media.ErrNameTaken):
		respondError(w, http.StatusConflict, "A file named "+filepath.Base(newPathFor(path))+" is already in that folder")
		return
	case err != nil:
		log.Printf("[REPAIRS] Replacing %s: %v", path, err)
		respondError(w, http.StatusInternalServerError, "Couldn't replace it: "+withoutDiskPath(err.Error(), path))
		return
	}
	if err := h.db.MoveFile(path, clientPath(path), clientPath(newPath)); err != nil {
		log.Printf("[REPAIRS] %s: moving its progress and the rest to %s: %v", path, filepath.Base(newPath), err)
	}
	h.thumbnailGenerator.Forget(path)
	go h.scan() // the new file joins the library (and the old one leaves it)
	respondJSON(w, http.StatusOK, map[string]any{"path": clientPath(newPath)})
}

// playing says whether someone else is playing or streaming path now: not a stream gone idle (kept
// on the Streams page a while), nor the admin's own (who usually just compared the two).
func playing(streams *media.StreamTracker, path, me string) bool {
	for _, s := range streams.List() {
		if s.Path == path && s.State != "idle" && s.User != me {
			return true
		}
	}
	return false
}

func newPathFor(path string) string {
	return path[:len(path)-len(filepath.Ext(path))] + ".mkv"
}

// FixDamaged makes a damaged video's fixed copy (POST /api/admin/damaged/fix with {"path": base64
// client path}), in the background: the list shows it as "fixing" until it's done. Refused when
// the copy wouldn't fit.
func (h *MediaHandler) FixDamaged(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(w, r) {
		return
	}
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}
	path, ok := h.decodeMediaPath(w, r, req.Path)
	if !ok {
		return
	}
	if h.repairs == nil {
		respondError(w, http.StatusServiceUnavailable, "Fixing damaged videos is off (no media.repairPath)")
		return
	}
	if err := h.repairs.Fix(path); err != nil {
		respondError(w, http.StatusConflict, "Couldn't fix it: "+withoutDiskPath(err.Error(), path))
		return
	}
	respondJSON(w, http.StatusAccepted, map[string]any{"state": "fixing"})
}

// EmptyTrash deletes every replaced original in the trash (POST /api/admin/trash/empty), answering
// the trash as GET /api/admin/trash does.
func (h *MediaHandler) EmptyTrash(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(w, r) {
		return
	}
	n, err := media.EmptyTrash(0)
	if err != nil {
		log.Printf("trash: %v", err)
		respondError(w, http.StatusInternalServerError, "Couldn't empty the trash")
		return
	}
	if n > 0 {
		log.Printf("trash: emptied by an admin (%d file(s))", n)
	}
	respondJSON(w, http.StatusOK, trashState())
}

// An admin comparing a damaged video's original with its fixed copy plays the original as it is
// for a while (POST /api/admin/damaged/play {"path", "original": true}): that admin's requests for
// it read the original, the players unchanged. "original": false (or an hour) ends it.
var originals = struct {
	sync.Mutex
	until map[string]time.Time // by user and disk path
}{until: map[string]time.Time{}}

const originalFor = time.Hour

func originalKey(r *http.Request, path string) string {
	if claims := auth.GetClaims(r.Context()); claims != nil {
		return claims.Username + "\x00" + path
	}
	return ""
}

// playsOriginal says whether this request's user asked to play path's damaged original.
func playsOriginal(r *http.Request, path string) bool {
	key := originalKey(r, path)
	if key == "" {
		return false
	}
	originals.Lock()
	defer originals.Unlock()
	until, ok := originals.until[key]
	if ok && time.Now().After(until) {
		delete(originals.until, key)
		return false
	}
	return ok
}

// PlayDamaged chooses which of a damaged video an admin's players get: the original as it is,
// or (the default) its fixed copy.
func (h *MediaHandler) PlayDamaged(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(w, r) {
		return
	}
	var req struct {
		Path     string `json:"path"`
		Original bool   `json:"original"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}
	path, ok := h.decodeMediaPath(w, r, req.Path)
	if !ok {
		return
	}
	key := originalKey(r, path)
	originals.Lock()
	for k, until := range originals.until { // the expired go
		if time.Now().After(until) {
			delete(originals.until, k)
		}
	}
	if req.Original {
		originals.until[key] = time.Now().Add(originalFor)
	} else {
		delete(originals.until, key)
	}
	originals.Unlock()
	respondJSON(w, http.StatusOK, map[string]any{"original": req.Original})
}

// thumbnailOf is a library file's thumbnail URL, as listings give it, or "" when it has none.
func (h *MediaHandler) thumbnailOf(path string) string {
	if item, err := h.db.GetMediaItem(path); err == nil {
		return item.ThumbnailURL
	}
	return ""
}
