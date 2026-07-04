package ipmi

import (
	"fmt"
	"os"
	"os/exec"
)

// ipmitoolPath is resolved once at startup. Daemon launch environments
// (rc/daemon(8)) have PATH=/sbin:/bin:/usr/sbin:/usr/bin, which does not
// include /usr/local/bin where the ipmitool port installs.
var ipmitoolPath = findIpmitool()

func findIpmitool() string {
	if p, err := exec.LookPath("ipmitool"); err == nil {
		return p
	}
	return "/usr/local/bin/ipmitool"
}

// Client wraps ipmitool for Dell iDRAC fan control.
type Client struct {
	host  string
	user  string
	pass  string
	iface string
}

func NewClient(host, user, pass, iface string) *Client {
	return &Client{host: host, user: user, pass: pass, iface: iface}
}

// SetManualFanSpeed disables automatic fan control and sets the speed as a
// percentage (0–100). Dell iDRAC interprets the percentage as a hex byte
// sent via raw command 0x30 0x30 0x02.
func (c *Client) SetManualFanSpeed(pct int) error {
	if pct < 0 || pct > 100 {
		return fmt.Errorf("fan speed %d out of range [0, 100]", pct)
	}

	if err := c.run("raw", "0x30", "0x30", "0x01", "0x00"); err != nil {
		return fmt.Errorf("disable auto fan: %w", err)
	}

	hexSpeed := fmt.Sprintf("0x%02x", pct)
	if err := c.run("raw", "0x30", "0x30", "0x02", "0xff", hexSpeed); err != nil {
		return fmt.Errorf("set fan speed: %w", err)
	}

	return nil
}

// EnableAutoFan re-enables automatic BIOS fan control.
func (c *Client) EnableAutoFan() error {
	if err := c.run("raw", "0x30", "0x30", "0x01", "0x01"); err != nil {
		return fmt.Errorf("enable auto fan: %w", err)
	}
	return nil
}

func (c *Client) run(args ...string) error {
	// Pass the password via the IPMI_PASSWORD environment variable (-E) rather
	// than -P on the command line, which would expose it to any local user via
	// ps(1) / procfs. FreeBSD does not reveal another user's environment.
	base := []string{"-I", c.iface, "-H", c.host, "-U", c.user, "-E"}
	cmd := exec.Command(ipmitoolPath, append(base, args...)...)
	cmd.Env = append(os.Environ(), "IPMI_PASSWORD="+c.pass)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, out)
	}
	return nil
}
