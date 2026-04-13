package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
	"github.com/usemindex/cli/config"
)

const mcpURL = "https://mcp.usemindex.dev"

// mcpToolInfo describes an AI tool supported by the mcp install command.
type mcpToolInfo struct {
	name        string
	configPath  func() string
	displayPath string // path displayed to the user
}

// mcpTools maps the tool identifier to its information.
var mcpTools = map[string]*mcpToolInfo{
	"claude-code": {
		name: "Claude Code",
		configPath: func() string {
			home, _ := os.UserHomeDir()
			return filepath.Join(home, ".claude.json")
		},
		displayPath: "~/.claude.json",
	},
	"cursor": {
		name: "Cursor",
		configPath: func() string {
			return filepath.Join(".cursor", "mcp.json")
		},
		displayPath: ".cursor/mcp.json",
	},
	"windsurf": {
		name: "Windsurf",
		configPath: func() string {
			home, _ := os.UserHomeDir()
			return filepath.Join(home, ".codeium", "windsurf", "mcp_config.json")
		},
		displayPath: "~/.codeium/windsurf/mcp_config.json",
	},
	"claude-desktop": {
		name: "Claude Desktop",
		configPath: func() string {
			home, _ := os.UserHomeDir()
			switch runtime.GOOS {
			case "darwin":
				return filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json")
			case "windows":
				appdata := os.Getenv("APPDATA")
				if appdata == "" {
					appdata = filepath.Join(home, "AppData", "Roaming")
				}
				return filepath.Join(appdata, "Claude", "claude_desktop_config.json")
			default:
				return filepath.Join(home, ".config", "Claude", "claude_desktop_config.json")
			}
		},
		displayPath: "claude_desktop_config.json",
	},
}

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Manage MCP integration with AI tools",
	Long: `Manages the Mindex MCP server integration with AI tools.

  Supported tools:
    claude-code     ~/.claude.json (global, all projects)
    cursor          .cursor/mcp.json in the current directory
    windsurf        ~/.codeium/windsurf/mcp_config.json
    claude-desktop  Claude Desktop config file

  Examples:
    mindex mcp install claude-code
    mindex mcp install cursor
    mindex mcp status`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var mcpInstallCmd = &cobra.Command{
	Use:   "install <tool>",
	Short: "Configure the Mindex MCP server in an AI tool",
	Long: `Configures the Mindex MCP server in an AI tool.

  Supported tools: claude-code, cursor, windsurf, claude-desktop

  Examples:
    mindex mcp install claude-code
    mindex mcp install cursor`,
	Args: cobra.ExactArgs(1),
	RunE: runMcpInstall,
}

var mcpStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show which AI tools have the Mindex MCP configured",
	RunE:  runMcpStatus,
}

func init() {
	mcpCmd.AddCommand(mcpInstallCmd)
	mcpCmd.AddCommand(mcpStatusCmd)
	rootCmd.AddCommand(mcpCmd)
}

func runMcpInstall(cmd *cobra.Command, args []string) error {
	toolKey := args[0]

	tool, ok := mcpTools[toolKey]
	if !ok {
		return fmt.Errorf("unknown tool '%s'. Supported: claude-code, cursor, windsurf, claude-desktop", toolKey)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("error loading configuration: %w", err)
	}
	if cfg.APIKey == "" {
		return fmt.Errorf("not authenticated. Run 'mindex auth' first.")
	}

	configPath := tool.configPath()

	if err := writeMCPConfig(configPath, cfg.APIKey); err != nil {
		return fmt.Errorf("error writing configuration: %w", err)
	}

	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "  ✓ Mindex MCP configured for %s\n", tool.name)
	fmt.Fprintf(w, "    Config: %s\n", tool.displayPath)
	fmt.Fprintf(w, "    Restart %s to activate.\n", tool.name)

	return nil
}

func runMcpStatus(cmd *cobra.Command, args []string) error {
	w := cmd.OutOrStdout()

	// display order
	ordem := []string{"claude-code", "cursor", "windsurf", "claude-desktop"}

	for _, key := range ordem {
		tool := mcpTools[key]
		configPath := tool.configPath()
		configurado := isMCPConfigured(configPath)

		if configurado {
			fmt.Fprintf(w, "  %-16s ✓ configured (%s)\n", tool.name+":", tool.displayPath)
		} else {
			fmt.Fprintf(w, "  %-16s ✗ not configured\n", tool.name+":")
		}
	}

	return nil
}

// writeMCPConfig reads the existing config file (if any), adds/updates
// the "mindex" entry in "mcpServers" preserving all other keys, and saves.
func writeMCPConfig(path, apiKey string) error {
	// load existing configuration or initialize an empty map
	existing := map[string]any{}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("error reading %s: %w", path, err)
	}
	if err == nil && len(data) > 0 {
		if err := json.Unmarshal(data, &existing); err != nil {
			return fmt.Errorf("invalid JSON in %s: %w", path, err)
		}
	}

	// ensure mcpServers exists
	mcpServers, _ := existing["mcpServers"].(map[string]any)
	if mcpServers == nil {
		mcpServers = map[string]any{}
	}

	// add/update only the "mindex" entry
	mcpServers["mindex"] = map[string]any{
		"type": "http",
		"url":  mcpURL,
		"headers": map[string]any{
			"Authorization": "Bearer " + apiKey,
		},
	}
	existing["mcpServers"] = mcpServers

	// create intermediate directories if needed
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("error creating directory %s: %w", dir, err)
	}

	// serialize with indentation
	out, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf("error serializing configuration: %w", err)
	}

	return os.WriteFile(path, out, 0600)
}

// isMCPConfigured checks whether the file exists and contains the "mindex" entry in "mcpServers".
func isMCPConfigured(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		return false
	}
	servers, ok := cfg["mcpServers"].(map[string]any)
	if !ok {
		return false
	}
	_, exists := servers["mindex"]
	return exists
}
