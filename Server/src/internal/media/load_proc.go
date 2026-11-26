//go:build !windows

package media

import (
	"bufio"
	"os"
	"regexp"
	"strconv"
	"strings"
)

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
