// Package backup copies the server's databases and config.yml into one .tar.gz in the backup
// folder (database.backupPath), on the schedule Settings sets (database.backupSchedule, Time,
// Day) and on demand, keeping the newest backupKeep, and restores one (cli backup restore).
//
// A backup holds users.db and media.db (consistent copies, made while the server runs: SQLite's
// VACUUM INTO), config.yml and manifest.json (the server's version and when). It's readable by
// the server's account only: it holds the password hashes and the config's secrets.
package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/fileio"
	"github.com/weirdstreamer/server/internal/jobs"
)

const (
	prefix     = "weirdstreamer-backup-"
	suffix     = ".tar.gz"
	timeLayout = "20060102-150405"
)

// Info is one backup in the folder.
type Info struct {
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	Created time.Time `json:"created"`
}

// Manifest says what made a backup and when.
type Manifest struct {
	Version string    `json:"version"`
	Created time.Time `json:"created"`
	Server  string    `json:"server,omitempty"`
}

// Dir is the backup folder.
func Dir() string { return config.GlobalConfig.Database.BackupDir() }

// creating keeps two backups from being made at once (the schedule and Back up now).
var creating sync.Mutex

// Create makes a backup now, then removes the oldest past backupKeep.
func Create(db *database.DB, version string) (Info, error) {
	creating.Lock()
	defer creating.Unlock()
	dir := Dir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Info{}, err
	}
	if err := fileio.OwnerOnly(dir); err != nil { // Windows: the mode doesn't keep others out
		return Info{}, err
	}
	work, err := os.MkdirTemp(dir, ".making-")
	if err != nil {
		return Info{}, err
	}
	defer os.RemoveAll(work)
	if err := db.CopyTo(work); err != nil {
		return Info{}, err
	}
	now := time.Now()
	manifest, _ := json.MarshalIndent(Manifest{Version: version, Created: now, Server: config.GlobalConfig.Server.Name}, "", "  ")
	if err := os.WriteFile(filepath.Join(work, "manifest.json"), manifest, 0o600); err != nil {
		return Info{}, err
	}
	files := []string{"manifest.json", "users.db", "media.db"}
	if cfg, err := fileio.ReadFile(config.Path()); err == nil {
		if err := os.WriteFile(filepath.Join(work, "config.yml"), cfg, 0o600); err != nil {
			return Info{}, err
		}
		files = append(files, "config.yml")
	}

	// Two in the same second (Back up now twice) get -2, -3… rather than writing over the first.
	name := prefix + now.Format(timeLayout) + suffix
	for n := 2; ; n++ {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			break
		}
		name = fmt.Sprintf("%s%s-%d%s", prefix, now.Format(timeLayout), n, suffix)
	}
	part := filepath.Join(dir, name+".part")
	if err := writeArchive(part, work, files); err != nil {
		os.Remove(part)
		return Info{}, err
	}
	if err := fileio.Rename(part, filepath.Join(dir, name)); err != nil {
		os.Remove(part)
		return Info{}, err
	}
	prune(dir, config.GlobalConfig.Database.BackupKeep)
	st, _ := os.Stat(filepath.Join(dir, name))
	info := Info{Name: name, Created: now}
	if st != nil {
		info.Size = st.Size()
	}
	return info, nil
}

func writeArchive(path, dir string, files []string) error {
	f, err := fileio.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, name := range files {
		if err := addFile(tw, filepath.Join(dir, name), name); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	return f.Close()
}

func addFile(tw *tar.Writer, path, name string) error {
	src, err := fileio.Open(path)
	if err != nil {
		return err
	}
	defer src.Close()
	st, err := src.Stat()
	if err != nil {
		return err
	}
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: st.Size(), ModTime: st.ModTime()}); err != nil {
		return err
	}
	_, err = io.Copy(tw, src)
	return err
}

// List is the backups in dir, newest first.
func List(dir string) []Info {
	entries, _ := os.ReadDir(dir)
	var list []Info
	for _, e := range entries {
		created, ok := backupTime(e.Name())
		if !ok || e.IsDir() {
			continue
		}
		info := Info{Name: e.Name(), Created: created}
		if st, err := e.Info(); err == nil {
			info.Size = st.Size()
		}
		list = append(list, info)
	}
	sort.Slice(list, func(i, j int) bool { // newest first; in one second, the later name (-2 after none)
		if !list[i].Created.Equal(list[j].Created) {
			return list[i].Created.After(list[j].Created)
		}
		return len(list[i].Name) > len(list[j].Name) || len(list[i].Name) == len(list[j].Name) && list[i].Name > list[j].Name
	})
	return list
}

// backupTime reads when a backup was made from its name (one of ours only).
func backupTime(name string) (time.Time, bool) {
	if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) {
		return time.Time{}, false
	}
	stamp := strings.TrimSuffix(strings.TrimPrefix(name, prefix), suffix)
	if len(stamp) > len(timeLayout) && stamp[len(timeLayout)] == '-' { // a second one that second: -2
		stamp = stamp[:len(timeLayout)]
	}
	t, err := time.ParseInLocation(timeLayout, stamp, time.Local)
	return t, err == nil
}

// prune removes the oldest backups past keep (only the folder's backups: nothing else in it).
func prune(dir string, keep int) {
	if keep < 1 {
		keep = config.DefaultBackupKeep
	}
	for i, b := range List(dir) {
		if i >= keep {
			if err := os.Remove(filepath.Join(dir, b.Name)); err == nil {
				log.Printf("[BACKUP] Removed the old backup %s", b.Name)
			}
		}
	}
}

// Next is when the schedule makes a backup after since (zero time when off).
func Next(since time.Time, schedule, at, day string) time.Time {
	if schedule == "off" {
		return time.Time{}
	}
	clock, err := time.Parse("15:04", at)
	if err != nil {
		clock, _ = time.Parse("15:04", config.DefaultBackupTime)
	}
	next := time.Date(since.Year(), since.Month(), since.Day(), clock.Hour(), clock.Minute(), 0, 0, since.Location())
	for !next.After(since) {
		next = next.AddDate(0, 0, 1)
	}
	if schedule == "weekly" {
		want := weekday(day)
		for next.Weekday() != want {
			next = next.AddDate(0, 0, 1)
		}
	}
	return next
}

func weekday(day string) time.Weekday {
	for i, d := range config.BackupDays {
		if d == day {
			return time.Weekday(i)
		}
	}
	return time.Sunday
}

// scheduler runs the backups on the schedule: after the newest backup (or, with none, after the
// server started), at the next time the schedule names. Settings' changes apply at once.
type scheduler struct {
	started time.Time
	db      *database.DB
	version string
	job     *jobs.Job
	wake    chan struct{}
}

var current *scheduler

// Start registers the Backups job (the Jobs page) and runs the schedule until ctx ends.
func Start(ctx context.Context, db *database.DB, version string) {
	s := &scheduler{started: time.Now(), db: db, version: version, wake: make(chan struct{}, 1),
		job: jobs.Register("backup", "Backups", "Copies both databases and config.yml into the backup folder, on the schedule Settings sets (Server › Backups), keeping the newest.")}
	s.job.OnRunNow(func() { s.poke(true) })
	current = s
	go s.run(ctx)
}

// NextTime is when the next scheduled backup is (zero when backups are off).
func NextTime() time.Time {
	if current == nil {
		return time.Time{}
	}
	return current.next()
}

// next is the schedule's next time after the newest backup, or with none after the server started.
func (s *scheduler) next() time.Time {
	d := config.GlobalConfig.Database
	since := s.started
	if list := List(d.BackupDir()); len(list) > 0 && list[0].Created.After(since) {
		since = list[0].Created
	}
	return Next(since, d.BackupSchedule, d.BackupTime, d.BackupDay)
}

// Changed has the schedule look again at once (Settings saved it).
func Changed() {
	if current != nil {
		current.poke(false)
	}
}

var runNow = make(chan struct{}, 1)

func (s *scheduler) poke(now bool) {
	if now {
		select {
		case runNow <- struct{}{}:
		default:
		}
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *scheduler) run(ctx context.Context) {
	for {
		next := s.next()
		forced := false
		select {
		case <-runNow:
			forced = true
		default:
		}
		if forced || !next.IsZero() && !time.Now().Before(next) {
			s.backup()
			continue
		}
		wait := time.Minute
		if next.IsZero() {
			s.job.Schedule("Off (Settings › Server › Backups)")
		} else {
			s.job.Schedule("")
			s.job.NextIn(time.Until(next))
			wait = min(wait, time.Until(next))
		}
		select {
		case <-ctx.Done():
			return
		case <-s.wake:
		case <-time.After(wait):
		}
	}
}

func (s *scheduler) backup() (Info, error) {
	s.job.Begin(0)
	start := time.Now()
	info, err := Create(s.db, s.version)
	if err != nil {
		log.Printf("[BACKUP] Failed: %v", err)
		s.job.End("Failed: " + err.Error())
		return info, err
	}
	log.Printf("[BACKUP] Made %s (%s) in %s", info.Name, Size(info.Size), time.Since(start).Round(100*time.Millisecond))
	s.job.End(fmt.Sprintf("Made %s (%s)", info.Name, Size(info.Size)))
	return info, nil
}

// RunNow makes a backup now, as the schedule would (Settings' Back up now), and returns it.
func RunNow() (Info, error) {
	if current == nil {
		return Info{}, fmt.Errorf("backups aren't running")
	}
	info, err := current.backup()
	current.poke(false) // the next scheduled one counts from this one
	return info, err
}

// Size says a backup's size for people ("42 MB").
func Size(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.0f MB", float64(n)/(1<<20))
	default:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
}

// Resolve finds a backup by its file name in the backup folder, or by its path.
func Resolve(name string) string {
	if _, err := os.Stat(name); err == nil || strings.ContainsRune(name, filepath.Separator) {
		return name
	}
	return filepath.Join(Dir(), name)
}

// Restore puts a backup's databases in place of the current ones; the server must be stopped.
// The current files (with their WAL) go first to before-restore-<time>/ beside the users
// database, which it returns. withConfig puts the backup's config.yml in place of the current one
// too (kept there as well): for a new machine; on the same one the current config usually stays.
func Restore(file string, withConfig bool) (string, error) {
	d := config.GlobalConfig.Database
	dataDir := filepath.Dir(d.UserPath)
	work, err := os.MkdirTemp(dataDir, ".restoring-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(work)
	got, err := extract(file, work)
	if err != nil {
		return "", err
	}
	if !got["users.db"] || !got["media.db"] {
		return "", fmt.Errorf("%s isn't a backup: it has no users.db and media.db", filepath.Base(file))
	}
	if withConfig && !got["config.yml"] {
		return "", fmt.Errorf("%s has no config.yml", filepath.Base(file))
	}

	aside := filepath.Join(dataDir, "before-restore-"+time.Now().Format(timeLayout))
	if err := os.MkdirAll(aside, 0o700); err != nil {
		return "", err
	}
	put := func(from, to string) error {
		// The current file (and a database's WAL and shared memory) aside, then the backup's.
		for _, ext := range []string{"", "-wal", "-shm"} {
			if _, err := os.Stat(to + ext); err == nil {
				if err := moveFile(to+ext, filepath.Join(aside, filepath.Base(to)+ext)); err != nil {
					return err
				}
			}
		}
		return moveFile(from, to)
	}
	if err := put(filepath.Join(work, "users.db"), d.UserPath); err != nil {
		return aside, err
	}
	if err := put(filepath.Join(work, "media.db"), d.MediaPath); err != nil {
		return aside, err
	}
	if withConfig {
		if err := put(filepath.Join(work, "config.yml"), config.Path()); err != nil {
			return aside, err
		}
	}
	return aside, nil
}

// extract unpacks a backup's known files (nothing else, and no paths) into dir.
func extract(file, dir string) (map[string]bool, error) {
	f, err := fileio.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("%s isn't a backup: %w", filepath.Base(file), err)
	}
	tr := tar.NewReader(gz)
	got := map[string]bool{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return got, nil
		}
		if err != nil {
			return nil, err
		}
		switch h.Name {
		case "users.db", "media.db", "config.yml", "manifest.json":
		default:
			continue
		}
		out, err := fileio.OpenFile(filepath.Join(dir, h.Name), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return nil, err
		}
		_, err = io.Copy(out, tr)
		out.Close()
		if err != nil {
			return nil, err
		}
		got[h.Name] = true
	}
}

// ReadManifest is what a backup says about itself, read from the archive as it is.
func ReadManifest(file string) (Manifest, error) {
	var m Manifest
	f, err := fileio.Open(file)
	if err != nil {
		return m, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return m, err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err != nil {
			return m, fmt.Errorf("no manifest.json: %w", err)
		}
		if h.Name == "manifest.json" {
			return m, json.NewDecoder(tr).Decode(&m)
		}
	}
}

// moveFile renames, or copies across disks; the copy is the owner's only. Any other failure (on
// Windows, a file another program has open) is returned as it is: a copy wouldn't remove it either.
func moveFile(from, to string) error {
	if err := fileio.Rename(from, to); err == nil || !fileio.CrossDevice(err) {
		return err
	}
	src, err := fileio.Open(from)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := fileio.OpenFile(to, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return err
	}
	if err := dst.Close(); err != nil {
		return err
	}
	return os.Remove(from)
}
