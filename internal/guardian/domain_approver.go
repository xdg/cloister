package guardian

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

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
}

// NewDomainApprover creates a new DomainApproverImpl.
// The sessionAllowlist and allowlistCache parameters are required for updating
// the in-memory allowlists after approval. The sessionDenylist parameter is
// optional (may be nil) and is used for session-scoped denial persistence.
func NewDomainApprover(queue *approval.DomainQueue, sessionAllowlist SessionAllowlist, sessionDenylist SessionDenylist, allowlistCache *AllowlistCache) *DomainApproverImpl {
	return &DomainApproverImpl{
		queue:            queue,
		sessionAllowlist: sessionAllowlist,
		sessionDenylist:  sessionDenylist,
		allowlistCache:   allowlistCache,
	}
}

// RequestApproval submits a domain approval request and blocks until the human
// responds with approval/denial or the request times out.
//
// The token parameter is used for session allowlist updates (token-based isolation),
// while project is used for the approval queue/UI display.
//
// On "session" scope approval:
//   - Adds domain to SessionAllowlist using token (token-based isolation)
//   - Adds domain to the project's cached Allowlist for current guardian session
//
// On "project" or "global" scope approval:
//   - ConfigPersister handles persistence from the server handler
//   - AllowlistCache is invalidated/reloaded by the guardian's config reloader
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
			}
		case "global":
			if err := d.persistDenial("global", "", target, isPattern); err != nil {
				clog.Warn("failed to persist global denial for %s: %v", target, err)
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
func (d *DomainApproverImpl) persistDenial(scope, project, target string, isPattern bool) error {
	switch scope {
	case "project":
		decisions, err := config.LoadProjectDecisions(project)
		if err != nil {
			return fmt.Errorf("load project decisions: %w", err)
		}
		if isPattern {
			decisions.DeniedPatterns = appendUnique(decisions.DeniedPatterns, target)
		} else {
			decisions.DeniedDomains = appendUnique(decisions.DeniedDomains, target)
		}
		return config.WriteProjectDecisions(project, decisions)
	case "global":
		decisions, err := config.LoadGlobalDecisions()
		if err != nil {
			return fmt.Errorf("load global decisions: %w", err)
		}
		if isPattern {
			decisions.DeniedPatterns = appendUnique(decisions.DeniedPatterns, target)
		} else {
			decisions.DeniedDomains = appendUnique(decisions.DeniedDomains, target)
		}
		return config.WriteGlobalDecisions(decisions)
	default:
		return fmt.Errorf("unknown scope: %s", scope)
	}
}

// appendUnique appends value to slice if not already present.
func appendUnique(slice []string, value string) []string {
	for _, s := range slice {
		if s == value {
			return slice
		}
	}
	return append(slice, value)
}
