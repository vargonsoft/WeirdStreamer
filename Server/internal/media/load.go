package media

import (
	"bufio"
	"cmp"
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

// LoadMonitor samples the machine's CPU and disk use (Linux /proc), so background jobs run only
// with room to spare: busy above maxCPU of all cores or maxDisk of the busiest whole disk's time,
// smoothed over the last half-minute or so. Network shares' disks don't show in /proc, so it also
// times small reads from a big file on each share: an idle hard disk answers in 10-20ms, a busy
// one queues them.
type LoadMonitor struct {
	mu        sync.Mutex
	cpu, disk float64 // smoothed, 0..1
	ok        bool    // readings available
	shares    map[string]*shareProbe
	probes    func() []ShareProbe
}

// ShareProbe is a file to time reads from for a library on a network share.
type ShareProbe struct {
	Library, Path string
}

// ShareLoad is a share's read time, smoothed (ms).
type ShareLoad struct {
	Library string  `json:"library"`
	Ms      float64 `json:"ms"`
}

type shareProbe struct {
	ShareProbe
	ms     float64 // smoothed; 0 before the first reading
	busy   bool    // a read is still going (a share that hangs)
	since  time.Time
	failed bool
}

const (
	maxCPU     = 0.60
	maxDisk    = 0.50
	stopCPU    = 0.85 // a job already running stops above this (it adds load of its own)
	loadSample = 5 * time.Second

	maxShareMs  = 60  // a share's reads take this long when its disk is busy
	stopShareMs = 150 // a job already running stops above this
	probeReads  = 3   // reads per sample; their median counts
	probeSize   = 64 << 10
	probeEvery  = 10 * time.Minute // how often the files to read are chosen again
)

// NewLoadMonitor starts sampling until ctx ends. probes (when given) names a big file for each
// library on a network share.
func NewLoadMonitor(ctx context.Context, probes func() []ShareProbe) *LoadMonitor {
	m := &LoadMonitor{shares: map[string]*shareProbe{}, probes: probes}
	go m.run(ctx)
	if probes != nil {
		go m.runShares(ctx)
	}
	return m
}

// runShares times reads from each share every loadSample, choosing the files again every
// probeEvery (or when one can't be read).
func (m *LoadMonitor) runShares(ctx context.Context) {
	var chosen time.Time
	for {
		if time.Since(chosen) > probeEvery {
			m.chooseProbes()
			chosen = time.Now()
		}
		m.mu.Lock()
		var due []*shareProbe
		for _, p := range m.shares {
			if p.failed && time.Since(chosen) > time.Minute {
				chosen = time.Time{} // choose again next time (its file may be gone)
			}
			if !p.busy {
				p.busy, p.since = true, time.Now()
				due = append(due, p)
			} else if waited := float64(time.Since(p.since).Milliseconds()); waited > p.ms {
				p.ms = waited // a read that hasn't come back is at least this slow
			}
		}
		m.mu.Unlock()
		for _, p := range due {
			go func() {
				ms, err := timeReads(p.Path)
				m.mu.Lock()
				defer m.mu.Unlock()
				p.busy, p.failed = false, err != nil
				if err != nil {
					return
				}
				if p.ms == 0 {
					p.ms = ms
				} else {
					p.ms = 0.7*p.ms + 0.3*ms
				}
			}()
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(loadSample):
		}
	}
}

// chooseProbes takes the probe files now named, keeping the readings of libraries that stay.
func (m *LoadMonitor) chooseProbes() {
	probes := m.probes()
	m.mu.Lock()
	defer m.mu.Unlock()
	keep := map[string]*shareProbe{}
	for _, pr := range probes {
		if p, ok := m.shares[pr.Library]; ok && p.Path == pr.Path {
			keep[pr.Library] = p
		} else if ok && p.busy {
			keep[pr.Library] = p // its read is still going; its goroutine still updates it
			p.Path = pr.Path
		} else {
			keep[pr.Library] = &shareProbe{ShareProbe: pr}
		}
	}
	m.shares = keep
}

// timeReads reads probeSize bytes at probeReads random places in a file, dropping them from the
// page cache after so a later read goes to the disk again, and returns the median time (ms).
func timeReads(path string) (float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return 0, err
	}
	if st.Size() < 2*probeSize {
		return 0, errors.New("too small to time")
	}
	buf := make([]byte, probeSize)
	times := make([]float64, 0, probeReads)
	for range probeReads {
		at := rand.Int64N(st.Size()-probeSize) &^ 4095
		start := time.Now()
		if _, err := f.ReadAt(buf, at); err != nil {
			return 0, err
		}
		times = append(times, float64(time.Since(start).Microseconds())/1000)
		unix.Fadvise(int(f.Fd()), at, probeSize, unix.FADV_DONTNEED)
	}
	slices.Sort(times)
	return times[len(times)/2], nil
}

// Shares returns each network share's smoothed read time, slowest first (none before a reading).
func (m *LoadMonitor) Shares() []ShareLoad {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []ShareLoad
	for _, p := range m.shares {
		if p.ms > 0 {
			out = append(out, ShareLoad{Library: p.Library, Ms: math.Round(p.ms*10) / 10})
		}
	}
	slices.SortFunc(out, func(a, b ShareLoad) int { return cmp.Compare(b.Ms, a.Ms) })
	return out
}

// IsNetworkShare says whether a folder is on an SMB or NFS mount.
func IsNetworkShare(dir string) bool {
	var st unix.Statfs_t
	if unix.Statfs(dir, &st) != nil {
		return false
	}
	switch uint32(st.Type) {
	case unix.SMB2_SUPER_MAGIC, unix.CIFS_SUPER_MAGIC, unix.NFS_SUPER_MAGIC, unix.SMB_SUPER_MAGIC:
		return true
	}
	return false
}

func (m *LoadMonitor) run(ctx context.Context) {
	busy0, total0, err1 := readCPU()
	disks0, err2 := readDisks()
	at0 := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(loadSample):
		}
		busy, total, errC := readCPU()
		disks, errD := readDisks()
		now := time.Now()
		if err1 == nil && errC == nil && err2 == nil && errD == nil && total > total0 {
			cpu := float64(busy-busy0) / float64(total-total0)
			elapsed := float64(now.Sub(at0).Milliseconds())
			disk := 0.0
			for name, ms := range disks {
				if prev, seen := disks0[name]; seen && elapsed > 0 {
					disk = max(disk, min(float64(ms-prev)/elapsed, 1))
				}
			}
			m.mu.Lock()
			if m.ok {
				m.cpu, m.disk = 0.7*m.cpu+0.3*cpu, 0.7*m.disk+0.3*disk
			} else {
				m.cpu, m.disk, m.ok = cpu, disk, true
			}
			m.mu.Unlock()
		}
		busy0, total0, err1, disks0, err2, at0 = busy, total, errC, disks, errD, now
	}
}

// Readings returns the smoothed CPU and disk use, and whether there are any.
func (m *LoadMonitor) Readings() (cpu, disk float64, ok bool) {
	if m == nil {
		return 0, 0, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cpu, m.disk, m.ok
}

// readCPU returns the busy and total jiffies of all CPUs.
func readCPU() (busy, total uint64, err error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	line, err := bufio.NewReader(f).ReadString('\n')
	if err != nil {
		return 0, 0, err
	}
	fields := strings.Fields(line) // cpu user nice system idle iowait irq softirq steal ...
	for i, field := range fields[1:] {
		v, _ := strconv.ParseUint(field, 10, 64)
		total += v
		if i != 3 && i != 4 { // idle and iowait aren't busy
			busy += v
		}
	}
	return busy, total, nil
}

// Whole disks (not partitions, loop or RAM devices): sda, nvme0n1, mmcblk0, md0, vda...
var wholeDisk = regexp.MustCompile(`^(sd[a-z]+|vd[a-z]+|xvd[a-z]+|hd[a-z]+|nvme\d+n\d+|mmcblk\d+|md\d+)$`)

// readDisks returns each whole disk's milliseconds spent doing I/O.
func readDisks() (map[string]uint64, error) {
	b, err := os.ReadFile("/proc/diskstats")
	if err != nil {
		return nil, err
	}
	out := map[string]uint64{}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 13 || !wholeDisk.MatchString(f[2]) {
			continue
		}
		ms, _ := strconv.ParseUint(f[12], 10, 64)
		out[f[2]] = ms
	}
	return out, nil
}
