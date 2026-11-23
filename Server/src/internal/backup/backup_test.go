package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
)

func TestNext(t *testing.T) {
	at := func(s string) time.Time {
		v, err := time.ParseInLocation("2006-01-02 15:04 Mon", s, time.Local)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	for _, c := range []struct{ since, schedule, clock, day, want string }{
		{"2026-11-23 03:00 Mon", "daily", "04:00", "sunday", "2026-11-23 04:00 Mon"},
		{"2026-11-23 04:00 Mon", "daily", "04:00", "sunday", "2026-11-24 04:00 Tue"},
		{"2026-11-23 18:40 Mon", "weekly", "02:30", "wednesday", "2026-11-25 02:30 Wed"},
		{"2026-11-25 02:30 Wed", "weekly", "02:30", "wednesday", "2026-12-02 02:30 Wed"},
	} {
		if got := Next(at(c.since), c.schedule, c.clock, c.day); !got.Equal(at(c.want)) {
			t.Errorf("%s after %s: %s, want %s", c.schedule, c.since, got.Format("2006-01-02 15:04 Mon"), c.want)
		}
	}
	if !Next(time.Now(), "off", "04:00", "sunday").IsZero() {
		t.Error("off has a next time")
	}
}

// A backup, then a change, then a restore: the change is undone, and what it replaced kept.
func TestCreateAndRestore(t *testing.T) {
	saved := config.GlobalConfig
	defer func() { config.GlobalConfig = saved }()
	root := t.TempDir()
	config.GlobalConfig = &config.Config{Database: config.DatabaseConfig{
		UserPath: filepath.Join(root, "data", "users.db"), MediaPath: filepath.Join(root, "data", "media.db"), BackupKeep: 2}}
	os.MkdirAll(filepath.Join(root, "data"), 0o700)
	db, err := database.New(config.GlobalConfig.Database.UserPath, config.GlobalConfig.Database.MediaPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateUser("alice", "a-password-1", false, "parents"); err != nil {
		t.Fatal(err)
	}
	var made []Info
	for range 3 { // three in a second: three names, the oldest pruned (keep 2)
		info, err := Create(db, "test")
		if err != nil {
			t.Fatal(err)
		}
		made = append(made, info)
	}
	list := List(Dir())
	if len(list) != 2 || list[0].Name != made[2].Name || list[1].Name != made[1].Name {
		t.Fatalf("kept %v, want the newest two of %v", list, made)
	}
	if st, _ := os.Stat(filepath.Join(Dir(), list[0].Name)); st == nil || st.Mode().Perm() != 0o600 {
		t.Errorf("backup's mode: %v, want 0600", st)
	}
	if m, err := ReadManifest(filepath.Join(Dir(), list[0].Name)); err != nil || m.Version != "test" {
		t.Errorf("manifest %+v, %v", m, err)
	}
	if err := db.DeleteUser("alice"); err != nil {
		t.Fatal(err)
	}
	db.Close()

	aside, err := Restore(Resolve(list[0].Name), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(aside, "users.db")); err != nil {
		t.Errorf("the replaced users.db isn't kept: %v", err)
	}
	db, err = database.New(config.GlobalConfig.Database.UserPath, config.GlobalConfig.Database.MediaPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.GetUserByUsername("alice"); err != nil {
		t.Errorf("alice isn't back: %v", err)
	}
}
