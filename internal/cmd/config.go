package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/xdg/cloister/internal/config"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage global configuration",
	Long: `Manage cloister's global configuration.

The global configuration file is stored at ~/.config/cloister/config.yaml
(or $XDG_CONFIG_HOME/cloister/config.yaml if XDG_CONFIG_HOME is set).

Use the subcommands to view, edit, or initialize the configuration.`,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show effective global config",
	Long: `Print the effective global configuration as YAML.

If no config file exists, shows the default configuration.`,
	RunE: runConfigShow,
}

var configEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Edit global config in $EDITOR",
	Long: `Open the global configuration file in your editor.

The editor is determined by the EDITOR environment variable, falling back to vi.
If the configuration file doesn't exist, a default one is created first.`,
	RunE: runConfigEdit,
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print config file path",
	Long:  `Print the path to the global configuration file.`,
	Run:   runConfigPath,
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create default config file",
	Long: `Create the default global configuration file if it doesn't exist.

This creates a fully-commented configuration file with all default values.
If the file already exists, this command does nothing.`,
	RunE: runConfigInit,
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configEditCmd)
	configCmd.AddCommand(configPathCmd)
	configCmd.AddCommand(configInitCmd)
}

func runConfigShow(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadGlobalConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to serialize config: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

func runConfigEdit(cmd *cobra.Command, args []string) error {
	if err := config.EditGlobalConfig(); err != nil {
		return fmt.Errorf("failed to edit config: %w", err)
	}
	return nil
}

func runConfigPath(cmd *cobra.Command, args []string) {
	fmt.Println(config.GlobalConfigPath())
}

func runConfigInit(cmd *cobra.Command, args []string) error {
	path := config.GlobalConfigPath()

	if err := config.WriteDefaultConfig(); err != nil {
		return fmt.Errorf("failed to create config: %w", err)
	}

	fmt.Printf("Created default config at: %s\n", path)
	return nil
}
