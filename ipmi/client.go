package ipmi

import (
	"fmt"
	"os/exec"
)

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
	base := []string{"-I", c.iface, "-H", c.host, "-U", c.user, "-P", c.pass}
	cmd := exec.Command("ipmitool", append(base, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, out)
	}
	return nil
}
