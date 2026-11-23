package config

import "testing"

func TestBackupDir(t *testing.T) {
	for _, c := range []struct{ users, path, want string }{
		{"./var/data/users.db", "", "var/backups"},
		{"/srv/ws/e2e/users.db", "", "/srv/ws/e2e/backups"},
		{"./var/data/users.db", "/mnt/nas/ws-backups", "/mnt/nas/ws-backups"},
	} {
		if got := (DatabaseConfig{UserPath: c.users, BackupPath: c.path}).BackupDir(); got != c.want {
			t.Errorf("%s with %q: %s, want %s", c.users, c.path, got, c.want)
		}
	}
}
