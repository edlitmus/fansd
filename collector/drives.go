package collector

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// DriveTemp returns the temperature in Celsius for the given block device
// by parsing `smartctl -A` output.
func DriveTemp(device string) (float64, error) {
	out, err := exec.Command("smartctl", "-A", device).Output()
	if err != nil {
		// smartctl exits non-zero when the drive has warnings but still outputs data.
		// Only treat it as a hard error when there's no output at all.
		if len(out) == 0 {
			return 0, fmt.Errorf("smartctl %s: %w", device, err)
		}
	}

	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		// SMART attribute line format (ATA):
		// ID# ATTRIBUTE_NAME FLAG VALUE WORST THRESH TYPE UPDATED WHEN_FAILED RAW_VALUE
		// Attribute 194 is Temperature_Celsius; 190 is Airflow_Temperature_Cel.
		if len(fields) < 10 {
			continue
		}
		id := fields[0]
		if id != "194" && id != "190" {
			continue
		}
		// RAW_VALUE is the last field and holds the actual temperature.
		raw := fields[len(fields)-1]
		// Some drives report "22 (Min/Max 18/26)" — take just the first number.
		rawNum := strings.Fields(raw)[0]
		f, err := strconv.ParseFloat(rawNum, 64)
		if err != nil {
			continue
		}
		return f, nil
	}

	return 0, fmt.Errorf("smartctl %s: temperature attribute not found", device)
}
