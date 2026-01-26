package config

import (
	"fmt"
	"log"
	"os"
	"os/exec"
)

// EditProjectConfig opens the project configuration file in the user's editor.
// If the configuration file doesn't exist, it creates a minimal one first using
// InitProjectConfig with empty values.
// The editor is determined by the EDITOR environment variable, falling back to "vi".
// After the editor exits, the configuration is loaded and validated. If validation
// fails, a warning is logged but the function returns nil (the user may want to
// fix the file manually later).
func EditProjectConfig(name string) error {
	path := ProjectConfigPath(name)

	// Create minimal config if file doesn't exist
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := InitProjectConfig(name, "", ""); err != nil {
			return fmt.Errorf("create initial config: %w", err)
		}
	}

	// Open editor
	if err := openEditor(path); err != nil {
		return err
	}

	// Validate the edited config
	_, err := LoadProjectConfig(name)
	if err != nil {
		log.Printf("warning: project config %q has errors after edit: %v", name, err)
		// Don't fail - user may want to fix it later
	}

	return nil
}

// EditGlobalConfig opens the global configuration file in the user's editor.
// If the configuration file doesn't exist, it creates the default one first using
// WriteDefaultConfig.
// The editor is determined by the EDITOR environment variable, falling back to "vi".
// After the editor exits, the configuration is loaded and validated. If validation
// fails, a warning is logged but the function returns nil (the user may want to
// fix the file manually later).
func EditGlobalConfig() error {
	path := GlobalConfigPath()

	// Create default config if file doesn't exist
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := WriteDefaultConfig(); err != nil {
			return fmt.Errorf("create default config: %w", err)
		}
	}

	// Open editor
	if err := openEditor(path); err != nil {
		return err
	}

	// Validate the edited config
	_, err := LoadGlobalConfig()
	if err != nil {
		log.Printf("warning: global config has errors after edit: %v", err)
		// Don't fail - user may want to fix it later
	}

	return nil
}

// openEditor opens the specified file in the user's editor.
// The editor is determined by the EDITOR environment variable, falling back to "vi".
func openEditor(path string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	cmd := exec.Command(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("editor %q failed: %w", editor, err)
	}

	return nil
}
