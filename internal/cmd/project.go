package cmd

import (
	"errors"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/xdg/cloister/internal/clog"
	"github.com/xdg/cloister/internal/config"
	"github.com/xdg/cloister/internal/container"
	"github.com/xdg/cloister/internal/project"
	"github.com/xdg/cloister/internal/term"
)

var projectRemoveConfig bool

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage project registry",
	Long: `Manage the cloister project registry.

The project registry tracks all projects that have been used with cloister.
Each project can have its own configuration file with project-specific
allowlist additions and command patterns.`,
}

var projectListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List registered projects",
	Long:    `List all projects registered with cloister.`,
	Aliases: []string{"ls"},
	RunE:    runProjectList,
}

var projectShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show project details",
	Long: `Show details for a registered project.

Displays the project name, root path, remote URL, config file path,
and any project-specific allowlist additions.`,
	Args: cobra.ExactArgs(1),
	RunE: runProjectShow,
}

var projectEditCmd = &cobra.Command{
	Use:   "edit <name>",
	Short: "Edit project config",
	Long: `Open the project configuration file in your editor.

The editor is determined by the EDITOR environment variable, falling back to vi.
If the configuration file doesn't exist, a minimal one is created first.`,
	Args: cobra.ExactArgs(1),
	RunE: runProjectEdit,
}

var projectRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove project from registry",
	Long: `Remove a project from the cloister registry.

By default, only removes the project from the registry. Use --config to also
delete the project's configuration file.

Refuses to remove a project if there are running cloisters for that project.`,
	Aliases: []string{"rm"},
	Args:    cobra.ExactArgs(1),
	RunE:    runProjectRemove,
}

func init() {
	rootCmd.AddCommand(projectCmd)
	projectCmd.AddCommand(projectListCmd)
	projectCmd.AddCommand(projectShowCmd)
	projectCmd.AddCommand(projectEditCmd)
	projectCmd.AddCommand(projectRemoveCmd)

	projectRemoveCmd.Flags().BoolVar(&projectRemoveConfig, "config", false, "Also remove project config file")
}

func runProjectList(_ *cobra.Command, _ []string) error {
	reg, err := project.LoadRegistry()
	if err != nil {
		return fmt.Errorf("failed to load registry: %w", err)
	}

	projects := reg.List()
	if len(projects) == 0 {
		term.Println("No registered projects.")
		return nil
	}

	// Create tabwriter for table formatting
	w := tabwriter.NewWriter(term.Stdout(), 0, 0, 2, ' ', 0)

	// Print header
	_, _ = fmt.Fprintln(w, "NAME\tROOT\tREMOTE\tLAST USED")

	// Print each project
	for _, p := range projects {
		// Truncate remote URL if too long
		remote := p.Remote
		if len(remote) > 50 {
			remote = remote[:47] + "..."
		}

		// Format last used time
		lastUsed := "never"
		if !p.LastUsed.IsZero() {
			lastUsed = p.LastUsed.Format("2006-01-02 15:04")
		}

		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			p.Name,
			p.Root,
			remote,
			lastUsed,
		)
	}

	if err := w.Flush(); err != nil {
		clog.Warn("failed to flush output: %v", err)
	}
	return nil
}

func runProjectShow(_ *cobra.Command, args []string) error {
	name := args[0]

	// Look up project in registry
	reg, err := project.LoadRegistry()
	if err != nil {
		return fmt.Errorf("failed to load registry: %w", err)
	}

	entry := reg.FindByName(name)
	if entry == nil {
		return fmt.Errorf("project %q not found in registry\n\nHint: Use 'cloister project list' to see registered projects", name)
	}

	// Load project config (may not exist, use defaults)
	cfg, err := config.LoadProjectConfig(name)
	if err != nil {
		return fmt.Errorf("failed to load project config: %w", err)
	}

	// Print project details
	term.Printf("Name:        %s\n", entry.Name)
	term.Printf("Root:        %s\n", entry.Root)
	term.Printf("Remote:      %s\n", entry.Remote)
	term.Printf("Config:      %s\n", config.ProjectConfigPath(name))

	// Print last used time
	if !entry.LastUsed.IsZero() {
		term.Printf("Last Used:   %s\n", entry.LastUsed.Format("2006-01-02 15:04:05"))
	} else {
		term.Printf("Last Used:   never\n")
	}

	// Print allowlist additions if any
	if len(cfg.Proxy.Allow) > 0 {
		term.Println()
		term.Println("Allowlist Additions:")
		for _, allow := range cfg.Proxy.Allow {
			term.Printf("  - %s\n", allow.Domain)
		}
	}

	// Print auto-approve patterns if any
	if len(cfg.Hostexec.AutoApprove) > 0 {
		term.Println()
		term.Println("Auto-Approve Patterns:")
		for _, pattern := range cfg.Hostexec.AutoApprove {
			term.Printf("  - %s\n", pattern.Pattern)
		}
	}

	// Print refs if any
	if len(cfg.Refs) > 0 {
		term.Println()
		term.Println("Reference Paths:")
		for _, ref := range cfg.Refs {
			term.Printf("  - %s\n", ref)
		}
	}

	return nil
}

func runProjectEdit(_ *cobra.Command, args []string) error {
	name := args[0]

	// Check if project exists in registry (warn but don't fail)
	reg, err := project.LoadRegistry()
	if err != nil {
		return fmt.Errorf("failed to load registry: %w", err)
	}

	entry := reg.FindByName(name)
	if entry == nil {
		term.Warn("project %q not found in registry", name)
		term.Printf("Use 'cloister project list' to see registered projects.\n")
		term.Printf("Creating config file anyway. Register with 'cloister start' from the project directory.\n\n")
	}

	if err := config.EditProjectConfig(name); err != nil {
		return fmt.Errorf("failed to edit config: %w", err)
	}

	return nil
}

func runProjectRemove(_ *cobra.Command, args []string) error {
	name := args[0]

	reg, err := project.LoadRegistry()
	if err != nil {
		return fmt.Errorf("failed to load registry: %w", err)
	}

	if entry := reg.FindByName(name); entry == nil {
		return fmt.Errorf("project %q not found in registry\n\nHint: Use 'cloister project list' to see registered projects", name)
	}

	// Error ignored: HasRunningCloister returns ("", nil) when Docker is unavailable,
	// so failure means "no running cloisters" — safe to proceed with removal.
	if running, _ := container.NewManager().HasRunningCloister(name); running != "" {
		return fmt.Errorf("cannot remove project %q: cloister %q is running; stop it first with 'cloister stop %s'",
			name, running, running)
	}

	if err := reg.Remove(name); err != nil {
		if errors.Is(err, project.ErrProjectNotFound) {
			return fmt.Errorf("project %q not found in registry", name)
		}
		return fmt.Errorf("failed to remove project: %w", err)
	}

	if err := project.SaveRegistry(reg); err != nil {
		return fmt.Errorf("failed to save registry: %w", err)
	}

	term.Printf("Removed project %q from registry\n", name)

	if projectRemoveConfig {
		removeProjectConfigFile(name)
	}

	return nil
}

// removeProjectConfigFile removes the project config file, printing the result.
func removeProjectConfigFile(name string) {
	configPath := config.ProjectConfigPath(name)
	if err := os.Remove(configPath); err != nil {
		if !os.IsNotExist(err) {
			term.Printf("Warning: failed to remove config file: %v\n", err)
		} else {
			term.Println("Config file does not exist.")
		}
	} else {
		term.Printf("Removed config file: %s\n", configPath)
	}
}
