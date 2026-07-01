package collector

import (
	"fmt"
	"os"
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
	// /proc/loadavg is Linux; FreeBSD also exposes it via procfs when mounted.
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, fmt.Errorf("read /proc/loadavg: %w", err)
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
