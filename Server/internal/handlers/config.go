package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/weirdstreamer/server/internal/auth"
	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
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
		respondJSON(w, http.StatusOK, config.GlobalConfig)
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

	if err := newConfig.Validate(); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Save to file
	if err := config.Save(config.Path(), &newConfig); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to save configuration")
		return
	}

	// Reload configuration so the server can generate stable IDs and normalize values
	if err := config.Load(config.Path()); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to reload configuration")
		return
	}

	if h.onUpdate != nil {
		h.onUpdate()
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Configuration updated successfully.",
		"config":  config.GlobalConfig,
	})
}

// GetAvailableGroups returns a list of all groups defined in the configuration (admin only)
func (h *ConfigHandler) GetAvailableGroups(w http.ResponseWriter, r *http.Request) {
	// Check if user is admin
	claims := auth.GetClaims(r.Context())
	if claims == nil || !claims.IsAdmin {
		respondError(w, http.StatusForbidden, "Admin access required")
		return
	}

	groupsMap := make(map[string]bool)
	for _, path := range config.GlobalConfig.Media.Paths {
		for _, group := range path.Groups {
			groupsMap[group] = true
		}
	}

	groups := make([]string, 0, len(groupsMap))
	for group := range groupsMap {
		groups = append(groups, group)
	}

	respondJSON(w, http.StatusOK, groups)
}
