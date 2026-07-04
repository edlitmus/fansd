package collector

import (
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// smartctlPath is resolved once at startup. Daemon launch environments
// (rc/daemon(8)) have PATH=/sbin:/bin:/usr/sbin:/usr/bin, which does not
// include /usr/local/sbin where smartmontools installs smartctl.
var smartctlPath = findSmartctl()

func findSmartctl() string {
	if p, err := exec.LookPath("smartctl"); err == nil {
		return p
	}
	return "/usr/local/sbin/smartctl"
}

// ErrNoMedium is returned when the device is virtual or has no medium
// (e.g. an iDRAC virtual floppy). Callers can treat this as permanent.
var ErrNoMedium = errors.New("no medium or virtual device")

// DriveTemp returns the temperature in Celsius for the given block device.
// For da* devices (which may be SAS or SATA-on-HBA), auto-detection is
// tried first; -d scsi is used as a fallback for native SAS drives.
func DriveTemp(device string) (float64, error) {
	base := filepath.Base(device)
	switch {
	case strings.HasPrefix(base, "ada"),
		strings.HasPrefix(base, "nvd"),
		strings.HasPrefix(base, "nda"):
		return smartctlTemp(device)
	case strings.HasPrefix(base, "da"):
		// da* can be SAS or SATA-behind-HBA; try auto-detect first.
		if t, err := smartctlTemp(device); err == nil {
			return t, nil
		}
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
	out, err := exec.Command(smartctlPath, args...).CombinedOutput()
	s := string(out)
	if err != nil {
		switch {
		case errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist):
			return 0, fmt.Errorf("smartctl not found (looked for %s): install smartmontools or fix PATH", smartctlPath)
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
	// TripTemp is the manufacturer's thermal limit in °C. Currently populated
	// only for SCSI/SAS drives via "Drive Trip Temperature". Zero means the
	// drive did not report a value and the caller should use defaults.
	TripTemp float64
}

// ProbeDriveThresholds queries a drive for its built-in temperature limits.
// It uses the same device-type detection as DriveTemp.
func ProbeDriveThresholds(device string) (DriveThresholds, error) {
	base := filepath.Base(device)

	probe := func(extra ...string) (DriveThresholds, error) {
		args := append([]string{"-a"}, extra...)
		args = append(args, device)
		out, err := exec.Command(smartctlPath, args...).CombinedOutput()
		s := string(out)
		if err != nil {
			if errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
				return DriveThresholds{}, fmt.Errorf("smartctl not found (looked for %s): install smartmontools or fix PATH", smartctlPath)
			}
			if len(out) == 0 || strings.Contains(s, "Permission denied") {
				return DriveThresholds{}, fmt.Errorf("smartctl %s: permission denied", device)
			}
			if strings.Contains(s, "NO MEDIUM") || strings.Contains(s, "Virtual") {
				return DriveThresholds{}, ErrNoMedium
			}
		}
		return parseDriveThresholds(out), nil
	}

	if strings.HasPrefix(base, "da") {
		// Try auto-detect first (covers SATA-behind-HBA); fall back to scsi.
		if dt, err := probe(); err == nil {
			return dt, nil
		}
		return probe("-d", "scsi")
	}
	return probe()
}

// parseDriveThresholds extracts the manufacturer thermal limit from smartctl
// output. Only SCSI/SAS "Drive Trip Temperature" is used; ATA drives do not
// expose a reliable trip temperature via SMART attributes and will return a
// zero TripTemp, causing the caller to fall back to defaults.
func parseDriveThresholds(out []byte) DriveThresholds {
	var dt DriveThresholds
	for _, line := range strings.Split(string(out), "\n") {
		// SCSI/SAS: "Drive Trip Temperature:        60 C"
		if strings.Contains(strings.ToLower(line), "drive trip temperature") {
			for _, f := range strings.Fields(line) {
				if v, err := strconv.ParseFloat(f, 64); err == nil && v > 0 && v < 200 {
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

		// ATA SMART attribute table format (10 fixed columns):
		//   ID# ATTR_NAME FLAG VALUE WORST THRESH TYPE UPDATED WHEN_FAILED RAW_VALUE [extra...]
		// fields[9] is always the base RAW_VALUE; extra tokens (parenthesized
		// data like "(Min/Max 31/46)" or "(0 22 0 0 0)") appear as additional
		// fields and must be ignored.
		// Attribute 194 = Temperature_Celsius, 190 = Airflow_Temperature_Cel.
		if len(fields) >= 10 && (fields[0] == "194" || fields[0] == "190") {
			if f, err := strconv.ParseFloat(fields[9], 64); err == nil {
				return f, nil
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
