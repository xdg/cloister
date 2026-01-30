package cmd

import (
	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Configure agent credentials",
	Long: `Configure credentials for AI coding agents.

The setup wizard helps you configure authentication for various AI coding tools
so they work seamlessly inside cloister containers.

Use the subcommands to set up specific agents:
  cloister setup claude    Configure Claude Code authentication`,
}

func init() {
	rootCmd.AddCommand(setupCmd)
}
