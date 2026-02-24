package cmd

import (
	"errors"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/xdg/cloister/internal/clog"
	"github.com/xdg/cloister/internal/cloister"
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

	// Load registry once for accurate project/branch resolution.
	// If loading fails, reg will be nil and we fall back to ParseCloisterName.
	reg, err := cloister.LoadRegistry()
	if err != nil {
		clog.Debug("failed to load cloister registry, falling back to name parsing: %v", err)
		reg = nil
	}

	// Print each cloister
	for _, c := range cloisters {
		cloisterName := container.NameToCloisterName(c.Name)
		project, branch := resolveCloisterInfo(cloisterName, reg)
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

// resolveCloisterInfo resolves a cloister name to its project and branch.
// If the registry is available and contains the cloister name, it uses the
// registry entry for accurate resolution. Otherwise, it falls back to
// ParseCloisterName, which is ambiguous for hyphenated project names.
func resolveCloisterInfo(cloisterName string, reg *cloister.Registry) (project, branch string) {
	if reg != nil {
		if entry := reg.FindByName(cloisterName); entry != nil {
			return entry.ProjectName, entry.Branch
		}
	}
	return container.ParseCloisterName(cloisterName)
}
