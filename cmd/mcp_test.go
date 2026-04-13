package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// --- writeMCPConfig ---

func TestWriteMCPConfig_NewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mcp.json")

	if err := writeMCPConfig(path, "sk-test-key"); err != nil {
		t.Fatalf("writeMCPConfig() error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("error reading created file: %v", err)
	}

	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	servers, ok := cfg["mcpServers"].(map[string]any)
	if !ok {
		t.Fatal("mcpServers missing or wrong type")
	}

	mindex, ok := servers["mindex"].(map[string]any)
	if !ok {
		t.Fatal("'mindex' entry missing in mcpServers")
	}

	if mindex["type"] != "http" {
		t.Errorf("type: expected 'http', got %q", mindex["type"])
	}
	if mindex["url"] != mcpURL {
		t.Errorf("url: expected %q, got %q", mcpURL, mindex["url"])
	}

	headers, ok := mindex["headers"].(map[string]any)
	if !ok {
		t.Fatal("headers missing or wrong type")
	}
	if headers["Authorization"] != "Bearer sk-test-key" {
		t.Errorf("Authorization: expected 'Bearer sk-test-key', got %q", headers["Authorization"])
	}
}

func TestWriteMCPConfig_MergeExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")

	// existing file with another server configured
	existing := map[string]any{
		"mcpServers": map[string]any{
			"other-server": map[string]any{
				"type":    "stdio",
				"command": "other-bin",
			},
		},
		"otherKey": "preserved-value",
	}
	data, _ := json.Marshal(existing)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("error creating test file: %v", err)
	}

	if err := writeMCPConfig(path, "sk-new-key"); err != nil {
		t.Fatalf("writeMCPConfig() error: %v", err)
	}

	result, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("error reading result file: %v", err)
	}

	var cfg map[string]any
	if err := json.Unmarshal(result, &cfg); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// top-level key preserved
	if cfg["otherKey"] != "preserved-value" {
		t.Errorf("otherKey should be preserved, got %v", cfg["otherKey"])
	}

	servers, ok := cfg["mcpServers"].(map[string]any)
	if !ok {
		t.Fatal("mcpServers missing or wrong type")
	}

	// existing server preserved
	if _, ok := servers["other-server"]; !ok {
		t.Error("'other-server' should have been preserved")
	}

	// mindex entry added
	mindex, ok := servers["mindex"].(map[string]any)
	if !ok {
		t.Fatal("'mindex' entry missing in mcpServers")
	}

	headers, _ := mindex["headers"].(map[string]any)
	if headers["Authorization"] != "Bearer sk-new-key" {
		t.Errorf("Authorization incorrect: %v", headers["Authorization"])
	}
}

func TestWriteMCPConfig_UpdatesExistingEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")

	// file with mindex already configured with old key
	existing := map[string]any{
		"mcpServers": map[string]any{
			"mindex": map[string]any{
				"type": "http",
				"url":  mcpURL,
				"headers": map[string]any{
					"Authorization": "Bearer sk-old-key",
				},
			},
		},
	}
	data, _ := json.Marshal(existing)
	os.WriteFile(path, data, 0600)

	if err := writeMCPConfig(path, "sk-new-key"); err != nil {
		t.Fatalf("writeMCPConfig() error: %v", err)
	}

	result, _ := os.ReadFile(path)
	var cfg map[string]any
	json.Unmarshal(result, &cfg)

	servers := cfg["mcpServers"].(map[string]any)
	mindex := servers["mindex"].(map[string]any)
	headers := mindex["headers"].(map[string]any)

	if headers["Authorization"] != "Bearer sk-new-key" {
		t.Errorf("key should have been updated, got %v", headers["Authorization"])
	}
}

func TestWriteMCPConfig_CreatesSubdirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".cursor", "mcp.json")

	if err := writeMCPConfig(path, "sk-key"); err != nil {
		t.Fatalf("writeMCPConfig() should create subdirectory, error: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("file was not created in subdirectory: %v", err)
	}
}

func TestWriteMCPConfig_NoMcpServersInExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	// file without mcpServers
	existing := map[string]any{"otherConfig": true}
	data, _ := json.Marshal(existing)
	os.WriteFile(path, data, 0600)

	if err := writeMCPConfig(path, "sk-key"); err != nil {
		t.Fatalf("writeMCPConfig() error: %v", err)
	}

	result, _ := os.ReadFile(path)
	var cfg map[string]any
	json.Unmarshal(result, &cfg)

	if _, ok := cfg["mcpServers"]; !ok {
		t.Error("mcpServers should have been created")
	}
	if cfg["otherConfig"] != true {
		t.Error("otherConfig should have been preserved")
	}
}

// --- isMCPConfigured ---

func TestIsMCPConfigured_Configured(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")

	writeMCPConfig(path, "sk-key")

	if !isMCPConfigured(path) {
		t.Error("isMCPConfigured() should return true after writeMCPConfig()")
	}
}

func TestIsMCPConfigured_NotConfigured(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")

	existing := map[string]any{
		"mcpServers": map[string]any{
			"other": map[string]any{"type": "stdio"},
		},
	}
	data, _ := json.Marshal(existing)
	os.WriteFile(path, data, 0600)

	if isMCPConfigured(path) {
		t.Error("isMCPConfigured() should return false when 'mindex' does not exist in mcpServers")
	}
}

func TestIsMCPConfigured_NonExistentFile(t *testing.T) {
	if isMCPConfigured("/path/that/does/not/exist/mcp.json") {
		t.Error("isMCPConfigured() should return false for a non-existent file")
	}
}

// --- path resolution ---

func TestMcpToolsPathResolution(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		key     string
		check   func(string) bool
		desc    string
	}{
		{
			key: "claude-code",
			check: func(p string) bool { return p == ".mcp.json" },
			desc:  "should be .mcp.json in the current directory",
		},
		{
			key: "cursor",
			check: func(p string) bool {
				return strings.HasSuffix(filepath.ToSlash(p), ".cursor/mcp.json")
			},
			desc: "should be .cursor/mcp.json in the current directory",
		},
		{
			key: "windsurf",
			check: func(p string) bool {
				expected := filepath.Join(home, ".codeium", "windsurf", "mcp_config.json")
				return p == expected
			},
			desc: "should be ~/.codeium/windsurf/mcp_config.json",
		},
		{
			key: "claude-desktop",
			check: func(p string) bool {
				switch runtime.GOOS {
				case "darwin":
					return p == filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json")
				case "windows":
					appdata := os.Getenv("APPDATA")
					if appdata == "" {
						appdata = filepath.Join(home, "AppData", "Roaming")
					}
					return p == filepath.Join(appdata, "Claude", "claude_desktop_config.json")
				default:
					return p == filepath.Join(home, ".config", "Claude", "claude_desktop_config.json")
				}
			},
			desc: "should resolve the correct path based on the OS",
		},
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			tool, ok := mcpTools[tc.key]
			if !ok {
				t.Fatalf("tool '%s' not found in mcpTools", tc.key)
			}
			path := tool.configPath()
			if !tc.check(path) {
				t.Errorf("%s: got path %q which does not satisfy check (%s)", tc.key, path, tc.desc)
			}
		})
	}
}

func TestMcpTools_AllToolsPresent(t *testing.T) {
	expected := []string{"claude-code", "cursor", "windsurf", "claude-desktop"}
	for _, key := range expected {
		if _, ok := mcpTools[key]; !ok {
			t.Errorf("tool '%s' not found in mcpTools", key)
		}
	}
}
