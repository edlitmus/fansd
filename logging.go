package main

import (
	"io"
	"log/slog"
	"log/syslog"
	"os"
)

// newLogHandler returns the handler for the process-wide logger. With
// useSyslog set, records go directly to syslogd instead of stderr: writing
// through daemon(8)'s -S pipe coalesces bursts of records into a single
// syslog line, so the daemon has to speak to syslog itself to keep one
// message per record.
func newLogHandler(level slog.Level, useSyslog bool) (slog.Handler, error) {
	if !useSyslog {
		return slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}), nil
	}
	w, err := syslog.New(syslog.LOG_DAEMON|syslog.LOG_NOTICE, "fansd")
	if err != nil {
		return nil, err
	}
	return newSyslogTextHandler(w, level), nil
}

// newSyslogTextHandler formats records as slog text without the time
// attribute, since syslogd stamps every message itself.
func newSyslogTextHandler(w io.Writer, level slog.Level) slog.Handler {
	return slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) == 0 && a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})
}
