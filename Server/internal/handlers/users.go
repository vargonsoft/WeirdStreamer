package handlers

import (
	"encoding/json"
	"fmt"
	"github.com/weirdstreamer/server/internal/config"
	"net/http"
	"strings"

	"github.com/weirdstreamer/server/internal/auth"
	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/models"
	"golang.org/x/crypto/bcrypt"
)

type UserHandler struct {
	db *database.DB
}

func NewUserHandler(db *database.DB) *UserHandler {
	return &UserHandler{db: db}
}

type CreateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	IsAdmin  bool   `json:"is_admin"`
	Group    string `json:"group"`
}

type ChangePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

type UserResponse struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	IsAdmin  bool   `json:"is_admin"`
	Group    string `json:"group"`
}

// ListUsers returns all users (admin only)
func (h *UserHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil || !claims.IsAdmin {
		respondError(w, http.StatusForbidden, "Admin access required")
		return
	}

	users, err := h.db.ListUsers()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to list users")
		return
	}

	response := make([]UserResponse, len(users))
	for i, user := range users {
		response[i] = UserResponse{
			ID:       user.ID,
			Username: user.Username,
			IsAdmin:  user.IsAdmin,
			Group:    user.Group,
		}
	}

	respondJSON(w, http.StatusOK, response)
}

// checkUserGroup reports why a user can't be in group: every user is in one defined group.
func checkUserGroup(group string) error {
	if group == "" {
		return fmt.Errorf("Choose a group for the user")
	}
	if !config.GlobalConfig.HasGroup(group) {
		return fmt.Errorf("The group %q doesn't exist (groups: %s)", group, strings.Join(config.GlobalConfig.Groups, ", "))
	}
	return nil
}

// CreateUser creates a new user (admin only)
func (h *UserHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil || !claims.IsAdmin {
		respondError(w, http.StatusForbidden, "Admin access required")
		return
	}

	var req CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Username == "" || req.Password == "" {
		respondError(w, http.StatusBadRequest, "Username and password are required")
		return
	}
	if err := checkUserGroup(req.Group); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	user, err := h.db.CreateUser(req.Username, req.Password, req.IsAdmin, req.Group)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to create user")
		return
	}

	response := UserResponse{
		ID:       user.ID,
		Username: user.Username,
		IsAdmin:  user.IsAdmin,
		Group:    user.Group,
	}

	respondJSON(w, http.StatusCreated, response)
}

// UpdateUser updates an existing user (admin only)
func (h *UserHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil || !claims.IsAdmin {
		respondError(w, http.StatusForbidden, "Admin access required")
		return
	}

	username := r.URL.Query().Get("username")
	if username == "" {
		respondError(w, http.StatusBadRequest, "Username is required")
		return
	}

	var req models.UserUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Group != nil {
		if err := checkUserGroup(*req.Group); err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	if err := h.db.UpdateUser(username, req); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to update user")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "User updated successfully"})
}

// DeleteUser deletes a user (admin only)
func (h *UserHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil || !claims.IsAdmin {
		respondError(w, http.StatusForbidden, "Admin access required")
		return
	}

	username := r.URL.Query().Get("username")
	if username == "" {
		respondError(w, http.StatusBadRequest, "Username is required")
		return
	}

	// Prevent user from deleting themselves
	if username == claims.Username {
		respondError(w, http.StatusBadRequest, "Cannot delete your own account")
		return
	}

	if err := h.db.DeleteUser(username); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to delete user")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "User deleted successfully"})
}

// ChangePassword allows any authenticated user to change their own password
func (h *UserHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		respondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.NewPassword == "" {
		respondError(w, http.StatusBadRequest, "New password is required")
		return
	}

	// The current password must be given and right, so a session left signed in (or a stolen
	// token) can't be used to take over the account. 403, not 401: a 401 signs the web UI out.
	user, err := h.db.GetUserByUsername(claims.Username)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to get user")
		return
	}
	if req.OldPassword == "" {
		respondError(w, http.StatusBadRequest, "Current password is required")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.OldPassword)) != nil {
		respondError(w, http.StatusForbidden, "Current password is incorrect")
		return
	}

	if err := h.db.UpdateUserPassword(user.Username, req.NewPassword); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to update password")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Password changed successfully"})
}
