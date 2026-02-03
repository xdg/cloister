// Package clog provides structured operational logging for cloister.
// This is distinct from user-facing output (see internal/term).
//
// Log levels:
//   - Debug: Verbose diagnostic information, only with --debug
//   - Info: Normal operational events
//   - Warn: Unexpected conditions that don't prevent operation
//   - Error: Failures that affect functionality
//
// Output destinations:
//   - File: All levels (debug only with --debug flag)
//   - Stderr: Warn and Error only, disabled in daemon mode
package clog

import "strings"

// Level represents the severity of a log message.
type Level int

const (
	// LevelDebug is for verbose diagnostic information.
	// Only logged when debug mode is enabled.
	LevelDebug Level = iota
	// LevelInfo is for normal operational events.
	LevelInfo
	// LevelWarn is for unexpected conditions that don't prevent operation.
	LevelWarn
	// LevelError is for failures that affect functionality.
	LevelError
)

// String returns the uppercase name of the level.
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// ParseLevel parses a level string (case-insensitive).
// Returns LevelInfo if the string is not recognized.
func ParseLevel(s string) Level {
	switch strings.ToLower(s) {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn", "warning":
		return LevelWarn
	case "error", "err":
		return LevelError
	default:
		return LevelInfo
	}
}
