package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/xdg/cloister/internal/config"
	"github.com/xdg/cloister/internal/container"
	"github.com/xdg/cloister/internal/project"
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

func runProjectList(cmd *cobra.Command, args []string) error {
	reg, err := project.LoadRegistry()
	if err != nil {
		return fmt.Errorf("failed to load registry: %w", err)
	}

	projects := reg.List()
	if len(projects) == 0 {
		fmt.Println("No registered projects.")
		return nil
	}

	// Create tabwriter for table formatting
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	// Print header
	fmt.Fprintln(w, "NAME\tROOT\tREMOTE\tLAST USED")

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

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			p.Name,
			p.Root,
			remote,
			lastUsed,
		)
	}

	w.Flush()
	return nil
}

func runProjectShow(cmd *cobra.Command, args []string) error {
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
	fmt.Printf("Name:        %s\n", entry.Name)
	fmt.Printf("Root:        %s\n", entry.Root)
	fmt.Printf("Remote:      %s\n", entry.Remote)
	fmt.Printf("Config:      %s\n", config.ProjectConfigPath(name))

	// Print last used time
	if !entry.LastUsed.IsZero() {
		fmt.Printf("Last Used:   %s\n", entry.LastUsed.Format("2006-01-02 15:04:05"))
	} else {
		fmt.Printf("Last Used:   never\n")
	}

	// Print allowlist additions if any
	if len(cfg.Proxy.Allow) > 0 {
		fmt.Println()
		fmt.Println("Allowlist Additions:")
		for _, allow := range cfg.Proxy.Allow {
			fmt.Printf("  - %s\n", allow.Domain)
		}
	}

	// Print auto-approve patterns if any
	if len(cfg.Commands.AutoApprove) > 0 {
		fmt.Println()
		fmt.Println("Auto-Approve Patterns:")
		for _, pattern := range cfg.Commands.AutoApprove {
			fmt.Printf("  - %s\n", pattern.Pattern)
		}
	}

	// Print refs if any
	if len(cfg.Refs) > 0 {
		fmt.Println()
		fmt.Println("Reference Paths:")
		for _, ref := range cfg.Refs {
			fmt.Printf("  - %s\n", ref)
		}
	}

	return nil
}

func runProjectEdit(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Check if project exists in registry (warn but don't fail)
	reg, err := project.LoadRegistry()
	if err != nil {
		return fmt.Errorf("failed to load registry: %w", err)
	}

	entry := reg.FindByName(name)
	if entry == nil {
		fmt.Fprintf(os.Stderr, "Warning: project %q not found in registry\n", name)
		fmt.Fprintf(os.Stderr, "Use 'cloister project list' to see registered projects.\n")
		fmt.Fprintf(os.Stderr, "Creating config file anyway. Register with 'cloister start' from the project directory.\n\n")
	}

	if err := config.EditProjectConfig(name); err != nil {
		return fmt.Errorf("failed to edit config: %w", err)
	}

	return nil
}

func runProjectRemove(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Load registry
	reg, err := project.LoadRegistry()
	if err != nil {
		return fmt.Errorf("failed to load registry: %w", err)
	}

	// Check if project exists
	entry := reg.FindByName(name)
	if entry == nil {
		return fmt.Errorf("project %q not found in registry\n\nHint: Use 'cloister project list' to see registered projects", name)
	}

	// Check for running cloisters for this project
	mgr := container.NewManager()
	containers, err := mgr.List()
	if err != nil {
		// If Docker isn't running, we can't check - proceed with warning
		fmt.Fprintf(os.Stderr, "Warning: could not check for running cloisters (Docker may not be running)\n")
	} else {
		for _, c := range containers {
			if c.Name == "cloister-guardian" || c.State != "running" {
				continue
			}
			cloisterName := container.ContainerNameToCloisterName(c.Name)
			// Check if this cloister belongs to the project
			if strings.HasPrefix(cloisterName, name+"-") || cloisterName == name {
				return fmt.Errorf("cannot remove project %q: cloister %q is running; stop it first with 'cloister stop %s'",
					name, cloisterName, cloisterName)
			}
		}
	}

	// Remove from registry
	if err := reg.Remove(name); err != nil {
		if errors.Is(err, project.ErrProjectNotFound) {
			return fmt.Errorf("project %q not found in registry", name)
		}
		return fmt.Errorf("failed to remove project: %w", err)
	}

	// Save updated registry
	if err := project.SaveRegistry(reg); err != nil {
		return fmt.Errorf("failed to save registry: %w", err)
	}

	fmt.Printf("Removed project %q from registry\n", name)

	// Optionally remove config file
	if projectRemoveConfig {
		configPath := config.ProjectConfigPath(name)
		if err := os.Remove(configPath); err != nil {
			if os.IsNotExist(err) {
				fmt.Println("Config file does not exist.")
			} else {
				return fmt.Errorf("failed to remove config file: %w", err)
			}
		} else {
			fmt.Printf("Removed config file: %s\n", configPath)
		}
	}

	return nil
}
