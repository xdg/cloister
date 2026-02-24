package cmd

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/xdg/cloister/internal/clog"
	"github.com/xdg/cloister/internal/cloister"
	"github.com/xdg/cloister/internal/container"
	"github.com/xdg/cloister/internal/project"
	"github.com/xdg/cloister/internal/term"
)

var worktreeProjectFlag string

var worktreeCmd = &cobra.Command{
	Use:   "worktree",
	Short: "Manage worktrees",
	Long: `Manage git worktrees associated with cloister projects.

Provides subcommands to list and manage worktrees that have been registered
with cloister.`,
}

var worktreeListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List managed worktrees for a project",
	Aliases: []string{"ls"},
	RunE:    runWorktreeList,
}

func init() {
	rootCmd.AddCommand(worktreeCmd)
	worktreeCmd.AddCommand(worktreeListCmd)

	worktreeListCmd.Flags().StringVarP(&worktreeProjectFlag, "project", "p", "", "project name (default: detect from cwd)")
}

func runWorktreeList(_ *cobra.Command, _ []string) error {
	projectName := worktreeProjectFlag

	if projectName == "" {
		gitRoot, err := project.DetectGitRoot(".")
		if err != nil {
			return fmt.Errorf("failed to detect project: %w\n\nHint: Use -p to specify a project name", err)
		}
		name, err := project.Name(gitRoot)
		if err != nil {
			return fmt.Errorf("failed to determine project name: %w", err)
		}
		projectName = name
	}

	reg, err := cloister.LoadRegistry()
	if err != nil {
		return fmt.Errorf("failed to load cloister registry: %w", err)
	}

	entries := reg.FindByProject(projectName)
	if len(entries) == 0 {
		term.Printf("No managed worktrees for project %q.\n", projectName)
		return nil
	}

	mgr := container.NewManager()

	w := tabwriter.NewWriter(term.Stdout(), 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "WORKTREE\tPATH\tCLOISTER\tSTATUS")

	for _, entry := range entries {
		worktreeLabel := entry.Branch
		if worktreeLabel == "" {
			worktreeLabel = "(main)"
		}

		status := "stopped"
		containerName := container.CloisterNameToContainerName(entry.CloisterName)
		running, err := mgr.IsRunning(containerName)
		if err != nil {
			clog.Debug("failed to check container status for %s: %v", containerName, err)
		} else if running {
			status = "running"
		}

		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			worktreeLabel,
			entry.HostPath,
			entry.CloisterName,
			status,
		)
	}

	if err := w.Flush(); err != nil {
		clog.Warn("failed to flush output: %v", err)
	}
	return nil
}
