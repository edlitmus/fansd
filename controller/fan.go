package controller

import (
	"fmt"
	"log/slog"
)

// Reading is a single sensor observation with its configured thresholds.
type Reading struct {
	Name   string
	Value  float64
	MinVal float64 // value at which min fan speed applies
	MaxVal float64 // value at which max fan speed applies
}

// FanController calculates the target fan speed from a set of readings and
// applies hysteresis to avoid rapid oscillation.
type FanController struct {
	minSpeed   int
	maxSpeed   int
	hysteresis int
	current    int
}

func NewFanController(minSpeed, maxSpeed, hysteresis int) *FanController {
	return &FanController{
		minSpeed:   minSpeed,
		maxSpeed:   maxSpeed,
		hysteresis: hysteresis,
		current:    -1, // -1 signals "not yet set"
	}
}

// Compute takes a set of sensor readings and returns the target fan speed
// percentage after applying hysteresis. Returns (speed, changed, nil) where
// changed is true when the caller should apply the new speed.
func (fc *FanController) Compute(readings []Reading) (int, bool, error) {
	if len(readings) == 0 {
		return 0, false, fmt.Errorf("no readings provided")
	}

	target := fc.minSpeed
	for _, r := range readings {
		s := fc.speedForReading(r)
		slog.Debug("sensor reading", "name", r.Name, "value", r.Value, "speed", s)
		if s > target {
			target = s
		}
	}

	if fc.current < 0 {
		fc.current = target
		return target, true, nil
	}

	delta := target - fc.current
	if delta < 0 {
		delta = -delta
	}

	// Always apply increases immediately; only decrease when delta exceeds hysteresis.
	if target > fc.current || delta >= fc.hysteresis {
		fc.current = target
		return target, true, nil
	}

	return fc.current, false, nil
}

func (fc *FanController) speedForReading(r Reading) int {
	if r.Value <= r.MinVal {
		return fc.minSpeed
	}
	if r.Value >= r.MaxVal {
		return fc.maxSpeed
	}
	ratio := (r.Value - r.MinVal) / (r.MaxVal - r.MinVal)
	speed := fc.minSpeed + int(ratio*float64(fc.maxSpeed-fc.minSpeed))
	return clamp(speed, fc.minSpeed, fc.maxSpeed)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
