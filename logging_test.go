package main

import (
	"log/slog"
	"strings"
	"testing"
)

// recordingWriter captures each Write call separately so tests can assert
// that one log record maps to exactly one syslog message.
type recordingWriter struct {
	writes []string
}

func (r *recordingWriter) Write(p []byte) (int, error) {
	r.writes = append(r.writes, string(p))
	return len(p), nil
}

func TestSyslogHandlerEmitsOneWritePerRecord(t *testing.T) {
	w := &recordingWriter{}
	logger := slog.New(newSyslogTextHandler(w, slog.LevelDebug))

	logger.Debug("sensor reading", "name", "drive:/dev/da0", "value", 34)
	logger.Debug("sensor reading", "name", "drive:/dev/da1", "value", 36)
	logger.Info("setting fan speed", "speed_pct", 40)

	if len(w.writes) != 3 {
		t.Fatalf("got %d writes, want 3 (one per record): %q", len(w.writes), w.writes)
	}
	for _, msg := range w.writes {
		if !strings.HasSuffix(msg, "\n") {
			t.Fatalf("record %q is not newline-terminated", msg)
		}
	}
}

func TestSyslogHandlerOmitsTimeAttribute(t *testing.T) {
	w := &recordingWriter{}
	logger := slog.New(newSyslogTextHandler(w, slog.LevelInfo))

	logger.Info("fansd started", "host", "10.0.0.1")

	if len(w.writes) != 1 {
		t.Fatalf("got %d writes, want 1", len(w.writes))
	}
	if strings.Contains(w.writes[0], "time=") {
		t.Fatalf("record %q contains time attribute; syslogd adds its own timestamp", w.writes[0])
	}
	if !strings.Contains(w.writes[0], "msg=\"fansd started\"") {
		t.Fatalf("record %q missing message", w.writes[0])
	}
}

func TestSyslogHandlerFiltersBelowLevel(t *testing.T) {
	w := &recordingWriter{}
	logger := slog.New(newSyslogTextHandler(w, slog.LevelInfo))

	logger.Debug("sensor reading", "name", "cpu", "value", 58)

	if len(w.writes) != 0 {
		t.Fatalf("debug record was written despite info level: %q", w.writes)
	}
}
