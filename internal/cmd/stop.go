package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop [container-name]",
	Short: "Stop a cloister container",
	Long: `Stop a cloister container and clean up its resources.

If no container name is provided, stops the container for the current project.
Revokes the container's token from the guardian and removes the container.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("cloister stop: not implemented")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(stopCmd)
}
