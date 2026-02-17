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

// guardianState holds all shared state for the guardian proxy runtime.
type guardianState struct {
	registry     *token.Registry
	cfg          *config.GlobalConfig
	policyEngine *guardian.PolicyEngine
	patternCache *guardian.PatternCache
	auditLogger  *audit.Logger
}

// runGuardianProxy starts the proxy and API servers and blocks until interrupted.
func runGuardianProxy(_ *cobra.Command, _ []string) error {
	// Switch to daemon mode: logs go to file only, not stderr
	clog.SetDaemonMode(true)

	gs := &guardianState{}
	gs.registry = token.NewRegistry()

	loadPersistedTokens(gs.registry)

	gs.cfg = loadGuardianConfig()
	globalDecisions := loadGuardianDecisions()

	gs.policyEngine = setupPolicyEngine(gs, globalDecisions)

	proxy := setupProxyServer(gs)
	gs.patternCache = setupPatternCache(gs)
	patternLookup := func(projectName string) request.PatternMatcher {
		return gs.patternCache.GetProject(projectName)
	}

	apiAddr := fmt.Sprintf(":%d", guardian.DefaultAPIPort)
	api := guardian.NewAPIServer(apiAddr, gs.registry)

	gs.auditLogger = setupAuditLogger(gs.cfg)

	requestTokenLookup := func(tok string) (token.Info, bool) {
		return gs.registry.Lookup(tok)
	}

	approvalQueue := approval.NewQueue()
	dar := setupDomainApproval(gs)

	configPersister := &guardian.PolicyConfigPersister{Recorder: gs.policyEngine}

	proxy.DomainApprover = dar.Approver
	proxy.OnReload = func() {
		gs.patternCache.Clear()
	}
	api.TokenRevoker = gs.policyEngine

	execClient := setupExecutorClient()

	reqServer := request.NewServer(requestTokenLookup, patternLookup, execClient, gs.auditLogger)
	reqServer.Queue = approvalQueue

	approvalServer := approval.NewServer(approvalQueue, gs.auditLogger)
	approvalServer.SetDomainQueue(dar.DomainQueue)
	approvalServer.ConfigPersister = configPersister

	if dar.DomainQueue != nil && gs.auditLogger != nil {
		dar.DomainQueue.SetAuditLogger(gs.auditLogger)
	}

	if err := startAllServers(proxy, api, reqServer, approvalServer); err != nil {
		return err
	}

	awaitShutdownSignal()

	return shutdownAllServers(proxy, api, reqServer, approvalServer)
}

// loadPersistedTokens loads tokens from disk into the registry.
func loadPersistedTokens(registry *token.Registry) {
	store, err := token.NewStore(guardian.ContainerTokenDir)
	if err != nil {
		clog.Warn("failed to open token store: %v", err)
		return
	}
	tokens, err := store.Load()
	if err != nil {
		clog.Warn("failed to load tokens: %v", err)
		return
	}
	for tok, info := range tokens {
		registry.RegisterFull(tok, info.CloisterName, info.ProjectName, info.WorktreePath)
	}
	if len(tokens) > 0 {
		clog.Info("recovered %d tokens from disk", len(tokens))
	}
}

// loadGuardianConfig loads the global config, falling back to defaults.
func loadGuardianConfig() *config.GlobalConfig {
	cfg, err := config.LoadGlobalConfig()
	if err != nil {
		clog.Warn("failed to load config, using defaults: %v", err)
		return config.DefaultGlobalConfig()
	}
	return cfg
}

// loadGuardianDecisions loads global decisions, falling back to empty.
func loadGuardianDecisions() *config.Decisions {
	decisions, err := config.LoadGlobalDecisions()
	if err != nil {
		clog.Warn("failed to load global decisions: %v", err)
		return &config.Decisions{}
	}
	return decisions
}

// setupPolicyEngine creates the PolicyEngine that owns all domain access policy state.
func setupPolicyEngine(gs *guardianState, globalDecisions *config.Decisions) *guardian.PolicyEngine {
	pe, err := guardian.NewPolicyEngine(gs.cfg, globalDecisions, gs.registry)
	if err != nil {
		clog.Warn("failed to create policy engine: %v", err)
		// Create a minimal engine with defaults; this cannot fail with valid inputs.
		pe, _ = guardian.NewPolicyEngine(config.DefaultGlobalConfig(), &config.Decisions{}, nil) //nolint:errcheck
	}
	return pe
}

// setupProxyServer creates and configures the proxy server.
func setupProxyServer(gs *guardianState) *guardian.ProxyServer {
	proxyAddr := fmt.Sprintf(":%d", guardian.DefaultProxyPort)
	proxy := guardian.NewProxyServer(proxyAddr)
	proxy.PolicyEngine = gs.policyEngine
	proxy.TokenValidator = gs.registry
	proxy.TokenLookup = guardian.TokenLookupFromRegistry(gs.registry)
	return proxy
}

// setupPatternCache creates the pattern cache for command approval.
func setupPatternCache(gs *guardianState) *guardian.PatternCache {
	autoApprovePatterns := extractPatterns(gs.cfg.Hostexec.AutoApprove)
	manualApprovePatterns := extractPatterns(gs.cfg.Hostexec.ManualApprove)
	regexMatcher := patterns.NewRegexMatcher(autoApprovePatterns, manualApprovePatterns)
	clog.Info("loaded approval patterns: %d auto-approve, %d manual-approve",
		len(autoApprovePatterns), len(manualApprovePatterns))

	cache := guardian.NewPatternCache(regexMatcher)
	cfg := gs.cfg
	cache.SetProjectLoader(func(projectName string) patterns.Matcher {
		projectCfg, err := config.LoadProjectConfig(projectName)
		if err != nil {
			clog.Warn("failed to load project config for patterns %s: %v", projectName, err)
			return nil
		}
		if len(projectCfg.Hostexec.AutoApprove) == 0 && len(projectCfg.Hostexec.ManualApprove) == 0 {
			return nil
		}
		mergedAuto := config.MergeCommandPatterns(cfg.Hostexec.AutoApprove, projectCfg.Hostexec.AutoApprove)
		mergedManual := config.MergeCommandPatterns(cfg.Hostexec.ManualApprove, projectCfg.Hostexec.ManualApprove)
		matcher := patterns.NewRegexMatcher(extractPatterns(mergedAuto), extractPatterns(mergedManual))
		clog.Info("loaded command patterns for project %s (%d auto-approve, %d manual-approve)",
			projectName, len(mergedAuto), len(mergedManual))
		return matcher
	})

	return cache
}

// setupAuditLogger creates the audit logger if configured.
func setupAuditLogger(cfg *config.GlobalConfig) *audit.Logger {
	if cfg.Log.File == "" {
		return nil
	}
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
		return nil
	}
	clog.Info("audit logging enabled: %s", auditLogPath)
	return audit.NewLogger(auditFile)
}

// domainApprovalResult groups the components returned by setupDomainApproval.
type domainApprovalResult struct {
	Approver    guardian.DomainApprover
	DomainQueue *approval.DomainQueue
}

// setupDomainApproval configures domain approval components if enabled.
func setupDomainApproval(gs *guardianState) domainApprovalResult {
	cfg := gs.cfg
	if cfg.Proxy.UnlistedDomainBehavior != "" &&
		cfg.Proxy.UnlistedDomainBehavior != "reject" &&
		cfg.Proxy.UnlistedDomainBehavior != "request_approval" {
		clog.Warn("invalid unlisted_domain_behavior %q, using default 'request_approval'", cfg.Proxy.UnlistedDomainBehavior)
		cfg.Proxy.UnlistedDomainBehavior = "request_approval"
	}

	if cfg.Proxy.UnlistedDomainBehavior != "request_approval" {
		clog.Info("domain approval disabled (unlisted domains will be rejected)")
		return domainApprovalResult{}
	}

	approvalTimeout := 60 * time.Second
	if cfg.Proxy.ApprovalTimeout != "" {
		parsed, err := time.ParseDuration(cfg.Proxy.ApprovalTimeout)
		if err != nil {
			clog.Warn("invalid approval_timeout %q, using default 60s: %v", cfg.Proxy.ApprovalTimeout, err)
		} else {
			approvalTimeout = parsed
		}
	}

	domainQueue := approval.NewDomainQueueWithTimeout(approvalTimeout)
	domainApprover := guardian.NewDomainApprover(domainQueue, gs.policyEngine, gs.auditLogger)
	clog.Info("domain approval enabled (timeout: %v)", approvalTimeout)

	return domainApprovalResult{
		Approver:    domainApprover,
		DomainQueue: domainQueue,
	}
}

// setupExecutorClient creates the executor client if environment is configured.
func setupExecutorClient() request.CommandExecutor {
	sharedSecret := os.Getenv(guardian.SharedSecretEnvVar)
	executorPortStr := os.Getenv(guardian.ExecutorPortEnvVar)
	if sharedSecret == "" || executorPortStr == "" {
		if sharedSecret == "" {
			clog.Warn("%s not set, command execution disabled", guardian.SharedSecretEnvVar)
		}
		if executorPortStr == "" {
			clog.Warn("%s not set, command execution disabled", guardian.ExecutorPortEnvVar)
		}
		return nil
	}
	port, err := strconv.Atoi(executorPortStr)
	if err != nil {
		clog.Warn("invalid executor port %q: %v", executorPortStr, err)
		return nil
	}
	clog.Info("executor client configured (host.docker.internal:%d)", port)
	return guardianexec.NewTCPClient(port, sharedSecret)
}

// stoppable is a server that can be started and stopped.
type stoppable interface {
	Start() error
	Stop(ctx context.Context) error
	ListenAddr() string
}

// startAllServers starts all guardian servers, cleaning up on failure.
func startAllServers(proxy, api, reqServer, approvalServer stoppable) error {
	servers := []struct {
		server stoppable
		name   string
	}{
		{proxy, "proxy"},
		{api, "API"},
		{reqServer, "request"},
		{approvalServer, "approval"},
	}

	var started []stoppable
	for _, s := range servers {
		if err := s.server.Start(); err != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			for _, running := range started {
				if stopErr := running.Stop(ctx); stopErr != nil {
					clog.Warn("failed to stop server on cleanup: %v", stopErr)
				}
			}
			cancel()
			return fmt.Errorf("failed to start %s server: %w", s.name, err)
		}
		clog.Info("guardian %s server listening on %s", s.name, s.server.ListenAddr())
		started = append(started, s.server)
	}

	return nil
}

// awaitShutdownSignal blocks until SIGINT or SIGTERM is received.
func awaitShutdownSignal() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	clog.Debug("shutting down guardian servers...")
}

// shutdownAllServers gracefully shuts down all guardian servers.
func shutdownAllServers(proxy, api, reqServer, approvalServer stoppable) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	errs := []struct {
		err  error
		name string
	}{
		{proxy.Stop(ctx), "proxy"},
		{api.Stop(ctx), "API"},
		{reqServer.Stop(ctx), "request server"},
		{approvalServer.Stop(ctx), "approval server"},
	}

	for _, e := range errs {
		if e.err != nil {
			return fmt.Errorf("error during %s shutdown: %w", e.name, e.err)
		}
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
func extractPatterns(cmds []config.CommandPattern) []string {
	result := make([]string, len(cmds))
	for i, p := range cmds {
		result[i] = p.Pattern
	}
	return result
}
