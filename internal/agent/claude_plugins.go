package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xdg/cloister/internal/clog"
)

// claudePluginTransform returns a transform function that rewrites Claude Code
// plugin config files in the staging directory so they work inside a container.
//
// It transforms:
//   - plugins/known_marketplaces.json: changes github/git sources to directory sources
//   - plugins/installed_plugins.json: rewrites installPath host paths to container paths
//
// The transform is non-fatal: individual failures are logged as warnings but
// do not block container startup.
func claudePluginTransform(hostHomeDir string) func(tmpDir string) error {
	return func(tmpDir string) error {
		if hostHomeDir == "" {
			return nil
		}

		pluginsDir := filepath.Join(tmpDir, "plugins")

		// Transform known_marketplaces.json
		marketplacesPath := filepath.Join(pluginsDir, "known_marketplaces.json")
		if data, err := os.ReadFile(marketplacesPath); err == nil {
			// Determine which marketplaces have valid data in staging dir
			validNames := findValidMarketplaces(pluginsDir, extractMarketplaceNames(data))
			if transformed, err := transformMarketplaces(data, hostHomeDir, validNames); err != nil {
				clog.Warn("failed to transform known_marketplaces.json: %v", err)
			} else {
				if err := os.WriteFile(marketplacesPath, transformed, 0o600); err != nil { //nolint:gosec // G306: plugin config readable by owner only in container
					clog.Warn("failed to write transformed known_marketplaces.json: %v", err)
				}
			}
		}

		// Transform installed_plugins.json
		installedPath := filepath.Join(pluginsDir, "installed_plugins.json")
		if data, err := os.ReadFile(installedPath); err == nil {
			if transformed, err := transformInstalledPlugins(data, hostHomeDir); err != nil {
				clog.Warn("failed to transform installed_plugins.json: %v", err)
			} else {
				if err := os.WriteFile(installedPath, transformed, 0o600); err != nil { //nolint:gosec // G306: plugin config readable by owner only in container
					clog.Warn("failed to write transformed installed_plugins.json: %v", err)
				}
			}
		}

		return nil
	}
}

// extractMarketplaceNames returns the top-level keys from a known_marketplaces.json blob.
func extractMarketplaceNames(data []byte) []string {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	names := make([]string, 0, len(raw))
	for name := range raw {
		names = append(names, name)
	}
	return names
}

// transformMarketplaces transforms known_marketplaces.json for container use.
// For each marketplace in validNames whose source is "github" or "git", it:
//   - Replaces the source with {"source": "directory", "path": "<containerPath>"}
//   - Rewrites installLocation from host path to container path
//
// Marketplaces not in validNames or with other source types are left unchanged.
// All unknown fields are preserved.
func transformMarketplaces(data []byte, hostHomeDir string, validNames map[string]bool) ([]byte, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal marketplaces: %w", err)
	}

	for name, entry := range raw {
		if !validNames[name] {
			continue
		}

		var marketplace map[string]any
		if err := json.Unmarshal(entry, &marketplace); err != nil {
			continue
		}

		// Check source type
		sourceRaw, ok := marketplace["source"]
		if !ok {
			continue
		}
		sourceMap, ok := sourceRaw.(map[string]any)
		if !ok {
			continue
		}
		sourceType, ok := sourceMap["source"].(string)
		if !ok || (sourceType != "github" && sourceType != "git") {
			continue
		}

		// Rewrite source to directory type
		containerPath := rewritePath(
			filepath.Join(hostHomeDir, ".claude", "plugins", "marketplaces", name),
			hostHomeDir,
		)
		marketplace["source"] = map[string]any{
			"source": "directory",
			"path":   containerPath,
		}

		// Rewrite installLocation if present
		if loc, ok := marketplace["installLocation"].(string); ok {
			marketplace["installLocation"] = rewritePath(loc, hostHomeDir)
		}

		// Marshal back to preserve in raw map
		updated, err := json.Marshal(marketplace)
		if err != nil {
			continue
		}
		raw[name] = json.RawMessage(updated)
	}

	result, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal marketplaces: %w", err)
	}
	return result, nil
}

// transformInstalledPlugins transforms installed_plugins.json for container use.
// It rewrites all installPath values from host paths to container paths.
// All other fields are preserved unchanged.
func transformInstalledPlugins(data []byte, hostHomeDir string) ([]byte, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal installed plugins: %w", err)
	}

	for name, entry := range raw {
		var plugin map[string]any
		if err := json.Unmarshal(entry, &plugin); err != nil {
			continue
		}

		if installPath, ok := plugin["installPath"].(string); ok {
			rewritten := rewritePath(installPath, hostHomeDir)
			if rewritten != installPath {
				plugin["installPath"] = rewritten
				updated, err := json.Marshal(plugin)
				if err != nil {
					continue
				}
				raw[name] = json.RawMessage(updated)
			}
		}
	}

	result, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal installed plugins: %w", err)
	}
	return result, nil
}

// findValidMarketplaces checks which named marketplaces have valid plugin data
// in the staging directory. A marketplace is valid if its
// marketplaces/<name>/.claude-plugin/marketplace.json file exists.
func findValidMarketplaces(pluginsDir string, names []string) map[string]bool {
	valid := make(map[string]bool)
	for _, name := range names {
		markerPath := filepath.Join(pluginsDir, "marketplaces", name, ".claude-plugin", "marketplace.json")
		if _, err := os.Stat(markerPath); err == nil {
			valid[name] = true
		}
	}
	return valid
}

// rewritePath replaces the host home directory prefix with the container home
// directory. If the path doesn't start with hostHomeDir, it's returned unchanged.
func rewritePath(path, hostHomeDir string) string {
	if path == hostHomeDir {
		return containerHomeDir
	}
	if strings.HasPrefix(path, hostHomeDir+"/") {
		return containerHomeDir + path[len(hostHomeDir):]
	}
	return path
}
