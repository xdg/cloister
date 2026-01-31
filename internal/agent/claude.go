package agent

import (
	"fmt"

	"github.com/xdg/cloister/internal/claude"
	"github.com/xdg/cloister/internal/config"
)

func init() {
	Register(NewClaudeAgent())
}

const (
	agentName      = "claude"
	settingsDir    = ".claude"
	configFileName = ".claude.json"
)

// ClaudeAgent handles setup for Claude Code in containers.
type ClaudeAgent struct {
	// Injector handles credential extraction and injection.
	// If nil, claude.NewInjector() is used.
	Injector *claude.Injector
}

// NewClaudeAgent creates a new ClaudeAgent with default dependencies.
func NewClaudeAgent() *ClaudeAgent {
	return &ClaudeAgent{
		Injector: claude.NewInjector(),
	}
}

// Name returns the agent identifier.
func (a *ClaudeAgent) Name() string {
	return agentName
}

// settingsExcludePatterns lists directories/files to exclude when copying ~/.claude/
// These are machine-local files that don't need to be in the container.
// Based on ~/.claude/.gitignore patterns.
var settingsExcludePatterns = []string{
	".update.lock",
	"cache",
	"debug/",
	"downloads/",
	"file-history/",
	"history.jsonl",
	"plans/",
	"plugins/install-counts-cache.json",
	"projects/",
	"session-env/",
	"shell-snapshots/",
	"stats-cache.json",
	"statsig/",
	"tasks/",
	"telemetry",
	"todos/",
}

// configJSONPath is the path to the Claude config file in the container.
var configJSONPath = containerHomeDir + "/.claude.json"

// configFieldsToCopy lists the top-level fields from ~/.claude.json that should
// be copied to the container. These are identity fields that ensure consistent user ID.
var configFieldsToCopy = []string{
	"userID",
	"lastOnboardingVersion",
}

// configForcedValues are fields that are always set to specific values.
// These ensure smooth operation in the container environment.
var configForcedValues = map[string]any{
	"hasCompletedOnboarding":        true,
	"bypassPermissionsModeAccepted": true,
	"installMethod":                 "native",
}

// Setup performs Claude-specific container setup.
// This copies the ~/.claude/ settings directory, injects credentials,
// and generates the ~/.claude.json config file.
func (a *ClaudeAgent) Setup(containerName string, agentCfg *config.AgentConfig) (*SetupResult, error) {
	result := &SetupResult{
		EnvVars: make(map[string]string),
	}

	// Step 1: Copy settings directory (~/.claude/)
	// This is a one-way snapshot - writes inside container are isolated
	if err := CopyDirToContainer(containerName, settingsDir, settingsExcludePatterns); err != nil {
		// Log but don't fail - missing settings is not fatal
		_ = err
	}

	// Step 2: Inject credentials (if configured)
	var authMethod string
	if agentCfg != nil && agentCfg.AuthMethod != "" {
		authMethod = agentCfg.AuthMethod

		injector := a.Injector
		if injector == nil {
			injector = claude.NewInjector()
		}

		injectionConfig, err := injector.InjectCredentials(agentCfg)
		if err != nil {
			return nil, fmt.Errorf("credential injection failed: %w", err)
		}

		// Collect env vars for container
		for key, value := range injectionConfig.EnvVars {
			result.EnvVars[key] = value
		}

		// Write credential files to container
		for destPath, content := range injectionConfig.Files {
			if err := WriteFileToContainer(containerName, destPath, content); err != nil {
				return nil, fmt.Errorf("failed to write credential file %s: %w", destPath, err)
			}
		}
	}

	// Step 3: Generate ~/.claude.json config file
	// Build conditional fields based on auth method
	var conditionalCopy map[string]any
	if authMethod == claude.AuthMethodExisting {
		// Copy oauthAccount from host when using "existing" auth
		// (it's tied to the credentials being injected)
		conditionalCopy = map[string]any{
			"oauthAccount": nil, // nil means "copy from host if present"
		}
	}

	configJSON, err := MergeJSONConfig(configFileName, configFieldsToCopy, configForcedValues, conditionalCopy)
	if err != nil {
		return nil, fmt.Errorf("failed to generate config: %w", err)
	}

	if err := WriteFileToContainer(containerName, configJSONPath, configJSON); err != nil {
		return nil, fmt.Errorf("failed to write config file: %w", err)
	}

	return result, nil
}
