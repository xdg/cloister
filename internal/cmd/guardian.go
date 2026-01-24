package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var guardianCmd = &cobra.Command{
	Use:   "guardian",
	Short: "Manage the guardian proxy service",
	Long: `Manage the guardian proxy service that provides allowlist-enforced network
access for cloister containers.

The guardian runs as a separate container and provides:
- HTTP CONNECT proxy with domain allowlist
- Per-cloister token authentication
- Host command execution approval (future)`,
}

var guardianStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the guardian service",
	Long:  `Start the guardian container if not already running.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("cloister guardian start: not implemented")
		return nil
	},
}

var guardianStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the guardian service",
	Long: `Stop the guardian container.

Warns if there are running cloister containers that depend on the guardian.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("cloister guardian stop: not implemented")
		return nil
	},
}

var guardianStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show guardian service status",
	Long:  `Show guardian status including uptime and active token count.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("cloister guardian status: not implemented")
		return nil
	},
}

func init() {
	guardianCmd.AddCommand(guardianStartCmd)
	guardianCmd.AddCommand(guardianStopCmd)
	guardianCmd.AddCommand(guardianStatusCmd)
	rootCmd.AddCommand(guardianCmd)
}
