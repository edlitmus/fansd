package ipmi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindIpmitoolFallsBackWhenNotInPath(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	if got := findIpmitool(); got != "/usr/local/bin/ipmitool" {
		t.Fatalf("findIpmitool() = %q, want /usr/local/bin/ipmitool", got)
	}
}

func TestFindIpmitoolPrefersPath(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "ipmitool")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	if got := findIpmitool(); got != fake {
		t.Fatalf("findIpmitool() = %q, want %q", got, fake)
	}
}

// swapIpmitoolPath overrides the package-level binary path for one test.
func swapIpmitoolPath(t *testing.T, path string) {
	t.Helper()
	orig := ipmitoolPath
	ipmitoolPath = path
	t.Cleanup(func() { ipmitoolPath = orig })
}

func TestSetManualFanSpeedRejectsOutOfRange(t *testing.T) {
	c := NewClient("host", "user", "pass", "lanplus")

	for _, pct := range []int{-1, 101} {
		if err := c.SetManualFanSpeed(pct); err == nil {
			t.Fatalf("SetManualFanSpeed(%d) = nil, want out-of-range error", pct)
		}
	}
}

func TestRunInvokesResolvedBinaryWithPasswordEnv(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	fake := filepath.Join(dir, "ipmitool")
	script := "#!/bin/sh\necho \"$IPMI_PASSWORD|$@\" > " + argsFile + "\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	swapIpmitoolPath(t, fake)

	c := NewClient("10.0.0.1", "admin", "s3cret", "lanplus")
	if err := c.run("raw", "0x30"); err != nil {
		t.Fatalf("run() error: %v", err)
	}

	got, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	recorded := strings.TrimSpace(string(got))
	want := "s3cret|-I lanplus -H 10.0.0.1 -U admin -E raw 0x30"
	if recorded != want {
		t.Fatalf("fake ipmitool recorded %q, want %q", recorded, want)
	}
}

func TestRunReportsMissingBinary(t *testing.T) {
	swapIpmitoolPath(t, filepath.Join(t.TempDir(), "missing-ipmitool"))

	c := NewClient("10.0.0.1", "admin", "s3cret", "lanplus")
	if err := c.run("raw", "0x30"); err == nil {
		t.Fatal("run() = nil, want error for missing binary")
	}
}
