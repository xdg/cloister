// Package cmd implements the CLI commands for cloister.
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/xdg/cloister/internal/clog"
	"github.com/xdg/cloister/internal/term"
	"github.com/xdg/cloister/internal/version"
)

var (
	debugFlag  bool
	silentFlag bool
)

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "cloister",
	Short: "AI agent sandboxing system",
	Long: `Cloister isolates CLI-based AI coding tools (Claude Code, Codex, Gemini CLI, etc.)
in Docker containers with strict security controls.

It prevents unintentional destruction, blocks data exfiltration via allowlist-only
network access, and maintains development velocity without constant permission prompts.`,
	Version: version.Version,
	PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
		// Initialize logging
		if err := clog.Configure(clog.DefaultLogPath(), debugFlag, false); err != nil {
			// Log to stderr if we can't set up file logging
			term.Warn("failed to configure logging: %v", err)
		}

		// Set silent mode for terminal output
		term.SetSilent(silentFlag)

		return nil
	},
	PersistentPostRun: func(_ *cobra.Command, _ []string) {
		// Ensure logs are flushed on exit
		_ = clog.Close() //nolint:errcheck // clog's own close
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&debugFlag, "debug", false, "enable debug logging")
	rootCmd.PersistentFlags().BoolVar(&silentFlag, "silent", false, "suppress non-essential output")
}

// Execute runs the root command and returns any error.
func Execute() error {
	if err := rootCmd.Execute(); err != nil {
		return fmt.Errorf("command failed: %w", err)
	}
	return nil
}
