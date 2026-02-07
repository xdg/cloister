package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Approvals represents approved domains and patterns persisted separately
// from static config files. These are written by the guardian when users
// click "Save to Project" or "Save to Global" in the approval web UI.
type Approvals struct {
	Domains  []string `yaml:"domains,omitempty"`
	Patterns []string `yaml:"patterns,omitempty"`
}

// ApprovalDir returns the approval persistence directory path.
// If the CLOISTER_APPROVAL_DIR environment variable is set (container context),
// it uses that value directly. Otherwise, it falls back to ConfigDir() + "approvals".
func ApprovalDir() string {
	if dir := os.Getenv("CLOISTER_APPROVAL_DIR"); dir != "" {
		return dir
	}
	return ConfigDir() + "approvals"
}

// GlobalApprovalPath returns the full path to the global approvals file.
func GlobalApprovalPath() string {
	return ApprovalDir() + "/global.yaml"
}

// ProjectApprovalPath returns the full path to a project approvals file.
func ProjectApprovalPath(project string) string {
	return ApprovalDir() + "/projects/" + project + ".yaml"
}

// LoadGlobalApprovals loads the global approvals from the default approval path.
// If the file doesn't exist, it returns an empty Approvals (not an error).
// If the file exists but has invalid YAML, it returns an error.
func LoadGlobalApprovals() (*Approvals, error) {
	return loadApprovals(GlobalApprovalPath())
}

// LoadProjectApprovals loads project-specific approvals by project name.
// If the file doesn't exist, it returns an empty Approvals (not an error).
// If the file exists but has invalid YAML, it returns an error.
func LoadProjectApprovals(project string) (*Approvals, error) {
	return loadApprovals(ProjectApprovalPath(project))
}

func loadApprovals(path string) (*Approvals, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Approvals{}, nil
		}
		return nil, fmt.Errorf("read approvals %q: %w", path, err)
	}

	var approvals Approvals
	if err := strictUnmarshal(data, &approvals); err != nil {
		return nil, fmt.Errorf("parse approvals %q: %w", path, err)
	}
	return &approvals, nil
}

// WriteGlobalApprovals writes global approvals atomically to the approval directory.
// The approval directory is created with 0700 permissions if it doesn't exist.
func WriteGlobalApprovals(approvals *Approvals) error {
	dir := ApprovalDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create approval dir: %w", err)
	}
	return writeApprovalsAtomic(GlobalApprovalPath(), approvals)
}

// WriteProjectApprovals writes project-specific approvals atomically.
// The approvals/projects/ directory is created with 0700 permissions if it doesn't exist.
func WriteProjectApprovals(project string, approvals *Approvals) error {
	dir := ApprovalDir() + "/projects"
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create approval projects dir: %w", err)
	}
	return writeApprovalsAtomic(ProjectApprovalPath(project), approvals)
}

func writeApprovalsAtomic(path string, approvals *Approvals) error {
	data, err := yaml.Marshal(approvals)
	if err != nil {
		return fmt.Errorf("marshal approvals: %w", err)
	}

	// Write to a temp file in the same directory, then rename for atomicity.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".approvals-*.yaml.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("chmod temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}
