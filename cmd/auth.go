package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/usemindex/cli/api"
	"github.com/usemindex/cli/config"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Configure your API key",
	Long:  `Validates and saves your Mindex API key to ~/.mindex/config.json.`,
	RunE:  runAuth,
}

func init() {
	rootCmd.AddCommand(authCmd)
}

func runAuth(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("error loading configuration: %w", err)
	}

	fmt.Print("Paste your API key (sk-...): ")
	reader := bufio.NewReader(os.Stdin)
	apiKey, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("error reading API key: %w", err)
	}
	apiKey = strings.TrimSpace(apiKey)

	if apiKey == "" {
		return fmt.Errorf("API key cannot be empty")
	}

	client := api.New(cfg.APIURL, apiKey)
	me, err := client.GetMe()
	if err != nil {
		return fmt.Errorf("invalid API key: %w", err)
	}

	cfg.APIKey = apiKey

	// Extract org slug and email from response
	email := ""
	if user, ok := me["user"].(map[string]any); ok {
		if e, ok := user["email"].(string); ok {
			email = e
		}
	}
	if org, ok := me["org"].(map[string]any); ok {
		if slug, ok := org["slug"].(string); ok {
			cfg.OrgSlug = slug
		}
	}

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("error saving configuration: %w", err)
	}

	if !quiet {
		if email != "" {
			fmt.Printf("Authenticated as %s\n", email)
		} else {
			fmt.Println("Authenticated successfully!")
		}
	}

	// Auto-update any installed MCP configs with the new API key
	updated := updateInstalledMCPs(apiKey)
	if !quiet {
		for _, name := range updated {
			fmt.Printf("  ✓ Updated MCP config for %s\n", name)
		}
		fmt.Println()
		fmt.Println("Next step: mindex context \"your question\"")
	}

	return nil
}

// updateInstalledMCPs finds all MCP configs that have mindex configured
// and updates them with the new API key. Returns names of updated tools.
func updateInstalledMCPs(apiKey string) []string {
	var updated []string
	for key, tool := range mcpTools {
		path := tool.configPath()
		if isMCPConfigured(path) {
			if err := writeMCPConfig(path, apiKey); err == nil {
				updated = append(updated, tool.name)
				_ = key
			}
		}
	}
	return updated
}
