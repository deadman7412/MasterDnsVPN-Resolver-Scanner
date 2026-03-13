// Package logger provides structured output for the scanner.
//
// Every message is written to stdout using the project's bracketed tag format.
// When a log file is configured, every message is also written to that file
// with a millisecond-precision timestamp prepended. The file is the debug
// artifact; stdout is the user-facing surface.
//
// File logging is off by default. Enable it with --log <path> or --log auto.
// "auto" generates a timestamped filename in the current directory.
//
// All methods are safe for concurrent use.
package logger

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// Logger writes tagged messages to stdout and optionally to a log file.
type Logger struct {
	mu     sync.Mutex
	file   *os.File
	stdout io.Writer
}

// New creates a Logger. path controls file logging:
//
//	""     — stdout only, no file (default)
//	"auto" — creates scanner-YYYY-MM-DDTHH-MM-SS.log in the current directory
//	other  — treated as an explicit file path
func New(path string) (*Logger, error) {
	l := &Logger{stdout: os.Stdout}
	if path == "" {
		return l, nil
	}
	if path == "auto" {
		path = fmt.Sprintf("scanner-%s.log", time.Now().Format("2006-01-02T15-04-05"))
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	l.file = f
	return l, nil
}

// LogPath returns the path of the log file, or empty string if not logging to file.
func (l *Logger) LogPath() string {
	if l.file == nil {
		return ""
	}
	return l.file.Name()
}

// SessionStart writes a header block to the log file marking the start of a
// run. fields is a flat list of alternating key, value pairs.
// Has no effect if file logging is disabled.
func (l *Logger) SessionStart(fields ...string) {
	if l.file == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	ts := time.Now().Format("2006-01-02 15:04:05")
	sep := strings.Repeat("-", 60)
	fmt.Fprintf(l.file, "%s\n", sep)
	fmt.Fprintf(l.file, "MasterDnsVPN Resolver Scanner\n")
	fmt.Fprintf(l.file, "session started: %s\n", ts)
	for i := 0; i+1 < len(fields); i += 2 {
		fmt.Fprintf(l.file, "  %-10s %s\n", fields[i]+":", fields[i+1])
	}
	fmt.Fprintf(l.file, "%s\n", sep)
}

// SessionEnd writes a footer block to the log file. Has no effect if file
// logging is disabled.
func (l *Logger) SessionEnd() {
	if l.file == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	ts := time.Now().Format("2006-01-02 15:04:05")
	sep := strings.Repeat("-", 60)
	fmt.Fprintf(l.file, "session ended:  %s\n", ts)
	fmt.Fprintf(l.file, "%s\n", sep)
}

// Close flushes and closes the log file. Safe to call when file logging is
// disabled (no-op).
func (l *Logger) Close() {
	if l.file != nil {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.file.Close()
	}
}

// write is the internal sink. Writes untagged to stdout, timestamped to file.
func (l *Logger) write(tag, msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintf(l.stdout, "[%s] %s\n", tag, msg)
	if l.file != nil {
		ts := time.Now().Format("2006-01-02 15:04:05.000")
		fmt.Fprintf(l.file, "%s [%-7s] %s\n", ts, tag, msg)
	}
}

// Info logs an informational message. Use for general status updates.
func (l *Logger) Info(format string, args ...any) {
	l.write("info", fmt.Sprintf(format, args...))
}

// Ok logs a successful per-item result (e.g. a resolver that passed a stage).
func (l *Logger) Ok(format string, args ...any) {
	l.write("ok", fmt.Sprintf(format, args...))
}

// Success logs a top-level success (e.g. scan completed, resolver fully validated).
func (l *Logger) Success(format string, args ...any) {
	l.write("success", fmt.Sprintf(format, args...))
}

// Warn logs a non-fatal warning that the user should be aware of.
func (l *Logger) Warn(format string, args ...any) {
	l.write("warn", fmt.Sprintf(format, args...))
}

// Error logs an error. Use when something failed but the scan can continue.
func (l *Logger) Error(format string, args ...any) {
	l.write("error", fmt.Sprintf(format, args...))
}

// Fail logs a per-item failure (e.g. a resolver that failed a stage).
func (l *Logger) Fail(format string, args ...any) {
	l.write("fail", fmt.Sprintf(format, args...))
}

// Skip logs a skipped item (e.g. an IP that was not tested due to early exit).
func (l *Logger) Skip(format string, args ...any) {
	l.write("skip", fmt.Sprintf(format, args...))
}
