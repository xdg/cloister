package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/xdg/cloister/internal/cloister"
	"github.com/xdg/cloister/internal/container"
	"github.com/xdg/cloister/internal/docker"
	"github.com/xdg/cloister/internal/guardian"
	"github.com/xdg/cloister/internal/project"
)

var stopCmd = &cobra.Command{
	Use:   "stop [cloister-name]",
	Short: "Stop a cloister",
	Long: `Stop a cloister and clean up its resources.

If no cloister name is provided, stops the cloister for the current project.
Revokes the cloister's token from the guardian and removes the container.`,
	RunE: runStop,
}

func init() {
	rootCmd.AddCommand(stopCmd)
}

func runStop(cmd *cobra.Command, args []string) error {
	var cloisterName string

	if len(args) > 0 {
		// Cloister name provided as argument
		cloisterName = args[0]
	} else {
		// Default to current project
		name, err := detectCloisterName()
		if err != nil {
			return err
		}
		cloisterName = name
	}

	// Convert cloister name to container name for Docker operations
	containerName := container.CloisterNameToContainerName(cloisterName)

	// Look up the token for this cloister from the guardian
	token := findTokenForCloister(containerName)

	// Stop the container (this also revokes the token)
	err := cloister.Stop(containerName, token)
	if err != nil {
		if errors.Is(err, docker.ErrDockerNotRunning) {
			return fmt.Errorf("Docker is not running; please start Docker and try again")
		}
		if errors.Is(err, container.ErrContainerNotFound) {
			return fmt.Errorf("cloister %q not found", cloisterName)
		}
		return fmt.Errorf("failed to stop cloister: %w", err)
	}

	// Print confirmation message
	fmt.Printf("Stopped cloister: %s\n", cloisterName)
	if token != "" {
		fmt.Println("Token revoked from guardian.")
	}

	return nil
}

// detectCloisterName determines the cloister name based on the current project.
func detectCloisterName() (string, error) {
	// Detect git root from current directory
	gitRoot, err := project.DetectGitRoot(".")
	if err != nil {
		if errors.Is(err, project.ErrNotGitRepo) {
			return "", fmt.Errorf("not in a git repository; specify cloister name or run from within a git project")
		}
		if errors.Is(err, project.ErrGitNotInstalled) {
			return "", fmt.Errorf("git is not installed or not in PATH")
		}
		return "", fmt.Errorf("failed to detect git repository: %w", err)
	}

	// Get project name
	projectName, err := project.ProjectName(gitRoot)
	if err != nil {
		return "", fmt.Errorf("failed to determine project name: %w", err)
	}

	return container.GenerateCloisterName(projectName), nil
}

// findTokenForCloister looks up the token associated with a cloister (by container name).
// Returns empty string if no token is found (cloister may have been started externally
// or guardian is not running).
func findTokenForCloister(containerName string) string {
	// Get all registered tokens from guardian
	tokens, err := guardian.ListTokens()
	if err != nil {
		// If we can't reach guardian, continue with empty token
		// The container stop will still work
		return ""
	}

	// Find the token for this cloister (tokens are keyed by container name)
	for token, name := range tokens {
		if name == containerName {
			return token
		}
	}

	// No token found for this cloister
	return ""
}
