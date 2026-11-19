package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/mux"

	"github.com/weirdstreamer/server/internal/auth"
	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/tlscert"
)

type ConfigHandler struct {
	db       *database.DB
	onUpdate func()
}

// OnUpdate registers a callback run after the configuration is saved and reloaded.
func (h *ConfigHandler) OnUpdate(fn func()) {
	h.onUpdate = fn
}

func NewConfigHandler(db *database.DB) *ConfigHandler {
	return &ConfigHandler{db: db}
}

// GetConfig returns the configuration. Admins get the full config,
// regular users get only authorized library names/IDs.
func (h *ConfigHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		respondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	if claims.IsAdmin {
		respondJSON(w, http.StatusOK, adminConfig())
		return
	}

	// For non-admins, return a filtered config containing only authorized libraries
	user, err := h.db.GetUserByUsername(claims.Username)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to get user info")
		return
	}

	filteredPaths := []config.MediaPath{}
	for _, path := range config.GlobalConfig.Media.Paths {
		authorized := false
		for _, g := range path.Groups {
			if g == user.Group {
				authorized = true
				break
			}
		}
		if authorized {
			// Don't leak actual system paths or group lists to non-admins
			filteredPaths = append(filteredPaths, config.MediaPath{
				ID:   path.ID,
				Name: path.Name,
			})
		}
	}

	publicConfig := map[string]interface{}{
		"media": map[string]interface{}{
			"paths": filteredPaths,
		},
	}

	respondJSON(w, http.StatusOK, publicConfig)
}

// adminConfig is the configuration as admins' Settings get it: all of it but the JWT secret, with
// which anyone could make sign-in tokens (the TMDB token is there, for Settings to show and change).
func adminConfig() config.Config {
	c := *config.GlobalConfig
	c.Auth.JWTSecret = ""
	return c
}

// Certificate is the HTTPS certificate's expiry, for admins' Settings: GET /api/admin/certificate
// ({"state": "none"} without HTTPS).
func (h *ConfigHandler) Certificate(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if s := tlscert.Current(); s != nil {
		respondJSON(w, http.StatusOK, s)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"state": "none"})
}

// UpdateConfig updates the configuration (admin only)
func (h *ConfigHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	// Check if user is admin
	claims := auth.GetClaims(r.Context())
	if claims == nil || !claims.IsAdmin {
		respondError(w, http.StatusForbidden, "Admin access required")
		return
	}

	var newConfig config.Config
	if err := json.NewDecoder(r.Body).Decode(&newConfig); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// The JWT secret is never sent to clients, so it can't be changed here: it stays as it is.
	newConfig.Auth.JWTSecret = config.GlobalConfig.Auth.JWTSecret

	// A client that doesn't know about groups keeps them as they are.
	if newConfig.Groups == nil {
		newConfig.Groups = config.GlobalConfig.Groups
	}
	// Groups dropped here must have no users (the libraries are checked by Validate).
	for _, g := range config.GlobalConfig.Groups {
		if newConfig.HasGroup(g) {
			continue
		}
		if users, err := h.db.UsersInGroup(g); err != nil || len(users) > 0 {
			respondError(w, http.StatusConflict, groupInUse(g, users, nil))
			return
		}
	}

	if status, err := h.saveConfig(&newConfig); err != nil {
		respondError(w, status, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Configuration updated successfully.",
		"config":  adminConfig(),
	})
}

// saveConfig validates, saves and reloads a new configuration, returning the HTTP status for
// its error.
func (h *ConfigHandler) saveConfig(c *config.Config) (int, error) {
	if err := c.Validate(); err != nil {
		return http.StatusBadRequest, err
	}
	if err := config.Save(config.Path(), c); err != nil {
		return http.StatusInternalServerError, fmt.Errorf("failed to save configuration")
	}
	// Reload so the server can generate stable IDs and normalize values
	if err := config.Load(config.Path()); err != nil {
		return http.StatusInternalServerError, fmt.Errorf("failed to reload configuration")
	}
	if h.onUpdate != nil {
		h.onUpdate()
	}
	return http.StatusOK, nil
}

// currentConfig is a copy of the configuration to change, with its own lists.
func currentConfig() config.Config {
	c := *config.GlobalConfig
	c.Groups = append([]string{}, c.Groups...)
	c.Media.Paths = append([]config.MediaPath{}, c.Media.Paths...)
	for i := range c.Media.Paths {
		c.Media.Paths[i].Groups = append([]string{}, c.Media.Paths[i].Groups...)
	}
	return c
}

// groupInUse says why a group can't be removed: the users and libraries still in it.
func groupInUse(group string, users, libraries []string) string {
	var in []string
	if len(users) > 0 {
		in = append(in, fmt.Sprintf("%s (%s)", plural(len(users), "user"), strings.Join(users, ", ")))
	}
	if len(libraries) > 0 {
		in = append(in, fmt.Sprintf("%s (%s)", plural(len(libraries), "library"), strings.Join(libraries, ", ")))
	}
	return fmt.Sprintf("The group %q is still used by %s. Move them to another group first.", group, strings.Join(in, " and "))
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	if strings.HasSuffix(word, "y") {
		return fmt.Sprintf("%d %sies", n, strings.TrimSuffix(word, "y"))
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func (h *ConfigHandler) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if claims := auth.GetClaims(r.Context()); claims == nil || !claims.IsAdmin {
		respondError(w, http.StatusForbidden, "Admin access required")
		return false
	}
	return true
}

// CreateGroup adds a group: POST /api/groups {"name": "kids"}
func (h *ConfigHandler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	c := currentConfig()
	if err := c.CheckGroupName(req.Name); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	c.Groups = append(c.Groups, req.Name)
	if status, err := h.saveConfig(&c); err != nil {
		respondError(w, status, err.Error())
		return
	}
	respondJSON(w, http.StatusCreated, config.GlobalConfig.Groups)
}

// MakeDefaultGroup makes a group the default for new users, by moving it to the front of the
// groups: PUT /api/groups/{name}/default. The others keep their order.
func (h *ConfigHandler) MakeDefaultGroup(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	name := mux.Vars(r)["name"]
	c := currentConfig()
	if !c.HasGroup(name) {
		respondError(w, http.StatusNotFound, fmt.Sprintf("The group %q doesn't exist", name))
		return
	}
	groups := []string{name}
	for _, g := range c.Groups {
		if g != name {
			groups = append(groups, g)
		}
	}
	c.Groups = groups
	if status, err := h.saveConfig(&c); err != nil {
		respondError(w, status, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, config.GlobalConfig.Groups)
}

// RenameGroup renames a group, in the libraries that list it and for its users:
// PUT /api/groups/{name} {"name": "new name"}
func (h *ConfigHandler) RenameGroup(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	from := mux.Vars(r)["name"]
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	c := currentConfig()
	if !c.HasGroup(from) {
		respondError(w, http.StatusNotFound, fmt.Sprintf("The group %q doesn't exist", from))
		return
	}
	// The new name is checked against the other groups (a change of case alone is fine).
	others := c
	others.Groups = nil
	for _, g := range c.Groups {
		if g != from {
			others.Groups = append(others.Groups, g)
		}
	}
	if err := others.CheckGroupName(req.Name); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	for i, g := range c.Groups {
		if g == from {
			c.Groups[i] = req.Name
		}
	}
	for i := range c.Media.Paths {
		for j, g := range c.Media.Paths[i].Groups {
			if g == from {
				c.Media.Paths[i].Groups[j] = req.Name
			}
		}
	}
	if status, err := h.saveConfig(&c); err != nil {
		respondError(w, status, err.Error())
		return
	}
	if err := h.db.RenameGroup(from, req.Name); err != nil {
		respondError(w, http.StatusInternalServerError, "Renamed the group, but not for its users")
		return
	}
	respondJSON(w, http.StatusOK, config.GlobalConfig.Groups)
}

// DeleteGroup removes a group nobody uses: DELETE /api/groups/{name}
func (h *ConfigHandler) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	name := mux.Vars(r)["name"]
	c := currentConfig()
	if !c.HasGroup(name) {
		respondError(w, http.StatusNotFound, fmt.Sprintf("The group %q doesn't exist", name))
		return
	}
	users, err := h.db.UsersInGroup(name)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to check the group's users")
		return
	}
	var libraries []string
	for _, p := range c.Media.Paths {
		for _, g := range p.Groups {
			if g == name {
				libraries = append(libraries, p.Name)
			}
		}
	}
	if len(users) > 0 || len(libraries) > 0 {
		respondError(w, http.StatusConflict, groupInUse(name, users, libraries))
		return
	}
	kept := []string{}
	for _, g := range c.Groups {
		if g != name {
			kept = append(kept, g)
		}
	}
	c.Groups = kept
	if status, err := h.saveConfig(&c); err != nil {
		respondError(w, status, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, config.GlobalConfig.Groups)
}

// GetAvailableGroups lists the defined groups (admin only): GET /api/groups
func (h *ConfigHandler) GetAvailableGroups(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	respondJSON(w, http.StatusOK, config.GlobalConfig.Groups)
}
