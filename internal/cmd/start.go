package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start a cloister container for the current project",
	Long: `Start a cloister container for the current project directory.

Detects the project from the current git repository and starts a sandboxed
container with the project mounted at /work. The guardian proxy is automatically
started if not already running.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("cloister start: not implemented")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(startCmd)
}
