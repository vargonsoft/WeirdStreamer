package media

import (
	"os"
	"path/filepath"
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
