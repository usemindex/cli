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

	// try to extract org slug from authenticated user
	if orgs, ok := me["organizations"].([]any); ok && len(orgs) > 0 {
		if org, ok := orgs[0].(map[string]any); ok {
			if slug, ok := org["slug"].(string); ok && cfg.OrgSlug == "" {
				cfg.OrgSlug = slug
			}
		}
	}

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("error saving configuration: %w", err)
	}

	email := ""
	if e, ok := me["email"].(string); ok {
		email = e
	}

	if !quiet {
		if email != "" {
			fmt.Printf("Authenticated as %s\n", email)
		} else {
			fmt.Println("Authenticated successfully!")
		}
		fmt.Println()
		fmt.Println("Next step: mindex context \"your question\"")
	}

	return nil
}
