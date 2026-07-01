# fansd

A Go daemon for intelligent fan speed control on Dell servers via IPMI.
It reads CPU temperature (lm-sensors with ipmitool fallback), drive
temperature (SMART), and system load, then continuously adjusts fan speed
using linear interpolation across configurable thresholds.  Speed increases
are applied immediately; decreases are gated by a hysteresis value to prevent
rapid oscillation.  On shutdown the BIOS automatic fan control is restored.

## Features

- CPU temperature via `sensors` (falls back to `ipmitool sdr`)
- Drive temperature via `smartctl` (SMART attribute 194/190)
- System load average normalized by CPU count
- Per-sensor linear interpolation — the hottest sensor wins
- Configurable hysteresis to avoid fan hunting
- Auto-detects drives and CPU count with `-init-config`
- FreeBSD rc.d service script included

## Requirements

| Tool | Purpose |
|---|---|
| `ipmitool` | Fan control and IPMI temperature fallback |
| `smartctl` | Drive temperature (SMART) |
| `sensors` | CPU temperature (optional, falls back to ipmitool) |

On FreeBSD:

```sh
pkg install ipmitool smartmontools lm-sensors
```

## Building

```sh
go build -o fansd .
install -m 755 fansd /usr/local/sbin/fansd
```

## Configuration

Generate a starter config from detected hardware:

```sh
fansd -init-config /usr/local/etc/fansd/fansd.toml
```

Then edit the file and fill in the IPMI credentials.  See
[fansd.toml.example](fansd.toml.example) for an annotated reference.

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
min_temp = 35
max_temp = 50

[[drives]]
device   = "/dev/da1"
min_temp = 35
max_temp = 50
```

### Fan speed calculation

For each enabled sensor the daemon maps the current reading to a fan speed
percentage via linear interpolation between `min_temp`/`min_load` (→
`fan.min_speed`) and `max_temp`/`max_load` (→ `fan.max_speed`).  Values
below the lower bound map to `min_speed`; values at or above the upper bound
map to `max_speed`.  The final fan speed is the **maximum** across all
sensors.

## Usage

```
fansd [-config path] [-debug] [-init-config path|-]
```

| Flag | Default | Description |
|---|---|---|
| `-config` | `/usr/local/etc/fansd/fansd.toml` | Path to config file |
| `-debug` | false | Enable debug-level logging |
| `-init-config` | — | Write auto-detected starter config to path (use `-` for stdout) |

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

The service uses `daemon(8)` with `-r` for automatic restart on crash.
On `service fansd stop` the daemon restores BIOS automatic fan control
before exiting.

## License

MIT — see [LICENSE](LICENSE).
