package main

import (
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fansd/collector"
	"fansd/controller"
	"fansd/ipmi"
)

func main() {
	cfgPath := flag.String("config", "/usr/local/etc/fansd/fansd.toml", "path to config file")
	debug := flag.Bool("debug", false, "enable debug logging")
	flag.Parse()

	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	ipmiClient := ipmi.NewClient(cfg.IPMI.Host, cfg.IPMI.User, cfg.IPMI.Password, cfg.IPMI.Interface)
	fanCtrl := controller.NewFanController(cfg.Fan.MinSpeed, cfg.Fan.MaxSpeed, cfg.Fan.Hysteresis)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(cfg.Fan.PollInterval.Duration)
	defer ticker.Stop()

	slog.Info("fansd started", "host", cfg.IPMI.Host, "poll", cfg.Fan.PollInterval.Duration)

	// Run once immediately, then on each tick.
	runCycle(cfg, ipmiClient, fanCtrl)

	for {
		select {
		case <-ticker.C:
			runCycle(cfg, ipmiClient, fanCtrl)
		case sig := <-stop:
			slog.Info("shutting down", "signal", sig)
			if err := ipmiClient.EnableAutoFan(); err != nil {
				slog.Warn("restore auto fan", "err", err)
			} else {
				slog.Info("auto fan control restored")
			}
			os.Exit(0)
		}
	}
}

func runCycle(cfg *Config, ipmiClient *ipmi.Client, fanCtrl *controller.FanController) {
	readings := collectReadings(cfg)
	if len(readings) == 0 {
		slog.Warn("no readings collected, skipping cycle")
		return
	}

	speed, changed, err := fanCtrl.Compute(readings)
	if err != nil {
		slog.Error("compute fan speed", "err", err)
		return
	}

	if !changed {
		slog.Debug("fan speed unchanged", "speed", speed)
		return
	}

	slog.Info("setting fan speed", "speed_pct", speed)
	if err := ipmiClient.SetManualFanSpeed(speed); err != nil {
		slog.Error("set fan speed", "err", err)
	}
}

func collectReadings(cfg *Config) []controller.Reading {
	var readings []controller.Reading

	if cfg.CPU.Enabled {
		temp, err := collector.CPUTemp(
			cfg.CPU.SensorsCmd,
			cfg.IPMI.Host, cfg.IPMI.User, cfg.IPMI.Password, cfg.IPMI.Interface,
		)
		if err != nil {
			slog.Warn("cpu temp", "err", err)
		} else {
			readings = append(readings, controller.Reading{
				Name:   "cpu",
				Value:  temp,
				MinVal: cfg.CPU.MinTemp,
				MaxVal: cfg.CPU.MaxTemp,
			})
		}
	}

	if cfg.Load.Enabled {
		load, err := collector.SystemLoad()
		if err != nil {
			slog.Warn("system load", "err", err)
		} else {
			readings = append(readings, controller.Reading{
				Name:   "load",
				Value:  load,
				MinVal: cfg.Load.MinLoad,
				MaxVal: cfg.Load.MaxLoad,
			})
		}
	}

	for _, d := range cfg.Drives {
		temp, err := collector.DriveTemp(d.Device)
		if err != nil {
			slog.Warn("drive temp", "device", d.Device, "err", err)
			continue
		}
		readings = append(readings, controller.Reading{
			Name:   "drive:" + d.Device,
			Value:  temp,
			MinVal: d.MinTemp,
			MaxVal: d.MaxTemp,
		})
	}

	return readings
}
