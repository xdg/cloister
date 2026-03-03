// Package pathutil provides path manipulation utilities.
package pathutil

import (
	"os"
	"path/filepath"
)

// XDGStateHome returns the XDG state home directory.
// It checks XDG_STATE_HOME first, falling back to ~/.local/state per the spec.
func XDGStateHome() string {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".local", "state")
	}
	return filepath.Join(home, ".local", "state")
}

// XDGDataHome returns the XDG data home directory.
// It checks XDG_DATA_HOME first, falling back to ~/.local/share per the spec.
func XDGDataHome() string {
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".local", "share")
	}
	return filepath.Join(home, ".local", "share")
}
