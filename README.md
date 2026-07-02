# fansd

A Go daemon for intelligent fan speed control on Dell servers via IPMI.
It reads CPU temperature, drive temperature, and system load, then
continuously adjusts fan speed using linear interpolation across configurable
thresholds.  Speed increases are applied immediately; decreases are gated by a
hysteresis value to prevent rapid oscillation.  On shutdown the BIOS automatic
fan control is restored.

## Features

- CPU temperature via FreeBSD sysctl (`dev.cpu.N.temperature`), then `sensors`, then `ipmitool sdr`
- Drive temperature via `smartctl` (ATA attribute 194/190; SCSI/SAS formats also supported)
- System load average normalized by CPU count
- Per-sensor linear interpolation — the hottest sensor wins
- Configurable hysteresis to avoid fan hunting
- `-init-config` probes CPU TjMax and drive trip temperatures to derive thresholds automatically
- Prometheus metrics endpoint (`/metrics`) for CPU temp, drive temps, load, and fan speed
- FreeBSD rc.d service script included

## Requirements

| Tool | Purpose |
|---|---|
| `ipmitool` | Fan control and IPMI temperature fallback |
| `smartctl` | Drive temperature (SMART) |
| `sensors` | CPU temperature fallback (optional) |

`sensors` and `ipmitool sdr` are only used for CPU temperature when the
FreeBSD `coretemp`/`amdtemp` kernel module is not loaded.  On a typical
FreeBSD host only `ipmitool` and `smartctl` are required:

```sh
pkg install ipmitool smartmontools
```

To also enable the `sensors` fallback:

```sh
pkg install lm-sensors
```

## Building

```sh
go build -o fansd .
install -m 755 fansd /usr/local/sbin/fansd
```

## Configuration

Generate a starter config from detected hardware:

```sh
sudo fansd -init-config /usr/local/etc/fansd/fansd.toml
```

Then edit the file and fill in the IPMI credentials.  See
[fansd.toml.example](fansd.toml.example) for an annotated reference.

Running as root is required for accurate drive detection.  `init-config`
probes each disk with `smartctl` to filter out virtual devices (e.g. iDRAC
virtual floppy) and reads the drive trip temperature to derive per-drive
thresholds (`max_temp = trip − 5°C`, `min_temp = max_temp − 15°C`).
CPU thresholds are derived from the TjMax sysctl
(`dev.cpu.N.coretemp.tjmax` / `amdtemp.tjmax`), which is world-readable,
so CPU probing works without root.  Running without root skips drive probing
with a warning.

### Config reference

```toml
[ipmi]
host      = "192.168.1.100"   # iDRAC IP or hostname
user      = "root"
password  = "calvin"
interface = "lanplus"

[fan]
min_speed     = 10            # minimum fan speed (%)
max_speed     = 100           # maximum fan speed (%)
poll_interval = "30s"         # how often to sample and adjust
hysteresis    = 5             # only decrease speed when delta exceeds this (%)

[cpu]
enabled     = true
sensors_cmd = "sensors"       # path to lm-sensors binary
min_temp    = 50              # °C at which min fan speed applies
max_temp    = 80              # °C at which max fan speed applies

[load]
enabled  = true
min_load = 1.0                # normalized load per core → min fan speed
max_load = 4.0                # normalized load per core → max fan speed

[[drives]]
device   = "/dev/da0"
min_temp = 40    # derived from drive trip temperature when using -init-config
max_temp = 55

[[drives]]
device   = "/dev/da1"
min_temp = 40
max_temp = 55

[prometheus]
enabled = false                # set to true to expose /metrics
listen  = ":9105"             # address:port to listen on
```

### Fan speed calculation

For each enabled sensor the daemon maps the current reading to a fan speed
percentage via linear interpolation between `min_temp`/`min_load` (→
`fan.min_speed`) and `max_temp`/`max_load` (→ `fan.max_speed`).  Values
below the lower bound map to `min_speed`; values at or above the upper bound
map to `max_speed`.  The final fan speed is the **maximum** across all
sensors.

## Prometheus metrics

When `prometheus.enabled = true`, fansd serves Prometheus text-format gauges at
`http://<listen>/metrics`:

| Metric | Labels | Description |
|---|---|---|
| `fansd_cpu_temperature_celsius` | — | Current CPU temperature in °C |
| `fansd_system_load_normalized` | — | 1-minute load average divided by CPU count |
| `fansd_fan_speed_percent` | — | Current IPMI fan speed target (%) |
| `fansd_drive_temperature_celsius` | `device` | Per-drive temperature in °C |

Metrics whose collectors failed during the most recent poll cycle are omitted
rather than reported as zero, so Prometheus can distinguish a missing read from
a genuinely cold sensor.

## Usage

```
fansd [-config path] [-debug]
fansd -init-config path|-
```

| Flag | Default | Description |
|---|---|---|
| `-config` | `/usr/local/etc/fansd/fansd.toml` | Path to config file |
| `-debug` | false | Enable debug-level logging |
| `-init-config` | — | Probe hardware and write starter config to path (use `-` for stdout); run as root for full drive detection |

## FreeBSD rc.d integration

```sh
# Install the service script
install -m 555 rc.d/fansd /usr/local/etc/rc.d/fansd

# Enable in rc.conf
echo 'fansd_enable="YES"' >> /etc/rc.conf

# Optional overrides
echo 'fansd_config="/usr/local/etc/fansd/fansd.toml"' >> /etc/rc.conf
echo 'fansd_flags="-debug"' >> /etc/rc.conf

# Start
service fansd start
```

The service uses `daemon(8)` with `-r` for automatic restart on crash and
`-S` to send fansd's log output (including `-debug`) to syslog, where it
appears under the `daemon` facility tagged `fansd` — typically in
`/var/log/daemon.log`.
On `service fansd stop` the daemon restores BIOS automatic fan control
before exiting.

## Security notes

- The IPMI password is passed to `ipmitool` through the `IPMI_PASSWORD`
  environment variable (`-E`), not as a `-P` command-line argument, so it is
  not exposed to other local users via `ps`.
- The config file contains the IPMI password in plaintext. `-init-config`
  creates it with mode `0600`; keep it owner-only readable if you write it
  by hand (`chmod 600`).
- The Prometheus endpoint is unauthenticated and exposes only non-sensitive
  telemetry (temperatures, load, fan speed). Bind `listen` to a trusted
  interface (e.g. `127.0.0.1:9105`) rather than all interfaces if the host is
  on an untrusted network.

## License

MIT — see [LICENSE](LICENSE).
