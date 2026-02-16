package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRewritePath_MatchingPrefix(t *testing.T) {
	got := rewritePath("/Users/xdg/.claude/plugins/cache/foo", "/Users/xdg")
	want := "/home/cloister/.claude/plugins/cache/foo"
	if got != want {
		t.Errorf("rewritePath() = %q, want %q", got, want)
	}
}

func TestRewritePath_ExactMatch(t *testing.T) {
	got := rewritePath("/Users/xdg", "/Users/xdg")
	want := "/home/cloister"
	if got != want {
		t.Errorf("rewritePath() = %q, want %q", got, want)
	}
}

func TestRewritePath_NonMatchingPrefix(t *testing.T) {
	path := "/other/path/.claude/plugins/cache/foo"
	got := rewritePath(path, "/Users/xdg")
	if got != path {
		t.Errorf("rewritePath() = %q, want unchanged %q", got, path)
	}
}

func TestRewritePath_OverlappingPrefix(t *testing.T) {
	path := "/Users/xdgfoo/something"
	got := rewritePath(path, "/Users/xdg")
	if got != path {
		t.Errorf("rewritePath() should not match overlapping prefix, got %q, want unchanged %q", got, path)
	}
}

func TestTransformMarketplaces_GithubToDirectory(t *testing.T) {
	input := `{
  "xdg-claude": {
    "source": {"source": "github", "repo": "xdg/xdg-claude"},
    "installLocation": "/Users/xdg/.claude/plugins/marketplaces/xdg-claude",
    "lastUpdated": "2026-02-04T02:29:10.262Z"
  }
}`
	validNames := map[string]bool{"xdg-claude": true}

	result, err := transformMarketplaces([]byte(input), "/Users/xdg", validNames)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	mp := parsed["xdg-claude"]

	// Verify source was changed to directory
	source := mp["source"].(map[string]any)
	if source["source"] != "directory" {
		t.Errorf("source.source = %q, want %q", source["source"], "directory")
	}
	if source["path"] != "/home/cloister/.claude/plugins/marketplaces/xdg-claude" {
		t.Errorf("source.path = %q, want container path", source["path"])
	}
	// Verify repo field was removed
	if _, ok := source["repo"]; ok {
		t.Error("source.repo should be removed")
	}

	// Verify installLocation was rewritten
	if mp["installLocation"] != "/home/cloister/.claude/plugins/marketplaces/xdg-claude" {
		t.Errorf("installLocation = %q, want container path", mp["installLocation"])
	}

	// Verify lastUpdated preserved
	if mp["lastUpdated"] != "2026-02-04T02:29:10.262Z" {
		t.Errorf("lastUpdated = %q, want preserved value", mp["lastUpdated"])
	}
}

func TestTransformMarketplaces_GitToDirectory(t *testing.T) {
	input := `{
  "my-plugin": {
    "source": {"source": "git", "url": "https://example.com/repo.git"},
    "installLocation": "/Users/xdg/.claude/plugins/marketplaces/my-plugin"
  }
}`
	validNames := map[string]bool{"my-plugin": true}

	result, err := transformMarketplaces([]byte(input), "/Users/xdg", validNames)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	source := parsed["my-plugin"]["source"].(map[string]any)
	if source["source"] != "directory" {
		t.Errorf("source.source = %q, want %q", source["source"], "directory")
	}
}

func TestTransformMarketplaces_SkipsInvalidName(t *testing.T) {
	input := `{
  "unknown-plugin": {
    "source": {"source": "github", "repo": "someone/unknown-plugin"},
    "installLocation": "/Users/xdg/.claude/plugins/marketplaces/unknown-plugin"
  }
}`
	validNames := map[string]bool{} // empty — not validated

	result, err := transformMarketplaces([]byte(input), "/Users/xdg", validNames)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	// Source should be unchanged
	source := parsed["unknown-plugin"]["source"].(map[string]any)
	if source["source"] != "github" {
		t.Errorf("source.source = %q, want %q (unchanged)", source["source"], "github")
	}
}

func TestTransformMarketplaces_SkipsDirectorySource(t *testing.T) {
	input := `{
  "local-plugin": {
    "source": {"source": "directory", "path": "/some/path"},
    "installLocation": "/Users/xdg/.claude/plugins/marketplaces/local-plugin"
  }
}`
	validNames := map[string]bool{"local-plugin": true}

	result, err := transformMarketplaces([]byte(input), "/Users/xdg", validNames)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	// Should still be directory with original path
	source := parsed["local-plugin"]["source"].(map[string]any)
	if source["source"] != "directory" {
		t.Errorf("source.source = %q, want %q", source["source"], "directory")
	}
	if source["path"] != "/some/path" {
		t.Errorf("source.path = %q, want unchanged %q", source["path"], "/some/path")
	}
}

func TestTransformMarketplaces_EmptyObject(t *testing.T) {
	result, err := transformMarketplaces([]byte(`{}`), "/Users/xdg", map[string]bool{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if len(parsed) != 0 {
		t.Errorf("expected empty object, got %v", parsed)
	}
}

func TestTransformMarketplaces_InvalidJSON(t *testing.T) {
	_, err := transformMarketplaces([]byte(`not json`), "/Users/xdg", map[string]bool{})
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestTransformMarketplaces_MultipleMixed(t *testing.T) {
	input := `{
  "valid-gh": {
    "source": {"source": "github", "repo": "user/valid-gh"},
    "installLocation": "/Users/xdg/.claude/plugins/marketplaces/valid-gh"
  },
  "invalid-gh": {
    "source": {"source": "github", "repo": "user/invalid-gh"},
    "installLocation": "/Users/xdg/.claude/plugins/marketplaces/invalid-gh"
  },
  "already-dir": {
    "source": {"source": "directory", "path": "/some/path"},
    "installLocation": "/some/path"
  }
}`
	validNames := map[string]bool{"valid-gh": true}

	result, err := transformMarketplaces([]byte(input), "/Users/xdg", validNames)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	// valid-gh: should be transformed
	if parsed["valid-gh"]["source"].(map[string]any)["source"] != "directory" {
		t.Error("valid-gh should be transformed to directory")
	}

	// invalid-gh: not in validNames, should be unchanged
	if parsed["invalid-gh"]["source"].(map[string]any)["source"] != "github" {
		t.Error("invalid-gh should remain github (not in validNames)")
	}

	// already-dir: should be unchanged
	if parsed["already-dir"]["source"].(map[string]any)["path"] != "/some/path" {
		t.Error("already-dir should remain unchanged")
	}
}

func TestTransformInstalledPlugins_RewritesPaths(t *testing.T) {
	input := `{
  "plugin-a": {
    "installPath": "/Users/xdg/.claude/plugins/cache/plugin-a-abc123",
    "version": "1.0.0",
    "scope": "user",
    "gitCommitSha": "abc123"
  }
}`

	result, err := transformInstalledPlugins([]byte(input), "/Users/xdg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	plugin := parsed["plugin-a"]
	if plugin["installPath"] != "/home/cloister/.claude/plugins/cache/plugin-a-abc123" {
		t.Errorf("installPath = %q, want container path", plugin["installPath"])
	}
	// Verify other fields preserved
	if plugin["version"] != "1.0.0" {
		t.Errorf("version = %v, want preserved", plugin["version"])
	}
	if plugin["scope"] != "user" {
		t.Errorf("scope = %v, want preserved", plugin["scope"])
	}
	if plugin["gitCommitSha"] != "abc123" {
		t.Errorf("gitCommitSha = %v, want preserved", plugin["gitCommitSha"])
	}
}

func TestTransformInstalledPlugins_EmptyMap(t *testing.T) {
	result, err := transformInstalledPlugins([]byte(`{}`), "/Users/xdg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if len(parsed) != 0 {
		t.Errorf("expected empty object, got %v", parsed)
	}
}

func TestTransformInstalledPlugins_NonMatchingPath(t *testing.T) {
	input := `{
  "plugin-a": {
    "installPath": "/other/path/plugins/cache/plugin-a"
  }
}`
	result, err := transformInstalledPlugins([]byte(input), "/Users/xdg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	if parsed["plugin-a"]["installPath"] != "/other/path/plugins/cache/plugin-a" {
		t.Errorf("installPath should be unchanged, got %q", parsed["plugin-a"]["installPath"])
	}
}

func TestFindValidMarketplaces_WithValidMarker(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid marketplace directory structure
	markerDir := filepath.Join(tmpDir, "marketplaces", "good-plugin", ".claude-plugin")
	if err := os.MkdirAll(markerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(markerDir, "marketplace.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	valid := findValidMarketplaces(tmpDir, []string{"good-plugin", "bad-plugin"})

	if !valid["good-plugin"] {
		t.Error("good-plugin should be valid")
	}
	if valid["bad-plugin"] {
		t.Error("bad-plugin should not be valid")
	}
}

func TestFindValidMarketplaces_MissingDir(t *testing.T) {
	tmpDir := t.TempDir()
	// No marketplaces dir at all
	valid := findValidMarketplaces(tmpDir, []string{"anything"})
	if len(valid) != 0 {
		t.Errorf("expected empty set, got %v", valid)
	}
}

func TestClaudePluginTransform_Integration(t *testing.T) {
	tmpDir := t.TempDir()
	hostHome := "/Users/testuser"

	// Set up plugins directory structure
	pluginsDir := filepath.Join(tmpDir, "plugins")
	if err := os.MkdirAll(filepath.Join(pluginsDir, "marketplaces", "test-mp", ".claude-plugin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(pluginsDir, "marketplaces", "test-mp", ".claude-plugin", "marketplace.json"),
		[]byte(`{}`), 0o644,
	); err != nil {
		t.Fatal(err)
	}

	// Write known_marketplaces.json
	marketplacesJSON := `{
  "test-mp": {
    "source": {"source": "github", "repo": "user/test-mp"},
    "installLocation": "/Users/testuser/.claude/plugins/marketplaces/test-mp",
    "lastUpdated": "2026-01-01T00:00:00.000Z"
  }
}`
	if err := os.WriteFile(filepath.Join(pluginsDir, "known_marketplaces.json"), []byte(marketplacesJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write installed_plugins.json
	installedJSON := `{
  "my-tool": {
    "installPath": "/Users/testuser/.claude/plugins/cache/my-tool-abc",
    "version": "2.0.0"
  }
}`
	if err := os.WriteFile(filepath.Join(pluginsDir, "installed_plugins.json"), []byte(installedJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	// Run the transform
	transform := claudePluginTransform(hostHome)
	if err := transform(tmpDir); err != nil {
		t.Fatalf("transform returned error: %v", err)
	}

	// Verify known_marketplaces.json was transformed
	data, err := os.ReadFile(filepath.Join(pluginsDir, "known_marketplaces.json"))
	if err != nil {
		t.Fatalf("failed to read transformed marketplaces: %v", err)
	}
	var marketplaces map[string]map[string]any
	if err := json.Unmarshal(data, &marketplaces); err != nil {
		t.Fatalf("failed to parse transformed marketplaces: %v", err)
	}
	source := marketplaces["test-mp"]["source"].(map[string]any)
	if source["source"] != "directory" {
		t.Errorf("marketplace source = %q, want %q", source["source"], "directory")
	}
	if source["path"] != "/home/cloister/.claude/plugins/marketplaces/test-mp" {
		t.Errorf("marketplace path = %q, want container path", source["path"])
	}

	// Verify installed_plugins.json was transformed
	data, err = os.ReadFile(filepath.Join(pluginsDir, "installed_plugins.json"))
	if err != nil {
		t.Fatalf("failed to read transformed installed: %v", err)
	}
	var installed map[string]map[string]any
	if err := json.Unmarshal(data, &installed); err != nil {
		t.Fatalf("failed to parse transformed installed: %v", err)
	}
	if installed["my-tool"]["installPath"] != "/home/cloister/.claude/plugins/cache/my-tool-abc" {
		t.Errorf("installPath = %q, want container path", installed["my-tool"]["installPath"])
	}
}

func TestClaudePluginTransform_EmptyHomeDir(t *testing.T) {
	// Transform with empty hostHomeDir should be a no-op
	transform := claudePluginTransform("")
	if err := transform(t.TempDir()); err != nil {
		t.Fatalf("transform returned error: %v", err)
	}
}

func TestClaudePluginTransform_NoPluginsDir(t *testing.T) {
	// Transform on a dir without plugins/ should be a no-op
	transform := claudePluginTransform("/Users/xdg")
	if err := transform(t.TempDir()); err != nil {
		t.Fatalf("transform returned error: %v", err)
	}
}
