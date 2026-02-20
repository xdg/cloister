package cloister

import (
	"fmt"

	"github.com/xdg/cloister/internal/container"
	"github.com/xdg/cloister/internal/project"
)

// detectGitRoot is the function used to detect the git root directory.
// It is a package-level variable to allow overriding in tests.
var detectGitRoot = project.DetectGitRoot

// projectName is the function used to determine the project name from a git root.
// It is a package-level variable to allow overriding in tests.
var projectName = project.Name

// DetectName determines the cloister name based on the current working directory.
// It detects the git root, extracts the project name, and generates the
// corresponding cloister container name.
func DetectName() (string, error) {
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
