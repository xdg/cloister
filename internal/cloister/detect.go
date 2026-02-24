package cloister

import (
	"fmt"
	"os"
	"strings"

	"github.com/xdg/cloister/internal/container"
	"github.com/xdg/cloister/internal/project"
)

// detectGitRoot is the function used to detect the git root directory.
// It is a package-level variable to allow overriding in tests.
var detectGitRoot = project.DetectGitRoot

// projectName is the function used to determine the project name from a git root.
// It is a package-level variable to allow overriding in tests.
var projectName = project.Name

// loadCloisterRegistry is the function used to load the cloister registry.
// It is a package-level variable to allow overriding in tests.
var loadCloisterRegistry func() (*Registry, error) = LoadRegistry

// getWorkingDir is the function used to get the current working directory.
// It is a package-level variable to allow overriding in tests.
var getWorkingDir = os.Getwd

// DetectName determines the cloister name based on the current working directory.
// It first checks the cloister registry for an entry whose HostPath matches the
// current working directory (or a parent of it). If no match is found, it falls
// back to detecting the git root, extracting the project name, and generating
// the corresponding cloister container name.
func DetectName() (string, error) {
	cwd, err := getWorkingDir()
	if err == nil {
		if name, found := detectNameFromRegistry(cwd); found {
			return name, nil
		}
	}

	// Fallback to project-based detection.
	gitRoot, err := detectGitRoot(".")
	if err != nil {
		return "", fmt.Errorf("detect git root: %w", err)
	}
	name, err := projectName(gitRoot)
	if err != nil {
		return "", fmt.Errorf("determine project name: %w", err)
	}
	return container.GenerateCloisterName(name), nil
}

// detectNameFromRegistry checks the cloister registry for an entry whose
// HostPath matches the given directory path. Returns the cloister name and
// true if found, or empty string and false otherwise.
func detectNameFromRegistry(cwd string) (string, bool) {
	reg, err := loadCloisterRegistry()
	if err != nil {
		return "", false
	}

	for _, entry := range reg.List() {
		if cwd == entry.HostPath || strings.HasPrefix(cwd, entry.HostPath+"/") {
			return entry.CloisterName, true
		}
	}

	return "", false
}
