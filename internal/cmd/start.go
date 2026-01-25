package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/xdg/cloister/internal/cloister"
	"github.com/xdg/cloister/internal/container"
	"github.com/xdg/cloister/internal/docker"
	"github.com/xdg/cloister/internal/project"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start a cloister container for the current project",
	Long: `Start a cloister container for the current project directory.

Detects the project from the current git repository and starts a sandboxed
container with the project mounted at /work. The guardian proxy is automatically
started if not already running.

After the container starts, an interactive shell is attached. When you exit
the shell, the container remains running. Use 'cloister stop' to terminate it.`,
	RunE: runStart,
}

func init() {
	rootCmd.AddCommand(startCmd)
}

func runStart(cmd *cobra.Command, args []string) error {
	// Step 1: Detect git root from current directory
	gitRoot, err := project.DetectGitRoot(".")
	if err != nil {
		if errors.Is(err, project.ErrNotGitRepo) {
			return fmt.Errorf("not in a git repository; cloister must be run from within a git project")
		}
		if errors.Is(err, project.ErrGitNotInstalled) {
			return fmt.Errorf("git is not installed or not in PATH")
		}
		return fmt.Errorf("failed to detect git repository: %w", err)
	}

	// Step 2: Detect branch
	branch, err := project.DetectBranch(gitRoot)
	if err != nil {
		return fmt.Errorf("failed to detect git branch: %w", err)
	}

	// Step 3: Get project name
	projectName, err := project.ProjectName(gitRoot)
	if err != nil {
		return fmt.Errorf("failed to determine project name: %w", err)
	}

	// Step 4: Start the cloister container
	containerID, tok, err := cloister.Start(cloister.StartOptions{
		ProjectPath: gitRoot,
		ProjectName: projectName,
		BranchName:  branch,
	})
	if err != nil {
		// Check for common error conditions
		if errors.Is(err, docker.ErrDockerNotRunning) {
			return fmt.Errorf("Docker is not running; please start Docker and try again")
		}
		if errors.Is(err, container.ErrContainerExists) {
			containerName := container.GenerateContainerName(projectName, branch)
			return fmt.Errorf("container %q already exists; use 'cloister stop %s' first or attach with 'docker exec -it %s /bin/bash'",
				containerName, containerName, containerName)
		}
		return fmt.Errorf("failed to start cloister: %w", err)
	}

	// Compute container name for display
	containerName := container.GenerateContainerName(projectName, branch)

	// Print startup information
	fmt.Printf("Started cloister container: %s\n", containerName)
	fmt.Printf("Container ID: %s\n", containerID[:12])
	fmt.Printf("Project: %s (branch: %s)\n", projectName, branch)
	fmt.Printf("Token: %s\n", tok)
	fmt.Println()
	fmt.Println("Attaching interactive shell...")
	fmt.Println()

	// Step 5: Attach interactive shell
	exitCode, err := cloister.Attach(containerName)
	if err != nil {
		return fmt.Errorf("failed to attach to container: %w", err)
	}

	// Step 6: Print exit message
	fmt.Println()
	fmt.Printf("Shell exited with code %d. Container still running.\n", exitCode)
	fmt.Printf("Use 'cloister stop %s' to terminate.\n", containerName)

	// Step 7: Propagate shell exit code
	if exitCode != 0 {
		os.Exit(exitCode)
	}

	return nil
}
