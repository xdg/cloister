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
	if strings.Contains(domain, "://") {
		return fmt.Errorf("domain contains scheme prefix (use hostname:port format, not URLs)")
	}

	host, portStr, err := net.SplitHostPort(domain)
	if err != nil {
		host = domain
		portStr = ""
	}

	if host == "" {
		return fmt.Errorf("hostname is empty")
	}
	if portStr != "" {
		if err := validatePort(portStr); err != nil {
			return err
		}
	}
	return validateHostnameChars(host)
}

// invalidHostnameChars contains characters that are not allowed in hostnames.
const invalidHostnameChars = " /\\?#@"

// validatePort checks that a port string is a valid, non-blocked port number.
func validatePort(portStr string) error {
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
	return nil
}

// validateHostnameChars checks for obviously invalid characters in a hostname.
func validateHostnameChars(host string) error {
	if idx := strings.IndexAny(host, invalidHostnameChars); idx >= 0 {
		return fmt.Errorf("hostname contains invalid character %q", host[idx])
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
	domain = strings.ToLower(stripPort(domain))

	// Create response channel (buffered to prevent goroutine leaks)
	respChan := make(chan approval.DomainResponse, 1)

	// Create and submit the request
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

	resp := <-respChan

	switch resp.Status {
	case "timeout":
		return DomainApprovalResult{Approved: false}, nil
	case "denied":
		d.handleDenial(project, cloister, domain, token, resp)
		return DomainApprovalResult{Approved: false}, nil
	case "approved":
		d.handleApproval(project, domain, token, resp)
		return DomainApprovalResult{Approved: true, Scope: resp.Scope}, nil
	default:
		return DomainApprovalResult{Approved: false}, nil
	}
}

// handleDenial processes a denied domain response with scope-based persistence.
func (d *DomainApproverImpl) handleDenial(project, cloister, domain, token string, resp approval.DomainResponse) {
	if d.auditLogger != nil {
		if err := d.auditLogger.LogDomainDenyWithScope(project, cloister, domain, resp.Scope, resp.Pattern); err != nil {
			clog.Warn("failed to log domain deny audit event: %v", err)
		}
	}

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
	}
}

// handleApproval processes an approved domain response with scope-based caching.
func (d *DomainApproverImpl) handleApproval(project, domain, token string, resp approval.DomainResponse) {
	if resp.Scope != "session" {
		// project/global scopes are handled by ConfigPersister + config reloader.
		return
	}
	if d.sessionAllowlist != nil && token != "" {
		if err := d.sessionAllowlist.Add(token, domain); err != nil {
			clog.Warn("failed to add domain %s to session allowlist for token: %v", domain, err)
		}
	}
	if d.allowlistCache != nil {
		projectAllowlist, err := d.allowlistCache.GetProject(project)
		if err != nil {
			clog.Warn("failed to get project allowlist for %s: %v", project, err)
		} else if projectAllowlist != nil {
			projectAllowlist.Add([]string{domain})
		}
	}
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
			d.allowlistCache.SetGlobalDeny(NewAllowlistFromConfig(denyEntry(target, isPattern)))
		} else {
			addToDenylist(denylist, target, isPattern)
		}
	case "project":
		denylist, err := d.allowlistCache.GetProjectDeny(project)
		if err != nil {
			clog.Warn("failed to get project denylist for %s: %v", project, err)
			return
		}
		globalDeny := d.allowlistCache.GetGlobalDeny()
		if denylist == nil || denylist == globalDeny {
			d.allowlistCache.SetProjectDeny(project, NewAllowlistFromConfig(denyEntry(target, isPattern)))
		} else {
			addToDenylist(denylist, target, isPattern)
		}
	}
}

// denyEntry creates a single-entry AllowEntry slice for denylist creation.
func denyEntry(target string, isPattern bool) []config.AllowEntry {
	if isPattern {
		return []config.AllowEntry{{Pattern: target}}
	}
	return []config.AllowEntry{{Domain: target}}
}

// addToDenylist adds a domain or pattern to an existing denylist.
func addToDenylist(denylist *Allowlist, target string, isPattern bool) {
	if isPattern {
		denylist.AddPatterns([]string{target})
	} else {
		denylist.Add([]string{target})
	}
}
