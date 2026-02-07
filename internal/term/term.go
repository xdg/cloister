// Package term provides user-facing terminal output for cloister CLI.
// This is distinct from operational logging (see internal/clog).
//
// Output functions:
//   - Print/Printf/Println: Normal output to stdout (suppressed with --silent)
//   - Warn: Warnings to stderr (NOT suppressed with --silent)
//   - Error: Errors to stderr (NOT suppressed with --silent)
//
// This package exists to:
//  1. Centralize terminal output for consistent formatting
//  2. Enable --silent flag support
//  3. Allow linting to enforce no direct fmt.Print* outside this package
package term

import (
	"fmt"
	"io"
	"os"
	"sync"
)

var (
	mu     sync.Mutex
	stdout io.Writer = os.Stdout
	stderr io.Writer = os.Stderr
	silent bool
)

// SetSilent enables or disables silent mode.
// When silent, Print/Printf/Println are suppressed.
// Warn and Error are NOT suppressed (users should always see these).
func SetSilent(s bool) {
	mu.Lock()
	defer mu.Unlock()
	silent = s
}

// IsSilent returns whether silent mode is enabled.
func IsSilent() bool {
	mu.Lock()
	defer mu.Unlock()
	return silent
}

// SetOutput sets the writer for stdout output.
// Pass nil to use os.Stdout.
func SetOutput(w io.Writer) {
	mu.Lock()
	defer mu.Unlock()
	if w == nil {
		stdout = os.Stdout
	} else {
		stdout = w
	}
}

// SetErrOutput sets the writer for stderr output.
// Pass nil to use os.Stderr.
func SetErrOutput(w io.Writer) {
	mu.Lock()
	defer mu.Unlock()
	if w == nil {
		stderr = os.Stderr
	} else {
		stderr = w
	}
}

// Print formats and writes to stdout.
// Suppressed when silent mode is enabled.
func Print(a ...any) {
	mu.Lock()
	defer mu.Unlock()
	if silent {
		return
	}
	_, _ = fmt.Fprint(stdout, a...)
}

// Printf formats according to a format specifier and writes to stdout.
// Suppressed when silent mode is enabled.
func Printf(format string, a ...any) {
	mu.Lock()
	defer mu.Unlock()
	if silent {
		return
	}
	_, _ = fmt.Fprintf(stdout, format, a...)
}

// Println formats and writes to stdout with a trailing newline.
// Suppressed when silent mode is enabled.
func Println(a ...any) {
	mu.Lock()
	defer mu.Unlock()
	if silent {
		return
	}
	_, _ = fmt.Fprintln(stdout, a...)
}

// Warn writes a warning message to stderr with "Warning: " prefix.
// NOT suppressed by silent mode.
func Warn(format string, a ...any) {
	mu.Lock()
	defer mu.Unlock()
	msg := fmt.Sprintf(format, a...)
	_, _ = fmt.Fprintf(stderr, "Warning: %s\n", msg)
}

// Error writes an error message to stderr with "Error: " prefix.
// NOT suppressed by silent mode.
func Error(format string, a ...any) {
	mu.Lock()
	defer mu.Unlock()
	msg := fmt.Sprintf(format, a...)
	_, _ = fmt.Fprintf(stderr, "Error: %s\n", msg)
}

// Stdout returns the current stdout writer.
// Useful for passing to libraries that need an io.Writer (e.g., tabwriter).
func Stdout() io.Writer {
	mu.Lock()
	defer mu.Unlock()
	if silent {
		return io.Discard
	}
	return stdout
}

// Stderr returns the current stderr writer.
func Stderr() io.Writer {
	mu.Lock()
	defer mu.Unlock()
	return stderr
}

// Reset resets the package to default state.
// Primarily useful for testing.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	stdout = os.Stdout
	stderr = os.Stderr
	silent = false
}

// Discard configures the package to discard all output.
// Useful for silencing output in tests.
func Discard() {
	mu.Lock()
	defer mu.Unlock()
	stdout = io.Discard
	stderr = io.Discard
}
