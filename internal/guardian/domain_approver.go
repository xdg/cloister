package guardian

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/xdg/cloister/internal/audit"
	"github.com/xdg/cloister/internal/clog"
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
	queue       *approval.DomainQueue
	recorder    DecisionRecorder
	auditLogger *audit.Logger
}

// NewDomainApprover creates a new DomainApproverImpl.
// The recorder parameter handles all decision persistence (session, project,
// global scopes) via RecordDecision. The auditLogger parameter is optional
// (may be nil) and is used for logging domain denial events.
func NewDomainApprover(queue *approval.DomainQueue, recorder DecisionRecorder, auditLogger *audit.Logger) *DomainApproverImpl {
	return &DomainApproverImpl{
		queue:       queue,
		recorder:    recorder,
		auditLogger: auditLogger,
	}
}

// RequestApproval submits a domain approval request and blocks until the human
// responds with approval/denial or the request times out.
//
// The token parameter identifies the session for token-scoped decisions, while
// project is used for the approval queue/UI display and project-scoped persistence.
//
// On approval/denial, delegates to DecisionRecorder.RecordDecision with the
// appropriate scope and parameters. Denial events are also logged via auditLogger.
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

	scope := Scope(resp.Scope)
	if scope == ScopeOnce {
		return
	}

	if d.recorder != nil {
		if err := d.recorder.RecordDecision(RecordDecisionParams{
			Token:     token,
			Project:   project,
			Domain:    target,
			Scope:     scope,
			Allowed:   false,
			IsPattern: isPattern,
		}); err != nil {
			clog.Warn("failed to record denial for %s (scope=%s): %v", target, scope, err)
		}
	}
}

// handleApproval processes an approved domain response with scope-based caching.
func (d *DomainApproverImpl) handleApproval(project, domain, token string, resp approval.DomainResponse) {
	if resp.Scope != "session" {
		// project/global scopes are handled by ConfigPersister on the
		// approval.Server side; DomainApprover only records session approvals.
		return
	}
	if d.recorder != nil {
		if err := d.recorder.RecordDecision(RecordDecisionParams{
			Token:   token,
			Project: project,
			Domain:  domain,
			Scope:   ScopeSession,
			Allowed: true,
		}); err != nil {
			clog.Warn("failed to record session approval for %s: %v", domain, err)
		}
	}
}
