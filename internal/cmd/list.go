package cmd

import (
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/xdg/cloister/internal/clog"
	"github.com/xdg/cloister/internal/container"
	"github.com/xdg/cloister/internal/docker"
	"github.com/xdg/cloister/internal/term"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List running cloisters",
	Long: `List all running cloisters.

Displays cloister name, project, branch, uptime, and status in a table format.`,
	Aliases: []string{"ls"},
	RunE:    runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
}

func runList(_ *cobra.Command, _ []string) error {
	mgr := container.NewManager()
	containers, err := mgr.List()
	if err != nil {
		if errors.Is(err, docker.ErrDockerNotRunning) {
			return dockerNotRunningError()
		}
		return fmt.Errorf("failed to list containers: %w", err)
	}

	// Filter out the guardian container and non-running containers
	var cloisters []container.Info
	for _, c := range containers {
		// Skip the guardian container
		if c.Name == "cloister-guardian" {
			continue
		}
		// Only include running containers
		if c.State != "running" {
			continue
		}
		cloisters = append(cloisters, c)
	}

	// Handle empty list
	if len(cloisters) == 0 {
		term.Println("No running cloisters.")
		return nil
	}

	// Create tabwriter for table formatting
	w := tabwriter.NewWriter(term.Stdout(), 0, 0, 2, ' ', 0)

	// Print header
	_, _ = fmt.Fprintln(w, "NAME\tPROJECT\tBRANCH\tUPTIME\tSTATUS")

	// Print each cloister
	for _, c := range cloisters {
		cloisterName := container.NameToCloisterName(c.Name)
		project, branch := parseCloisterName(cloisterName)
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			cloisterName,
			project,
			branch,
			c.RunningFor,
			c.State,
		)
	}

	if err := w.Flush(); err != nil {
		clog.Warn("failed to flush output: %v", err)
	}
	return nil
}

// parseCloisterName extracts project and branch from a cloister name.
// Cloister names follow the pattern: <project>-<branch>
// Returns project and branch, or the full name and empty string if unparseable.
func parseCloisterName(cloisterName string) (project, branch string) {
	// Find the last hyphen to split project and branch
	// Branch names can contain hyphens, so we use the last hyphen as the delimiter
	// This assumes project names don't contain hyphens (or we use last hyphen)
	lastHyphen := strings.LastIndex(cloisterName, "-")
	if lastHyphen == -1 {
		// No hyphen found, entire string is project
		return cloisterName, ""
	}

	return cloisterName[:lastHyphen], cloisterName[lastHyphen+1:]
}
