package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
)

// runInitConfig auto-detects hardware and writes a starter fansd.toml to
// outPath. If outPath is "-" or empty, output goes to stdout.
func runInitConfig(outPath string) error {
	drives, err := detectDrives()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: drive detection failed: %v\n", err)
	}

	sensorsAvail := commandExists("sensors")
	cpus := runtime.NumCPU()

	// Scale load thresholds by CPU count so defaults feel reasonable regardless
	// of core count: warn at 75% utilization, max at 200%.
	minLoad := float64(cpus) * 0.75
	maxLoad := float64(cpus) * 2.0

	out := os.Stdout
	if outPath != "" && outPath != "-" {
		f, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0640)
		if err != nil {
			return fmt.Errorf("open output: %w", err)
		}
		defer f.Close()
		out = f
		fmt.Fprintf(os.Stderr, "writing config to %s\n", outPath)
	}

	sensorsNote := ""
	if !sensorsAvail {
		sensorsNote = "  # WARNING: 'sensors' not found in PATH; will fall back to ipmitool sdr\n"
	}

	fmt.Fprintf(out, `[ipmi]
host      = "CHANGEME"         # iDRAC IP or hostname
user      = "root"
password  = "CHANGEME"
interface = "lanplus"

[fan]
min_speed     = 10             # minimum fan speed (%%)
max_speed     = 100            # maximum fan speed (%%)
poll_interval = "30s"
hysteresis    = 5              # only decrease speed when delta exceeds this (%%)

[cpu]
enabled     = true
%ssensors_cmd = "sensors"
min_temp    = 50               # °C → min fan speed
max_temp    = 80               # °C → max fan speed

[load]
enabled  = true
# host has %d CPU(s); thresholds set to 75%% / 200%% aggregate utilization
min_load = %.1f
max_load = %.1f
`, sensorsNote, cpus, minLoad, maxLoad)

	if len(drives) == 0 {
		fmt.Fprintf(out, `
# No drives detected automatically. Add [[drives]] sections manually:
# [[drives]]
# device   = "/dev/da0"
# min_temp = 35
# max_temp = 50
`)
	} else {
		for _, dev := range drives {
			fmt.Fprintf(out, `
[[drives]]
device   = "%s"
min_temp = 35                  # °C → min fan speed
max_temp = 50                  # °C → max fan speed
`, dev)
		}
	}

	return nil
}

// detectDrives returns base disk device paths on FreeBSD by querying
// kern.disks via sysctl and filtering to known storage device prefixes.
func detectDrives() ([]string, error) {
	out, err := exec.Command("sysctl", "-n", "kern.disks").Output()
	if err != nil {
		return nil, fmt.Errorf("sysctl kern.disks: %w", err)
	}

	var drives []string
	seen := map[string]bool{}
	for _, name := range strings.Fields(string(out)) {
		if !isStorageDisk(name) {
			continue
		}
		dev := "/dev/" + name
		if !seen[dev] {
			seen[dev] = true
			drives = append(drives, dev)
		}
	}
	sort.Strings(drives)
	return drives, nil
}

// isStorageDisk returns true for ATA, SCSI/SAS, and NVMe disk names,
// excluding optical drives, pass-through devices, and memory disks.
func isStorageDisk(name string) bool {
	for _, prefix := range []string{"ada", "da", "nvd", "nda"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
