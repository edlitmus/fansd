package collector

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// SysctlCPUTemp returns the highest CPU core temperature in Celsius by
// reading dev.cpu.N.temperature on FreeBSD (provided by coretemp/amdtemp).
// Returns an error when no temperature OIDs are found, which is the case on
// non-FreeBSD systems or when the driver is not loaded.
func SysctlCPUTemp() (float64, error) {
	out, err := exec.Command("sysctl", "-a", "dev.cpu").Output()
	if err != nil && len(out) == 0 {
		return 0, fmt.Errorf("sysctl dev.cpu: %w", err)
	}

	var max float64
	found := false
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, ".temperature:") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		// Value format is "48.0C" — strip the trailing unit character.
		valStr := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(parts[1]), "C"))
		f, err := strconv.ParseFloat(valStr, 64)
		if err != nil {
			continue
		}
		if !found || f > max {
			max = f
			found = true
		}
	}

	if !found {
		return 0, fmt.Errorf("sysctl: no dev.cpu.N.temperature OIDs found")
	}
	return max, nil
}

// SysctlCPUTjMax returns the thermal junction maximum temperature in Celsius
// from dev.cpu.N.coretemp.tjmax (Intel) or dev.cpu.N.amdtemp.tjmax (AMD).
// When cores report different values the minimum is returned (conservative).
func SysctlCPUTjMax() (float64, error) {
	out, err := exec.Command("sysctl", "-a", "dev.cpu").Output()
	if err != nil && len(out) == 0 {
		return 0, fmt.Errorf("sysctl dev.cpu: %w", err)
	}

	var min float64
	found := false
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, ".tjmax:") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		valStr := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(parts[1]), "C"))
		f, err := strconv.ParseFloat(valStr, 64)
		if err != nil {
			continue
		}
		if !found || f < min {
			min = f
			found = true
		}
	}

	if !found {
		return 0, fmt.Errorf("sysctl: no .tjmax OIDs found (coretemp/amdtemp not loaded?)")
	}
	return min, nil
}
