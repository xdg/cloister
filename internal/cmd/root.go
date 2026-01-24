// Package cmd implements the CLI commands for cloister.
package cmd

import (
	"github.com/spf13/cobra"
)

// Version is set at build time via ldflags.
var Version = "0.1.0-dev"

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "cloister",
	Short: "AI agent sandboxing system",
	Long: `Cloister isolates CLI-based AI coding tools (Claude Code, Codex, Gemini CLI, etc.)
in Docker containers with strict security controls.

It prevents unintentional destruction, blocks data exfiltration via allowlist-only
network access, and maintains development velocity without constant permission prompts.`,
	Version: Version,
}

// Execute runs the root command and returns any error.
func Execute() error {
	return rootCmd.Execute()
}
