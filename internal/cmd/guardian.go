package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/xdg/cloister/internal/config"
	"github.com/xdg/cloister/internal/container"
	"github.com/xdg/cloister/internal/docker"
	"github.com/xdg/cloister/internal/guardian"
	"github.com/xdg/cloister/internal/guardian/patterns"
	"github.com/xdg/cloister/internal/guardian/request"
	"github.com/xdg/cloister/internal/token"
)

var guardianCmd = &cobra.Command{
	Use:   "guardian",
	Short: "Manage the guardian proxy service",
	Long: `Manage the guardian proxy service that provides allowlist-enforced network
access for cloister containers.

The guardian runs as a separate container and provides:
- HTTP CONNECT proxy with domain allowlist
- Per-cloister token authentication
- Host command execution approval (future)`,
}

var guardianStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the guardian service",
	Long:  `Start the guardian container if not already running.`,
	RunE: func(cmd *cobra.Command, args []string) error {
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
			fmt.Println("Guardian is already running")
			return nil
		}

		// Start the guardian
		fmt.Println("Starting guardian...")
		if err := guardian.Start(); err != nil {
			if errors.Is(err, guardian.ErrGuardianAlreadyRunning) {
				fmt.Println("Guardian is already running")
				return nil
			}
			return fmt.Errorf("failed to start guardian: %w", err)
		}

		fmt.Println("Guardian started successfully")
		return nil
	},
}

var guardianStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the guardian service",
	Long: `Stop the guardian container.

Warns if there are running cloister containers that depend on the guardian.`,
	RunE: func(cmd *cobra.Command, args []string) error {
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
			fmt.Println("Guardian is not running")
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
			if c.Name != guardian.ContainerName && c.State == "running" {
				runningCloisters = append(runningCloisters, c.Name)
			}
		}

		if len(runningCloisters) > 0 {
			fmt.Fprintf(os.Stderr, "Warning: %d cloister container(s) are still running and will lose network access:\n", len(runningCloisters))
			for _, name := range runningCloisters {
				fmt.Fprintf(os.Stderr, "  - %s\n", name)
			}
			fmt.Fprintln(os.Stderr)
		}

		// Stop the guardian
		fmt.Println("Stopping guardian...")
		if err := guardian.Stop(); err != nil {
			return fmt.Errorf("failed to stop guardian: %w", err)
		}

		fmt.Println("Guardian stopped successfully")
		return nil
	},
}

var guardianStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show guardian service status",
	Long:  `Show guardian status including uptime and active token count.`,
	RunE: func(cmd *cobra.Command, args []string) error {
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
			fmt.Println("Status: not running")
			return nil
		}

		fmt.Println("Status: running")

		// Get container details for uptime
		uptime, err := getGuardianUptime()
		if err == nil && uptime != "" {
			fmt.Printf("Uptime: %s\n", uptime)
		}

		// Get active token count
		tokens, err := guardian.ListTokens()
		if err != nil {
			fmt.Printf("Active tokens: (unable to retrieve: %v)\n", err)
		} else {
			fmt.Printf("Active tokens: %d\n", len(tokens))
		}

		return nil
	},
}

var guardianRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the guardian proxy server (internal)",
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
	guardianCmd.AddCommand(guardianRunCmd)
	rootCmd.AddCommand(guardianCmd)
}

// runGuardianProxy starts the proxy and API servers and blocks until interrupted.
func runGuardianProxy(cmd *cobra.Command, args []string) error {
	// Create an in-memory token registry
	registry := token.NewRegistry()

	// Load persisted tokens from disk (mounted from host)
	store, err := token.NewStore(guardian.ContainerTokenDir)
	if err != nil {
		log.Printf("Warning: failed to open token store: %v", err)
	} else {
		tokens, err := store.Load()
		if err != nil {
			log.Printf("Warning: failed to load tokens: %v", err)
		} else {
			for tok, info := range tokens {
				registry.RegisterWithProject(tok, info.CloisterName, info.ProjectName)
			}
			if len(tokens) > 0 {
				log.Printf("Recovered %d tokens from disk", len(tokens))
			}
		}
	}

	// Load config and create allowlist
	cfg, err := config.LoadGlobalConfig()
	if err != nil {
		log.Printf("Warning: failed to load config, using defaults: %v", err)
		cfg = config.DefaultGlobalConfig()
	}
	globalAllowlist := guardian.NewAllowlistFromConfig(cfg.Proxy.Allow)
	log.Printf("Loaded global allowlist with %d domains", len(globalAllowlist.Domains()))

	// Create allowlist cache for per-project allowlists
	allowlistCache := guardian.NewAllowlistCache(globalAllowlist)

	// Create the proxy server with config-derived allowlist
	proxyAddr := fmt.Sprintf(":%d", guardian.DefaultProxyPort)
	proxy := guardian.NewProxyServerWithConfig(proxyAddr, globalAllowlist)
	proxy.TokenValidator = registry
	proxy.Logger = log.New(os.Stderr, "[guardian] ", log.LstdFlags)

	// Set up per-project allowlist support
	proxy.AllowlistCache = allowlistCache
	proxy.TokenLookup = func(token string) (string, bool) {
		info, ok := registry.Lookup(token)
		return info.ProjectName, ok
	}

	// Create project allowlist loader that merges project config with global
	loadProjectAllowlist := func(projectName string) *guardian.Allowlist {
		projectCfg, err := config.LoadProjectConfig(projectName)
		if err != nil {
			log.Printf("Warning: failed to load project config for %s: %v", projectName, err)
			return nil
		}
		if len(projectCfg.Proxy.Allow) == 0 {
			return nil
		}
		// Merge global + project allowlists
		merged := config.MergeAllowlists(cfg.Proxy.Allow, projectCfg.Proxy.Allow)
		allowlist := guardian.NewAllowlistFromConfig(merged)
		log.Printf("Loaded allowlist for project %s (%d domains)", projectName, len(allowlist.Domains()))
		return allowlist
	}

	// Set the project loader for lazy loading
	allowlistCache.SetProjectLoader(loadProjectAllowlist)

	// Set up config reloader for SIGHUP
	proxy.SetConfigReloader(func() (*guardian.Allowlist, error) {
		newCfg, err := config.LoadGlobalConfig()
		if err != nil {
			return nil, err
		}
		// Update the cached global config for project merging
		cfg = newCfg
		newGlobal := guardian.NewAllowlistFromConfig(newCfg.Proxy.Allow)
		allowlistCache.SetGlobal(newGlobal)
		// Clear project cache so they get reloaded with new global
		allowlistCache.Clear()
		// Reload all project allowlists
		for _, info := range registry.List() {
			if info.ProjectName != "" {
				allowlist := loadProjectAllowlist(info.ProjectName)
				allowlistCache.SetProject(info.ProjectName, allowlist)
			}
		}
		return newGlobal, nil
	})

	// Create the API server
	apiAddr := fmt.Sprintf(":%d", guardian.DefaultAPIPort)
	api := guardian.NewAPIServer(apiAddr, &registryAdapter{registry})

	// Create the request server for hostexec commands
	// Token lookup adapter for the request server
	requestTokenLookup := func(tok string) (request.TokenInfo, bool) {
		info, ok := registry.Lookup(tok)
		if !ok {
			return request.TokenInfo{}, false
		}
		return request.TokenInfo{
			CloisterName: info.CloisterName,
			ProjectName:  info.ProjectName,
		}, true
	}

	// Create pattern matcher from global config approval patterns
	autoApprovePatterns := extractPatterns(cfg.Approval.AutoApprove)
	manualApprovePatterns := extractPatterns(cfg.Approval.ManualApprove)
	regexMatcher := patterns.NewRegexMatcher(autoApprovePatterns, manualApprovePatterns)
	log.Printf("Loaded approval patterns: %d auto-approve, %d manual-approve",
		len(autoApprovePatterns), len(manualApprovePatterns))

	// Executor is nil for now (Phase 4.4)
	reqServer := request.NewServer(requestTokenLookup, regexMatcher, nil)

	// Start the proxy server
	if err := proxy.Start(); err != nil {
		return fmt.Errorf("failed to start proxy server: %w", err)
	}

	log.Printf("Guardian proxy server listening on %s", proxy.ListenAddr())

	// Start the API server
	if err := api.Start(); err != nil {
		// Clean up proxy server on failure
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = proxy.Stop(ctx)
		return fmt.Errorf("failed to start API server: %w", err)
	}

	log.Printf("Guardian API server listening on %s", api.ListenAddr())

	// Start the request server for hostexec commands
	if err := reqServer.Start(); err != nil {
		// Clean up other servers on failure
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = proxy.Stop(ctx)
		_ = api.Stop(ctx)
		return fmt.Errorf("failed to start request server: %w", err)
	}

	log.Printf("Guardian request server listening on %s", reqServer.ListenAddr())

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down guardian servers...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop all servers
	var proxyErr, apiErr, reqErr error
	proxyErr = proxy.Stop(ctx)
	apiErr = api.Stop(ctx)
	reqErr = reqServer.Stop(ctx)

	if proxyErr != nil {
		return fmt.Errorf("error during proxy shutdown: %w", proxyErr)
	}
	if apiErr != nil {
		return fmt.Errorf("error during API shutdown: %w", apiErr)
	}
	if reqErr != nil {
		return fmt.Errorf("error during request server shutdown: %w", reqErr)
	}

	log.Println("Guardian servers stopped")
	return nil
}

// getGuardianUptime returns the human-readable uptime of the guardian container.
// It uses docker inspect to get the StartedAt time and calculates the duration.
func getGuardianUptime() (string, error) {
	// Use docker inspect to get the container state
	output, err := docker.Run("inspect", guardian.ContainerName)
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

// registryAdapter wraps token.Registry to implement guardian.TokenRegistry.
// This is needed because token.TokenInfo and guardian.TokenInfo are structurally
// identical but Go considers them different types.
type registryAdapter struct {
	*token.Registry
}

func (r *registryAdapter) List() map[string]guardian.TokenInfo {
	tokens := r.Registry.List()
	result := make(map[string]guardian.TokenInfo, len(tokens))
	for k, v := range tokens {
		result[k] = guardian.TokenInfo{
			CloisterName: v.CloisterName,
			ProjectName:  v.ProjectName,
		}
	}
	return result
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

// extractPatterns extracts pattern strings from a slice of CommandPattern.
func extractPatterns(patterns []config.CommandPattern) []string {
	result := make([]string, len(patterns))
	for i, p := range patterns {
		result[i] = p.Pattern
	}
	return result
}
