package database

import (
	"errors"
	"testing"
)

// Tables keyed by media path take client paths only: a disk path is refused, never stored.
func TestClientPathKeysOnly(t *testing.T) {
	db := testDB(t)
	const disk, client = "/srv/shows/Show/S01E01.mkv", "8838533a/Show/S01E01.mkv"
	span := &Span{Start: 1, End: 2, Source: "manual"}
	writes := map[string]func(key string) error{
		"SaveAnalysis":       func(k string) error { return db.SaveAnalysis(k, span, nil, 1) },
		"SetManualSegment":   func(k string) error { return db.SetManualSegment(k, "intro", span) },
		"SetSubtitleOffset":  func(k string) error { return db.SetSubtitleOffset(k, "stream:0", 100) },
		"SaveDiscInfo":       func(k string) error { return db.SaveDiscInfo(k, 1, 2, []byte("{}")) },
		"SetFolderKey":       func(k string) error { return db.SetFolderKey("8838533a", k, "tv", "show") },
		"SetEpisodeOverride": func(k string) error { return db.SetEpisodeOverride("8838533a", k, 1, 2) },
		"SavePlayback":       func(k string) error { return db.SavePlayback(1, k, PlaybackUpdate{Position: 5}, false) },
		"ClearPosition":      func(k string) error { return db.ClearPosition(1, k) },
	}
	for name, write := range writes {
		for _, bad := range []string{disk, ""} {
			if err := write(bad); !errors.Is(err, ErrNotClientPath) {
				t.Errorf("%s(%q) = %v, want ErrNotClientPath", name, bad, err)
			}
		}
		if err := write(client); err != nil {
			t.Errorf("%s(%q): %v", name, client, err)
		}
	}
	if segs, _ := db.GetSegments(client); segs.Intro == nil {
		t.Error("segments not stored by client path")
	}
	if offsets, _ := db.SubtitleOffsets(client); offsets["stream:0"] != 100 {
		t.Errorf("offsets %v", offsets)
	}
	if info, _ := db.GetDiscInfo(client, 1, 2); string(info) != "{}" {
		t.Errorf("disc info %q", info)
	}
	if keys, _ := db.FolderKeys("8838533a", []string{"8838533a/Show/S01E01.mkv"}); keys[client] != "show" {
		t.Errorf("folder keys %v", keys)
	}

	// Forgetting a file: its scanner rows by disk path, the rest by client path.
	if err := db.ForgetMedia([]GoneFile{{Path: disk, Key: client}}); err != nil {
		t.Fatal(err)
	}
	if segs, _ := db.GetSegments(client); segs.Intro != nil {
		t.Error("segments kept after ForgetMedia")
	}
	if info, _ := db.GetDiscInfo(client, 1, 2); info != nil {
		t.Error("disc info kept after ForgetMedia")
	}
}
