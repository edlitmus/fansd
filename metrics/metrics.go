// Package metrics serves per-device temperature and fan speed gauges in
// Prometheus text exposition format (no external dependencies).
package metrics

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"sync"
)

// Store holds the latest sensor readings and serves them as Prometheus gauges.
// All methods are safe for concurrent use.
type Store struct {
	mu         sync.RWMutex
	cpuTemp    float64
	load       float64
	fanSpeed   float64
	driveTemps map[string]float64
}

func New() *Store {
	return &Store{
		cpuTemp:    math.NaN(),
		load:       math.NaN(),
		fanSpeed:   math.NaN(),
		driveTemps: make(map[string]float64),
	}
}

func (s *Store) SetCPUTemp(v float64) {
	s.mu.Lock()
	s.cpuTemp = v
	s.mu.Unlock()
}

func (s *Store) SetLoad(v float64) {
	s.mu.Lock()
	s.load = v
	s.mu.Unlock()
}

func (s *Store) SetFanSpeed(v float64) {
	s.mu.Lock()
	s.fanSpeed = v
	s.mu.Unlock()
}

func (s *Store) SetDriveTemp(device string, v float64) {
	s.mu.Lock()
	s.driveTemps[device] = v
	s.mu.Unlock()
}

// ServeHTTP renders the Prometheus text exposition format on GET /metrics.
func (s *Store) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	writeGauge(w, "fansd_cpu_temperature_celsius",
		"Current CPU temperature in Celsius.", s.cpuTemp)

	writeGauge(w, "fansd_system_load_normalized",
		"System 1-minute load average divided by CPU count.", s.load)

	writeGauge(w, "fansd_fan_speed_percent",
		"Current IPMI fan speed target as a percentage.", s.fanSpeed)

	if len(s.driveTemps) > 0 {
		fmt.Fprintf(w, "# HELP fansd_drive_temperature_celsius Drive temperature in Celsius.\n")
		fmt.Fprintf(w, "# TYPE fansd_drive_temperature_celsius gauge\n")
		devs := make([]string, 0, len(s.driveTemps))
		for dev := range s.driveTemps {
			devs = append(devs, dev)
		}
		sort.Strings(devs)
		for _, dev := range devs {
			fmt.Fprintf(w, "fansd_drive_temperature_celsius{device=%q} %g\n", dev, s.driveTemps[dev])
		}
		fmt.Fprintf(w, "\n")
	}
}

func writeGauge(w http.ResponseWriter, name, help string, value float64) {
	if math.IsNaN(value) {
		return
	}
	fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s gauge\n%s %g\n\n",
		name, help, name, name, value)
}
