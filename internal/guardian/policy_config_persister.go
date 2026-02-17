package guardian

import "github.com/xdg/cloister/internal/guardian/approval"

// Compile-time check that PolicyConfigPersister implements approval.ConfigPersister.
var _ approval.ConfigPersister = (*PolicyConfigPersister)(nil)

// PolicyConfigPersister adapts DecisionRecorder to the approval.ConfigPersister
// interface. It maps the 4 ConfigPersister methods to RecordDecision calls with
// the appropriate scope. This adapter allows the approval.Server to persist
// domain decisions through PolicyEngine without knowing about it directly.
type PolicyConfigPersister struct {
	Recorder DecisionRecorder
}

// AddDomainToProject persists a domain allow decision at project scope.
func (p *PolicyConfigPersister) AddDomainToProject(project, domain string) error {
	return p.Recorder.RecordDecision(RecordDecisionParams{
		Project: project,
		Domain:  domain,
		Scope:   ScopeProject,
		Allowed: true,
	})
}

// AddDomainToGlobal persists a domain allow decision at global scope.
func (p *PolicyConfigPersister) AddDomainToGlobal(domain string) error {
	return p.Recorder.RecordDecision(RecordDecisionParams{
		Domain:  domain,
		Scope:   ScopeGlobal,
		Allowed: true,
	})
}

// AddPatternToProject persists a pattern allow decision at project scope.
func (p *PolicyConfigPersister) AddPatternToProject(project, pattern string) error {
	return p.Recorder.RecordDecision(RecordDecisionParams{
		Project:   project,
		Domain:    pattern,
		Scope:     ScopeProject,
		Allowed:   true,
		IsPattern: true,
	})
}

// AddPatternToGlobal persists a pattern allow decision at global scope.
func (p *PolicyConfigPersister) AddPatternToGlobal(pattern string) error {
	return p.Recorder.RecordDecision(RecordDecisionParams{
		Domain:    pattern,
		Scope:     ScopeGlobal,
		Allowed:   true,
		IsPattern: true,
	})
}
