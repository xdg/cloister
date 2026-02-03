package clog

import (
	"io"
	"os"
)

// std is the global logger instance used by package-level functions.
var std = NewLogger()

// Configure sets up the global logger based on configuration.
// If logPath is empty, file logging is disabled.
// If debug is true, debug-level messages are logged.
// If daemonMode is true, stderr output is disabled.
func Configure(logPath string, debug bool, daemonMode bool) error {
	level := LevelInfo
	if debug {
		level = LevelDebug
	}
	std.SetLevel(level)
	std.SetDaemonMode(daemonMode)

	if logPath != "" {
		f, err := OpenLogFile(logPath)
		if err != nil {
			return err
		}
		std.SetFileOutput(f)
	}

	return nil
}

// ConfigureWithDefaults sets up the global logger with default log path.
// This is a convenience function for common initialization.
func ConfigureWithDefaults(debug bool, daemonMode bool) error {
	return Configure(DefaultLogPath(), debug, daemonMode)
}

// SetLevel sets the minimum log level for the global logger.
func SetLevel(level Level) {
	std.SetLevel(level)
}

// SetFileOutput sets the file writer for the global logger.
func SetFileOutput(w io.Writer) {
	std.SetFileOutput(w)
}

// SetErrOutput sets the stderr writer for the global logger.
func SetErrOutput(w io.Writer) {
	std.SetErrOutput(w)
}

// SetDaemonMode enables or disables daemon mode for the global logger.
func SetDaemonMode(daemon bool) {
	std.SetDaemonMode(daemon)
}

// Debug logs a debug message using the global logger.
func Debug(format string, args ...any) {
	std.Debug(format, args...)
}

// Info logs an informational message using the global logger.
func Info(format string, args ...any) {
	std.Info(format, args...)
}

// Warn logs a warning message using the global logger.
func Warn(format string, args ...any) {
	std.Warn(format, args...)
}

// Error(format string, args ...any) logs an error message using the global logger.
func Error(format string, args ...any) {
	std.Error(format, args...)
}

// Close closes the file writer if it implements io.Closer.
// This should be called during shutdown to ensure logs are flushed.
func Close() error {
	std.mu.Lock()
	defer std.mu.Unlock()

	if closer, ok := std.fileWriter.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// Reset resets the global logger to default state.
// This is primarily useful for testing.
func Reset() {
	std = NewLogger()
}

// Discard configures the global logger to discard all output.
// This is useful for silencing logs in tests.
func Discard() {
	std.SetFileOutput(io.Discard)
	std.SetErrOutput(io.Discard)
}

// TestLogger returns a logger that writes to the provided writer.
// Useful for capturing log output in tests.
func TestLogger(w io.Writer) *Logger {
	l := NewLogger()
	l.SetFileOutput(w)
	l.SetErrOutput(w)
	l.SetLevel(LevelDebug)
	return l
}

// ReplaceGlobal replaces the global logger and returns the previous one.
// Useful for testing. Caller should restore the original logger after test.
func ReplaceGlobal(l *Logger) *Logger {
	old := std
	std = l
	return old
}

// RedirectStdLog redirects the standard library's log package to clog.
// Messages are logged at Info level.
func RedirectStdLog() {
	// Create a writer that sends to clog.Info
	std.mu.Lock()
	defer std.mu.Unlock()
	// Note: This would require implementing an io.Writer adapter.
	// For now, this is a placeholder for future implementation.
}

// Writer returns an io.Writer that writes to clog at the specified level.
// This is useful for integrating with libraries that expect an io.Writer.
func Writer(level Level) io.Writer {
	return &levelWriter{level: level}
}

type levelWriter struct {
	level Level
}

func (w *levelWriter) Write(p []byte) (n int, err error) {
	msg := string(p)
	// Trim trailing newline since log functions add their own
	if len(msg) > 0 && msg[len(msg)-1] == '\n' {
		msg = msg[:len(msg)-1]
	}
	switch w.level {
	case LevelDebug:
		Debug("%s", msg)
	case LevelInfo:
		Info("%s", msg)
	case LevelWarn:
		Warn("%s", msg)
	case LevelError:
		Error("%s", msg)
	}
	return len(p), nil
}

func init() {
	// By default, only write to stderr (no file logging until Configure is called)
	std.SetErrOutput(os.Stderr)
}
