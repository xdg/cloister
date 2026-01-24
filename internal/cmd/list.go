package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List running cloister containers",
	Long: `List all running cloister containers.

Displays container name, project, branch, uptime, and status in a table format.`,
	Aliases: []string{"ls"},
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("cloister list: not implemented")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
