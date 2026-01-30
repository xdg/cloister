// Package claude provides credential extraction and management for Claude Code.
package claude

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
)

// ErrCredentialsNotFound indicates that Claude Code credentials could not be found.
// The user should run `claude login` or `cloister setup claude` to configure credentials.
var ErrCredentialsNotFound = errors.New("Claude Code credentials not found: run `claude login` or `cloister setup claude`")

// Credentials represents the extracted Claude Code credentials.
// On macOS, this is extracted from the Keychain.
// On Linux, this is the path to the credentials file.
type Credentials struct {
	// JSON contains the raw JSON credentials (macOS only).
	// This is the JSON string extracted from the Keychain.
	JSON string

	// FilePath is the path to the credentials file (Linux only).
	// On Linux, credentials are stored in ~/.claude/.credentials.json
	// and we store the path rather than copying the contents.
	FilePath string

	// Platform indicates the platform the credentials were extracted from.
	Platform string
}

// CommandRunner is an interface for running external commands.
// This allows mocking exec.Command for testing.
type CommandRunner interface {
	// Run executes the command and returns stdout and any error.
	// stderr is included in the error message if the command fails.
	Run(name string, args ...string) (string, error)
}

// FileChecker is an interface for checking file existence.
// This allows mocking os.Stat for testing.
type FileChecker interface {
	// Exists returns true if the file exists and is a regular file.
	Exists(path string) bool
}

// UserLookup is an interface for looking up the current user.
// This allows mocking os/user for testing.
type UserLookup interface {
	// CurrentUsername returns the current user's username.
	CurrentUsername() (string, error)

	// HomeDir returns the current user's home directory.
	HomeDir() (string, error)
}

// Extractor extracts Claude Code credentials from the system.
type Extractor struct {
	// CommandRunner runs external commands (for macOS Keychain access).
	CommandRunner CommandRunner

	// FileChecker checks file existence (for Linux credential file).
	FileChecker FileChecker

	// UserLookup gets current user information.
	UserLookup UserLookup

	// Platform overrides runtime.GOOS for testing.
	// If empty, runtime.GOOS is used.
	Platform string
}

// NewExtractor creates a new Extractor with production implementations.
func NewExtractor() *Extractor {
	return &Extractor{
		CommandRunner: &execCommandRunner{},
		FileChecker:   &osFileChecker{},
		UserLookup:    &osUserLookup{},
	}
}

// Extract extracts Claude Code credentials from the system.
// On macOS, it reads from the system Keychain using the `security` command.
// On Linux, it checks for ~/.claude/.credentials.json.
//
// Returns ErrCredentialsNotFound if credentials cannot be found.
func (e *Extractor) Extract() (*Credentials, error) {
	platform := e.Platform
	if platform == "" {
		platform = runtime.GOOS
	}

	switch platform {
	case "darwin":
		return e.extractMacOS()
	case "linux":
		return e.extractLinux()
	default:
		return nil, fmt.Errorf("unsupported platform: %s", platform)
	}
}

// extractMacOS extracts credentials from the macOS Keychain.
func (e *Extractor) extractMacOS() (*Credentials, error) {
	// Get current username
	username, err := e.UserLookup.CurrentUsername()
	if err != nil {
		return nil, fmt.Errorf("failed to get current user: %w", err)
	}

	// Run: security find-generic-password -s 'Claude Code-credentials' -a "<username>" -w
	output, err := e.CommandRunner.Run(
		"security",
		"find-generic-password",
		"-s", "Claude Code-credentials",
		"-a", username,
		"-w",
	)
	if err != nil {
		return nil, ErrCredentialsNotFound
	}

	// Validate that the output is valid JSON
	output = trimOutput(output)
	if !json.Valid([]byte(output)) {
		return nil, fmt.Errorf("keychain entry is not valid JSON")
	}

	return &Credentials{
		JSON:     output,
		Platform: "darwin",
	}, nil
}

// extractLinux extracts credentials from the Linux file system.
func (e *Extractor) extractLinux() (*Credentials, error) {
	// Get home directory
	homeDir, err := e.UserLookup.HomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	// Check if ~/.claude/.credentials.json exists
	credPath := filepath.Join(homeDir, ".claude", ".credentials.json")
	if !e.FileChecker.Exists(credPath) {
		return nil, ErrCredentialsNotFound
	}

	return &Credentials{
		FilePath: credPath,
		Platform: "linux",
	}, nil
}

// trimOutput trims whitespace and newlines from command output.
func trimOutput(s string) string {
	return string(bytes.TrimSpace([]byte(s)))
}

// execCommandRunner implements CommandRunner using os/exec.
type execCommandRunner struct{}

func (r *execCommandRunner) Run(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("%w: %s", err, stderr.String())
		}
		return "", err
	}

	return stdout.String(), nil
}

// osFileChecker implements FileChecker using os.Stat.
type osFileChecker struct{}

func (c *osFileChecker) Exists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// osUserLookup implements UserLookup using os/user.
type osUserLookup struct{}

func (u *osUserLookup) CurrentUsername() (string, error) {
	currentUser, err := user.Current()
	if err != nil {
		return "", err
	}
	return currentUser.Username, nil
}

func (u *osUserLookup) HomeDir() (string, error) {
	return os.UserHomeDir()
}
