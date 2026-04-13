package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/usemindex/cli/config"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Show current configuration",
	Long:  `Displays the settings saved in ~/.mindex/config.json with the API key masked.`,
	RunE:  runConfig,
}

func init() {
	rootCmd.AddCommand(configCmd)
}

func runConfig(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("error loading configuration: %w", err)
	}

	w := cmd.OutOrStdout()

	apiKeyDisplay := "(not set)"
	if cfg.APIKey != "" {
		apiKeyDisplay = maskAPIKey(cfg.APIKey)
	}

	apiURL := cfg.APIURL
	if apiURL == "" {
		apiURL = "(default)"
	}

	orgSlug := cfg.OrgSlug
	if orgSlug == "" {
		orgSlug = "(not set)"
	}

	defaultNS := cfg.DefaultNamespace
	if defaultNS == "" {
		defaultNS = "(not set)"
	}

	fmt.Fprintf(w, "API Key:           %s\n", apiKeyDisplay)
	fmt.Fprintf(w, "API URL:           %s\n", apiURL)
	fmt.Fprintf(w, "Org Slug:          %s\n", orgSlug)
	fmt.Fprintf(w, "Default Namespace: %s\n", defaultNS)
	fmt.Fprintf(w, "Config file:       %s\n", config.Path())

	return nil
}

// maskAPIKey keeps the prefix and the last 4 characters visible.
func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	visible := 4
	prefix := key[:4]
	suffix := key[len(key)-visible:]
	masked := prefix + "..." + suffix
	return masked
}
