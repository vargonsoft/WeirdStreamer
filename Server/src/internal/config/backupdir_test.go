package config

import (
	"path/filepath"
	"testing"
)

func TestBackupDir(t *testing.T) {
	for _, c := range []struct{ users, path, want string }{
		{"./var/data/users.db", "", filepath.FromSlash("var/backups")},
		{"/srv/ws/e2e/users.db", "", filepath.FromSlash("/srv/ws/e2e/backups")},
		{"./var/data/users.db", "/mnt/nas/ws-backups", "/mnt/nas/ws-backups"},
	} {
		if got := (DatabaseConfig{UserPath: c.users, BackupPath: c.path}).BackupDir(); got != c.want {
			t.Errorf("%s with %q: %s, want %s", c.users, c.path, got, c.want)
		}
	}
}
