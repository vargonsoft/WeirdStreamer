package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/fileio"
	"golang.org/x/sys/windows"
)

// On Windows the data folders, the databases' folder, the backups', config.yml and the TLS key get
// owner-only ACLs; a library, and a data folder inside one, keep theirs.
func TestKeepDataPrivateWindows(t *testing.T) {
	dir := t.TempDir()
	// Open to everyone first, as a folder under C:\ProgramData would be (the temp folder is already
	// the user's only).
	everyone, _ := windows.CreateWellKnownSid(windows.WinWorldSid)
	acl, _ := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{AccessPermissions: windows.GENERIC_ALL,
		AccessMode: windows.GRANT_ACCESS, Inheritance: windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT,
		Trustee: windows.TRUSTEE{TrusteeForm: windows.TRUSTEE_IS_SID, TrusteeType: windows.TRUSTEE_IS_WELL_KNOWN_GROUP,
			TrusteeValue: windows.TrusteeValueFromSID(everyone)}}}, nil)
	if err := windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
	mk := func(p string) string {
		p = filepath.Join(dir, p)
		os.MkdirAll(p, 0o755)
		return p
	}
	file := func(p string) string {
		p = filepath.Join(dir, p)
		os.WriteFile(p, []byte("x"), 0o644)
		return p
	}
	thumbs, logs, movies, backups, data := mk("thumbnails"), mk("logs"), mk("Movies"), mk("backups"), mk("data")
	trickInLibrary := mk("Movies/.trickplay")
	users, cfg, key := file("data/users.db"), file("config.yml"), file("key.pem")
	c := &config.Config{}
	c.Database.UserPath = users
	c.Database.BackupPath = backups
	c.Server.LogsPath = logs
	c.Server.KeyFile = key
	c.Media.ThumbnailPath = thumbs
	c.Media.TrickplayPath = trickInLibrary
	c.Media.Paths = []config.MediaPath{{Path: movies}}

	keepDataPrivate(c, cfg)
	wal := file("data/users.db-wal") // made after startup, in the closed folder

	for path, want := range map[string]bool{thumbs: true, logs: true, backups: true, data: true, users: true,
		wal: true, cfg: true, key: true, movies: false, trickInLibrary: false} {
		got, err := fileio.IsOwnerOnly(path)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("%s: owner only %v, want %v", filepath.Base(path), got, want)
		}
	}
}
