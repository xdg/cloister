package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/xdg/cloister/internal/clog"
	"github.com/xdg/cloister/internal/container"
	"github.com/xdg/cloister/internal/docker"
	"github.com/xdg/cloister/internal/executor"
	"github.com/xdg/cloister/internal/guardian"
	"github.com/xdg/cloister/internal/term"
	"github.com/xdg/cloister/internal/token"
)

var guardianCmd = &cobra.Command{
	Use:   "guardian",
	Short: "Manage the guardian proxy service",
	Long: `Manage the guardian proxy service that provides allowlist-enforced network
access for cloister containers.

The guardian runs as a separate container and provides:
- HTTP CONNECT proxy with domain allowlist/denylist
- Per-cloister token authentication
- Host command execution approval via web UI

Domain allowlist and denylist decisions are persisted in ~/.config/cloister/decisions/
using global.yaml for global decisions and projects/<name>.yaml for per-project decisions.`,
}

var guardianStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the guardian service",
	Long:  `Start the guardian container if not already running.`,
	RunE: func(_ *cobra.Command, _ []string) error {
		// Check if Docker is running
		if err := docker.CheckDaemon(); err != nil {
			if errors.Is(err, docker.ErrDockerNotRunning) {
				return dockerNotRunningError()
			}
			return fmt.Errorf("docker is not available: %w", err)
		}

		// Check if already running
		running, err := guardian.IsRunning()
		if err != nil {
			return fmt.Errorf("failed to check guardian status: %w", err)
		}
		if running {
			term.Println("Guardian is already running")
			return nil
		}

		term.Println("Starting executor...")
		term.Println("Starting guardian...")

		if err := guardian.EnsureRunning(); err != nil {
			if errors.Is(err, guardian.ErrGuardianAlreadyRunning) {
				term.Println("Guardian is already running")
				return nil
			}
			return fmt.Errorf("failed to start guardian: %w", err)
		}

		term.Println("Guardian started successfully")
		return nil
	},
}

var guardianStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the guardian service",
	Long: `Stop the guardian container.

Warns if there are running cloister containers that depend on the guardian.`,
	RunE: func(_ *cobra.Command, _ []string) error {
		// Check if Docker is running
		if err := docker.CheckDaemon(); err != nil {
			if errors.Is(err, docker.ErrDockerNotRunning) {
				return dockerNotRunningError()
			}
			return fmt.Errorf("docker is not available: %w", err)
		}

		// Check if guardian is running
		running, err := guardian.IsRunning()
		if err != nil {
			return fmt.Errorf("failed to check guardian status: %w", err)
		}
		if !running {
			term.Println("Guardian is not running")
			return nil
		}

		// Check for running cloister containers and warn
		mgr := container.NewManager()
		containers, err := mgr.List()
		if err != nil {
			return fmt.Errorf("failed to list containers: %w", err)
		}

		// Filter for running cloisters (exclude guardian itself)
		var runningCloisters []string
		for _, c := range containers {
			if c.Name != guardian.ContainerName() && c.State == "running" {
				runningCloisters = append(runningCloisters, c.Name)
			}
		}

		if len(runningCloisters) > 0 {
			term.Warn("%d cloister container(s) are still running and will lose network access:", len(runningCloisters))
			for _, name := range runningCloisters {
				term.Printf("  - %s\n", name)
			}
			term.Println()
		}

		term.Println("Stopping executor...")
		term.Println("Stopping guardian...")

		if err := guardian.Stop(); err != nil {
			return fmt.Errorf("failed to stop guardian: %w", err)
		}

		term.Println("Guardian stopped successfully")
		return nil
	},
}

var guardianStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show guardian service status",
	Long:  `Show guardian status including uptime and active token count.`,
	RunE: func(_ *cobra.Command, _ []string) error {
		// Check if Docker is running
		if err := docker.CheckDaemon(); err != nil {
			if errors.Is(err, docker.ErrDockerNotRunning) {
				return dockerNotRunningError()
			}
			return fmt.Errorf("docker is not available: %w", err)
		}

		// Check if guardian is running
		running, err := guardian.IsRunning()
		if err != nil {
			return fmt.Errorf("failed to check guardian status: %w", err)
		}

		if !running {
			term.Println("Status: not running")
			return nil
		}

		term.Println("Status: running")

		// Get container details for uptime
		uptime, err := getGuardianUptime()
		if err == nil && uptime != "" {
			term.Printf("Uptime: %s\n", uptime)
		}

		// Get active token count
		tokens, err := guardian.ListTokens()
		if err != nil {
			term.Printf("Active tokens: (unable to retrieve: %v)\n", err)
		} else {
			term.Printf("Active tokens: %d\n", len(tokens))
		}

		// Check executor status
		state, err := executor.LoadDaemonState()
		if err != nil {
			term.Printf("Executor: (unable to retrieve: %v)\n", err)
		} else if state == nil {
			term.Println("Executor: not running")
		} else if executor.IsDaemonRunning(state) {
			term.Printf("Executor: running (PID %d)\n", state.PID)
		} else {
			term.Println("Executor: not running (stale state)")
		}

		return nil
	},
}

var guardianReloadCmd = &cobra.Command{
	Use:   "reload",
	Short: "Reload guardian configuration",
	Long: `Send SIGHUP to the guardian container to reload configuration from disk.

This reloads the global config, domain allowlist/denylist decisions, and
per-project decisions without restarting the guardian.`,
	RunE: func(_ *cobra.Command, _ []string) error {
		// Check if Docker is running
		if err := docker.CheckDaemon(); err != nil {
			if errors.Is(err, docker.ErrDockerNotRunning) {
				return dockerNotRunningError()
			}
			return fmt.Errorf("docker is not available: %w", err)
		}

		if err := guardian.Reload(); err != nil {
			return err
		}

		term.Println("Guardian configuration reloaded")
		return nil
	},
}

var guardianRunCmd = &cobra.Command{
	Use:    "run",
	Short:  "Run the guardian proxy server (internal)",
	Hidden: true,
	Long: `Run the guardian proxy and API servers.

This command is intended to be run inside the guardian container and should
not normally be invoked directly by users.

The proxy server (port 3128):
- Listens for HTTP CONNECT requests
- Enforces the domain allowlist
- Validates per-cloister authentication tokens

The API server (port 9997):
- Provides token registration/revocation endpoints
- Used by the CLI to manage tokens

Both servers block until interrupted (SIGINT/SIGTERM).`,
	RunE: runGuardianProxy,
}

func init() {
	guardianCmd.AddCommand(guardianStartCmd)
	guardianCmd.AddCommand(guardianStopCmd)
	guardianCmd.AddCommand(guardianStatusCmd)
	guardianCmd.AddCommand(guardianReloadCmd)
	guardianCmd.AddCommand(guardianRunCmd)
	rootCmd.AddCommand(guardianCmd)
}

// runGuardianProxy starts the proxy and API servers and blocks until interrupted.
func runGuardianProxy(_ *cobra.Command, _ []string) error {
	clog.SetDaemonMode(true)

	registry := token.NewRegistry()
	guardian.LoadPersistedTokens(registry)

	cfg := guardian.LoadGuardianConfig()
	decisions := guardian.LoadGuardianDecisions()

	srv, err := guardian.NewServer(registry, cfg, decisions)
	if err != nil {
		return err
	}

	return srv.Run()
}

// getGuardianUptime returns the human-readable uptime of the guardian container.
// It uses docker inspect to get the StartedAt time and calculates the duration.
func getGuardianUptime() (string, error) {
	// Use docker inspect to get the container state
	output, err := docker.Run("inspect", guardian.ContainerName())
	if err != nil {
		return "", err
	}

	// Parse the JSON output (docker inspect returns an array)
	var containers []struct {
		State struct {
			StartedAt string `json:"StartedAt"`
		} `json:"State"`
	}

	output = strings.TrimSpace(output)
	if output == "" {
		return "", fmt.Errorf("empty inspect output")
	}

	if err := json.Unmarshal([]byte(output), &containers); err != nil {
		return "", fmt.Errorf("failed to parse inspect output: %w", err)
	}

	if len(containers) == 0 {
		return "", fmt.Errorf("no container found")
	}

	startedAt := containers[0].State.StartedAt
	if startedAt == "" {
		return "", fmt.Errorf("no StartedAt time found")
	}

	// Parse the time
	startTime, err := time.Parse(time.RFC3339Nano, startedAt)
	if err != nil {
		return "", fmt.Errorf("failed to parse StartedAt time: %w", err)
	}

	// Calculate uptime
	uptime := time.Since(startTime)

	// Format as human-readable duration
	return formatDuration(uptime), nil
}

// formatDuration formats a duration in a human-readable way.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		secs := int(d.Seconds()) % 60
		if secs > 0 {
			return fmt.Sprintf("%d minutes, %d seconds", mins, secs)
		}
		return fmt.Sprintf("%d minutes", mins)
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		mins := int(d.Minutes()) % 60
		if mins > 0 {
			return fmt.Sprintf("%d hours, %d minutes", hours, mins)
		}
		return fmt.Sprintf("%d hours", hours)
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	if hours > 0 {
		return fmt.Sprintf("%d days, %d hours", days, hours)
	}
	return fmt.Sprintf("%d days", days)
}
