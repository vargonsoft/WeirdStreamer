package media

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDiskLimitsDependOnPlayback(t *testing.T) {
	cases := []struct {
		name string
		d    diskLoad
		wait bool
	}{
		{"share, someone playing, 70 ms", diskLoad{known: true, share: true, ms: 70, contended: true}, true},
		{"share, nobody playing, 70 ms", diskLoad{known: true, share: true, ms: 70}, false},
		{"share, nobody playing, 600 ms", diskLoad{known: true, share: true, ms: 600}, true},
		{"disk, someone playing, 60%", diskLoad{known: true, use: 0.6, contended: true}, true},
		{"disk, nobody playing, 60%", diskLoad{known: true, use: 0.6}, false},
		{"disk, nobody playing, 95%", diskLoad{known: true, use: 0.95}, true},
	}
	for _, c := range cases {
		if wait, why := c.d.waitReason(); wait != c.wait {
			t.Errorf("%s: wait = %v (%q), want %v", c.name, wait, why, c.wait)
		}
	}
}

// filesOnDisk makes two files on a real disk (not /tmp, which may be in memory) and returns them
// with their device; it skips the test where the folder isn't on a whole disk /proc lists.
func filesOnDisk(t *testing.T) (a, b string, dev uint64, disk string) {
	t.Helper()
	dir, err := os.MkdirTemp(".", "disktest")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	a, b = filepath.Join(dir, "playing.mkv"), filepath.Join(dir, "frames.mkv")
	for _, f := range []string{a, b} {
		if err := os.WriteFile(f, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	dev, ok := deviceOf(a)
	if !ok {
		t.Skip("no device for the test folder")
	}
	return a, b, dev, wholeDiskOf(dev)
}

func TestLoadForSeesAStreamOnTheSameDisk(t *testing.T) {
	a, b, _, disk := filesOnDisk(t)
	if disk == "" {
		t.Skip("the test folder isn't on a block device")
	}
	old := load
	defer func() { load = old }()
	load = &LoadMonitor{ok: true, disks: map[string]float64{disk: 0.6}, shares: map[string]*shareProbe{}}

	streams := NewStreamTracker()
	if d := loadFor(b, streams); !d.known || d.share || d.name != disk || d.contended {
		t.Fatalf("nobody playing: %+v", d)
	}
	if wait, _ := loadFor(b, streams).waitReason(); wait {
		t.Error("waited at 60% with nobody playing from the disk")
	}
	streams.Begin("k", func(i *StreamInfo) { i.Path = a })
	if d := loadFor(b, streams); !d.contended {
		t.Fatalf("a stream on the same disk wasn't seen: %+v", d)
	}
	if wait, _ := loadFor(b, streams).waitReason(); !wait {
		t.Error("didn't wait at 60% with someone playing from the disk")
	}
}

func TestLoadForFindsTheShare(t *testing.T) {
	a, b, dev, _ := filesOnDisk(t)
	old := load
	defer func() { load = old }()
	// The test folder stands in for a share: its probe file is on the same device.
	load = &LoadMonitor{ok: true, disks: map[string]float64{}, shares: map[string]*shareProbe{
		"Movies": {ShareProbe: ShareProbe{Library: "Movies", Path: a}, dev: dev, ms: 100},
	}}
	streams := NewStreamTracker()
	d := loadFor(b, streams)
	if !d.share || d.name != "Movies" || d.ms != 100 {
		t.Fatalf("share not found: %+v", d)
	}
	if wait, _ := d.waitReason(); wait {
		t.Error("waited at 100 ms with nobody playing from the share")
	}
	streams.Begin("k", func(i *StreamInfo) { i.Path = a })
	if wait, why := loadFor(b, streams).waitReason(); !wait {
		t.Errorf("didn't wait at 100 ms with someone playing from the share (%q)", why)
	}
}

// Devices are remembered by folder (one entry for a folder's files), and a symlinked file by the
// folder of the file it points to.
func TestDeviceOfByFolder(t *testing.T) {
	devices.mu.Lock()
	devices.byDir = nil
	devices.mu.Unlock()
	dir, other := t.TempDir(), t.TempDir()
	for _, p := range []string{filepath.Join(dir, "a.mkv"), filepath.Join(dir, "b.mkv"), filepath.Join(other, "real.mkv")} {
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(dir, "link.mkv")
	if err := os.Symlink(filepath.Join(other, "real.mkv"), link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("can't make a symlink here (Windows needs Developer Mode or admin rights): %v", err)
		}
		t.Fatal(err)
	}
	for _, p := range []string{filepath.Join(dir, "a.mkv"), filepath.Join(dir, "b.mkv"), link} {
		if _, ok := deviceOf(p); !ok {
			t.Fatalf("no device for %s", p)
		}
	}
	if _, ok := deviceOf(filepath.Join(dir, "gone.mkv")); ok {
		t.Error("a missing file has a device")
	}
	devices.mu.Lock()
	defer devices.mu.Unlock()
	realOther, _ := filepath.EvalSymlinks(other)
	if len(devices.byDir) != 2 {
		t.Errorf("remembered %v, want the two folders", devices.byDir)
	}
	if _, ok := devices.byDir[realOther]; !ok {
		t.Errorf("the symlink's target folder %s isn't remembered: %v", realOther, devices.byDir)
	}
}
