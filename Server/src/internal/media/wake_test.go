package media

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStatWaking(t *testing.T) {
	dir := t.TempDir()
	if _, err := StatWaking(dir); err != nil {
		t.Fatalf("a readable folder: %v", err)
	}
	// A missing folder isn't asleep: no wake-up, and the error comes back as it is.
	missing := filepath.Join(dir, "gone")
	if _, err := StatWaking(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a missing folder: %v", err)
	}
	if Wake(missing, os.ErrNotExist) {
		t.Fatal("woke a missing folder")
	}

	// A refused folder that stays refused: Wake lists the folders above it up to / and gives up,
	// then waits wakeEvery before trying again.
	if os.Geteuid() == 0 {
		t.Skip("root reads everything")
	}
	skipOnWindows(t, "a mode of 0 doesn't refuse a folder on Windows")
	locked := filepath.Join(dir, "locked")
	inner := filepath.Join(locked, "inner")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	os.Chmod(locked, 0)
	defer os.Chmod(locked, 0o755)
	_, err := StatWaking(inner)
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("a refused folder: %v", err)
	}
	if Wake(inner, err) {
		t.Fatal("tried again within wakeEvery")
	}
}
