package media

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSetTempDir: the folder is made, the server's old work files go, and nothing else in it is
// touched (it may be a shared folder such as /tmp).
func TestSetTempDir(t *testing.T) {
	t.Cleanup(func() { tempDir.dir = "" })
	dir := filepath.Join(t.TempDir(), "work")
	if err := SetTempDir(dir); err != nil {
		t.Fatal(err)
	}
	old, err := tempFileName("heic-*.jpg")
	if err != nil || filepath.Dir(old) != dir {
		t.Fatalf("tempFileName = %q, %v; want a file in %s", old, err, dir)
	}
	other := filepath.Join(dir, "someone-elses.txt")
	if err := os.WriteFile(other, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SetTempDir(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("the server's old work file %s is still there", old)
	}
	if _, err := os.Stat(other); err != nil {
		t.Errorf("another file in the folder was removed: %v", err)
	}
}
