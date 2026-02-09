package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/xdg/cloister/internal/audit"
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

	// Load global decisions and merge with static config
	globalDecisions, err := config.LoadGlobalDecisions()
	if err != nil {
		clog.Warn("failed to load global decisions: %v", err)
		globalDecisions = &config.Decisions{}
	}
	globalAllow := cfg.Proxy.Allow
	if len(globalDecisions.Proxy.Allow) > 0 {
		globalAllow = append(globalAllow, globalDecisions.Proxy.Allow...)
	}
	globalAllowlist := guardian.NewAllowlistFromConfig(globalAllow)
	staticCount := len(cfg.Proxy.Allow)
	approvedCount := len(globalDecisions.Proxy.Allow)
	clog.Info("loaded global allowlist: %d static + %d approved = %d total",
		staticCount, approvedCount, len(globalAllow))

	// Build global denylist from decisions
	var globalDenylist *guardian.Allowlist
	if len(globalDecisions.Proxy.Deny) > 0 {
		globalDenylist = guardian.NewAllowlistFromConfig(globalDecisions.Proxy.Deny)
		clog.Info("loaded global denylist: %d entries", len(globalDecisions.Proxy.Deny))
	}

	// Create allowlist cache for per-project allowlists
	allowlistCache := guardian.NewAllowlistCache(globalAllowlist)
	allowlistCache.SetGlobalDeny(globalDenylist)

	// Create the proxy server with config-derived allowlist
	proxyAddr := fmt.Sprintf(":%d", guardian.DefaultProxyPort)
	proxy := guardian.NewProxyServerWithConfig(proxyAddr, globalAllowlist)
	proxy.TokenValidator = registry

	// Set up per-project allowlist support
	proxy.AllowlistCache = allowlistCache
	proxy.TokenLookup = func(token string) (guardian.TokenLookupResult, bool) {
		info, ok := registry.Lookup(token)
		if !ok {
			return guardian.TokenLookupResult{}, false
		}
		return guardian.TokenLookupResult{
			ProjectName:  info.ProjectName,
			CloisterName: info.CloisterName,
		}, true
	}

	// Create project allowlist loader that merges project config with global
	loadProjectAllowlist := func(projectName string) *guardian.Allowlist {
		projectCfg, err := config.LoadProjectConfig(projectName)
		if err != nil {
			clog.Warn("failed to load project config for %s: %v", projectName, err)
			return nil
		}

		// Load project decisions
		projectDecisions, err := config.LoadProjectDecisions(projectName)
		if err != nil {
			clog.Warn("failed to load project decisions for %s: %v", projectName, err)
			projectDecisions = &config.Decisions{}
		}

		// Check if there's anything project-specific to merge
		hasProjectConfig := len(projectCfg.Proxy.Allow) > 0
		hasProjectDecisions := len(projectDecisions.Proxy.Allow) > 0
		if !hasProjectConfig && !hasProjectDecisions {
			return nil
		}

		// Merge: global config + project config + global decisions + project decisions
		merged := config.MergeAllowlists(cfg.Proxy.Allow, projectCfg.Proxy.Allow)
		if len(globalDecisions.Proxy.Allow) > 0 {
			merged = append(merged, globalDecisions.Proxy.Allow...)
		}
		if hasProjectDecisions {
			merged = append(merged, projectDecisions.Proxy.Allow...)
		}
		allowlist := guardian.NewAllowlistFromConfig(merged)
		clog.Info("loaded allowlist for project %s (%d entries)", projectName, len(allowlist.Domains()))
		return allowlist
	}

	// Create project denylist loader
	loadProjectDenylist := func(projectName string) *guardian.Allowlist {
		projectDecisions, err := config.LoadProjectDecisions(projectName)
		if err != nil {
			clog.Warn("failed to load project decisions (deny) for %s: %v", projectName, err)
			return nil
		}
		if len(projectDecisions.Proxy.Deny) == 0 {
			return nil
		}
		denylist := guardian.NewAllowlistFromConfig(projectDecisions.Proxy.Deny)
		clog.Info("loaded denylist for project %s (%d entries)", projectName, len(projectDecisions.Proxy.Deny))
		return denylist
	}

	// Set the project loaders for lazy loading
	allowlistCache.SetProjectLoader(loadProjectAllowlist)
	allowlistCache.SetDenylistLoader(loadProjectDenylist)

	// Create pattern matcher from global config hostexec patterns
	autoApprovePatterns := extractPatterns(cfg.Hostexec.AutoApprove)
	manualApprovePatterns := extractPatterns(cfg.Hostexec.ManualApprove)
	regexMatcher := patterns.NewRegexMatcher(autoApprovePatterns, manualApprovePatterns)
	clog.Info("loaded approval patterns: %d auto-approve, %d manual-approve",
		len(autoApprovePatterns), len(manualApprovePatterns))

	// Create pattern cache for per-project command patterns
	patternCache := guardian.NewPatternCache(regexMatcher)

	// Create project pattern loader that merges project config with global
	loadProjectPatterns := func(projectName string) patterns.Matcher {
		projectCfg, err := config.LoadProjectConfig(projectName)
		if err != nil {
			clog.Warn("failed to load project config for patterns %s: %v", projectName, err)
			return nil
		}

		// Check if there's anything project-specific to merge
		if len(projectCfg.Hostexec.AutoApprove) == 0 && len(projectCfg.Hostexec.ManualApprove) == 0 {
			return nil
		}

		// Merge: global + project patterns
		mergedAuto := config.MergeCommandPatterns(cfg.Hostexec.AutoApprove, projectCfg.Hostexec.AutoApprove)
		mergedManual := config.MergeCommandPatterns(cfg.Hostexec.ManualApprove, projectCfg.Hostexec.ManualApprove)
		matcher := patterns.NewRegexMatcher(extractPatterns(mergedAuto), extractPatterns(mergedManual))
		clog.Info("loaded command patterns for project %s (%d auto-approve, %d manual-approve)",
			projectName, len(mergedAuto), len(mergedManual))
		return matcher
	}
	patternCache.SetProjectLoader(loadProjectPatterns)

	// Create pattern lookup function for the request server
	patternLookup := func(projectName string) request.PatternMatcher {
		return patternCache.GetProject(projectName)
	}

	// Set up config reloader for SIGHUP
	proxy.SetConfigReloader(func() (*guardian.Allowlist, error) {
		newCfg, err := config.LoadGlobalConfig()
		if err != nil {
			return nil, err
		}
		// Update the cached global config for project merging
		cfg = newCfg

		// Reload global decisions
		newGlobalDecisions, err := config.LoadGlobalDecisions()
		if err != nil {
			clog.Warn("failed to reload global decisions: %v", err)
			newGlobalDecisions = &config.Decisions{}
		}
		globalDecisions = newGlobalDecisions

		// Build new global allowlist with decisions
		globalAllow := newCfg.Proxy.Allow
		if len(globalDecisions.Proxy.Allow) > 0 {
			globalAllow = append(globalAllow, globalDecisions.Proxy.Allow...)
		}
		newGlobal := guardian.NewAllowlistFromConfig(globalAllow)
		allowlistCache.SetGlobal(newGlobal)

		// Rebuild global denylist from decisions
		if len(globalDecisions.Proxy.Deny) > 0 {
			allowlistCache.SetGlobalDeny(guardian.NewAllowlistFromConfig(globalDecisions.Proxy.Deny))
		} else {
			allowlistCache.SetGlobalDeny(nil)
		}

		// Clear project cache so they get reloaded with new global
		allowlistCache.Clear()
		// Reload all project allowlists
		for _, info := range registry.List() {
			if info.ProjectName != "" {
				allowlist := loadProjectAllowlist(info.ProjectName)
				allowlistCache.SetProject(info.ProjectName, allowlist)
			}
		}

		// Rebuild global pattern matcher and clear project pattern cache
		newAutoApprove := extractPatterns(newCfg.Hostexec.AutoApprove)
		newManualApprove := extractPatterns(newCfg.Hostexec.ManualApprove)
		newRegexMatcher := patterns.NewRegexMatcher(newAutoApprove, newManualApprove)
		patternCache.SetGlobal(newRegexMatcher)
		patternCache.Clear()
		clog.Info("reloaded approval patterns: %d auto-approve, %d manual-approve",
			len(newAutoApprove), len(newManualApprove))

		return newGlobal, nil
	})

	// Create the API server
	apiAddr := fmt.Sprintf(":%d", guardian.DefaultAPIPort)
	api := guardian.NewAPIServer(apiAddr, &registryAdapter{registry})

	// Create audit logger if configured
	var auditLogger *audit.Logger
	if cfg.Log.File != "" {
		// Expand ~ to home directory if present
		auditLogPath := cfg.Log.File
		if strings.HasPrefix(auditLogPath, "~/") {
			home, err := os.UserHomeDir()
			if err == nil {
				auditLogPath = filepath.Join(home, auditLogPath[2:])
			}
		}
		auditFile, err := clog.OpenLogFile(auditLogPath)
		if err != nil {
			clog.Warn("failed to open audit log file %s: %v", auditLogPath, err)
		} else {
			auditLogger = audit.NewLogger(auditFile)
			clog.Info("audit logging enabled: %s", auditLogPath)
		}
	}

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

	// Create the approval queue for pending requests
	approvalQueue := approval.NewQueue()

	// Validate unlisted_domain_behavior config value
	if cfg.Proxy.UnlistedDomainBehavior != "" &&
		cfg.Proxy.UnlistedDomainBehavior != "reject" &&
		cfg.Proxy.UnlistedDomainBehavior != "request_approval" {
		clog.Warn("invalid unlisted_domain_behavior %q, using default 'request_approval'", cfg.Proxy.UnlistedDomainBehavior)
		cfg.Proxy.UnlistedDomainBehavior = "request_approval"
	}

	// Create domain approver if unlisted_domain_behavior is "request_approval"
	var domainApprover guardian.DomainApprover
	var sessionAllowlist guardian.SessionAllowlist
	var sessionDenylist guardian.SessionDenylist
	var domainQueue *approval.DomainQueue
	if cfg.Proxy.UnlistedDomainBehavior == "request_approval" {
		// Parse domain approval timeout from config (default 60s)
		approvalTimeout := 60 * time.Second
		if cfg.Proxy.ApprovalTimeout != "" {
			parsed, err := time.ParseDuration(cfg.Proxy.ApprovalTimeout)
			if err != nil {
				clog.Warn("invalid approval_timeout %q, using default 60s: %v", cfg.Proxy.ApprovalTimeout, err)
			} else {
				approvalTimeout = parsed
			}
		}

		// Create the domain approval queue with configured timeout
		domainQueue = approval.NewDomainQueueWithTimeout(approvalTimeout)

		// Create session allowlist and denylist for ephemeral domain decisions
		sessionAllowlist = guardian.NewSessionAllowlist()
		sessionDenylist = guardian.NewSessionDenylist()

		// Create domain approver with all dependencies
		domainApprover = guardian.NewDomainApprover(domainQueue, sessionAllowlist, sessionDenylist, allowlistCache, auditLogger)
		clog.Info("domain approval enabled (timeout: %v)", approvalTimeout)
	} else {
		clog.Info("domain approval disabled (unlisted domains will be rejected)")
	}

	// Create config persister with reload notifier
	configPersister := &guardian.ConfigPersisterImpl{
		ReloadNotifier: func() {
			// Clear and reload allowlist/denylist cache when config is updated
			clog.Debug("config updated, reloading allowlist/denylist cache")

			// Reload global decisions and rebuild global allowlist + denylist
			newGlobalDecisions, err := config.LoadGlobalDecisions()
			if err != nil {
				clog.Warn("failed to reload global decisions: %v", err)
				newGlobalDecisions = &config.Decisions{}
			}
			globalDecisions = newGlobalDecisions
			globalAllow := cfg.Proxy.Allow
			if len(globalDecisions.Proxy.Allow) > 0 {
				globalAllow = append(globalAllow, globalDecisions.Proxy.Allow...)
			}
			allowlistCache.SetGlobal(guardian.NewAllowlistFromConfig(globalAllow))

			// Rebuild global denylist
			if len(globalDecisions.Proxy.Deny) > 0 {
				allowlistCache.SetGlobalDeny(guardian.NewAllowlistFromConfig(globalDecisions.Proxy.Deny))
			} else {
				allowlistCache.SetGlobalDeny(nil)
			}

			// Clear and reload all project allowlists (denylists cleared too, reloaded lazily)
			allowlistCache.Clear()
			for _, info := range registry.List() {
				if info.ProjectName != "" {
					allowlist := loadProjectAllowlist(info.ProjectName)
					allowlistCache.SetProject(info.ProjectName, allowlist)
				}
			}

			// Clear project pattern cache (reloaded lazily on next request)
			patternCache.Clear()
		},
	}

	// Set domain approver and session allow/deny lists on proxy (all may be nil if disabled)
	proxy.DomainApprover = domainApprover
	proxy.SessionAllowlist = sessionAllowlist
	proxy.SessionDenylist = sessionDenylist

	// Set session allowlist on API server for cleanup on token revocation
	api.SessionAllowlist = sessionAllowlist

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

	reqServer := request.NewServer(requestTokenLookup, patternLookup, execClient, auditLogger)
	reqServer.Queue = approvalQueue

	// Create the approval server for the web UI (localhost only)
	approvalServer := approval.NewServer(approvalQueue, auditLogger)

	// Wire domain queue and config persister to approval server
	// SetDomainQueue will wire the EventHub if both are non-nil
	approvalServer.SetDomainQueue(domainQueue)
	approvalServer.ConfigPersister = configPersister

	// Wire audit logger to domain queue if both are non-nil
	if domainQueue != nil && auditLogger != nil {
		domainQueue.SetAuditLogger(auditLogger)
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
