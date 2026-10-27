package media

import (
	"bufio"
	"context"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// LoadMonitor samples the machine's CPU and disk use (Linux /proc), so background jobs run only
// with room to spare: busy above maxCPU of all cores or maxDisk of the busiest whole disk's time,
// smoothed over the last half-minute or so.
type LoadMonitor struct {
	mu        sync.Mutex
	cpu, disk float64 // smoothed, 0..1
	ok        bool    // readings available
}

const (
	maxCPU     = 0.60
	maxDisk    = 0.50
	stopCPU    = 0.85 // a job already running stops above this (it adds load of its own)
	loadSample = 5 * time.Second
)

// NewLoadMonitor starts sampling until ctx ends.
func NewLoadMonitor(ctx context.Context) *LoadMonitor {
	m := &LoadMonitor{}
	go m.run(ctx)
	return m
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
