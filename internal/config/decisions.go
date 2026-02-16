package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/xdg/cloister/internal/clog"
)

// NOTE: In the guardian container, XDG_CONFIG_HOME=/etc so Dir()
// returns /etc/cloister/. The host's decisions directory is mounted rw
// at /etc/cloister/decisions (overlaying the ro config mount), so
// DecisionDir() resolves correctly in both host and container contexts.

// Decisions represents approved and denied domains and patterns persisted in
// the decisions directory (~/.config/cloister/decisions/), separately from
// static config files. These are written by the guardian when users approve or
// deny domains via the web UI, with scope options (once, session, project, global).
// The structure mirrors the static config format: Proxy.Allow contains allowed
// domain/pattern entries, Proxy.Deny contains denied entries. Denied entries
// take precedence over allowed entries during proxy evaluation.
type Decisions struct {
	Proxy DecisionsProxy `yaml:"proxy,omitempty"`
}

// DecisionsProxy holds the allow and deny lists for proxy decisions.
type DecisionsProxy struct {
	Allow []AllowEntry `yaml:"allow,omitempty"`
	Deny  []AllowEntry `yaml:"deny,omitempty"`
}

// AllowedDomains returns the domain strings from the allow list.
func (d *Decisions) AllowedDomains() []string {
	var result []string
	for _, e := range d.Proxy.Allow {
		if e.Domain != "" {
			result = append(result, e.Domain)
		}
	}
	return result
}

// AllowedPatterns returns the pattern strings from the allow list.
func (d *Decisions) AllowedPatterns() []string {
	var result []string
	for _, e := range d.Proxy.Allow {
		if e.Pattern != "" {
			result = append(result, e.Pattern)
		}
	}
	return result
}

// DeniedDomains returns the domain strings from the deny list.
func (d *Decisions) DeniedDomains() []string {
	var result []string
	for _, e := range d.Proxy.Deny {
		if e.Domain != "" {
			result = append(result, e.Domain)
		}
	}
	return result
}

// DeniedPatterns returns the pattern strings from the deny list.
func (d *Decisions) DeniedPatterns() []string {
	var result []string
	for _, e := range d.Proxy.Deny {
		if e.Pattern != "" {
			result = append(result, e.Pattern)
		}
	}
	return result
}

// DecisionDir returns the decisions persistence directory path.
// This is always Dir() + "decisions" (e.g. ~/.config/cloister/decisions).
func DecisionDir() string {
	return Dir() + "decisions"
}

// GlobalDecisionPath returns the full path to the global decisions file.
func GlobalDecisionPath() string {
	return DecisionDir() + "/global.yaml"
}

// ProjectDecisionPath returns the full path to a project decisions file.
func ProjectDecisionPath(project string) string {
	return DecisionDir() + "/projects/" + project + ".yaml"
}

// LoadGlobalDecisions loads the global decisions from the default decisions path.
// If the file doesn't exist, it returns an empty Decisions (not an error).
// If the file exists but has invalid YAML, it returns an error.
func LoadGlobalDecisions() (*Decisions, error) {
	return loadDecisions(GlobalDecisionPath())
}

// LoadProjectDecisions loads project-specific decisions by project name.
// If the file doesn't exist, it returns an empty Decisions (not an error).
// If the file exists but has invalid YAML, it returns an error.
func LoadProjectDecisions(project string) (*Decisions, error) {
	return loadDecisions(ProjectDecisionPath(project))
}

func loadDecisions(path string) (*Decisions, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Decisions{}, nil
		}
		return nil, fmt.Errorf("read decisions %q: %w", path, err)
	}

	var decisions Decisions
	if err := strictUnmarshal(data, &decisions); err != nil {
		return nil, fmt.Errorf("parse decisions %q: %w", path, err)
	}
	return &decisions, nil
}

// WriteGlobalDecisions writes global decisions atomically to the decisions directory.
// The decisions directory is created with 0700 permissions if it doesn't exist.
func WriteGlobalDecisions(decisions *Decisions) error {
	dir := DecisionDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create decision dir: %w", err)
	}
	return writeDecisionsAtomic(GlobalDecisionPath(), decisions)
}

// WriteProjectDecisions writes project-specific decisions atomically.
// The decisions/projects/ directory is created with 0700 permissions if it doesn't exist.
func WriteProjectDecisions(project string, decisions *Decisions) error {
	dir := DecisionDir() + "/projects"
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create decision projects dir: %w", err)
	}
	return writeDecisionsAtomic(ProjectDecisionPath(project), decisions)
}

// MigrateDecisionDir checks if the old approvals/ directory exists and the new
// decisions/ directory does not. If so, it renames approvals/ to decisions/
// to migrate existing data. Returns true if migration occurred.
func MigrateDecisionDir() (bool, error) {
	oldDir := Dir() + "approvals"
	newDir := DecisionDir() // Dir() + "decisions"

	// Check if old directory exists
	if _, err := os.Stat(oldDir); os.IsNotExist(err) {
		return false, nil // Nothing to migrate
	} else if err != nil {
		return false, fmt.Errorf("check old approvals dir: %w", err)
	}

	// Check if new directory already exists
	if _, err := os.Stat(newDir); err == nil {
		clog.Warn("both approvals/ and decisions/ exist; skipping migration — remove approvals/ manually")
		return false, nil // Both exist — don't clobber
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("check decisions dir: %w", err)
	}

	// Rename old to new
	if err := os.Rename(oldDir, newDir); err != nil {
		return false, fmt.Errorf("migrate approvals to decisions: %w", err)
	}

	clog.Info("migrated approvals/ to decisions/ directory")
	return true, nil
}

func writeDecisionsAtomic(path string, decisions *Decisions) error {
	data, err := yaml.Marshal(decisions)
	if err != nil {
		return fmt.Errorf("marshal decisions: %w", err)
	}

	// Write to a temp file in the same directory, then rename for atomicity.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".decisions-*.yaml.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("chmod temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}
