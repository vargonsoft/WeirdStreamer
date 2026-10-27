package subtitles

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func touch(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func isVideo(name string) bool { return strings.HasSuffix(name, ".mkv") }

func TestSidecars(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "Example Movie (2008).mkv")
	touch(t, video, "")
	touch(t, filepath.Join(dir, "Example Movie (2008).srt"), "")
	touch(t, filepath.Join(dir, "Example Movie (2008).en.forced.srt"), "")
	touch(t, filepath.Join(dir, "Example Movie (2008).spa.sdh.ass"), "")
	touch(t, filepath.Join(dir, "Example Movie (2008).nfo"), "")
	touch(t, filepath.Join(dir, "Other Movie.en.srt"), "")    // another video's
	touch(t, filepath.Join(dir, "Subs", "2_English.srt"), "") // the only video: every Subs file

	got := map[string]Track{}
	for _, tr := range Sidecars(video, isVideo) {
		got[filepath.Base(tr.Path)] = tr
	}
	if len(got) != 4 {
		t.Fatalf("found %d: %v", len(got), got)
	}
	if tr := got["Example Movie (2008).en.forced.srt"]; tr.Language != "eng" || !tr.Forced || tr.Codec != "subrip" {
		t.Errorf("en.forced: %+v", tr)
	}
	if tr := got["Example Movie (2008).spa.sdh.ass"]; tr.Language != "spa" || !tr.HearingImpaired || tr.Codec != "ass" {
		t.Errorf("spa.sdh: %+v", tr)
	}
	if tr := got["2_English.srt"]; tr.Language != "eng" || tr.Source != SourceFile {
		t.Errorf("Subs folder: %+v", tr)
	}
	if _, ok := got["Other Movie.en.srt"]; ok {
		t.Error("another video's subtitles")
	}

	// With two videos in the folder, a Subs folder's files must be named after the video.
	touch(t, filepath.Join(dir, "Example Movie (2008) - Extras.mkv"), "")
	for _, tr := range Sidecars(video, isVideo) {
		if filepath.Base(tr.Path) == "2_English.srt" {
			t.Error("a Subs file not named after the video, beside another video")
		}
	}
}

func TestLanguage(t *testing.T) {
	for in, want := range map[string]string{"en": "eng", "ENG": "eng", "english": "eng", "pt-BR": "por",
		"pob": "por", "fra": "fre", "zh-cn": "chi", "x264": "", "2008": ""} {
		if got := Language(in); got != want {
			t.Errorf("Language(%q) = %q, want %q", in, got, want)
		}
	}
	if Short("ger") != "de" || Short("deu") != "de" || Short("xx") != "" {
		t.Error("Short")
	}
}

func TestStoreAndDownloaded(t *testing.T) {
	root := t.TempDir()
	video := "/media/Movies/Example Movie (2008)/Example Movie (2008).mkv"
	if len(Downloaded(root, video)) != 0 {
		t.Fatal("nothing stored yet")
	}
	r := Result{FileID: 42, Language: "en", Release: "Yes.Man.2008.720p", HearingImpaired: true}
	if _, err := Store(root, video, r, []byte("1\n00:00:01,000 --> 00:00:02,000\nHi\n")); err != nil {
		t.Fatal(err)
	}
	// Downloading the same file again replaces it rather than listing it twice.
	if _, err := Store(root, video, r, []byte("1\n00:00:01,000 --> 00:00:02,000\nHello\n")); err != nil {
		t.Fatal(err)
	}
	got := Downloaded(root, video)
	if len(got) != 1 || got[0].FileID != 42 || got[0].Language != "eng" || got[0].Title != "Yes.Man.2008.720p" ||
		!got[0].HearingImpaired || got[0].Source != SourceDownload {
		t.Fatalf("Downloaded: %+v", got)
	}
	if b, _ := os.ReadFile(got[0].Path); !strings.Contains(string(b), "Hello") {
		t.Error("the later download is kept")
	}
	if len(Downloaded(root, video+"x")) != 0 {
		t.Error("another video's downloads")
	}
	if _, err := Store("", video, r, nil); err == nil {
		t.Error("no subtitlePath: want an error")
	}
}

func TestCharset(t *testing.T) {
	dir := t.TempDir()
	utf := filepath.Join(dir, "a.srt")
	latin := filepath.Join(dir, "b.srt")
	touch(t, utf, "café")
	touch(t, latin, "caf\xe9")
	if Charset(utf) != "UTF-8" || Charset(latin) != "CP1252" {
		t.Error("Charset")
	}
}

func TestMovieHash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v.mkv")
	// 128 KiB of zeros but one byte: the hash is the size plus that byte's word.
	b := make([]byte, 128*1024)
	b[8] = 1
	touch(t, path, string(b))
	if got := MovieHash(path); got != "0000000000020001" {
		t.Errorf("MovieHash = %s", got)
	}
	small := filepath.Join(t.TempDir(), "s.mkv")
	touch(t, small, "tiny")
	if MovieHash(small) != "" {
		t.Error("a file under 64 KiB has no hash")
	}
}

func TestSearchAndDownload(t *testing.T) {
	var searched string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The API wants the key; the download link it hands out doesn't.
		if r.URL.Path != "/file" && (r.Header.Get("Api-Key") != "k" || r.Header.Get("User-Agent") == "") {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		switch r.URL.Path {
		case "/subtitles":
			searched = r.URL.RawQuery
			w.Write([]byte(`{"data": [
				{"attributes": {"language": "en", "release": "Popular", "download_count": 900, "files": [{"file_id": 1}]}},
				{"attributes": {"language": "en", "release": "Exact", "download_count": 5, "moviehash_match": true, "files": [{"file_id": 2}]}},
				{"attributes": {"language": "en", "release": "Robot", "download_count": 5000, "ai_translated": true, "files": [{"file_id": 3}]}},
				{"attributes": {"language": "en", "release": "Movie with the id", "feature_details": {"feature_type": "Movie"}, "files": [{"file_id": 4}]}},
				{"attributes": {"language": "en", "release": "No files", "files": []}}]}`))
		case "/download":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["file_id"] == float64(99) {
				w.WriteHeader(http.StatusNotAcceptable)
				w.Write([]byte(`{"message": "You have downloaded your allowed 100 subtitles for 24h", "remaining": 0}`))
				return
			}
			w.Write([]byte(`{"link": "` + "http://" + r.Host + `/file", "remaining": 99, "reset_time": "23 hours"}`))
		case "/file":
			w.Write([]byte("SRT"))
		}
	}))
	defer srv.Close()
	c := &Client{APIKey: "k", BaseURL: srv.URL, HTTP: srv.Client()}

	results, err := c.Search(context.Background(), Query{ParentTMDBID: 7, Season: 1, Episode: 2,
		MovieHash: "abc", Languages: []string{"es", "EN"}})
	if err != nil {
		t.Fatal(err)
	}
	// Parameters in order, lowercase: the API redirects anything else.
	if want := "episode_number=2&languages=en%2Ces&moviehash=abc&parent_tmdb_id=7&season_number=1"; searched != want {
		t.Errorf("query %q, want %q", searched, want)
	}
	var order []string
	for _, r := range results {
		order = append(order, r.Release)
	}
	if strings.Join(order, ",") != "Exact,Popular,Robot" {
		t.Errorf("order %v: made for the file, then people's by downloads, machine translations last", order)
	}

	data, quota, err := c.Download(context.Background(), 2)
	if err != nil || string(data) != "SRT" || quota.Remaining != 99 {
		t.Errorf("Download: %q %+v %v", data, quota, err)
	}
	if _, quota, err := c.Download(context.Background(), 99); err == nil ||
		!strings.Contains(err.Error(), "allowed 100") || quota.Remaining != 0 {
		t.Errorf("over the limit: %+v %v", quota, err)
	}
	if _, err := (&Client{BaseURL: srv.URL}).Search(context.Background(), Query{Title: "x"}); err != ErrNoKey {
		t.Errorf("no key: %v", err)
	}
}
