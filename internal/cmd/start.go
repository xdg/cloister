package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/xdg/cloister/internal/cloister"
	"github.com/xdg/cloister/internal/container"
	"github.com/xdg/cloister/internal/docker"
	"github.com/xdg/cloister/internal/project"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start a cloister for the current project",
	Long: `Start a cloister for the current project directory.

Detects the project from the current git repository and starts a sandboxed
cloister with the project mounted at /work. The guardian proxy is automatically
started if not already running.

After the cloister starts, an interactive shell is attached. When you exit
the shell, the cloister remains running. Use 'cloister stop' to terminate it.`,
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

	// Compute cloister name (user-facing) and container name (Docker internal)
	cloisterName := container.GenerateCloisterName(projectName)
	containerName := container.CloisterNameToContainerName(cloisterName)

	// Step 4: Start the cloister container
	_, tok, err := cloister.Start(cloister.StartOptions{
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
			// Container already exists - attach to it instead of erroring
			return attachToExisting(containerName)
		}
		// Check if this is a guardian failure and provide actionable guidance
		errStr := err.Error()
		if strings.Contains(errStr, "guardian failed to start") {
			// Detect common causes and provide specific guidance
			if strings.Contains(errStr, "address already in use") || strings.Contains(errStr, "port is already allocated") {
				return fmt.Errorf("%w\n\nHint: Port 9997 may be in use. Check if another guardian is running:\n  docker ps -a --filter name=cloister-guardian", err)
			}
			if strings.Contains(errStr, "No such image") || strings.Contains(errStr, "Unable to find image") {
				return fmt.Errorf("%w\n\nHint: The cloister image may not be built. Try:\n  docker build -t cloister:latest .", err)
			}
			// Generic guardian failure message
			return fmt.Errorf("%w\n\nHint: Check guardian status with:\n  cloister guardian status\n  docker logs cloister-guardian", err)
		}
		return fmt.Errorf("failed to start cloister: %w", err)
	}

	// Print startup information
	fmt.Printf("Started cloister: %s\n", cloisterName)
	fmt.Printf("Project: %s (branch: %s)\n", projectName, branch)
	fmt.Printf("Token: %s\n", tok)
	fmt.Println()
	fmt.Println("Attaching interactive shell...")
	fmt.Println()

	// Step 5: Attach interactive shell
	exitCode, err := cloister.Attach(containerName)
	if err != nil {
		return fmt.Errorf("failed to attach to cloister: %w", err)
	}

	// Step 6: Print exit message
	fmt.Println()
	fmt.Printf("Shell exited with code %d. Cloister still running.\n", exitCode)
	fmt.Printf("Use 'cloister stop %s' to terminate.\n", cloisterName)

	// Step 7: Propagate shell exit code
	if exitCode != 0 {
		os.Exit(exitCode)
	}

	return nil
}

// attachToExisting attaches to an existing cloister, starting it if necessary.
// containerName is the Docker container name (with "cloister-" prefix).
func attachToExisting(containerName string) error {
	mgr := container.NewManager()

	// Derive user-facing cloister name from container name
	cloisterName := container.ContainerNameToCloisterName(containerName)

	// Check if the container is running
	running, err := mgr.IsRunning(containerName)
	if err != nil {
		return fmt.Errorf("failed to check cloister status: %w", err)
	}

	if !running {
		// Container exists but is stopped - start it first
		fmt.Printf("Starting stopped cloister: %s\n", cloisterName)
		if err := mgr.StartContainer(containerName); err != nil {
			return fmt.Errorf("failed to start cloister: %w", err)
		}
	}

	// Attach to the cloister
	fmt.Printf("Entering cloister %s. Type 'exit' to leave.\n", cloisterName)
	fmt.Println()

	exitCode, err := cloister.Attach(containerName)
	if err != nil {
		return fmt.Errorf("failed to attach to cloister: %w", err)
	}

	// Print exit message
	fmt.Println()
	fmt.Printf("Shell exited with code %d. Cloister still running.\n", exitCode)
	fmt.Printf("Use 'cloister stop %s' to terminate.\n", cloisterName)

	// Propagate shell exit code
	if exitCode != 0 {
		os.Exit(exitCode)
	}

	return nil
}
