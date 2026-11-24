package media

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/weirdstreamer/server/internal/config"
)

// damagedFLV writes a 5-minute FLV with a seek index (good), and a copy whose index points at
// the wrong places (damaged): jumping into it finds nothing, as in FLVs broken that way.
func damagedFLV(t *testing.T, dir string) (good, damaged string) {
	t.Helper()
	good = filepath.Join(dir, "good.flv")
	out, err := exec.Command("ffmpeg", "-v", "error", "-y", "-f", "lavfi", "-i", "testsrc=s=160x120:r=10:d=300",
		"-f", "lavfi", "-i", "sine=d=300", "-c:v", "libx264", "-preset", "ultrafast", "-g", "20", "-c:a", "aac",
		"-flvflags", "add_keyframe_index", good).CombinedOutput()
	if err != nil {
		t.Fatalf("making the FLV: %v %s", err, out)
	}
	data, err := os.ReadFile(good)
	if err != nil {
		t.Fatal(err)
	}
	i := bytes.Index(data, []byte("filepositions"))
	if i < 0 {
		t.Fatal("no keyframe index in the FLV")
	}
	j := i + len("filepositions")
	n := int(binary.BigEndian.Uint32(data[j+1 : j+5]))
	for m, k := 0, j+5; m < n; m, k = m+1, k+9 {
		binary.BigEndian.PutUint64(data[k+1:k+9], math.Float64bits(float64((n-m)*1000)))
	}
	damaged = filepath.Join(dir, "Damaged Video.avi") // an FLV with another name, as found
	if err := os.WriteFile(damaged, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return good, damaged
}

func TestRepairs(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("no ffmpeg")
	}
	root := t.TempDir()
	library := filepath.Join(root, "Movies")
	os.MkdirAll(library, 0o755)
	saved := config.GlobalConfig
	config.GlobalConfig = &config.Config{Media: config.MediaConfig{
		Paths:      []config.MediaPath{{ID: "lib1", Name: "Movies", Path: library}},
		RepairPath: filepath.Join(root, "repairs"), TrashPath: filepath.Join(root, "trash"),
	}}
	repairIndex.mu.Lock()
	repairIndex.loaded, repairIndex.entries = false, nil
	repairIndex.mu.Unlock()
	t.Cleanup(func() {
		config.GlobalConfig = saved
		repairIndex.mu.Lock()
		repairIndex.loaded, repairIndex.entries = false, nil
		repairIndex.mu.Unlock()
	})
	good, damaged := damagedFLV(t, library)
	ctx := context.Background()

	if bad, reason, _, err := checkSeek(ctx, good); err != nil || bad {
		t.Fatalf("good FLV: damaged %v (%s) %v", bad, reason, err)
	}
	// DivX with B-frames in AVI: its packets have no pts (only a dts), and it seeks fine
	avi := filepath.Join(root, "bframes.avi")
	if out, err := exec.Command("ffmpeg", "-v", "error", "-y", "-f", "lavfi", "-i", "testsrc=s=160x120:r=25:d=300",
		"-c:v", "mpeg4", "-vtag", "DX50", "-bf", "2", "-g", "250", avi).CombinedOutput(); err != nil {
		t.Fatalf("making the AVI: %v %s", err, out)
	}
	if bad, reason, _, err := checkSeek(ctx, avi); err != nil || bad {
		t.Fatalf("B-frame AVI: damaged %v (%s) %v", bad, reason, err)
	}
	// MPEG-TS seeks fine (its packet lines have an extra field)
	ts := filepath.Join(root, "good.ts")
	if out, err := exec.Command("ffmpeg", "-v", "error", "-y", "-i", good, "-c", "copy", "-f", "mpegts", ts).CombinedOutput(); err != nil {
		t.Fatalf("making the TS: %v %s", err, out)
	}
	if bad, reason, _, err := checkSeek(ctx, ts); err != nil || bad {
		t.Fatalf("good TS: damaged %v (%s) %v", bad, reason, err)
	}
	bad, reason, claimed, err := checkSeek(ctx, damaged)
	if err != nil || !bad {
		t.Fatalf("damaged FLV: damaged %v %v", bad, err)
	}
	t.Logf("damaged: %s (claims %.0f s)", reason, claimed)

	// Fixed: a Matroska copy that seeks, as long as the video
	key := config.CacheKey(damaged)
	size, length, err := fixVideo(ctx, damaged, key)
	if err != nil {
		t.Fatalf("fixing: %v", err)
	}
	if math.Abs(length-300) > 2 || size == 0 {
		t.Fatalf("fixed copy: %d bytes, %.1f s long", size, length)
	}
	if Playable(damaged) != damaged {
		t.Fatal("played from the fixed copy before the index says so")
	}
	info, _ := os.Stat(damaged)
	repairIndex.mu.Lock()
	loadRepairIndex()
	repairIndex.entries[key] = repairEntry{Path: damaged, Size: info.Size(), ModTime: info.ModTime().Unix(),
		State: RepairFixed, Reason: reason, FixedSize: size, Duration: length}
	repairIndex.mu.Unlock()
	if Playable(damaged) != fixedFile(key) {
		t.Fatalf("not played from the fixed copy: %s", Playable(damaged))
	}
	if got := DamagedVideos(func(string) bool { return true }); len(got) != 1 || got[0].State != RepairFixed {
		t.Fatalf("damaged list: %+v", got)
	}

	// Replaced: the copy in its place as .mkv, the original in the trash
	newPath, err := ReplaceWithFixed(damaged)
	if err != nil {
		t.Fatalf("replacing: %v", err)
	}
	if newPath != filepath.Join(library, "Damaged Video.mkv") {
		t.Fatalf("new name %s", newPath)
	}
	if _, err := os.Stat(damaged); !os.IsNotExist(err) {
		t.Fatal("the original is still in the library")
	}
	if got, err := os.Stat(newPath); err != nil || got.ModTime().Unix() != info.ModTime().Unix() {
		t.Fatalf("the new file: %v (its time should be the original's)", err)
	}
	if bad, _, _, err := checkSeek(ctx, newPath); err != nil || bad {
		t.Fatalf("the new file doesn't seek: %v", err)
	}
	if got := DamagedVideos(func(string) bool { return true }); len(got) != 0 {
		t.Fatalf("still listed after replacing: %+v", got)
	}
	trash, err := Trash()
	if err != nil || trash.Files != 1 || trash.Bytes != info.Size() {
		t.Fatalf("trash: %+v %v", trash, err)
	}
	if n, _ := EmptyTrash(time.Hour); n != 0 {
		t.Fatal("emptied what went in just now")
	}
	if n, err := EmptyTrash(0); n != 1 || err != nil {
		t.Fatalf("emptying: %d %v", n, err)
	}
	if trash, _ := Trash(); trash.Files != 0 {
		t.Fatalf("trash after emptying: %+v", trash)
	}
	// A second replace finds nothing to do
	if _, err := ReplaceWithFixed(newPath); err == nil || !strings.Contains(err.Error(), "no fixed copy") {
		t.Fatalf("replacing again: %v", err)
	}
}

// The job only lists damaged videos; Fix makes the copy on request, in the background.
func TestFixOnRequest(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("no ffmpeg")
	}
	root := t.TempDir()
	library := filepath.Join(root, "Movies")
	os.MkdirAll(library, 0o755)
	saved := config.GlobalConfig
	config.GlobalConfig = &config.Config{Media: config.MediaConfig{
		Paths:      []config.MediaPath{{ID: "lib1", Name: "Movies", Path: library}},
		RepairPath: filepath.Join(root, "repairs"),
	}}
	repairIndex.mu.Lock()
	repairIndex.loaded, repairIndex.entries = false, nil
	repairIndex.mu.Unlock()
	t.Cleanup(func() {
		config.GlobalConfig = saved
		repairIndex.mu.Lock()
		repairIndex.loaded, repairIndex.entries = false, nil
		repairIndex.mu.Unlock()
	})
	_, damaged := damagedFLV(t, library)
	info, _ := os.Stat(damaged)
	key := config.CacheKey(damaged)
	repairIndex.mu.Lock()
	loadRepairIndex()
	repairIndex.entries[key] = repairEntry{Path: damaged, Size: info.Size(), ModTime: info.ModTime().Unix(), State: RepairDamaged, Reason: "test"}
	repairIndex.mu.Unlock()
	inLib := func(string) bool { return true }
	if got := DamagedVideos(inLib); len(got) != 1 || got[0].State != RepairDamaged || Playable(damaged) != damaged {
		t.Fatalf("before fixing: %+v", got)
	}
	j := &RepairJob{}
	if err := j.Fix(damaged); err != nil {
		t.Fatalf("fix: %v", err)
	}
	if err := j.Fix(damaged); err != ErrFixing {
		t.Fatalf("a second fix while the first runs: %v", err)
	}
	for i := 0; i < 100 && DamagedVideos(inLib)[0].State == RepairFixing; i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if got := DamagedVideos(inLib); got[0].State != RepairFixed || Playable(damaged) != fixedFile(key) {
		t.Fatalf("after fixing: %+v, playing %s", got, Playable(damaged))
	}
	// Its copy gone (deleted by hand), it's listed to fix again and plays as it is
	os.Remove(fixedFile(key))
	if got := DamagedVideos(inLib); got[0].State != RepairDamaged || Playable(damaged) != damaged {
		t.Fatalf("copy gone: %+v", got)
	}
}
