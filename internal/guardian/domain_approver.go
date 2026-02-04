package guardian

import (
	"fmt"
	"time"

	"github.com/xdg/cloister/internal/clog"
	"github.com/xdg/cloister/internal/guardian/approval"
)

// DomainApproverImpl implements the DomainApprover interface using DomainQueue
// to request human approval for unlisted domains.
type DomainApproverImpl struct {
	queue            *approval.DomainQueue
	sessionAllowlist SessionAllowlist
	allowlistCache   *AllowlistCache
}

// NewDomainApprover creates a new DomainApproverImpl.
// The sessionAllowlist and allowlistCache parameters are required for updating
// the in-memory allowlists after approval.
func NewDomainApprover(queue *approval.DomainQueue, sessionAllowlist SessionAllowlist, allowlistCache *AllowlistCache) *DomainApproverImpl {
	return &DomainApproverImpl{
		queue:            queue,
		sessionAllowlist: sessionAllowlist,
		allowlistCache:   allowlistCache,
	}
}

// RequestApproval submits a domain approval request and blocks until the human
// responds with approval/denial or the request times out.
//
// On "session" scope approval:
//   - Adds domain to SessionAllowlist for ephemeral access
//   - Adds domain to the project's cached Allowlist for current guardian session
//
// On "project" or "global" scope approval:
//   - ConfigPersister handles persistence from the server handler
//   - AllowlistCache is invalidated/reloaded by the guardian's config reloader
//
// Returns an error if the queue add operation fails, otherwise returns the
// approval result (approved/denied/timeout).
func (d *DomainApproverImpl) RequestApproval(project, cloister, domain string) (DomainApprovalResult, error) {
	// Create response channel (buffered to prevent goroutine leaks)
	respChan := make(chan approval.DomainResponse, 1)

	// Create and submit the request
	req := &approval.DomainRequest{
		Cloister:  cloister,
		Project:   project,
		Domain:    domain,
		Timestamp: time.Now(),
		Response:  respChan,
	}

	_, err := d.queue.Add(req)
	if err != nil {
		return DomainApprovalResult{}, fmt.Errorf("failed to add domain request to queue: %w", err)
	}

	// Block waiting for response
	resp := <-respChan

	// Handle timeout or denial
	if resp.Status != "approved" {
		return DomainApprovalResult{
			Approved: false,
		}, nil
	}

	// Handle approval based on scope
	switch resp.Scope {
	case "session":
		// Add to session allowlist for ephemeral access
		if d.sessionAllowlist != nil {
			if err := d.sessionAllowlist.Add(project, domain); err != nil {
				clog.Warn("failed to add domain %s to session allowlist for project %s: %v", domain, project, err)
			}
		}

		// Add to cached allowlist for this project so subsequent requests don't re-prompt.
		// Strip port from domain because IsAllowed() does port stripping internally.
		if d.allowlistCache != nil {
			projectAllowlist := d.allowlistCache.GetProject(project)
			if projectAllowlist != nil {
				domainNoPort := stripPort(domain)
				projectAllowlist.Add([]string{domainNoPort})
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
