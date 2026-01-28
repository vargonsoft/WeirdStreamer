package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/weirdstreamer/server/internal/auth"
	"github.com/weirdstreamer/server/internal/config"
)

type ConfigHandler struct{}

func NewConfigHandler() *ConfigHandler {
	return &ConfigHandler{}
}

// GetConfig returns the current configuration (admin only)
func (h *ConfigHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	// Check if user is admin
	claims := auth.GetClaims(r.Context())
	if claims == nil || !claims.IsAdmin {
		respondError(w, http.StatusForbidden, "Admin access required")
		return
	}

	respondJSON(w, http.StatusOK, config.GlobalConfig)
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

	// Save to file
	if err := config.Save("./config.json", &newConfig); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to save configuration")
		return
	}

	// Reload configuration so the server can generate stable IDs and normalize values
	if err := config.Load("./config.json"); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to reload configuration")
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Configuration updated successfully.",
		"config":  config.GlobalConfig,
	})
}
