package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/xdg/cloister/internal/agent"
	"github.com/xdg/cloister/internal/clog"
	"github.com/xdg/cloister/internal/cloister"
	"github.com/xdg/cloister/internal/config"
	"github.com/xdg/cloister/internal/container"
	"github.com/xdg/cloister/internal/docker"
	"github.com/xdg/cloister/internal/project"
	"github.com/xdg/cloister/internal/term"
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

// startAgentFlag holds the --agent flag value.
var startAgentFlag string

func init() {
	rootCmd.AddCommand(startCmd)
	startCmd.Flags().StringVar(&startAgentFlag, "agent", "", "AI agent to use (e.g., claude, codex). Overrides config default.")
}

func runStart(_ *cobra.Command, _ []string) error {
	// Step 0a: Validate agent flag if provided
	if startAgentFlag != "" {
		if agent.Get(startAgentFlag) == nil {
			availableAgents := agent.List()
			return fmt.Errorf("unknown agent %q. Available agents: %s", startAgentFlag, strings.Join(availableAgents, ", "))
		}
	}

	// Step 0b: Ensure config exists (creates default if missing)
	// This must happen before starting the guardian so the config file
	// exists when mounted into the container.
	globalCfg, err := config.LoadGlobalConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Step 1: Detect git root from current directory
	gitRoot, err := project.DetectGitRoot(".")
	if err != nil {
		if gitErr := gitDetectionError(err); gitErr != nil {
			return gitErr
		}
		return fmt.Errorf("failed to detect git repository: %w", err)
	}

	// Step 2: Detect branch
	branch, err := project.DetectBranch(gitRoot)
	if err != nil {
		return fmt.Errorf("failed to detect git branch: %w", err)
	}

	// Step 3: Get project name and remote URL
	projectName, err := project.Name(gitRoot)
	if err != nil {
		return fmt.Errorf("failed to determine project name: %w", err)
	}
	remoteURL := project.GetRemoteURL(gitRoot)

	// Step 4: Auto-register project in registry
	autoRegisterProject(projectName, gitRoot, remoteURL, branch)

	// Compute cloister name (user-facing) and container name (Docker internal)
	cloisterName := container.GenerateCloisterName(projectName)
	containerName := container.CloisterNameToContainerName(cloisterName)

	// Step 5: Start the cloister container
	_, tok, err := cloister.Start(cloister.StartOptions{
		ProjectPath: gitRoot,
		ProjectName: projectName,
		BranchName:  branch,
		Agent:       startAgentFlag,
	}, cloister.WithGlobalConfig(globalCfg))
	if err != nil {
		return handleStartError(err, containerName)
	}

	// Print startup information
	term.Printf("Started cloister: %s\n", cloisterName)
	term.Printf("Project: %s (branch: %s)\n", projectName, branch)
	term.Printf("Token: %s\n", tok)
	term.Println()
	term.Println("Attaching interactive shell...")
	term.Println()

	// Step 6: Attach interactive shell
	exitCode, err := cloister.Attach(containerName)
	if err != nil {
		return fmt.Errorf("failed to attach to cloister: %w", err)
	}

	// Step 7: Print exit message
	term.Println()
	term.Printf("Shell exited with code %d. Cloister still running.\n", exitCode)
	term.Printf("Use 'cloister stop %s' to terminate.\n", cloisterName)

	// Step 8: Propagate shell exit code
	if exitCode != 0 {
		return NewExitCodeError(exitCode)
	}

	return nil
}

// attachToExisting attaches to an existing cloister, starting it if necessary.
// containerName is the Docker container name (with "cloister-" prefix).
func attachToExisting(containerName string) error {
	mgr := container.NewManager()

	// Derive user-facing cloister name from container name
	cloisterName := container.NameToCloisterName(containerName)

	// Check if the container is running
	running, err := mgr.IsRunning(containerName)
	if err != nil {
		return fmt.Errorf("failed to check cloister status: %w", err)
	}

	if !running {
		// Container exists but is stopped - start it first
		term.Printf("Starting stopped cloister: %s\n", cloisterName)
		if err := mgr.StartContainer(containerName); err != nil {
			return fmt.Errorf("failed to start cloister: %w", err)
		}
	}

	// Attach to the cloister
	term.Printf("Entering cloister %s. Type 'exit' to leave.\n", cloisterName)
	term.Println()

	exitCode, err := cloister.Attach(containerName)
	if err != nil {
		return fmt.Errorf("failed to attach to cloister: %w", err)
	}

	// Print exit message
	term.Println()
	term.Printf("Shell exited with code %d. Cloister still running.\n", exitCode)
	term.Printf("Use 'cloister stop %s' to terminate.\n", cloisterName)

	// Propagate shell exit code
	if exitCode != 0 {
		return NewExitCodeError(exitCode)
	}

	return nil
}

// autoRegisterProject registers the project in the project registry (best effort).
func autoRegisterProject(projectName, gitRoot, remoteURL, branch string) {
	reg, err := project.LoadRegistry()
	if err != nil {
		clog.Warn("failed to load project registry: %v", err)
		return
	}
	info := &project.Info{
		Name:   projectName,
		Root:   gitRoot,
		Remote: remoteURL,
		Branch: branch,
	}
	if err := reg.Register(info); err != nil {
		var collisionErr *project.NameCollisionError
		if errors.As(err, &collisionErr) {
			clog.Warn("%v", err)
		} else {
			clog.Warn("failed to register project: %v", err)
		}
		return
	}
	if err := project.SaveRegistry(reg); err != nil {
		clog.Warn("failed to save project registry: %v", err)
	}
}

// handleStartError maps cloister.Start errors to user-friendly messages.
func handleStartError(err error, containerName string) error {
	if errors.Is(err, docker.ErrDockerNotRunning) {
		return dockerNotRunningError()
	}
	if errors.Is(err, container.ErrContainerExists) {
		return attachToExisting(containerName)
	}
	return guardianErrorHint(err)
}

// guardianErrorHint wraps guardian failures with actionable hints.
func guardianErrorHint(err error) error {
	errStr := err.Error()
	if !strings.Contains(errStr, "guardian failed to start") {
		return fmt.Errorf("failed to start cloister: %w", err)
	}
	if strings.Contains(errStr, "address already in use") || strings.Contains(errStr, "port is already allocated") {
		return fmt.Errorf("%w\n\nHint: Port 9997 may be in use. Check if another guardian is running:\n  docker ps -a --filter name=cloister-guardian", err)
	}
	if strings.Contains(errStr, "No such image") || strings.Contains(errStr, "Unable to find image") {
		return fmt.Errorf("%w\n\nhint: run 'docker build -t cloister:latest .' to build the image", err)
	}
	return fmt.Errorf("%w\n\nHint: Check guardian status with:\n  cloister guardian status\n  docker logs cloister-guardian", err)
}
