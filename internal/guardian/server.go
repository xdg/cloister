package guardian

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/xdg/cloister/internal/audit"
	"github.com/xdg/cloister/internal/clog"
	"github.com/xdg/cloister/internal/config"
	guardianexec "github.com/xdg/cloister/internal/guardian/executor"
	"github.com/xdg/cloister/internal/guardian/patterns"
	"github.com/xdg/cloister/internal/guardian/request"

	"github.com/xdg/cloister/internal/guardian/approval"
	"github.com/xdg/cloister/internal/token"
)

// stoppable is a server that can be started and stopped.
type stoppable interface {
	Start() error
	Stop(ctx context.Context) error
	ListenAddr() string
}

// domainApprovalResult groups the components returned by setupDomainApproval.
type domainApprovalResult struct {
	Approver    DomainApprover
	DomainQueue *approval.DomainQueue
}

// Server encapsulates the guardian server startup and shutdown.
type Server struct {
	registry       *token.Registry
	cfg            *config.GlobalConfig
	policyEngine   *PolicyEngine
	patternCache   *PatternCache
	auditLogger    *audit.Logger
	proxy          stoppable
	api            stoppable
	reqServer      stoppable
	approvalServer stoppable
}

// NewServer creates a fully-wired Server ready to run.
func NewServer(registry *token.Registry, cfg *config.GlobalConfig, decisions *config.Decisions) (*Server, error) {
	s := &Server{
		registry: registry,
		cfg:      cfg,
	}

	s.policyEngine = s.setupPolicyEngine(decisions)

	proxy := s.setupProxyServer()
	s.patternCache = s.setupPatternCache()
	patternLookup := func(projectName string) request.PatternMatcher {
		return s.patternCache.GetProject(projectName)
	}

	apiAddr := fmt.Sprintf(":%d", DefaultAPIPort)
	api := NewAPIServer(apiAddr, s.registry)

	s.auditLogger = setupAuditLogger(s.cfg)

	requestTokenLookup := func(tok string) (token.Info, bool) {
		return s.registry.Lookup(tok)
	}

	approvalQueue := approval.NewQueue()
	dar := s.setupDomainApproval()

	configPersister := &PolicyConfigPersister{Recorder: s.policyEngine}

	proxy.DomainApprover = dar.Approver
	proxy.OnReload = func() {
		s.patternCache.Clear()
	}
	proxy.OnTokenReload = func() {
		store, err := token.NewStore(ContainerTokenDir)
		if err != nil {
			clog.Warn("SIGHUP token reload: failed to open token store: %v", err)
			return
		}
		if err := token.ReconcileWithStore(s.registry, store); err != nil {
			clog.Warn("SIGHUP token reload: %v", err)
			return
		}
		clog.Info("SIGHUP token registry reconciled with disk")
	}
	api.TokenRevoker = s.policyEngine
	api.OnTokenRegistered = func(projectName string) {
		if err := s.policyEngine.EnsureProject(projectName); err != nil {
			clog.Warn("failed to load project policy on token register: %v", err)
		}
	}

	execClient := setupExecutorClient()

	reqServer := request.NewServer(requestTokenLookup, patternLookup, execClient, s.auditLogger)
	reqServer.Queue = approvalQueue

	approvalServer := approval.NewServer(approvalQueue, s.auditLogger)
	approvalServer.SetDomainQueue(dar.DomainQueue)
	approvalServer.ConfigPersister = configPersister

	if dar.DomainQueue != nil && s.auditLogger != nil {
		dar.DomainQueue.SetAuditLogger(s.auditLogger)
	}

	s.proxy = proxy
	s.api = api
	s.reqServer = reqServer
	s.approvalServer = approvalServer

	return s, nil
}

// Run starts all servers, blocks on a shutdown signal, then shuts down.
func (s *Server) Run() error {
	if err := s.startAllServers(); err != nil {
		return err
	}

	awaitShutdownSignal()

	return s.shutdownAllServers()
}

// Shutdown gracefully shuts down all servers.
func (s *Server) Shutdown() error {
	return s.shutdownAllServers()
}

// LoadPersistedTokens loads tokens from disk into the registry.
func LoadPersistedTokens(registry *token.Registry) {
	store, err := token.NewStore(ContainerTokenDir)
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

// LoadGuardianConfig loads the global config, falling back to defaults.
func LoadGuardianConfig() *config.GlobalConfig {
	cfg, err := config.LoadGlobalConfig()
	if err != nil {
		clog.Warn("failed to load config, using defaults: %v", err)
		return config.DefaultGlobalConfig()
	}
	return cfg
}

// LoadGuardianDecisions loads global decisions, falling back to empty.
func LoadGuardianDecisions() *config.Decisions {
	decisions, err := config.LoadGlobalDecisions()
	if err != nil {
		clog.Warn("failed to load global decisions: %v", err)
		return &config.Decisions{}
	}
	return decisions
}

// setupPolicyEngine creates the PolicyEngine that owns all domain access policy state.
func (s *Server) setupPolicyEngine(globalDecisions *config.Decisions) *PolicyEngine {
	pe, err := NewPolicyEngine(s.cfg, globalDecisions, s.registry)
	if err != nil {
		clog.Warn("failed to create policy engine: %v", err)
		// Create a minimal engine with defaults; this cannot fail with valid inputs.
		pe, _ = NewPolicyEngine(config.DefaultGlobalConfig(), &config.Decisions{}, nil) //nolint:errcheck
	}
	return pe
}

// setupProxyServer creates and configures the proxy server.
func (s *Server) setupProxyServer() *ProxyServer {
	proxyAddr := fmt.Sprintf(":%d", DefaultProxyPort)
	proxy := NewProxyServer(proxyAddr)
	proxy.PolicyEngine = s.policyEngine
	proxy.TokenValidator = s.registry
	proxy.TokenLookup = TokenLookupFromRegistry(s.registry)
	return proxy
}

// setupPatternCache creates the pattern cache for command approval.
func (s *Server) setupPatternCache() *PatternCache {
	autoApprovePatterns := extractPatterns(s.cfg.Hostexec.AutoApprove)
	manualApprovePatterns := extractPatterns(s.cfg.Hostexec.ManualApprove)
	regexMatcher := patterns.NewRegexMatcher(autoApprovePatterns, manualApprovePatterns)
	clog.Info("loaded approval patterns: %d auto-approve, %d manual-approve",
		len(autoApprovePatterns), len(manualApprovePatterns))

	cache := NewPatternCache(regexMatcher)
	cfg := s.cfg
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

// setupDomainApproval configures domain approval components if enabled.
func (s *Server) setupDomainApproval() domainApprovalResult {
	cfg := s.cfg
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
	domainApprover := NewDomainApprover(domainQueue, s.policyEngine, s.auditLogger)
	clog.Info("domain approval enabled (timeout: %v)", approvalTimeout)

	return domainApprovalResult{
		Approver:    domainApprover,
		DomainQueue: domainQueue,
	}
}

// setupExecutorClient creates the executor client if environment is configured.
func setupExecutorClient() request.CommandExecutor {
	sharedSecret := os.Getenv(SharedSecretEnvVar)
	executorPortStr := os.Getenv(ExecutorPortEnvVar)
	if sharedSecret == "" || executorPortStr == "" {
		if sharedSecret == "" {
			clog.Warn("%s not set, command execution disabled", SharedSecretEnvVar)
		}
		if executorPortStr == "" {
			clog.Warn("%s not set, command execution disabled", ExecutorPortEnvVar)
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

// startAllServers starts all guardian servers, cleaning up on failure.
func (s *Server) startAllServers() error {
	servers := []struct {
		server stoppable
		name   string
	}{
		{s.proxy, "proxy"},
		{s.api, "API"},
		{s.reqServer, "request"},
		{s.approvalServer, "approval"},
	}

	var started []stoppable
	for _, srv := range servers {
		if err := srv.server.Start(); err != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			for _, running := range started {
				if stopErr := running.Stop(ctx); stopErr != nil {
					clog.Warn("failed to stop server on cleanup: %v", stopErr)
				}
			}
			cancel()
			return fmt.Errorf("failed to start %s server: %w", srv.name, err)
		}
		clog.Info("guardian %s server listening on %s", srv.name, srv.server.ListenAddr())
		started = append(started, srv.server)
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
func (s *Server) shutdownAllServers() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	errs := []struct {
		err  error
		name string
	}{
		{s.proxy.Stop(ctx), "proxy"},
		{s.api.Stop(ctx), "API"},
		{s.reqServer.Stop(ctx), "request server"},
		{s.approvalServer.Stop(ctx), "approval server"},
	}

	for _, e := range errs {
		if e.err != nil {
			return fmt.Errorf("error during %s shutdown: %w", e.name, e.err)
		}
	}

	clog.Debug("guardian servers stopped")
	return nil
}

// extractPatterns extracts pattern strings from a slice of CommandPattern.
func extractPatterns(cmds []config.CommandPattern) []string {
	result := make([]string, len(cmds))
	for i, p := range cmds {
		result[i] = p.Pattern
	}
	return result
}
