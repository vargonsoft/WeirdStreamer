package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/weirdstreamer/server/internal/auth"
	"github.com/weirdstreamer/server/internal/clientip"
	"github.com/weirdstreamer/server/internal/logs"
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

	response, err := h.authService.Authenticate(req.Username, req.Password, clientIP(r))
	var locked *auth.LockedError
	switch {
	case errors.As(err, &locked):
		respondLocked(w, locked.Wait)
		return
	case errors.Is(err, auth.ErrInvalidCredentials):
		respondError(w, http.StatusUnauthorized, err.Error())
		return
	case err != nil:
		respondError(w, http.StatusInternalServerError, "Failed to sign in")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    response.Token,
		Path:     "/",
		Expires:  time.Unix(response.ExpiresAt, 0),
		HttpOnly: true,
		Secure:   clientip.HTTPS(r),
		SameSite: http.SameSiteStrictMode,
	})
	if req.Web {
		response.Token = "" // only in the cookie: the page's scripts never see it
	}
	respondJSON(w, http.StatusOK, response)
}

// Logout ends the session of the token it's sent with (the Authorization header, or the browser's
// cookie) and clears the cookie.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	token := ""
	if parts := strings.SplitN(r.Header.Get("Authorization"), " ", 2); len(parts) == 2 && parts[0] == "Bearer" {
		token = parts[1]
	} else if cookie, err := r.Cookie(sessionCookieName); err == nil {
		// Only ever ends the session: another site making a browser sign out can do no more.
		token = cookie.Value
	}
	if token != "" {
		if claims, err := h.authService.ValidateToken(token); err == nil {
			if err := h.authService.EndSession(claims); err != nil {
				respondError(w, http.StatusInternalServerError, "Failed to sign out")
				return
			}
			logs.Auth("Signed out: %q from %s", claims.Username, clientIP(r))
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   clientip.HTTPS(r),
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
			// Once an hour per address: a client with an expired token is refused on every request.
			ip := clientIP(r)
			logs.AuthOnce("token|"+ip, "Token refused from %s (%s %s): %v", ip, r.Method, r.URL.Path, err)
			respondError(w, http.StatusUnauthorized, "Invalid token")
			return
		}

		user, err := h.authService.UserForClaims(claims)
		if err != nil {
			ip := clientIP(r)
			if errors.Is(err, auth.ErrSessionEnded) || errors.Is(err, sql.ErrNoRows) {
				logs.AuthOnce("user|"+ip+"|"+claims.Username, "Token refused from %s: %q's session has ended", ip, claims.Username)
				respondError(w, http.StatusUnauthorized, "Signed out")
				return
			}
			respondError(w, http.StatusInternalServerError, "Failed to check the sign-in")
			return
		}
		logs.SetUser(r, user.Username)

		ctx := auth.WithClaims(r.Context(), claims)
		ctx = auth.WithUser(ctx, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// webClientHeader marks the web UI's requests (js/api.js WEB_CLIENT_HEADER).
const webClientHeader = "X-WeirdStreamer-Web"

func tokenFromRequest(r *http.Request) string {
	if parts := strings.SplitN(r.Header.Get("Authorization"), " ", 2); len(parts) == 2 && parts[0] == "Bearer" {
		return parts[1]
	}
	// The cookie (the web UI's) for reading, and for changes only with the web UI's header: other
	// sites' pages can't add it (no other origin is allowed), so they can't use the cookie.
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Header.Get(webClientHeader) == "1" {
		if cookie, err := r.Cookie(sessionCookieName); err == nil {
			return cookie.Value
		}
	}
	return ""
}

// respondLocked refuses a password check until the limiter's lock runs out. Rounded up, so a
// client that waits the number of seconds shown is never refused again.
func respondLocked(w http.ResponseWriter, wait time.Duration) {
	seconds := int(math.Ceil(wait.Seconds()))
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	respondJSON(w, http.StatusTooManyRequests, models.ErrorResponse{
		Error:      http.StatusText(http.StatusTooManyRequests),
		Message:    "Too many failed attempts. Try again later.",
		RetryAfter: seconds,
	})
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
