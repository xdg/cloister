package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/xdg/cloister/internal/config"
	"github.com/xdg/cloister/internal/prompt"
)

var setupCodexCmd = &cobra.Command{
	Use:   "codex",
	Short: "Configure Codex CLI authentication",
	Long: `Configure authentication for Codex CLI inside cloister containers.

This wizard helps you set up OpenAI API key authentication:

  1. Get your API key from platform.openai.com/api-keys
  2. Paste the key when prompted

The key is stored in cloister's config file (~/.config/cloister/config.yaml).`,
	RunE: runSetupCodex,
}

// setupCodexCredentialReader is the credential reader used by setup codex.
// It can be overridden for testing.
var setupCodexCredentialReader prompt.CredentialReader

// setupCodexYesNoPrompter is the yes/no prompter used by setup codex.
// It can be overridden for testing.
var setupCodexYesNoPrompter prompt.YesNoPrompter

// setupCodexConfigLoader is the config loader used by setup codex.
// It can be overridden for testing.
var setupCodexConfigLoader func() (*config.GlobalConfig, error)

// setupCodexConfigWriter is the config writer used by setup codex.
// It can be overridden for testing.
var setupCodexConfigWriter func(*config.GlobalConfig) error

// setupCodexConfigPath is the function to get the config path.
// It can be overridden for testing.
var setupCodexConfigPath func() string

func init() {
	setupCmd.AddCommand(setupCodexCmd)
}

// getSetupCodexCredentialReader returns the credential reader to use for setup codex.
func getSetupCodexCredentialReader(cmd *cobra.Command) prompt.CredentialReader {
	if setupCodexCredentialReader != nil {
		return setupCodexCredentialReader
	}
	return prompt.NewTerminalCredentialReader(os.Stdin, cmd.OutOrStdout())
}

// getSetupCodexYesNoPrompter returns the yes/no prompter to use for setup codex.
func getSetupCodexYesNoPrompter(cmd *cobra.Command) prompt.YesNoPrompter {
	if setupCodexYesNoPrompter != nil {
		return setupCodexYesNoPrompter
	}
	return prompt.NewStdinYesNoPrompter(os.Stdin, cmd.OutOrStdout())
}

// getSetupCodexConfigLoader returns the config loader to use for setup codex.
func getSetupCodexConfigLoader() func() (*config.GlobalConfig, error) {
	if setupCodexConfigLoader != nil {
		return setupCodexConfigLoader
	}
	return config.LoadGlobalConfig
}

// getSetupCodexConfigWriter returns the config writer to use for setup codex.
func getSetupCodexConfigWriter() func(*config.GlobalConfig) error {
	if setupCodexConfigWriter != nil {
		return setupCodexConfigWriter
	}
	return config.WriteGlobalConfig
}

// getSetupCodexConfigPath returns the config path getter to use for setup codex.
func getSetupCodexConfigPath() func() string {
	if setupCodexConfigPath != nil {
		return setupCodexConfigPath
	}
	return config.GlobalConfigPath
}

// hasExistingCodexCredentials checks if the config already has Codex credentials configured.
func hasExistingCodexCredentials(cfg *config.GlobalConfig) bool {
	if cfg == nil || cfg.Agents == nil {
		return false
	}
	codexCfg, ok := cfg.Agents["codex"]
	if !ok {
		return false
	}
	return codexCfg.AuthMethod != "" || codexCfg.APIKey != ""
}

func runSetupCodex(cmd *cobra.Command, _ []string) error {
	loadConfig := getSetupCodexConfigLoader()
	yesNo := getSetupCodexYesNoPrompter(cmd)

	// Check for existing credentials early
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if hasExistingCodexCredentials(cfg) {
		replace, err := yesNo.PromptYesNo("Credentials already configured. Replace? [y/N]: ", false)
		if err != nil {
			return fmt.Errorf("failed to get confirmation: %w", err)
		}
		if !replace {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Setup canceled. Existing credentials unchanged.")
			return nil
		}
	}

	// Prompt for API key
	apiKey, err := handleCodexAPIKeyInput(cmd)
	if err != nil {
		return err
	}

	// Prompt for full-auto setting (equivalent to skip-permissions)
	fullAuto, err := handleFullAutoPrompt(cmd)
	if err != nil {
		return err
	}

	// Display the result
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Full-auto mode: %v\n", fullAuto)

	// Save to config
	if err := saveCodexCredentialsToConfig(cmd, apiKey, fullAuto); err != nil {
		return err
	}

	return nil
}

// handleCodexAPIKeyInput prompts for and reads an OpenAI API key.
func handleCodexAPIKeyInput(cmd *cobra.Command) (string, error) {
	reader := getSetupCodexCredentialReader(cmd)

	_, _ = fmt.Fprintln(cmd.OutOrStdout())
	apiKey, err := reader.ReadCredential("Paste your OpenAI API key (from platform.openai.com/api-keys): ")
	if err != nil {
		return "", fmt.Errorf("failed to read API key: %w", err)
	}

	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return "", fmt.Errorf("API key cannot be empty")
	}

	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "API key received.")

	return apiKey, nil
}

// handleFullAutoPrompt prompts the user to enable or disable full-auto mode.
// Full-auto mode skips Codex's approval prompts, similar to Claude's skip-permissions.
func handleFullAutoPrompt(cmd *cobra.Command) (bool, error) {
	yesNo := getSetupCodexYesNoPrompter(cmd)

	_, _ = fmt.Fprintln(cmd.OutOrStdout())
	fullAuto, err := yesNo.PromptYesNo("Enable full-auto mode? (skips approval prompts, recommended inside cloister) [Y/n]: ", true)
	if err != nil {
		return false, fmt.Errorf("failed to get full-auto setting: %w", err)
	}

	return fullAuto, nil
}

// saveCodexCredentialsToConfig loads the global config, updates the codex agent
// settings with the provided credentials, and writes the config back.
func saveCodexCredentialsToConfig(cmd *cobra.Command, apiKey string, fullAuto bool) error {
	loadConfig := getSetupCodexConfigLoader()
	writeConfig := getSetupCodexConfigWriter()
	getConfigPath := getSetupCodexConfigPath()

	// Load existing config (or create default)
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Ensure agents map exists
	if cfg.Agents == nil {
		cfg.Agents = make(map[string]config.AgentConfig)
	}

	// Get existing codex config or create new one
	codexCfg := cfg.Agents["codex"]

	// Update auth settings
	codexCfg.AuthMethod = string(config.AuthMethodAPIKey)
	codexCfg.APIKey = apiKey
	codexCfg.Token = "" // Clear any token field (not used by Codex)

	// Update full-auto setting (stored as skip_permissions for consistency)
	codexCfg.SkipPerms = &fullAuto

	// Save back to config
	cfg.Agents["codex"] = codexCfg

	// Write config
	if err := writeConfig(cfg); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	// Print success message
	configPath := getConfigPath()
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nConfiguration saved to: %s\n", configPath)

	return nil
}
