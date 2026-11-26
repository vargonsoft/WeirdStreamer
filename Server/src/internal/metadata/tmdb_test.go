package metadata

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := NewClient("test-token", "en-US")
	c.apiBase = srv.URL + "/3"
	c.imageBase = srv.URL + "/t/p/"
	return c
}

func TestClientSendsAuthAndParsesMovie(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing bearer token")
		}
		if r.URL.Path != "/3/movie/9476" || r.URL.Query().Get("append_to_response") != "credits,images,release_dates" ||
			r.URL.Query().Get("include_image_language") != "en,null" || r.URL.Query().Get("language") != "en-US" {
			t.Errorf("unexpected request %s", r.URL)
		}
		w.Write([]byte(`{"id":9476,"title":"A Knight's Tale","release_date":"2001-05-11","runtime":133,
			"credits":{"cast":[{"id":1810,"name":"Heath Ledger","character":"William Thatcher","order":0}],
			"crew":[{"id":1,"name":"Brian Helgeland","job":"Director"}]},
			"images":{"logos":[{"file_path":"/l.png","iso_639_1":"en","vote_average":3.3},{"file_path":"/n.png","iso_639_1":null}]},
			"release_dates":{"results":[{"iso_3166_1":"GB","release_dates":[{"certification":"12","type":3}]},
			{"iso_3166_1":"US","release_dates":[{"certification":"","type":1},{"certification":"PG","type":5},{"certification":"PG-13","type":3}]}]}}`))
	})
	m, err := c.Movie(context.Background(), 9476)
	if err != nil {
		t.Fatal(err)
	}
	if m.Title != "A Knight's Tale" || m.Runtime != 133 || m.Credits.Cast[0].Character != "William Thatcher" ||
		m.Credits.Crew[0].Job != "Director" || len(m.Images.Logos) != 2 || m.Images.Logos[1].Language != nil ||
		m.Rating() != "PG-13" {
		t.Errorf("parsed movie = %+v", m)
	}
}

func TestClientRetriesRateLimitAndMapsNotFound(t *testing.T) {
	calls := 0
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path == "/3/movie/1" {
			http.NotFound(w, r)
			return
		}
		if calls == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte(`{"results":[{"id":5,"title":"Inception","release_date":"2010-07-15"}]}`))
	})
	results, err := c.SearchMovie(context.Background(), "Inception", 0)
	if err != nil || len(results) != 1 || results[0].ID != 5 || calls != 2 {
		t.Fatalf("search = %+v, %v after %d calls", results, err, calls)
	}
	if _, err := c.Movie(context.Background(), 1); err != ErrNotFound {
		t.Errorf("missing movie error = %v, want ErrNotFound", err)
	}
}

func TestDownloadImage(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/t/p/w500/poster.jpg" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte("jpeg-bytes"))
	})
	dest := filepath.Join(t.TempDir(), "movie", "1", "poster.jpg")
	if err := c.DownloadImage(context.Background(), "w500", "/poster.jpg", dest); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dest); string(b) != "jpeg-bytes" {
		t.Errorf("downloaded %q", b)
	}
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(dest), ".download-*"))
	if len(leftovers) != 0 {
		t.Errorf("temp files left behind: %v", leftovers)
	}
}

// A show's US rating and its first network are read from its details.
func TestTVRatingAndNetwork(t *testing.T) {
	var d TVDetails
	if err := json.Unmarshal([]byte(`{"networks":[{"id":6,"name":"NBC"},{"id":2,"name":"ABC"}],
		"content_ratings":{"results":[{"iso_3166_1":"DE","rating":"12"},{"iso_3166_1":"US","rating":"TV-PG"}]}}`), &d); err != nil {
		t.Fatal(err)
	}
	if d.Rating() != "TV-PG" || len(d.Networks) != 2 || d.Networks[0].ID != 6 || d.Networks[0].Name != "NBC" {
		t.Errorf("parsed show = %+v", d)
	}
	if (&TVDetails{}).Rating() != "" || (&MovieDetails{}).Rating() != "" {
		t.Error("a title without ratings has one")
	}
}
