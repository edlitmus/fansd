package collector

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// ErrNoMedium is returned when the device is virtual or has no medium
// (e.g. an iDRAC virtual floppy). Callers can treat this as permanent.
var ErrNoMedium = errors.New("no medium or virtual device")

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
	// errors that smartctl writes to stdout alongside the copyright header.
	out, err := exec.Command("smartctl", args...).CombinedOutput()
	s := string(out)
	if err != nil {
		switch {
		case len(out) == 0 || strings.Contains(s, "Permission denied"):
			return 0, fmt.Errorf("smartctl %s: permission denied (run as root)", device)
		case strings.Contains(s, "NO MEDIUM") || strings.Contains(s, "Virtual"):
			return 0, ErrNoMedium
		}
	}
	return parseSmartTemp(out)
}

// DriveThresholds holds the temperature limits reported by the drive itself.
type DriveThresholds struct {
	// TripTemp is the manufacturer's thermal limit in °C (SCSI "Drive Trip
	// Temperature" or the max from ATA attribute 194's raw min/max field).
	// Zero means the drive did not report a value.
	TripTemp float64
}

// ProbeDriveThresholds queries a drive for its built-in temperature limits.
// It uses the same device-type inference as DriveTemp.
func ProbeDriveThresholds(device string) (DriveThresholds, error) {
	base := filepath.Base(device)
	var args []string
	if strings.HasPrefix(base, "da") {
		args = []string{"-d", "scsi", "-a", device}
	} else {
		args = []string{"-a", device}
	}

	out, err := exec.Command("smartctl", args...).CombinedOutput()
	s := string(out)
	if err != nil {
		if len(out) == 0 || strings.Contains(s, "Permission denied") {
			return DriveThresholds{}, fmt.Errorf("smartctl %s: permission denied", device)
		}
		if strings.Contains(s, "NO MEDIUM") || strings.Contains(s, "Virtual") {
			return DriveThresholds{}, ErrNoMedium
		}
	}
	return parseDriveThresholds(out), nil
}

func parseDriveThresholds(out []byte) DriveThresholds {
	var dt DriveThresholds
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		lower := strings.ToLower(line)

		// SCSI/SAS: "Drive Trip Temperature:        60 C"
		if strings.Contains(lower, "drive trip temperature") {
			for _, f := range fields {
				if v, err := strconv.ParseFloat(f, 64); err == nil && v > 0 && v < 200 {
					dt.TripTemp = v
					return dt
				}
			}
		}

		// ATA attribute 194 raw value: "35 (Min/Max 18/53)" — use the max.
		if len(fields) >= 10 && (fields[0] == "194" || fields[0] == "190") {
			raw := fields[len(fields)-1]
			if idx := strings.Index(raw, "/"); idx != -1 {
				// raw looks like "18/53)" — take the part after the slash
				maxStr := strings.TrimRight(raw[idx+1:], ")")
				if v, err := strconv.ParseFloat(maxStr, 64); err == nil && v > 0 {
					dt.TripTemp = v
					return dt
				}
			}
		}
	}
	return dt
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
