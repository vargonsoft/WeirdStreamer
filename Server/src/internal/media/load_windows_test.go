package media

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The CPU and disk counters read, a local folder maps to a disk they list, and reading from the
// disk shows up as busy time.
func TestWindowsLoadReadings(t *testing.T) {
	busy, total, err := readCPU()
	if err != nil || total == 0 || busy > total {
		t.Fatalf("CPU: busy %d of %d, %v", busy, total, err)
	}
	disks, err := readDisks()
	if err != nil || len(disks) == 0 {
		t.Fatalf("disks: %v, %v", disks, err)
	}
	dir := t.TempDir()
	dev, ok := folderDevice(dir)
	if !ok {
		t.Fatal("no volume for the temp folder")
	}
	disk := wholeDiskOf(dev)
	if _, listed := disks[disk]; !listed {
		t.Fatalf("the temp folder's disk %q isn't among %v", disk, disks)
	}
	if isTimedFS(dir) {
		t.Error("the temp folder counts as a network share")
	}
	if other, _ := folderDevice(filepath.Join(dir, ".")); other != dev {
		t.Error("the same folder gave another volume")
	}
	// A share (the machine's own admin share, where it's on): timed, and on no disk of its own.
	if share := `\\localhost\C$\Windows`; func() bool { _, err := os.Stat(share); return err == nil }() {
		if !isTimedFS(share) {
			t.Errorf("%s isn't counted as a network share", share)
		}
		if sdev, ok := folderDevice(share); !ok || sdev == dev || wholeDiskOf(sdev) != "" {
			t.Errorf("%s: device %d (%v), disk %q; want its own device and no disk", share, sdev, ok, wholeDiskOf(sdev))
		}
	} else {
		t.Logf("no admin share to try a network path on")
	}

	// Writing through to the disk keeps it busy for a while.
	before, _ := readDisks()
	start := time.Now()
	f, err := os.OpenFile(filepath.Join(dir, "load"), os.O_CREATE|os.O_WRONLY|os.O_SYNC, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 1<<20)
	for time.Since(start) < 2*time.Second {
		f.Write(buf)
	}
	f.Close()
	after, _ := readDisks()
	elapsed := float64(time.Since(start).Milliseconds())
	use := float64(after[disk]-before[disk]) / elapsed
	t.Logf("disk %s: %.0f%% busy while writing", disk, use*100)
	if use < 0.05 {
		t.Errorf("disk %s %.1f%% busy while writing through to it", disk, use*100)
	}
}
