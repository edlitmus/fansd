package collector

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// DriveTemp returns the temperature in Celsius for the given block device.
// The smartctl device type is inferred from the device name:
//   - ada*        → ATA/SATA, auto-detect
//   - da*         → SCSI/SAS, explicit -d scsi
//   - nvd*, nda*  → NVMe, auto-detect
//   - anything else → auto-detect, then -d scsi fallback
func DriveTemp(device string) (float64, error) {
	base := filepath.Base(device)
	switch {
	case strings.HasPrefix(base, "ada"),
		strings.HasPrefix(base, "nvd"),
		strings.HasPrefix(base, "nda"):
		return smartctlTemp(device)
	case strings.HasPrefix(base, "da"):
		return smartctlTemp(device, "-d", "scsi")
	default:
		if t, err := smartctlTemp(device); err == nil {
			return t, nil
		}
		return smartctlTemp(device, "-d", "scsi")
	}
}

func smartctlTemp(device string, extra ...string) (float64, error) {
	args := append([]string{"-A"}, extra...)
	args = append(args, device)

	// CombinedOutput captures both stdout and stderr so we can detect
	// permission errors that smartctl writes to stdout alongside the header.
	out, err := exec.Command("smartctl", args...).CombinedOutput()
	if err != nil {
		if len(out) == 0 || strings.Contains(string(out), "Permission denied") {
			return 0, fmt.Errorf("smartctl %s: permission denied (run as root)", device)
		}
	}
	return parseSmartTemp(out)
}

// parseSmartTemp handles both ATA and SCSI/SAS output formats from smartctl -A.
func parseSmartTemp(out []byte) (float64, error) {
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)

		// ATA format: "194 Temperature_Celsius 0x0022 … 35" (10+ fields)
		// Attribute 190 is Airflow_Temperature_Cel on some drives.
		if len(fields) >= 10 {
			if fields[0] == "194" || fields[0] == "190" {
				// RAW_VALUE is the last field; may be "22 (Min/Max 18/26)"
				raw := strings.Fields(fields[len(fields)-1])[0]
				if f, err := strconv.ParseFloat(raw, 64); err == nil {
					return f, nil
				}
			}
		}

		// SCSI/SAS format: "Current Drive Temperature:     35 C"
		// NVMe format:     "Temperature:                   35 Celsius"
		// SAS log page:    "          Current Temperature:   35 Celsius"
		lower := strings.ToLower(line)
		if strings.Contains(lower, "current drive temperature") ||
			strings.Contains(lower, "current temperature") ||
			strings.HasPrefix(strings.TrimSpace(lower), "temperature:") {
			for _, f := range fields {
				if v, err := strconv.ParseFloat(f, 64); err == nil && v > 0 && v < 200 {
					return v, nil
				}
			}
		}
	}
	return 0, fmt.Errorf("temperature not found in smartctl output")
}
