package collector

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSmartTemp(t *testing.T) {
	tests := []struct {
		name    string
		out     string
		want    float64
		wantErr bool
	}{
		{
			name: "ATA attribute 194 with min/max suffix",
			out: `ID# ATTRIBUTE_NAME          FLAG     VALUE WORST THRESH TYPE      UPDATED  WHEN_FAILED RAW_VALUE
194 Temperature_Celsius     0x0022   036   014   000    Old_age   Always       -       36 (Min/Max 21/46)`,
			want: 36,
		},
		{
			name: "ATA attribute 190 airflow temperature",
			out: `ID# ATTRIBUTE_NAME          FLAG     VALUE WORST THRESH TYPE      UPDATED  WHEN_FAILED RAW_VALUE
190 Airflow_Temperature_Cel 0x0022   069   049   045    Old_age   Always       -       31`,
			want: 31,
		},
		{
			name: "SCSI current drive temperature",
			out:  "Current Drive Temperature:     42 C",
			want: 42,
		},
		{
			name: "NVMe temperature line",
			out:  "Temperature:                        35 Celsius",
			want: 35,
		},
		{
			name: "SAS log page current temperature",
			out:  "          Current Temperature:   28 Celsius",
			want: 28,
		},
		{
			name:    "no temperature in output",
			out:     "smartctl 7.4 2023-08-01 r5530\nSMART support is: Enabled",
			wantErr: true,
		},
		{
			name:    "empty output",
			out:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSmartTemp([]byte(tt.out))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseSmartTemp() = %v, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSmartTemp() error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("parseSmartTemp() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseDriveThresholds(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want float64
	}{
		{
			name: "SCSI drive trip temperature",
			out: `Current Drive Temperature:     31 C
Drive Trip Temperature:        60 C`,
			want: 60,
		},
		{
			name: "no trip temperature reported",
			out:  "Current Drive Temperature:     31 C",
			want: 0,
		},
		{
			name: "empty output",
			out:  "",
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDriveThresholds([]byte(tt.out))
			if got.TripTemp != tt.want {
				t.Fatalf("parseDriveThresholds().TripTemp = %v, want %v", got.TripTemp, tt.want)
			}
		})
	}
}

// writeFakeSmartctl installs a shell script standing in for smartctl and
// returns its path.
func writeFakeSmartctl(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "smartctl")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// swapSmartctlPath overrides the package-level binary path for one test.
func swapSmartctlPath(t *testing.T, path string) {
	t.Helper()
	orig := smartctlPath
	smartctlPath = path
	t.Cleanup(func() { smartctlPath = orig })
}

func TestSmartctlTempReportsMissingBinary(t *testing.T) {
	swapSmartctlPath(t, filepath.Join(t.TempDir(), "missing-smartctl"))

	_, err := smartctlTemp("/dev/da0")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("want not-found error, got %v", err)
	}
}

func TestSmartctlTempReportsPermissionDenied(t *testing.T) {
	swapSmartctlPath(t, writeFakeSmartctl(t,
		`echo "Smartctl open device: /dev/da0 failed: Permission denied"; exit 2`))

	_, err := smartctlTemp("/dev/da0")
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("want permission-denied error, got %v", err)
	}
}

func TestSmartctlTempReportsNoMedium(t *testing.T) {
	swapSmartctlPath(t, writeFakeSmartctl(t,
		`echo "Device: iDRAC Virtual Floppy, NO MEDIUM present"; exit 2`))

	_, err := smartctlTemp("/dev/da0")
	if !errors.Is(err, ErrNoMedium) {
		t.Fatalf("want ErrNoMedium, got %v", err)
	}
}

func TestSmartctlTempParsesTemperature(t *testing.T) {
	swapSmartctlPath(t, writeFakeSmartctl(t,
		`echo "194 Temperature_Celsius     0x0022   036   014   000    Old_age   Always       -       36"`))

	got, err := smartctlTemp("/dev/ada0")
	if err != nil {
		t.Fatalf("smartctlTemp() error: %v", err)
	}
	if got != 36 {
		t.Fatalf("smartctlTemp() = %v, want 36", got)
	}
}

func TestProbeDriveThresholdsReportsMissingBinary(t *testing.T) {
	swapSmartctlPath(t, filepath.Join(t.TempDir(), "missing-smartctl"))

	_, err := ProbeDriveThresholds("/dev/da0")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("want not-found error, got %v", err)
	}
}

func TestProbeDriveThresholdsReadsTripTemperature(t *testing.T) {
	swapSmartctlPath(t, writeFakeSmartctl(t,
		`echo "Drive Trip Temperature:        60 C"`))

	dt, err := ProbeDriveThresholds("/dev/ada0")
	if err != nil {
		t.Fatalf("ProbeDriveThresholds() error: %v", err)
	}
	if dt.TripTemp != 60 {
		t.Fatalf("TripTemp = %v, want 60", dt.TripTemp)
	}
}
