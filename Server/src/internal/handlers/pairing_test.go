package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/weirdstreamer/server/internal/auth"
	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/models"
)

// pairingTest is a pairing handler with a user to claim codes and the routes as main.go has them.
type pairingTest struct {
	t       *testing.T
	handler *PairingHandler
	service *auth.Service
	user    *models.User
	router  *mux.Router
}

func newPairingTest(t *testing.T) *pairingTest {
	t.Helper()
	setupConfig(t)
	dir := t.TempDir()
	db, err := database.New(filepath.Join(dir, "users.db"), filepath.Join(dir, "media.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.CreateUser("viewer", "pw", false, "normal"); err != nil {
		t.Fatal(err)
	}
	user, err := db.GetUserByUsername("viewer")
	if err != nil {
		t.Fatal(err)
	}
	service := auth.NewService(db)
	h := NewPairingHandler(service)
	r := mux.NewRouter()
	r.HandleFunc("/api/auth/pairing", h.Create).Methods("POST")
	r.HandleFunc("/api/auth/pairing/{code}", h.Poll).Methods("GET")
	return &pairingTest{t: t, handler: h, service: service, user: user, router: r}
}

func (p *pairingTest) create() string {
	p.t.Helper()
	rec := httptest.NewRecorder()
	p.router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/auth/pairing", nil))
	if rec.Code != http.StatusOK {
		p.t.Fatalf("create: status %d, body %s", rec.Code, rec.Body)
	}
	var body struct {
		Code      string `json:"code"`
		ExpiresAt string `json:"expires_at"`
	}
	json.Unmarshal(rec.Body.Bytes(), &body)
	if _, err := time.Parse(time.RFC3339, body.ExpiresAt); err != nil {
		p.t.Errorf("expires_at %q isn't RFC 3339: %v", body.ExpiresAt, err)
	}
	return body.Code
}

// poll returns the status and the whole reply.
func (p *pairingTest) poll(code string) (string, map[string]any) {
	p.t.Helper()
	rec := httptest.NewRecorder()
	p.router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/auth/pairing/"+code, nil))
	if rec.Code != http.StatusOK {
		p.t.Fatalf("poll: status %d, body %s", rec.Code, rec.Body)
	}
	var body map[string]any
	json.Unmarshal(rec.Body.Bytes(), &body)
	status, _ := body["status"].(string)
	return status, body
}

func (p *pairingTest) claim(code string) *httptest.ResponseRecorder {
	p.t.Helper()
	b, _ := json.Marshal(map[string]string{"code": code})
	req := httptest.NewRequest(http.MethodPost, "/api/pairing/claim", strings.NewReader(string(b)))
	req = req.WithContext(auth.WithUser(req.Context(), p.user))
	rec := httptest.NewRecorder()
	p.handler.Claim(rec, req)
	return rec
}

func TestPairingCodeAlphabetAndLength(t *testing.T) {
	for i := 0; i < 500; i++ {
		code, err := newPairingCode()
		if err != nil {
			t.Fatal(err)
		}
		if len(code) != pairingCodeLen {
			t.Fatalf("code %q has %d characters, want %d", code, len(code), pairingCodeLen)
		}
		for _, c := range code {
			if !strings.ContainsRune(pairingAlphabet, c) {
				t.Fatalf("code %q has %q, not in the alphabet", code, c)
			}
		}
	}
	for _, c := range "0O1I" {
		if strings.ContainsRune(pairingAlphabet, c) {
			t.Errorf("the alphabet has %q", c)
		}
	}
}

// A claimed code gives the TV a working session token for the claiming user, once.
func TestPairingApprovedOnce(t *testing.T) {
	p := newPairingTest(t)
	code := p.create()
	if status, _ := p.poll(code); status != "waiting" {
		t.Fatalf("before the claim: status %q, want waiting", status)
	}
	// Typed as people do: lower case, with a hyphen and a space.
	typed := strings.ToLower(code[:2]) + "-" + code[2:3] + " " + code[3:]
	if rec := p.claim(typed); rec.Code != http.StatusOK {
		t.Fatalf("claim %q: status %d, body %s", typed, rec.Code, rec.Body)
	}
	status, body := p.poll(code)
	if status != "approved" {
		t.Fatalf("after the claim: status %q, want approved", status)
	}
	token, _ := body["token"].(string)
	claims, err := p.service.ValidateToken(token)
	if err != nil {
		t.Fatalf("approved token doesn't validate: %v", err)
	}
	if claims.Username != "viewer" || claims.UserID != p.user.ID {
		t.Errorf("token is for %q (%d), want viewer (%d)", claims.Username, claims.UserID, p.user.ID)
	}
	if user, _ := body["user"].(map[string]any); user["username"] != "viewer" {
		t.Errorf("reply's user is %v, want viewer", body["user"])
	}
	if status, body := p.poll(code); status != "expired" || body["token"] != nil {
		t.Errorf("second poll: status %q, body %v; want expired, no token", status, body)
	}
}

func TestPairingClaimSingleUse(t *testing.T) {
	p := newPairingTest(t)
	code := p.create()
	if rec := p.claim(code); rec.Code != http.StatusOK {
		t.Fatalf("first claim: status %d", rec.Code)
	}
	rec := p.claim(code)
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "isn't valid") {
		t.Errorf("second claim: status %d, body %s; want 404", rec.Code, rec.Body)
	}
	// And once collected it's gone for good.
	p.poll(code)
	if rec := p.claim(code); rec.Code != http.StatusNotFound {
		t.Errorf("claim after collecting: status %d, want 404", rec.Code)
	}
}

func TestPairingUnknownCode(t *testing.T) {
	p := newPairingTest(t)
	if rec := p.claim("ZZZZZ"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown code claimed: status %d, want 404", rec.Code)
	}
	if status, _ := p.poll("ZZZZZ"); status != "expired" {
		t.Errorf("unknown code polled: status %q, want expired", status)
	}
}

func TestPairingExpiredCodeRefused(t *testing.T) {
	p := newPairingTest(t)
	now := time.Now()
	p.handler.store.now = func() time.Time { return now }
	code := p.create()
	now = now.Add(pairingTTL)
	if status, _ := p.poll(code); status != "expired" {
		t.Errorf("poll after expiry: status %q, want expired", status)
	}
	if rec := p.claim(code); rec.Code != http.StatusNotFound {
		t.Errorf("claim after expiry: status %d, want 404", rec.Code)
	}
	if len(p.handler.store.codes) != 0 {
		t.Errorf("expired code still held: %v", p.handler.store.codes)
	}

	// Claimed but not collected in time: gone too.
	code = p.create()
	if rec := p.claim(code); rec.Code != http.StatusOK {
		t.Fatalf("claim: status %d", rec.Code)
	}
	now = now.Add(pairingTTL)
	if status, _ := p.poll(code); status != "expired" {
		t.Errorf("approved code polled after expiry: status %q, want expired", status)
	}
}

// Only the address that made a code can poll it.
func TestPairingPollFromOtherAddress(t *testing.T) {
	p := newPairingTest(t)
	code := p.create()
	p.claim(code)
	req := httptest.NewRequest(http.MethodGet, "/api/auth/pairing/"+code, nil)
	req.RemoteAddr = "203.0.113.9:4000"
	rec := httptest.NewRecorder()
	p.router.ServeHTTP(rec, req)
	if strings.Contains(rec.Body.String(), "token") || !strings.Contains(rec.Body.String(), "expired") {
		t.Errorf("poll from another address: %s", rec.Body)
	}
	if status, _ := p.poll(code); status != "approved" {
		t.Errorf("the TV's own poll: status %q, want approved", status)
	}
}

// Each user gets 10 claim attempts a minute, right or wrong; a valid code is refused past that.
func TestPairingClaimLimitPerUser(t *testing.T) {
	p := newPairingTest(t)
	now := time.Now()
	p.handler.store.now = func() time.Time { return now }
	for i := 0; i < pairingClaimLimit; i++ {
		if rec := p.claim("ZZZZZ"); rec.Code != http.StatusNotFound {
			t.Fatalf("attempt %d: status %d, want 404", i+1, rec.Code)
		}
	}
	code := p.create()
	if rec := p.claim(code); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("attempt past the limit: status %d, want 429", rec.Code)
	}
	if status, _ := p.poll(code); status != "waiting" {
		t.Errorf("refused claim approved the code: status %q", status)
	}
	// Another user isn't held back.
	other := *p.user
	other.ID++
	other.Username = "other"
	p.user = &other
	if rec := p.claim("ZZZZZ"); rec.Code != http.StatusNotFound {
		t.Errorf("another user's attempt: status %d, want 404", rec.Code)
	}
	// A minute later the first user can try again.
	now = now.Add(pairingLimitWindow)
	p.user.ID--
	if rec := p.claim(code); rec.Code != http.StatusOK {
		t.Errorf("claim a minute later: status %d, want 200", rec.Code)
	}
}

func TestPairingCreateLimitPerAddress(t *testing.T) {
	p := newPairingTest(t)
	for i := 0; i < pairingCreateLimit; i++ {
		p.create()
	}
	rec := httptest.NewRecorder()
	p.router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/auth/pairing", nil))
	if rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), `"message"`) {
		t.Errorf("code past the limit: status %d, body %s; want 429 with a message", rec.Code, rec.Body)
	}
}
