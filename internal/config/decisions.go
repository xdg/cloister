package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// NOTE: In the guardian container, XDG_CONFIG_HOME=/etc so ConfigDir()
// returns /etc/cloister/. The host's decisions directory is mounted rw
// at /etc/cloister/decisions (overlaying the ro config mount), so
// DecisionDir() resolves correctly in both host and container contexts.

// Decisions represents approved and denied domains and patterns persisted
// separately from static config files. These are written by the guardian when
// users click "Save to Project" or "Save to Global" in the web UI.
type Decisions struct {
	Domains        []string `yaml:"domains,omitempty"`
	Patterns       []string `yaml:"patterns,omitempty"`
	DeniedDomains  []string `yaml:"denied_domains,omitempty"`
	DeniedPatterns []string `yaml:"denied_patterns,omitempty"`
}

// DecisionDir returns the decisions persistence directory path.
// This is always ConfigDir() + "decisions" (e.g. ~/.config/cloister/decisions).
func DecisionDir() string {
	return ConfigDir() + "decisions"
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
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create decision dir: %w", err)
	}
	return writeDecisionsAtomic(GlobalDecisionPath(), decisions)
}

// WriteProjectDecisions writes project-specific decisions atomically.
// The decisions/projects/ directory is created with 0700 permissions if it doesn't exist.
func WriteProjectDecisions(project string, decisions *Decisions) error {
	dir := DecisionDir() + "/projects"
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create decision projects dir: %w", err)
	}
	return writeDecisionsAtomic(ProjectDecisionPath(project), decisions)
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

	if err := tmp.Chmod(0600); err != nil {
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
