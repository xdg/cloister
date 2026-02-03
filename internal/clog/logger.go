package clog

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Logger handles leveled logging with support for multiple outputs.
type Logger struct {
	mu         sync.Mutex
	level      Level     // minimum level to log
	fileWriter io.Writer // always receives logs at or above level
	errWriter  io.Writer // receives warn/error in CLI mode, nil in daemon mode
	daemonMode bool      // when true, errWriter is ignored
}

// NewLogger creates a new logger with default settings.
// By default, logs go to stderr at Info level.
func NewLogger() *Logger {
	return &Logger{
		level:     LevelInfo,
		errWriter: os.Stderr,
	}
}

// SetLevel sets the minimum log level.
func (l *Logger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// SetFileOutput sets the file writer for log output.
// Pass nil to disable file logging.
func (l *Logger) SetFileOutput(w io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.fileWriter = w
}

// SetErrOutput sets the stderr writer for warn/error output in CLI mode.
// Pass nil to disable stderr logging.
func (l *Logger) SetErrOutput(w io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errWriter = w
}

// SetDaemonMode enables or disables daemon mode.
// In daemon mode, logs only go to the file writer, not stderr.
func (l *Logger) SetDaemonMode(daemon bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.daemonMode = daemon
}

// Debug logs a debug message.
func (l *Logger) Debug(format string, args ...any) {
	l.log(LevelDebug, format, args...)
}

// Info logs an informational message.
func (l *Logger) Info(format string, args ...any) {
	l.log(LevelInfo, format, args...)
}

// Warn logs a warning message.
func (l *Logger) Warn(format string, args ...any) {
	l.log(LevelWarn, format, args...)
}

// Error logs an error message.
func (l *Logger) Error(format string, args ...any) {
	l.log(LevelError, format, args...)
}

// log writes a log message to the appropriate outputs.
func (l *Logger) log(level Level, format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Skip if below minimum level
	if level < l.level {
		return
	}

	// Format the message
	msg := fmt.Sprintf(format, args...)
	timestamp := time.Now().UTC().Format(time.RFC3339)
	line := fmt.Sprintf("%s [%s] %s\n", timestamp, level, msg)

	// Write to file if configured
	if l.fileWriter != nil {
		_, _ = l.fileWriter.Write([]byte(line))
	}

	// Write warn/error to stderr in CLI mode
	if !l.daemonMode && l.errWriter != nil && level >= LevelWarn {
		// For stderr, use a simpler format without timestamp
		errLine := fmt.Sprintf("[%s] %s\n", level, msg)
		_, _ = l.errWriter.Write([]byte(errLine))
	}
}

// OpenLogFile opens a log file for writing, creating parent directories if needed.
// The file is opened in append mode.
func OpenLogFile(path string) (*os.File, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0640)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	return f, nil
}

// DefaultLogPath returns the default log file path following XDG conventions.
// Returns ~/.local/state/cloister/cloister.log
func DefaultLogPath() string {
	stateDir := os.Getenv("XDG_STATE_HOME")
	if stateDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		stateDir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(stateDir, "cloister", "cloister.log")
}

// CloisterLogPath returns the log path for a specific cloister.
// Returns ~/.local/state/cloister/cloisters/<name>.log
func CloisterLogPath(name string) string {
	stateDir := os.Getenv("XDG_STATE_HOME")
	if stateDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		stateDir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(stateDir, "cloister", "cloisters", name+".log")
}
