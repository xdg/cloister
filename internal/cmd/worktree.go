package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/xdg/cloister/internal/clog"
	"github.com/xdg/cloister/internal/cloister"
	"github.com/xdg/cloister/internal/container"
	"github.com/xdg/cloister/internal/project"
	"github.com/xdg/cloister/internal/term"
	"github.com/xdg/cloister/internal/worktree"
)

var worktreeProjectFlag string
var worktreeRemoveForceFlag bool
var worktreeRemoveProjectFlag string

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

var worktreeRemoveCmd = &cobra.Command{
	Use:   "remove <branch>",
	Short: "Remove a managed worktree",
	Args:  cobra.ExactArgs(1),
	RunE:  runWorktreeRemove,
}

func init() {
	rootCmd.AddCommand(worktreeCmd)
	worktreeCmd.AddCommand(worktreeListCmd)
	worktreeCmd.AddCommand(worktreeRemoveCmd)

	worktreeListCmd.Flags().StringVarP(&worktreeProjectFlag, "project", "p", "", "project name (default: detect from cwd)")

	worktreeRemoveCmd.Flags().BoolVarP(&worktreeRemoveForceFlag, "force", "f", false, "force removal even if worktree is dirty or container is running")
	worktreeRemoveCmd.Flags().StringVarP(&worktreeRemoveProjectFlag, "project", "p", "", "project name (default: detect from cwd)")
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

func runWorktreeRemove(_ *cobra.Command, args []string) error {
	branch := args[0]
	force := worktreeRemoveForceFlag

	projectName, err := resolveWorktreeProject(worktreeRemoveProjectFlag)
	if err != nil {
		return err
	}

	cloisterName := container.GenerateWorktreeCloisterName(projectName, branch)

	reg, err := cloister.LoadRegistry()
	if err != nil {
		return fmt.Errorf("failed to load cloister registry: %w", err)
	}

	entry := reg.FindByName(cloisterName)
	if entry == nil {
		return fmt.Errorf("worktree %q not found for project %q (not a cloister-managed worktree)", branch, projectName)
	}

	hostPath := entry.HostPath

	if err := checkWorktreeDirty(hostPath, force); err != nil {
		return err
	}

	if err := checkWorktreeContainer(cloisterName, force); err != nil {
		return err
	}

	// Remove the git worktree.
	if err := worktree.Remove(hostPath, force); err != nil {
		return fmt.Errorf("failed to remove worktree: %w", err)
	}

	// Remove registry entry and save.
	if err := reg.Remove(cloisterName); err != nil {
		clog.Warn("failed to remove registry entry: %v", err)
	}
	if err := cloister.SaveRegistry(reg); err != nil {
		return fmt.Errorf("failed to save registry: %w", err)
	}

	// Best-effort cleanup of empty parent directories under worktree base dir.
	cleanupEmptyWorktreeParent(hostPath)

	term.Printf("Removed worktree: %s\n", branch)
	return nil
}

// resolveWorktreeProject resolves the project name from the flag or by
// detecting the git root from the current working directory.
func resolveWorktreeProject(flagValue string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	gitRoot, err := project.DetectGitRoot(".")
	if err != nil {
		return "", fmt.Errorf("failed to detect project: %w\n\nHint: Use -p to specify a project name", err)
	}
	name, err := project.Name(gitRoot)
	if err != nil {
		return "", fmt.Errorf("failed to determine project name: %w", err)
	}
	return name, nil
}

// checkWorktreeDirty checks if the worktree has uncommitted changes.
// Returns an error if dirty and force is false.
func checkWorktreeDirty(hostPath string, force bool) error {
	dirty, err := worktree.IsDirty(hostPath)
	if err != nil {
		clog.Warn("failed to check worktree status: %v", err)
		return nil
	}
	if dirty && !force {
		return fmt.Errorf("worktree has uncommitted changes; commit, stash, or use -f to force")
	}
	return nil
}

// checkWorktreeContainer checks if the container is running and optionally stops it.
// Returns an error if running and force is false.
func checkWorktreeContainer(cloisterName string, force bool) error {
	containerName := container.CloisterNameToContainerName(cloisterName)
	mgr := container.NewManager()
	running, err := mgr.IsRunning(containerName)
	if err != nil {
		clog.Debug("failed to check container status for %s: %v", containerName, err)
		return nil
	}
	if !running {
		return nil
	}
	if !force {
		return fmt.Errorf("cloister container is still running; stop it first or use -f to force")
	}
	// Best-effort stop.
	if stopErr := cloister.Stop(containerName, ""); stopErr != nil {
		clog.Warn("failed to stop container %s: %v", containerName, stopErr)
	}
	return nil
}

// cleanupEmptyWorktreeParent removes the project directory under the worktree
// base dir if it is empty after a worktree removal.
func cleanupEmptyWorktreeParent(hostPath string) {
	baseDir, err := worktree.BaseDir()
	if err != nil {
		return
	}

	// hostPath is like <baseDir>/<project>/<branch>; parent is <baseDir>/<project>.
	projectDir := filepath.Dir(hostPath)

	// Safety: only clean up if projectDir is directly under baseDir.
	if filepath.Dir(projectDir) != baseDir {
		return
	}

	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return
	}
	if len(entries) == 0 {
		_ = os.Remove(projectDir)
	}
}
