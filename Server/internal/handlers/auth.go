package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/weirdstreamer/server/internal/auth"
	"github.com/weirdstreamer/server/internal/models"
)

type AuthHandler struct {
	authService *auth.Service
}

func NewAuthHandler(authService *auth.Service) *AuthHandler {
	return &AuthHandler{authService: authService}
}

// Login godoc
// @Summary Login user
// @Description Authenticate user and return JWT token
// @Tags auth
// @Accept json
// @Produce json
// @Param credentials body models.LoginRequest true "Login credentials"
// @Success 200 {object} models.LoginResponse
// @Failure 401 {object} models.ErrorResponse
// @Router /api/auth/login [post]
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req models.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	response, err := h.authService.Authenticate(req.Username, req.Password)
	if err != nil {
		respondError(w, http.StatusUnauthorized, err.Error())
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    response.Token,
		Path:     "/",
		Expires:  time.Unix(response.ExpiresAt, 0),
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
	})
	respondJSON(w, http.StatusOK, response)
}

// Logout clears the browser session cookie.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

// sessionCookieName holds the JWT for browser clients, which can't set an Authorization header
// on <img>, <video> or EventSource requests.
const sessionCookieName = "ws_session"

// AuthMiddleware validates the JWT from the Authorization header or, for GET/HEAD requests only,
// the session cookie. Restricting the cookie to safe methods means another site can't use a
// logged-in browser to change anything. Tokens in the query string are not accepted, since they
// end up in logs and browser history.
func (h *AuthHandler) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := tokenFromRequest(r)
		if token == "" {
			respondError(w, http.StatusUnauthorized, "Missing authorization")
			return
		}

		claims, err := h.authService.ValidateToken(token)
		if err != nil {
			respondError(w, http.StatusUnauthorized, "Invalid token")
			return
		}

		user, err := h.authService.UserForClaims(claims)
		if err != nil {
			respondError(w, http.StatusUnauthorized, "Unknown user")
			return
		}

		ctx := auth.WithClaims(r.Context(), claims)
		ctx = auth.WithUser(ctx, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func tokenFromRequest(r *http.Request) string {
	if parts := strings.SplitN(r.Header.Get("Authorization"), " ", 2); len(parts) == 2 && parts[0] == "Bearer" {
		return parts[1]
	}
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		if cookie, err := r.Cookie(sessionCookieName); err == nil {
			return cookie.Value
		}
	}
	return ""
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, models.ErrorResponse{
		Error:   http.StatusText(status),
		Message: message,
	})
}
