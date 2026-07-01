package collector

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// SystemLoad returns the 1-minute load average normalized by CPU count.
// A value of 1.0 means 100% load across all cores.
func SystemLoad() (float64, error) {
	raw, err := loadAvg1m()
	if err != nil {
		return 0, err
	}
	cpus := runtime.NumCPU()
	if cpus < 1 {
		cpus = 1
	}
	return raw / float64(cpus), nil
}

func loadAvg1m() (float64, error) {
	// FreeBSD: sysctl vm.loadavg returns "{ 1.16 1.11 1.11 }"
	if out, err := exec.Command("sysctl", "-n", "vm.loadavg").Output(); err == nil {
		s := strings.Trim(strings.TrimSpace(string(out)), "{}")
		if fields := strings.Fields(s); len(fields) > 0 {
			if f, err := strconv.ParseFloat(fields[0], 64); err == nil {
				return f, nil
			}
		}
	}

	// Linux: /proc/loadavg (also available on FreeBSD when procfs is mounted)
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, fmt.Errorf("load average unavailable: sysctl vm.loadavg and /proc/loadavg both failed")
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0, fmt.Errorf("/proc/loadavg: unexpected format")
	}
	f, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("/proc/loadavg: parse %q: %w", fields[0], err)
	}
	return f, nil
}
