package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/xdg/cloister/internal/cloister"
	"github.com/xdg/cloister/internal/container"
	"github.com/xdg/cloister/internal/docker"
	"github.com/xdg/cloister/internal/guardian"
	"github.com/xdg/cloister/internal/term"
)

var shutdownCmd = &cobra.Command{
	Use:   "shutdown",
	Short: "Stop all cloisters and the guardian",
	Long: `Stop all running cloister containers and the guardian service.

Stops each cloister container (revoking its token), then stops the executor
and guardian. This is equivalent to running "cloister stop" on every cloister
followed by "cloister guardian stop".`,
	RunE: runShutdown,
}

func init() {
	rootCmd.AddCommand(shutdownCmd)
}

func runShutdown(_ *cobra.Command, _ []string) error {
	// Check if Docker is running
	if err := docker.CheckDaemon(); err != nil {
		if errors.Is(err, docker.ErrDockerNotRunning) {
			return dockerNotRunningError()
		}
		return fmt.Errorf("docker is not available: %w", err)
	}

	// List all cloister containers
	mgr := container.NewManager()
	containers, err := mgr.List()
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	// Stop each cloister container (excluding guardian)
	guardianPrefix := guardian.ContainerName()
	stopped := 0
	for _, c := range containers {
		if strings.HasPrefix(c.Name, guardianPrefix) {
			continue
		}

		cloisterName := container.NameToCloisterName(c.Name)
		term.Printf("Stopping cloister: %s ...\n", cloisterName)

		// Find token for this container (best-effort; guardian may be down)
		token := guardian.FindTokenForContainer(c.Name)

		if err := cloister.Stop(c.Name, token); err != nil {
			term.Warn("failed to stop %s: %v", cloisterName, err)
			continue
		}
		stopped++
	}

	if stopped > 0 {
		term.Printf("Stopped %d cloister(s)\n", stopped)
	}

	// Stop guardian (which also stops executor)
	running, err := guardian.IsRunning()
	if err != nil {
		return fmt.Errorf("failed to check guardian status: %w", err)
	}
	if running {
		term.Println("Stopping executor...")
		term.Println("Stopping guardian...")
		if err := guardian.Stop(); err != nil {
			return fmt.Errorf("failed to stop guardian: %w", err)
		}
		term.Println("Guardian stopped successfully")
	}

	return nil
}
