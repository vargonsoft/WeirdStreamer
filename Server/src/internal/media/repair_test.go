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
	"github.com/weirdstreamer/server/internal/database"
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

// Replacing a damaged video and restoring it from the trash: the files, the progress and the
// repair index go back as they were, and the original is listed as fixed again.
func TestReplaceThenRestore(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("no ffmpeg")
	}
	root := t.TempDir()
	library := filepath.Join(root, "Movies")
	os.MkdirAll(filepath.Join(library, "Old"), 0o755)
	saved := config.GlobalConfig
	config.GlobalConfig = &config.Config{Media: config.MediaConfig{
		Paths:      []config.MediaPath{{ID: "lib1", Name: "Movies", Path: library}},
		RepairPath: filepath.Join(root, "repairs"), TrashPath: filepath.Join(root, "trash"),
		ThumbnailPath: filepath.Join(root, "thumbnails"),
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
	db, err := database.New(filepath.Join(root, "users.db"), filepath.Join(root, "media.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, made := damagedFLV(t, root)
	damaged := filepath.Join(library, "Old", "Damaged Video.avi")
	if err := os.Rename(made, damaged); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(damaged)
	key := config.CacheKey(damaged)
	size, length, err := fixVideo(context.Background(), damaged, key)
	if err != nil {
		t.Fatalf("fixing: %v", err)
	}
	repairIndex.mu.Lock()
	loadRepairIndex()
	repairIndex.entries[key] = repairEntry{Path: damaged, Size: info.Size(), ModTime: info.ModTime().Unix(),
		State: RepairFixed, Reason: "test reason", FixedSize: size, Duration: length}
	repairIndex.mu.Unlock()
	thumb, _ := GetThumbnailPath(config.GlobalConfig.Media.ThumbnailPath, key, ".jpg")
	os.MkdirAll(filepath.Dir(thumb), 0o755)
	os.WriteFile(thumb, []byte("a thumbnail"), 0o644)
	const user = 1
	oldClient, newClient := "lib1/Old/Damaged Video.avi", "lib1/Old/Damaged Video.mkv"
	if err := db.SavePlayback(user, oldClient, database.PlaybackUpdate{Position: 120, Duration: 300, UpdatedAt: time.Now()}, false); err != nil {
		t.Fatal(err)
	}
	position := func(path string) float64 {
		pb, err := db.UserPlayback(user)
		if err != nil {
			t.Fatal(err)
		}
		if p := pb[path]; p != nil {
			return p.Position
		}
		return -1
	}

	// Replaced (as the handler does: the file, then its progress)
	newPath, err := ReplaceWithFixed(damaged)
	if err != nil {
		t.Fatalf("replacing: %v", err)
	}
	if err := db.MoveFile(damaged, oldClient, newClient); err != nil {
		t.Fatal(err)
	}
	if position(newClient) != 120 || position(oldClient) != -1 {
		t.Fatalf("progress after replacing: %v / %v", position(newClient), position(oldClient))
	}
	os.Remove(thumb) // the Cleanup job deletes the thumbnail of a file gone from the libraries
	items, err := TrashItems()
	if err != nil || len(items) != 1 {
		t.Fatalf("trash: %+v %v", items, err)
	}
	it := items[0]
	if it.Name != "Damaged Video.avi" || it.Path != damaged || it.Replacement != newPath || it.Reason != "test reason" ||
		it.Size != info.Size() || it.Thumbnail == "" || it.Deletes().Sub(it.At) != TrashDays*24*time.Hour {
		t.Fatalf("trash item: %+v", it)
	}
	if got, _ := Trash(); got.Files != 1 || got.Bytes != info.Size() {
		t.Fatalf("trash counts its notes: %+v", got)
	}

	// Refused while a file is in its place
	os.WriteFile(damaged, []byte("another"), 0o644)
	if _, err := RestoreFromTrash(it.ID); err != ErrPathTaken {
		t.Fatalf("restoring over another file: %v", err)
	}
	os.Remove(damaged)
	if _, err := RestoreFromTrash("../x"); err != ErrNoTrashItem {
		t.Fatalf("restoring a bad ID: %v", err)
	}

	// Restored: the original back (its time kept), the replacement its fixed copy again
	restored, err := RestoreFromTrash(it.ID)
	if err != nil {
		t.Fatalf("restoring: %v", err)
	}
	if err := db.MoveFile(restored.Replacement, newClient, oldClient); err != nil {
		t.Fatal(err)
	}
	if got, err := os.Stat(damaged); err != nil || got.Size() != info.Size() || got.ModTime().Unix() != info.ModTime().Unix() {
		t.Fatalf("the original back: %v", err)
	}
	if fileExists(newPath) {
		t.Fatal("the replacement is still in the library")
	}
	if got, err := os.Stat(fixedFile(key)); err != nil || got.Size() != size {
		t.Fatalf("the fixed copy: %v", err)
	}
	if position(oldClient) != 120 || position(newClient) != -1 {
		t.Fatalf("progress after restoring: %v / %v", position(oldClient), position(newClient))
	}
	if b, err := os.ReadFile(thumb); err != nil || string(b) != "a thumbnail" {
		t.Fatalf("thumbnail back: %v", err)
	}
	if items, _ := TrashItems(); len(items) != 0 {
		t.Fatalf("still in the trash: %+v", items)
	}
	got := DamagedVideos(func(string) bool { return true })
	if len(got) != 1 || got[0].State != RepairFixed || got[0].Reason != "test reason" || got[0].FixedSize != size ||
		Playable(damaged) != fixedFile(key) {
		t.Fatalf("listed after restoring: %+v", got)
	}
	// And it can be replaced again
	if _, err := ReplaceWithFixed(damaged); err != nil {
		t.Fatalf("replacing again: %v", err)
	}

	// A file in the trash with no note (or whose replacement changed) isn't restored
	items, _ = TrashItems()
	os.WriteFile(newPath, []byte("changed"), 0o644)
	if _, err := RestoreFromTrash(items[0].ID); err != ErrReplacementChanged {
		t.Fatalf("restoring with the replacement changed: %v", err)
	}
	os.Remove(filepath.Join(trashDir(), items[0].ID, trashNoteFile))
	if _, err := RestoreFromTrash(items[0].ID); err != ErrNotRestorable {
		t.Fatalf("restoring with no note: %v", err)
	}
}
