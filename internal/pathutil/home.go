// Package pathutil provides path manipulation utilities.
package pathutil

import (
	"os"
	"path/filepath"
	"strings"
)

// ExpandHome replaces a leading ~ in path with the user's home directory.
// If the home directory cannot be determined, the path is returned unchanged.
func ExpandHome(path string) string {
	if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return home
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
