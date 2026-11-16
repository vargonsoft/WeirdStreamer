package handlers

// TV sign-in with a short code. The TV asks for a code (POST /api/auth/pairing) and shows it, then
// polls GET /api/auth/pairing/{code}. A signed-in user enters the code in Settings (POST
// /api/pairing/claim), and the TV's next poll gets a session token for that user, once.
//
// Codes live in memory only: a restart forgets them, and the TV makes a new one.

import (
	"crypto/rand"
	"encoding/json"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/weirdstreamer/server/internal/auth"
	"github.com/weirdstreamer/server/internal/logs"
	"github.com/weirdstreamer/server/internal/models"
)

const (
	// pairingAlphabet has no 0, O, 1 or I, which are easily mistaken for each other on a TV.
	pairingAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	pairingCodeLen  = 5
	// pairingTTL is how long a code can be claimed (and an approved one collected).
	pairingTTL = 5 * time.Minute
	// pairingMaxCodes caps the codes held at once, so many addresses can't fill memory.
	pairingMaxCodes = 10000

	pairingCreateLimit = 10 // new codes per client address per minute
	pairingClaimLimit  = 10 // claim attempts per user per minute
	pairingLimitWindow = time.Minute

	pairingInvalidMessage = "That code isn't valid. Check it, or make a new one on the TV."
)

// pairing is one code a TV is waiting on.
type pairing struct {
	expires time.Time
	// ip is the address that made the code: only it can poll the code, so nobody else can
	// collect the token or learn which codes exist.
	ip string
	// Set when a user claims the code.
	approved *models.LoginResponse
}

// PairingStore holds the codes and the rate limits. All access goes through mu; expired codes
// are removed whenever the store is used.
type PairingStore struct {
	mu      sync.Mutex
	now     func() time.Time // time.Now, replaced in tests
	codes   map[string]*pairing
	creates map[string][]time.Time // client address → when it made codes, in the last window
	claims  map[int64][]time.Time  // user ID → when they tried to claim, in the last window
}

func NewPairingStore() *PairingStore {
	return &PairingStore{
		now:     time.Now,
		codes:   map[string]*pairing{},
		creates: map[string][]time.Time{},
		claims:  map[int64][]time.Time{},
	}
}

// sweep removes expired codes and limiter entries with nothing left in the window. Callers hold mu.
func (s *PairingStore) sweep(now time.Time) {
	for code, p := range s.codes {
		if !now.Before(p.expires) {
			delete(s.codes, code)
		}
	}
	for ip, times := range s.creates {
		if times = recent(times, now); len(times) == 0 {
			delete(s.creates, ip)
		} else {
			s.creates[ip] = times
		}
	}
	for id, times := range s.claims {
		if times = recent(times, now); len(times) == 0 {
			delete(s.claims, id)
		} else {
			s.claims[id] = times
		}
	}
}

// recent keeps the times within the limit window before now.
func recent(times []time.Time, now time.Time) []time.Time {
	kept := times[:0]
	for _, t := range times {
		if now.Sub(t) < pairingLimitWindow {
			kept = append(kept, t)
		}
	}
	return kept
}

// Create makes a new code for the client at ip. ok is false when that address has made too many
// codes in the last minute, or the store is full.
func (s *PairingStore) Create(ip string) (code string, expires time.Time, ok bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	s.sweep(now)
	if len(s.creates[ip]) >= pairingCreateLimit || len(s.codes) >= pairingMaxCodes {
		return "", time.Time{}, false, nil
	}
	for {
		if code, err = newPairingCode(); err != nil {
			return "", time.Time{}, false, err
		}
		if _, taken := s.codes[code]; !taken {
			break
		}
	}
	s.creates[ip] = append(s.creates[ip], now)
	expires = now.Add(pairingTTL)
	s.codes[code] = &pairing{expires: expires, ip: ip}
	return code, expires, true, nil
}

// Poll is a TV asking after its code. It returns the sign-in once the code is approved, and
// removes the code then, so a second poll finds nothing. found is false for an unknown, expired
// or used code, and for one made from another address.
func (s *PairingStore) Poll(code, ip string) (approved *models.LoginResponse, found bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweep(s.now())
	p := s.codes[normalizePairingCode(code)]
	if p == nil || p.ip != ip {
		return nil, false
	}
	if p.approved != nil {
		delete(s.codes, normalizePairingCode(code))
	}
	return p.approved, true
}

// claimResult is what a claim came to.
type claimResult int

const (
	claimApproved claimResult = iota
	claimInvalid              // unknown, expired or already claimed
	claimLimited              // the user has tried too often in the last minute
)

// Claim approves code for userID with the sign-in the TV will collect, made by signIn only when
// the code is valid. Every attempt counts toward the user's limit, right or wrong.
func (s *PairingStore) Claim(code string, userID int64, signIn func() (*models.LoginResponse, error)) (claimResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	s.sweep(now)
	if len(s.claims[userID]) >= pairingClaimLimit {
		return claimLimited, nil
	}
	s.claims[userID] = append(s.claims[userID], now)
	p := s.codes[normalizePairingCode(code)]
	if p == nil || p.approved != nil {
		return claimInvalid, nil
	}
	response, err := signIn()
	if err != nil {
		return claimInvalid, err
	}
	p.approved = response
	return claimApproved, nil
}

// newPairingCode picks pairingCodeLen characters of pairingAlphabet with crypto/rand.
func newPairingCode() (string, error) {
	max := big.NewInt(int64(len(pairingAlphabet)))
	b := make([]byte, pairingCodeLen)
	for i := range b {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b[i] = pairingAlphabet[n.Int64()]
	}
	return string(b), nil
}

// normalizePairingCode accepts a code as people type it: lower case, with spaces or hyphens.
func normalizePairingCode(code string) string {
	code = strings.ToUpper(code)
	return strings.NewReplacer(" ", "", "-", "").Replace(code)
}

// PairingHandler serves the TV sign-in endpoints.
type PairingHandler struct {
	authService *auth.Service
	store       *PairingStore
}

func NewPairingHandler(authService *auth.Service) *PairingHandler {
	return &PairingHandler{authService: authService, store: NewPairingStore()}
}

// Create is POST /api/auth/pairing (no sign-in): a new code for a TV to show.
func (h *PairingHandler) Create(w http.ResponseWriter, r *http.Request) {
	code, expires, ok, err := h.store.Create(clientIP(r))
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Couldn't make a code")
		return
	}
	if !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(pairingLimitWindow.Seconds())))
		respondError(w, http.StatusTooManyRequests, "Too many codes asked for. Try again in a minute.")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{
		"code":       code,
		"expires_at": expires.UTC().Format(time.RFC3339),
	})
}

// Poll is GET /api/auth/pairing/{code} (no sign-in): waiting, expired, or approved with the
// session (token and user, as signing in returns them), given once.
func (h *PairingHandler) Poll(w http.ResponseWriter, r *http.Request) {
	approved, found := h.store.Poll(mux.Vars(r)["code"], clientIP(r))
	switch {
	case !found:
		respondJSON(w, http.StatusOK, map[string]string{"status": "expired"})
	case approved == nil:
		respondJSON(w, http.StatusOK, map[string]string{"status": "waiting"})
	default:
		respondJSON(w, http.StatusOK, map[string]any{
			"status":     "approved",
			"token":      approved.Token,
			"expires_at": approved.ExpiresAt,
			"user":       approved.User,
		})
	}
}

// Claim is POST /api/pairing/claim (signed in), body {"code"}: the TV showing the code signs in
// as the user making the request.
func (h *PairingHandler) Claim(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		respondError(w, http.StatusUnauthorized, "Missing authorization")
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	ip := clientIP(r)
	result, err := h.store.Claim(req.Code, user.ID, func() (*models.LoginResponse, error) {
		// The same session token signing in makes.
		token, expiresAt, err := h.authService.GenerateToken(user)
		if err != nil {
			return nil, err
		}
		return &models.LoginResponse{Token: token, ExpiresAt: expiresAt, User: *user}, nil
	})
	switch {
	case err != nil:
		respondError(w, http.StatusInternalServerError, "Couldn't sign the TV in")
	case result == claimLimited:
		logs.Auth("TV sign-in code refused: %q from %s tried too often", user.Username, ip)
		w.Header().Set("Retry-After", strconv.Itoa(int(pairingLimitWindow.Seconds())))
		respondError(w, http.StatusTooManyRequests, "Too many tries. Wait a minute, then try again.")
	case result == claimInvalid:
		logs.Auth("TV sign-in code refused: %q from %s entered an invalid code", user.Username, ip)
		respondError(w, http.StatusNotFound, pairingInvalidMessage)
	default:
		logs.Auth("Signed in a TV with a code: %q from %s", user.Username, ip)
		respondJSON(w, http.StatusOK, struct{}{})
	}
}
