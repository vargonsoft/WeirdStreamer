package media

import (
	"cmp"
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"slices"
	"sync"
	"time"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/fileio"
)

// LoadMonitor samples the machine's CPU and disk use (Linux /proc, Windows' disk performance
// counters: readCPU and readDisks), so background jobs run only
// with room to spare: busy above maxDisk of the busiest whole disk's time (the CPU is shown on
// the Jobs page, but the jobs run at low priority instead of waiting for it),
// smoothed over the last half-minute or so. The disks of network shares and FUSE pools (mergerfs)
// can't be told from /proc, so it also times small reads from a big file in each library on one: an idle hard disk answers in 10-20ms, a busy
// one queues them.
type LoadMonitor struct {
	mu        sync.Mutex
	cpu, disk float64            // smoothed, 0..1 (disk: the busiest whole disk)
	disks     map[string]float64 // each whole disk's use, smoothed, 0..1
	ok        bool               // readings available
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
	dev    uint64  // the device its file is on (the mount), to tell which share a file is on
	ms     float64 // smoothed; 0 before the first reading
	busy   bool    // a read is still going (a share that hangs)
	since  time.Time
	failed bool
}

// A disk or share someone is playing from holds the jobs back at the lower limits; one nobody is
// playing from only when it's close to full (the jobs run at idle disk priority anyway).
// The limits are config.yml's (jobs.wait…, the Jobs page; config.CurrentWaitLimits): a disk's busy share
// of its time and a share's read time, where a stream reads from it and where none does (relaxed). A job
// already running stops further up: a disk at four fifths of the way from its limit to full (50% stops
// at 90%, 90% at 98%), a share at twice its time or 90 ms more (60 at 150, 500 at 1000).
func maxDisk() float64                  { return float64(config.CurrentWaitLimits().DiskPercent) / 100 }
func relaxedDisk() float64              { return float64(config.CurrentWaitLimits().DiskIdlePercent) / 100 }
func stopDisk(limit float64) float64    { return limit + (1-limit)*0.8 }
func maxShareMs() float64               { return float64(config.CurrentWaitLimits().ShareMs) }
func relaxedShareMs() float64           { return float64(config.CurrentWaitLimits().ShareIdleMs) }
func stopShareMs(limit float64) float64 { return max(2*limit, limit+90) }

const (
	loadSample = 5 * time.Second

	probeReads = 3 // reads per sample; their median counts
	probeSize  = 64 << 10
	probeEvery = 10 * time.Minute // how often the files to read are chosen again
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
		if dev, ok := deviceOf(pr.Path); ok {
			keep[pr.Library].dev = dev
		}
	}
	m.shares = keep
}

// shareFor returns the read time (ms) of the share on device dev, and its library.
func (m *LoadMonitor) shareFor(dev uint64) (ms float64, library string, ok bool) {
	if m == nil {
		return 0, "", false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.shares {
		if p.dev == dev && p.ms > 0 {
			return p.ms, p.Library, true
		}
	}
	return 0, "", false
}

// diskUse returns a whole disk's smoothed use (0..1).
func (m *LoadMonitor) diskUse(name string) (float64, bool) {
	if m == nil {
		return 0, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	use, ok := m.disks[name]
	return use, ok
}

// timeReads reads probeSize bytes at probeReads random places in a file, dropping them from the
// page cache after so a later read goes to the disk again, and returns the median time (ms).
func timeReads(path string) (float64, error) {
	f, err := fileio.Open(path)
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
		dropCache(int(f.Fd()), at, probeSize)
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

// IsTimedMount says whether a folder's load is judged by timing reads from it: on an SMB or NFS
// share, or a FUSE file system such as a mergerfs pool (see isTimedFS).
func IsTimedMount(dir string) bool { return isTimedFS(dir) }

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
			uses := map[string]float64{}
			for name, ms := range disks {
				if prev, seen := disks0[name]; seen && elapsed > 0 {
					uses[name] = min(float64(ms-prev)/elapsed, 1)
					disk = max(disk, uses[name])
				}
			}
			m.mu.Lock()
			if m.ok {
				m.cpu, m.disk = 0.7*m.cpu+0.3*cpu, 0.7*m.disk+0.3*disk
				for name, use := range uses {
					if prev, seen := m.disks[name]; seen {
						uses[name] = 0.7*prev + 0.3*use
					}
				}
			} else {
				m.cpu, m.disk, m.ok = cpu, disk, true
			}
			m.disks = uses
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
