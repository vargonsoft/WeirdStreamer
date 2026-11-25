package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/weirdstreamer/server/internal/media"
)

// cli server stop|restart: only this machine with the key, refused while someone watches unless
// forced, and the stop or restart asked for is the one done.
func TestLocalShutdown(t *testing.T) {
	oldKey, oldShutdown := localKey, Shutdown
	defer func() { localKey, Shutdown = oldKey, oldShutdown }()
	localKey = "secret"
	calls := make(chan bool, 4)
	Shutdown = func(restart bool) { calls <- restart }

	streams := media.NewStreamTracker()
	h := &MediaHandler{streams: streams}
	post := func(path, from, key string) int {
		r := httptest.NewRequest(http.MethodPost, path, nil)
		r.RemoteAddr = from + ":40000"
		if key != "" {
			r.Header.Set("X-Local-Key", key)
		}
		w := httptest.NewRecorder()
		h.LocalShutdown(w, r)
		return w.Code
	}
	called := func() (bool, bool) {
		select {
		case restart := <-calls:
			return true, restart
		case <-time.After(time.Second):
			return false, false
		}
	}

	if code := post("/local/restart", "192.168.1.5", "secret"); code != http.StatusNotFound {
		t.Errorf("from another machine: %d, want 404", code)
	}
	if code := post("/local/restart", "127.0.0.1", "wrong"); code != http.StatusNotFound {
		t.Errorf("with the wrong key: %d, want 404", code)
	}

	watching := streams.Begin("someone", func(i *media.StreamInfo) { i.User = "someone" })
	if code := post("/local/restart", "127.0.0.1", "secret"); code != http.StatusConflict {
		t.Errorf("while someone watches: %d, want 409", code)
	}
	select {
	case <-calls:
		t.Fatal("shut down while someone watches")
	default:
	}
	if code := post("/local/restart?force=1", "127.0.0.1", "secret"); code != http.StatusAccepted {
		t.Errorf("forced: %d, want 202", code)
	}
	if ok, restart := called(); !ok || !restart {
		t.Errorf("forced restart: called %v, restart %v", ok, restart)
	}
	watching.End()

	if code := post("/local/stop", "[::1]", "secret"); code != http.StatusAccepted {
		t.Errorf("stop with nobody watching: %d, want 202", code)
	}
	if ok, restart := called(); !ok || restart {
		t.Errorf("stop: called %v, restart %v", ok, restart)
	}
}
