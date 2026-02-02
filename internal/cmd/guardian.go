package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/xdg/cloister/internal/clog"
	"github.com/xdg/cloister/internal/config"
	"github.com/xdg/cloister/internal/container"
	"github.com/xdg/cloister/internal/docker"
	"github.com/xdg/cloister/internal/executor"
	"github.com/xdg/cloister/internal/guardian"
	"github.com/xdg/cloister/internal/guardian/approval"
	guardianexec "github.com/xdg/cloister/internal/guardian/executor"
	"github.com/xdg/cloister/internal/guardian/patterns"
	"github.com/xdg/cloister/internal/guardian/request"
	"github.com/xdg/cloister/internal/term"
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
	// Switch to daemon mode: logs go to file only, not stderr
	clog.SetDaemonMode(true)

	// Create an in-memory token registry
	registry := token.NewRegistry()

	// The onRevoke callback will be set after allowlistCache is created

	// Load persisted tokens from disk (mounted from host)
	store, err := token.NewStore(guardian.ContainerTokenDir)
	if err != nil {
		clog.Warn("failed to open token store: %v", err)
	} else {
		tokens, err := store.Load()
		if err != nil {
			clog.Warn("failed to load tokens: %v", err)
		} else {
			for tok, info := range tokens {
				registry.RegisterFull(tok, info.CloisterName, info.ProjectName, info.WorktreePath)
			}
			if len(tokens) > 0 {
				clog.Info("recovered %d tokens from disk", len(tokens))
			}
		}
	}

	// Load config and create allowlist
	cfg, err := config.LoadGlobalConfig()
	if err != nil {
		clog.Warn("failed to load config, using defaults: %v", err)
		cfg = config.DefaultGlobalConfig()
	}
	globalAllowlist := guardian.NewAllowlistFromConfig(cfg.Proxy.Allow)
	clog.Info("loaded global allowlist with %d domains", len(globalAllowlist.Domains()))

	// Create allowlist cache for per-project allowlists
	allowlistCache := guardian.NewAllowlistCache(globalAllowlist)

	// Set up token revocation cleanup to clear session-scoped domains
	registry.SetOnRevoke(func(tok string) {
		allowlistCache.ClearSession(tok)
		log.Printf("Cleared session-scoped domains for revoked token")
	})

	// Create the proxy server with config-derived allowlist
	proxyAddr := fmt.Sprintf(":%d", guardian.DefaultProxyPort)
	proxy := guardian.NewProxyServerWithConfig(proxyAddr, globalAllowlist)
	proxy.TokenValidator = registry

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
			clog.Warn("failed to load project config for %s: %v", projectName, err)
			return nil
		}
		if len(projectCfg.Proxy.Allow) == 0 {
			return nil
		}
		// Merge global + project allowlists
		merged := config.MergeAllowlists(cfg.Proxy.Allow, projectCfg.Proxy.Allow)
		allowlist := guardian.NewAllowlistFromConfig(merged)
		clog.Info("loaded allowlist for project %s (%d domains)", projectName, len(allowlist.Domains()))
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

	// Create pattern matcher from global config hostexec patterns
	autoApprovePatterns := extractPatterns(cfg.Hostexec.AutoApprove)
	manualApprovePatterns := extractPatterns(cfg.Hostexec.ManualApprove)
	regexMatcher := patterns.NewRegexMatcher(autoApprovePatterns, manualApprovePatterns)
	clog.Info("loaded approval patterns: %d auto-approve, %d manual-approve",
		len(autoApprovePatterns), len(manualApprovePatterns))

	// Create the approval queue for pending requests
	approvalQueue := approval.NewQueue()

	// Create the domain approval queue for proxy domain requests
	domainQueue := approval.NewDomainQueueWithTimeout(guardian.DefaultApprovalTimeout)

	// Create executor client if shared secret and port are available
	var execClient request.CommandExecutor
	sharedSecret := os.Getenv(guardian.SharedSecretEnvVar)
	executorPortStr := os.Getenv(guardian.ExecutorPortEnvVar)
	if sharedSecret != "" && executorPortStr != "" {
		port, err := strconv.Atoi(executorPortStr)
		if err != nil {
			clog.Warn("invalid executor port %q: %v", executorPortStr, err)
		} else {
			execClient = guardianexec.NewTCPClient(port, sharedSecret)
			clog.Info("executor client configured (host.docker.internal:%d)", port)
		}
	} else {
		if sharedSecret == "" {
			clog.Warn("%s not set, command execution disabled", guardian.SharedSecretEnvVar)
		}
		if executorPortStr == "" {
			clog.Warn("%s not set, command execution disabled", guardian.ExecutorPortEnvVar)
		}
	}

	reqServer := request.NewServer(requestTokenLookup, regexMatcher, execClient, nil)
	reqServer.Queue = approvalQueue

	// Create the approval server for the web UI (localhost only)
	approvalServer := approval.NewServer(approvalQueue, nil)
	approvalServer.DomainQueue = domainQueue

	// Connect domain queue to the approval server's event hub for SSE broadcasts
	domainQueue.SetEventHub(approvalServer.Events)

	// Create the config persister for saving approved domains
	configPersister := guardian.NewFileConfigPersister()

	// Set up domain approval for the proxy
	proxy.UnlistedBehavior = guardian.DomainBehaviorRequestApproval
	proxy.ConfigPersister = configPersister
	proxy.TokenInfoLookup = func(tok string) (guardian.TokenInfo, bool) {
		info, ok := registry.Lookup(tok)
		if !ok {
			return guardian.TokenInfo{}, false
		}
		return guardian.TokenInfo{
			CloisterName: info.CloisterName,
			ProjectName:  info.ProjectName,
			WorktreePath: info.WorktreePath,
		}, true
	}

	// Create the DomainApprover callback that bridges the proxy to the domain queue
	proxy.DomainApprover = func(req guardian.DomainApprovalRequest) guardian.DomainApprovalResult {
		// Create a response channel
		respChan := make(chan approval.DomainResponse, 1)

		// Add to the domain queue
		domainReq := &approval.DomainRequest{
			Token:    req.Token,
			Cloister: req.Cloister,
			Project:  req.Project,
			Domain:   req.Domain,
			Response: respChan,
		}
		_, err := domainQueue.Add(domainReq)
		if err != nil {
			return guardian.DomainApprovalResult{
				Approved: false,
				Reason:   err.Error(),
			}
		}

		// Wait for response (blocking)
		resp := <-respChan
		return guardian.DomainApprovalResult{
			Approved: resp.Status == "approved",
			Scope:    string(resp.Scope),
			Reason:   resp.Reason,
		}
	}

	// Start the proxy server
	if err := proxy.Start(); err != nil {
		return fmt.Errorf("failed to start proxy server: %w", err)
	}

	clog.Info("guardian proxy server listening on %s", proxy.ListenAddr())

	// Start the API server
	if err := api.Start(); err != nil {
		// Clean up proxy server on failure
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = proxy.Stop(ctx)
		return fmt.Errorf("failed to start API server: %w", err)
	}

	clog.Info("guardian API server listening on %s", api.ListenAddr())

	// Start the request server for hostexec commands
	if err := reqServer.Start(); err != nil {
		// Clean up other servers on failure
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = proxy.Stop(ctx)
		_ = api.Stop(ctx)
		return fmt.Errorf("failed to start request server: %w", err)
	}

	clog.Info("guardian request server listening on %s", reqServer.ListenAddr())

	// Start the approval server for web UI (localhost only)
	if err := approvalServer.Start(); err != nil {
		// Clean up other servers on failure
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = proxy.Stop(ctx)
		_ = api.Stop(ctx)
		_ = reqServer.Stop(ctx)
		return fmt.Errorf("failed to start approval server: %w", err)
	}

	clog.Info("guardian approval server listening on %s", approvalServer.ListenAddr())

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	clog.Debug("shutting down guardian servers...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop all servers
	var proxyErr, apiErr, reqErr, approvalErr error
	proxyErr = proxy.Stop(ctx)
	apiErr = api.Stop(ctx)
	reqErr = reqServer.Stop(ctx)
	approvalErr = approvalServer.Stop(ctx)

	if proxyErr != nil {
		return fmt.Errorf("error during proxy shutdown: %w", proxyErr)
	}
	if apiErr != nil {
		return fmt.Errorf("error during API shutdown: %w", apiErr)
	}
	if reqErr != nil {
		return fmt.Errorf("error during request server shutdown: %w", reqErr)
	}
	if approvalErr != nil {
		return fmt.Errorf("error during approval server shutdown: %w", approvalErr)
	}

	clog.Debug("guardian servers stopped")
	return nil
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
			WorktreePath: v.WorktreePath,
		}
	}
	return result
}

func (r *registryAdapter) RegisterFull(tok, cloisterName, projectName, worktreePath string) {
	r.Registry.RegisterFull(tok, cloisterName, projectName, worktreePath)
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
