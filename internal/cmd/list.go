package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/xdg/cloister/internal/container"
	"github.com/xdg/cloister/internal/docker"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List running cloister containers",
	Long: `List all running cloister containers.

Displays container name, project, branch, uptime, and status in a table format.`,
	Aliases: []string{"ls"},
	RunE:    runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	mgr := container.NewManager()
	containers, err := mgr.List()
	if err != nil {
		if errors.Is(err, docker.ErrDockerNotRunning) {
			return fmt.Errorf("Docker is not running; please start Docker and try again")
		}
		return fmt.Errorf("failed to list containers: %w", err)
	}

	// Filter out the guardian container and non-running containers
	var cloisters []container.ContainerInfo
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
		fmt.Println("No running cloister containers.")
		return nil
	}

	// Create tabwriter for table formatting
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	// Print header
	fmt.Fprintln(w, "NAME\tPROJECT\tBRANCH\tUPTIME\tSTATUS")

	// Print each container
	for _, c := range cloisters {
		project, branch := parseContainerName(c.Name)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			c.Name,
			project,
			branch,
			c.RunningFor,
			c.State,
		)
	}

	w.Flush()
	return nil
}

// parseContainerName extracts project and branch from a container name.
// Container names follow the pattern: cloister-<project>-<branch>
// Returns project and branch, or the full name and empty string if unparseable.
func parseContainerName(name string) (project, branch string) {
	// Remove "cloister-" prefix
	rest := strings.TrimPrefix(name, "cloister-")
	if rest == name {
		// No prefix found, return as-is
		return name, ""
	}

	// Find the last hyphen to split project and branch
	// Branch names can contain hyphens, so we use the last hyphen as the delimiter
	// This assumes project names don't contain hyphens (or we use last hyphen)
	lastHyphen := strings.LastIndex(rest, "-")
	if lastHyphen == -1 {
		// No hyphen found, entire string is project
		return rest, ""
	}

	return rest[:lastHyphen], rest[lastHyphen+1:]
}
