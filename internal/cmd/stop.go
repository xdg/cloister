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
	Use:   "stop [container-name]",
	Short: "Stop a cloister container",
	Long: `Stop a cloister container and clean up its resources.

If no container name is provided, stops the container for the current project.
Revokes the container's token from the guardian and removes the container.`,
	RunE: runStop,
}

func init() {
	rootCmd.AddCommand(stopCmd)
}

func runStop(cmd *cobra.Command, args []string) error {
	var containerName string

	if len(args) > 0 {
		// Container name provided as argument
		containerName = args[0]
	} else {
		// Default to current project
		name, err := detectContainerName()
		if err != nil {
			return err
		}
		containerName = name
	}

	// Look up the token for this container from the guardian
	token := findTokenForContainer(containerName)

	// Stop the container (this also revokes the token)
	err := cloister.Stop(containerName, token)
	if err != nil {
		if errors.Is(err, docker.ErrDockerNotRunning) {
			return fmt.Errorf("Docker is not running; please start Docker and try again")
		}
		if errors.Is(err, container.ErrContainerNotFound) {
			return fmt.Errorf("container %q not found", containerName)
		}
		return fmt.Errorf("failed to stop cloister: %w", err)
	}

	// Print confirmation message
	fmt.Printf("Stopped cloister container: %s\n", containerName)
	if token != "" {
		fmt.Println("Token revoked from guardian.")
	}

	return nil
}

// detectContainerName determines the container name based on the current project.
func detectContainerName() (string, error) {
	// Detect git root from current directory
	gitRoot, err := project.DetectGitRoot(".")
	if err != nil {
		if errors.Is(err, project.ErrNotGitRepo) {
			return "", fmt.Errorf("not in a git repository; specify container name or run from within a git project")
		}
		if errors.Is(err, project.ErrGitNotInstalled) {
			return "", fmt.Errorf("git is not installed or not in PATH")
		}
		return "", fmt.Errorf("failed to detect git repository: %w", err)
	}

	// Detect branch
	branch, err := project.DetectBranch(gitRoot)
	if err != nil {
		return "", fmt.Errorf("failed to detect git branch: %w", err)
	}

	// Get project name
	projectName, err := project.ProjectName(gitRoot)
	if err != nil {
		return "", fmt.Errorf("failed to determine project name: %w", err)
	}

	return container.GenerateContainerName(projectName, branch), nil
}

// findTokenForContainer looks up the token associated with a container name.
// Returns empty string if no token is found (container may have been started externally
// or guardian is not running).
func findTokenForContainer(containerName string) string {
	// Get all registered tokens from guardian
	tokens, err := guardian.ListTokens()
	if err != nil {
		// If we can't reach guardian, continue with empty token
		// The container stop will still work
		return ""
	}

	// Find the token for this container
	for token, cloisterName := range tokens {
		if cloisterName == containerName {
			return token
		}
	}

	// No token found for this container
	return ""
}
