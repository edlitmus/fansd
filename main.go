package main

import (
	"errors"
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
	initCfg := flag.String("init-config", "", "auto-detect hardware and write a starter config to this path (use - for stdout)")
	flag.Parse()

	if *initCfg != "" {
		if err := runInitConfig(*initCfg); err != nil {
			slog.Error("init-config", "err", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

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

	d := newDaemon(cfg)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(cfg.Fan.PollInterval.Duration)
	defer ticker.Stop()

	slog.Info("fansd started", "host", cfg.IPMI.Host, "poll", cfg.Fan.PollInterval.Duration)

	d.runCycle()

	for {
		select {
		case <-ticker.C:
			d.runCycle()
		case sig := <-stop:
			slog.Info("shutting down", "signal", sig)
			if err := d.ipmiClient.EnableAutoFan(); err != nil {
				slog.Warn("restore auto fan", "err", err)
			} else {
				slog.Info("auto fan control restored")
			}
			os.Exit(0)
		}
	}
}

type daemon struct {
	cfg        *Config
	ipmiClient *ipmi.Client
	fanCtrl    *controller.FanController
	deadDrives map[string]bool
}

func newDaemon(cfg *Config) *daemon {
	return &daemon{
		cfg:        cfg,
		ipmiClient: ipmi.NewClient(cfg.IPMI.Host, cfg.IPMI.User, cfg.IPMI.Password, cfg.IPMI.Interface),
		fanCtrl:    controller.NewFanController(cfg.Fan.MinSpeed, cfg.Fan.MaxSpeed, cfg.Fan.Hysteresis),
		deadDrives: make(map[string]bool),
	}
}

func (d *daemon) runCycle() {
	readings := d.collectReadings()
	if len(readings) == 0 {
		slog.Warn("no readings collected, skipping cycle")
		return
	}

	speed, changed, err := d.fanCtrl.Compute(readings)
	if err != nil {
		slog.Error("compute fan speed", "err", err)
		return
	}

	if !changed {
		slog.Debug("fan speed unchanged", "speed", speed)
		return
	}

	slog.Info("setting fan speed", "speed_pct", speed)
	if err := d.ipmiClient.SetManualFanSpeed(speed); err != nil {
		slog.Error("set fan speed", "err", err)
	}
}

func (d *daemon) collectReadings() []controller.Reading {
	var readings []controller.Reading

	if d.cfg.CPU.Enabled {
		temp, err := collector.CPUTemp(
			d.cfg.CPU.SensorsCmd,
			d.cfg.IPMI.Host, d.cfg.IPMI.User, d.cfg.IPMI.Password, d.cfg.IPMI.Interface,
		)
		if err != nil {
			slog.Warn("cpu temp", "err", err)
		} else {
			readings = append(readings, controller.Reading{
				Name:   "cpu",
				Value:  temp,
				MinVal: d.cfg.CPU.MinTemp,
				MaxVal: d.cfg.CPU.MaxTemp,
			})
		}
	}

	if d.cfg.Load.Enabled {
		load, err := collector.SystemLoad()
		if err != nil {
			slog.Warn("system load", "err", err)
		} else {
			readings = append(readings, controller.Reading{
				Name:   "load",
				Value:  load,
				MinVal: d.cfg.Load.MinLoad,
				MaxVal: d.cfg.Load.MaxLoad,
			})
		}
	}

	for _, drv := range d.cfg.Drives {
		if d.deadDrives[drv.Device] {
			continue
		}
		temp, err := collector.DriveTemp(drv.Device)
		if err != nil {
			if errors.Is(err, collector.ErrNoMedium) {
				slog.Warn("excluding drive, no medium or virtual device", "device", drv.Device)
				d.deadDrives[drv.Device] = true
			} else {
				slog.Warn("drive temp", "device", drv.Device, "err", err)
			}
			continue
		}
		readings = append(readings, controller.Reading{
			Name:   "drive:" + drv.Device,
			Value:  temp,
			MinVal: drv.MinTemp,
			MaxVal: drv.MaxTemp,
		})
	}

	return readings
}
