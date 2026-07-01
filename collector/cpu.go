package collector

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// CPUTemp returns the highest CPU core temperature in Celsius.
// Sources are tried in order: FreeBSD sysctl (dev.cpu.N.temperature),
// lm-sensors, ipmitool sdr.
func CPUTemp(sensorsCmd, ipmiHost, ipmiUser, ipmiPass, ipmiIface string) (float64, error) {
	if t, err := SysctlCPUTemp(); err == nil {
		return t, nil
	}
	if t, err := cpuTempFromSensors(sensorsCmd); err == nil {
		return t, nil
	}
	return cpuTempFromIPMI(ipmiHost, ipmiUser, ipmiPass, ipmiIface)
}

// sensorsOutput is the JSON structure returned by `sensors -j`.
// It is a map of chip name → map of feature name → map of subfeature → value.
type sensorsOutput map[string]map[string]any

func cpuTempFromSensors(cmd string) (float64, error) {
	out, err := exec.Command(cmd, "-j").Output()
	if err != nil {
		return 0, fmt.Errorf("sensors: %w", err)
	}

	var chips sensorsOutput
	if err := json.Unmarshal(out, &chips); err != nil {
		return 0, fmt.Errorf("sensors json: %w", err)
	}

	var max float64
	found := false
	for _, features := range chips {
		for featName, featVal := range features {
			// lm-sensors JSON: feature value is a map of subfeature name → float64
			subfeats, ok := featVal.(map[string]any)
			if !ok {
				continue
			}
			// Temperature inputs are named like "temp1_input", "Core 0_input"
			for subfeatName, v := range subfeats {
				if !strings.HasSuffix(subfeatName, "_input") {
					continue
				}
				// Only consider temperature features (not voltage, fan RPM, etc.)
				lower := strings.ToLower(featName)
				if !strings.Contains(lower, "temp") && !strings.Contains(lower, "core") {
					continue
				}
				f, ok := v.(float64)
				if !ok {
					continue
				}
				if !found || f > max {
					max = f
					found = true
				}
			}
		}
	}

	if !found {
		return 0, fmt.Errorf("sensors: no temperature readings found")
	}
	return max, nil
}

func cpuTempFromIPMI(host, user, pass, iface string) (float64, error) {
	out, err := exec.Command(
		"ipmitool", "-I", iface, "-H", host, "-U", user, "-P", pass,
		"sdr", "type", "Temperature",
	).Output()
	if err != nil {
		return 0, fmt.Errorf("ipmitool sdr: %w", err)
	}

	var max float64
	found := false
	for _, line := range strings.Split(string(out), "\n") {
		// Format: "Temp             | 01h | ok  |  3.1 | 22 degrees C"
		parts := strings.Split(line, "|")
		if len(parts) < 5 {
			continue
		}
		valueField := strings.TrimSpace(parts[4])
		fields := strings.Fields(valueField)
		if len(fields) == 0 {
			continue
		}
		f, err := strconv.ParseFloat(fields[0], 64)
		if err != nil {
			continue
		}
		if !found || f > max {
			max = f
			found = true
		}
	}

	if !found {
		return 0, fmt.Errorf("ipmitool sdr: no temperature readings found")
	}
	return max, nil
}
