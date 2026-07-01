package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"

	"fansd/collector"
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
# max_temp = 55
`)
	} else {
		for _, dev := range drives {
			minTemp, maxTemp := driveTemps(dev)
			fmt.Fprintf(out, `
[[drives]]
device   = "%s"
min_temp = %d                  # °C → min fan speed
max_temp = %d                  # °C → max fan speed
`, dev, minTemp, maxTemp)
		}
	}

	return nil
}

// detectDrives returns physical disk device paths on FreeBSD. It queries
// kern.disks for candidates, then probes each with smartctl to exclude
// virtual or empty devices (e.g. iDRAC virtual floppy).
func detectDrives() ([]string, error) {
	out, err := exec.Command("sysctl", "-n", "kern.disks").Output()
	if err != nil {
		return nil, fmt.Errorf("sysctl kern.disks: %w", err)
	}

	seen := map[string]bool{}
	var candidates []string
	for _, name := range strings.Fields(string(out)) {
		if !isStorageDisk(name) {
			continue
		}
		dev := "/dev/" + name
		if !seen[dev] {
			seen[dev] = true
			candidates = append(candidates, dev)
		}
	}
	sort.Strings(candidates)

	if os.Getuid() != 0 {
		fmt.Fprintf(os.Stderr, "warning: not running as root; skipping drive probe — virtual devices will not be excluded\n")
		fmt.Fprintf(os.Stderr, "         re-run with sudo for accurate drive detection\n")
		return candidates, nil
	}

	var drives []string
	for _, dev := range candidates {
		if !isPhysicalDisk(dev) {
			fmt.Fprintf(os.Stderr, "skipping %s: virtual device or no medium\n", dev)
			continue
		}
		drives = append(drives, dev)
	}
	return drives, nil
}

// isPhysicalDisk probes the device with smartctl to check whether it is a
// real disk. When the output positively identifies the device as virtual or
// reports no medium, the device is excluded. Devices that cannot be opened
// (e.g. permission denied when not running as root) are assumed to be real
// and included; they will produce a clear error at runtime.
func isPhysicalDisk(device string) bool {
	out, _ := exec.Command("smartctl", "-d", "scsi", "-i", device).CombinedOutput()
	s := string(out)
	return !strings.Contains(s, "NO MEDIUM") && !strings.Contains(s, "Virtual")
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

// driveTemps returns (min_temp, max_temp) for a drive config entry.
// If the drive reports a trip temperature, max_temp is set 5°C below it and
// min_temp is set 15°C below max_temp. Falls back to safe defaults otherwise.
func driveTemps(device string) (minTemp, maxTemp int) {
	dt, err := collector.ProbeDriveThresholds(device)
	if err == nil && dt.TripTemp > 0 {
		maxTemp = int(dt.TripTemp) - 5
		minTemp = maxTemp - 15
		return
	}
	return 35, 55
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
