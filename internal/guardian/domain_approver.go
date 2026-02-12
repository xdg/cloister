package guardian

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/xdg/cloister/internal/audit"
	"github.com/xdg/cloister/internal/clog"
	"github.com/xdg/cloister/internal/config"
	"github.com/xdg/cloister/internal/guardian/approval"
)

// blockedPorts is the set of ports that are not valid for HTTP proxy requests.
// These are well-known ports for non-HTTP protocols.
var blockedPorts = map[int]bool{
	21:    true, // FTP
	22:    true, // SSH
	23:    true, // Telnet
	25:    true, // SMTP
	53:    true, // DNS
	110:   true, // POP3
	143:   true, // IMAP
	389:   true, // LDAP
	465:   true, // SMTPS
	587:   true, // SMTP submission
	636:   true, // LDAPS
	993:   true, // IMAPS
	995:   true, // POP3S
	3306:  true, // MySQL
	5432:  true, // PostgreSQL
	6379:  true, // Redis
	27017: true, // MongoDB
}

// ValidateDomain checks if a domain is valid for approval.
// Returns an error describing why the domain is invalid, or nil if valid.
//
// Validation rules:
//   - No scheme prefix (http://, https://, ftp://, etc.)
//   - Port, if present, must not be a well-known non-HTTP port (SSH, database, etc.)
//   - Hostname must not be empty and contain valid characters
func ValidateDomain(domain string) error {
	if domain == "" {
		return fmt.Errorf("domain is empty")
	}

	// Check for scheme prefixes (reject http://, https://, ftp://, etc.)
	if strings.Contains(domain, "://") {
		return fmt.Errorf("domain contains scheme prefix (use hostname:port format, not URLs)")
	}

	// Split host and port
	host, portStr, err := net.SplitHostPort(domain)
	if err != nil {
		// No port specified - that's valid, host is the entire domain
		host = domain
		portStr = ""
	}

	// Check hostname is not empty
	if host == "" {
		return fmt.Errorf("hostname is empty")
	}

	// Validate port if present
	if portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return fmt.Errorf("invalid port %q: %w", portStr, err)
		}
		if port < 1 || port > 65535 {
			return fmt.Errorf("port %d out of valid range (1-65535)", port)
		}
		if blockedPorts[port] {
			return fmt.Errorf("port %d not allowed (non-HTTP protocol)", port)
		}
	}

	// Basic hostname validation - check for obviously invalid characters
	// We allow: alphanumeric, hyphens, dots (for subdomains), and colons (for IPv6)
	// We reject: spaces, slashes, and other URL-like characters
	for _, r := range host {
		if r == ' ' || r == '/' || r == '\\' || r == '?' || r == '#' || r == '@' {
			return fmt.Errorf("hostname contains invalid character %q", r)
		}
	}

	return nil
}

// DomainApproverImpl implements the DomainApprover interface using DomainQueue
// to request human approval for unlisted domains.
type DomainApproverImpl struct {
	queue            *approval.DomainQueue
	sessionAllowlist SessionAllowlist
	sessionDenylist  SessionDenylist
	allowlistCache   *AllowlistCache
	auditLogger      *audit.Logger
}

// NewDomainApprover creates a new DomainApproverImpl.
// The sessionAllowlist and allowlistCache parameters are required for updating
// the in-memory allowlists after approval. The sessionDenylist parameter is
// optional (may be nil) and is used for session-scoped denial persistence.
// The auditLogger parameter is optional (may be nil) and is used for logging
// domain denial events with scope and pattern context.
func NewDomainApprover(queue *approval.DomainQueue, sessionAllowlist SessionAllowlist, sessionDenylist SessionDenylist, allowlistCache *AllowlistCache, auditLogger *audit.Logger) *DomainApproverImpl {
	return &DomainApproverImpl{
		queue:            queue,
		sessionAllowlist: sessionAllowlist,
		sessionDenylist:  sessionDenylist,
		allowlistCache:   allowlistCache,
		auditLogger:      auditLogger,
	}
}

// RequestApproval submits a domain approval request and blocks until the human
// responds with approval/denial or the request times out.
//
// The token parameter is used for session allowlist/denylist updates (token-based
// isolation), while project is used for the approval queue/UI display.
//
// On approval:
//   - "session" scope: adds domain to SessionAllowlist and project's cached Allowlist
//   - "project"/"global" scope: ConfigPersister handles persistence; AllowlistCache
//     is invalidated/reloaded by the guardian's config reloader
//
// On denial:
//   - "once" scope: no persistence, immediate rejection only
//   - "session" scope: adds domain (or wildcard pattern) to SessionDenylist
//   - "project"/"global" scope: persists to decisions file via persistDenial
//   - All denial scopes log an audit event (project, cloister, domain, scope, pattern)
//
// Returns an error if the queue add operation fails, otherwise returns the
// approval result (approved/denied/timeout).
func (d *DomainApproverImpl) RequestApproval(project, cloister, domain, token string) (DomainApprovalResult, error) {
	// Strip port from domain so all downstream usage (queue, UI, session cache,
	// config persistence) works with domain-only. The proxy also strips early,
	// but this is defensive for any caller.
	domain = stripPort(domain)

	// Create response channel (buffered to prevent goroutine leaks)
	respChan := make(chan approval.DomainResponse, 1)

	// Create and submit the request
	// Note: The DomainRequest uses project for display in the approval queue/UI
	// Token is used for request deduplication (same token+domain = same queue entry)
	req := &approval.DomainRequest{
		Cloister:  cloister,
		Project:   project,
		Domain:    domain,
		Token:     token,
		Timestamp: time.Now(),
		Responses: []chan<- approval.DomainResponse{respChan},
	}

	_, err := d.queue.Add(req)
	if err != nil {
		return DomainApprovalResult{}, fmt.Errorf("failed to add domain request to queue: %w", err)
	}

	// Block waiting for response
	resp := <-respChan

	// Handle timeout
	if resp.Status == "timeout" {
		return DomainApprovalResult{Approved: false}, nil
	}

	// Handle denial with scope-based persistence
	if resp.Status == "denied" {
		// Log the denial with scope and pattern context
		if d.auditLogger != nil {
			_ = d.auditLogger.LogDomainDenyWithScope(project, cloister, domain, resp.Scope, resp.Pattern)
		}

		// Determine what to persist: use pattern if provided (wildcard), else domain
		target := domain
		isPattern := false
		if resp.Pattern != "" {
			target = resp.Pattern
			isPattern = true
		}

		switch resp.Scope {
		case "session":
			if d.sessionDenylist != nil && token != "" {
				if err := d.sessionDenylist.Add(token, target); err != nil {
					clog.Warn("failed to add %s to session denylist: %v", target, err)
				}
			}
		case "project":
			if err := d.persistDenial("project", project, target, isPattern); err != nil {
				clog.Warn("failed to persist project denial for %s: %v", target, err)
			} else {
				d.updateDenylistCache("project", project, target, isPattern)
			}
		case "global":
			if err := d.persistDenial("global", "", target, isPattern); err != nil {
				clog.Warn("failed to persist global denial for %s: %v", target, err)
			} else {
				d.updateDenylistCache("global", "", target, isPattern)
			}
		case "once":
			// No persistence needed
		}

		return DomainApprovalResult{Approved: false}, nil
	}

	// Handle unknown status (treat as denial)
	if resp.Status != "approved" {
		return DomainApprovalResult{Approved: false}, nil
	}

	// Handle approval based on scope
	switch resp.Scope {
	case "session":
		// Add to session allowlist using token (token-based isolation)
		// This ensures each cloister session has independent session cache
		if d.sessionAllowlist != nil && token != "" {
			if err := d.sessionAllowlist.Add(token, domain); err != nil {
				clog.Warn("failed to add domain %s to session allowlist for token: %v", domain, err)
			}
		}

		// Add to cached allowlist for this project so subsequent requests don't re-prompt.
		if d.allowlistCache != nil {
			projectAllowlist := d.allowlistCache.GetProject(project)
			if projectAllowlist != nil {
				projectAllowlist.Add([]string{domain})
			}
		}

	case "project", "global":
		// ConfigPersister handles persistence from the server handler.
		// The guardian's config reloader will invalidate/reload AllowlistCache
		// after the config file is written.
		// No action needed here - the cache clear happens via SIGHUP or
		// the ReloadNotifier callback in ConfigPersister.
	}

	return DomainApprovalResult{
		Approved: true,
		Scope:    resp.Scope,
	}, nil
}

// persistDenial writes a denial to the project or global decisions file.
// If isPattern is true, the target is added as a Pattern entry to Proxy.Deny;
// otherwise as a Domain entry. Duplicate entries are skipped.
func (d *DomainApproverImpl) persistDenial(scope, project, target string, isPattern bool) error {
	entry := config.AllowEntry{}
	if isPattern {
		entry.Pattern = target
	} else {
		entry.Domain = target
	}

	switch scope {
	case "project":
		decisions, err := config.LoadProjectDecisions(project)
		if err != nil {
			return fmt.Errorf("load project decisions: %w", err)
		}
		if !containsDenyEntry(decisions.Proxy.Deny, entry) {
			decisions.Proxy.Deny = append(decisions.Proxy.Deny, entry)
		}
		return config.WriteProjectDecisions(project, decisions)
	case "global":
		decisions, err := config.LoadGlobalDecisions()
		if err != nil {
			return fmt.Errorf("load global decisions: %w", err)
		}
		if !containsDenyEntry(decisions.Proxy.Deny, entry) {
			decisions.Proxy.Deny = append(decisions.Proxy.Deny, entry)
		}
		return config.WriteGlobalDecisions(decisions)
	default:
		return fmt.Errorf("unknown scope: %s", scope)
	}
}

// containsDenyEntry checks if an AllowEntry already exists in the slice.
func containsDenyEntry(entries []config.AllowEntry, entry config.AllowEntry) bool {
	for _, e := range entries {
		if e.Domain == entry.Domain && e.Pattern == entry.Pattern {
			return true
		}
	}
	return false
}

// updateDenylistCache updates the in-memory AllowlistCache denylist after a
// denial is persisted to disk. This ensures subsequent proxy requests see the
// denial immediately without waiting for a SIGHUP/config reload.
func (d *DomainApproverImpl) updateDenylistCache(scope, project, target string, isPattern bool) {
	if d.allowlistCache == nil {
		return
	}

	switch scope {
	case "global":
		denylist := d.allowlistCache.GetGlobalDeny()
		if denylist == nil {
			// Create a new global denylist with this entry
			entries := []config.AllowEntry{}
			if isPattern {
				entries = append(entries, config.AllowEntry{Pattern: target})
			} else {
				entries = append(entries, config.AllowEntry{Domain: target})
			}
			d.allowlistCache.SetGlobalDeny(NewAllowlistFromConfig(entries))
		} else {
			if isPattern {
				denylist.AddPatterns([]string{target})
			} else {
				denylist.Add([]string{target})
			}
		}
	case "project":
		denylist := d.allowlistCache.GetProjectDeny(project)
		globalDeny := d.allowlistCache.GetGlobalDeny()
		if denylist == nil || denylist == globalDeny {
			// No project-specific denylist yet; create one
			entries := []config.AllowEntry{}
			if isPattern {
				entries = append(entries, config.AllowEntry{Pattern: target})
			} else {
				entries = append(entries, config.AllowEntry{Domain: target})
			}
			d.allowlistCache.SetProjectDeny(project, NewAllowlistFromConfig(entries))
		} else {
			if isPattern {
				denylist.AddPatterns([]string{target})
			} else {
				denylist.Add([]string{target})
			}
		}
	}
}
