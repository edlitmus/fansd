package main

import (
	"fmt"
	"time"

	"github.com/BurntSushi/toml"
)

type Config struct {
	IPMI       IPMIConfig       `toml:"ipmi"`
	Fan        FanConfig        `toml:"fan"`
	CPU        CPUConfig        `toml:"cpu"`
	Load       LoadConfig       `toml:"load"`
	Drives     []DriveConfig    `toml:"drives"`
	Prometheus PrometheusConfig `toml:"prometheus"`
}

type PrometheusConfig struct {
	Enabled bool   `toml:"enabled"`
	Listen  string `toml:"listen"`
}

type IPMIConfig struct {
	Host      string `toml:"host"`
	User      string `toml:"user"`
	Password  string `toml:"password"`
	Interface string `toml:"interface"`
}

type FanConfig struct {
	MinSpeed     int      `toml:"min_speed"`
	MaxSpeed     int      `toml:"max_speed"`
	PollInterval duration `toml:"poll_interval"`
	Hysteresis   int      `toml:"hysteresis"`
}

type CPUConfig struct {
	Enabled    bool    `toml:"enabled"`
	SensorsCmd string  `toml:"sensors_cmd"`
	MinTemp    float64 `toml:"min_temp"`
	MaxTemp    float64 `toml:"max_temp"`
}

type LoadConfig struct {
	Enabled bool    `toml:"enabled"`
	MinLoad float64 `toml:"min_load"`
	MaxLoad float64 `toml:"max_load"`
}

type DriveConfig struct {
	Device  string  `toml:"device"`
	MinTemp float64 `toml:"min_temp"`
	MaxTemp float64 `toml:"max_temp"`
}

// duration wraps time.Duration for TOML unmarshaling from strings like "30s".
type duration struct {
	time.Duration
}

func (d *duration) UnmarshalText(b []byte) error {
	var err error
	d.Duration, err = time.ParseDuration(string(b))
	return err
}

func loadConfig(path string) (*Config, error) {
	cfg := defaultConfig()
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return cfg, nil
}

func defaultConfig() *Config {
	return &Config{
		IPMI: IPMIConfig{
			Interface: "lanplus",
		},
		Fan: FanConfig{
			MinSpeed:     10,
			MaxSpeed:     100,
			PollInterval: duration{30 * time.Second},
			Hysteresis:   5,
		},
		CPU: CPUConfig{
			Enabled:    true,
			SensorsCmd: "sensors",
			MinTemp:    50,
			MaxTemp:    80,
		},
		Load: LoadConfig{
			Enabled: true,
			MinLoad: 1.0,
			MaxLoad: 4.0,
		},
		Prometheus: PrometheusConfig{
			Listen: ":9105",
		},
	}
}

func (c *Config) validate() error {
	if c.IPMI.Host == "" {
		return fmt.Errorf("ipmi.host is required")
	}
	if c.IPMI.User == "" {
		return fmt.Errorf("ipmi.user is required")
	}
	if c.Fan.MinSpeed < 0 || c.Fan.MinSpeed > 100 {
		return fmt.Errorf("fan.min_speed must be 0-100")
	}
	if c.Fan.MaxSpeed < 0 || c.Fan.MaxSpeed > 100 {
		return fmt.Errorf("fan.max_speed must be 0-100")
	}
	if c.Fan.MinSpeed >= c.Fan.MaxSpeed {
		return fmt.Errorf("fan.min_speed must be less than fan.max_speed")
	}
	if c.Fan.PollInterval.Duration <= 0 {
		return fmt.Errorf("fan.poll_interval must be positive")
	}
	if c.CPU.Enabled && c.CPU.MinTemp >= c.CPU.MaxTemp {
		return fmt.Errorf("cpu.min_temp must be less than cpu.max_temp")
	}
	if c.Load.Enabled && c.Load.MinLoad >= c.Load.MaxLoad {
		return fmt.Errorf("load.min_load must be less than load.max_load")
	}
	for i, d := range c.Drives {
		if d.Device == "" {
			return fmt.Errorf("drives[%d].device is required", i)
		}
		if d.MinTemp >= d.MaxTemp {
			return fmt.Errorf("drives[%d].min_temp must be less than max_temp", i)
		}
	}
	return nil
}
