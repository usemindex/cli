package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/usemindex/cli/config"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Exibe a configuração atual",
	Long:  `Exibe as configurações salvas em ~/.mindex/config.json com a API key mascarada.`,
	RunE:  runConfig,
}

func init() {
	rootCmd.AddCommand(configCmd)
}

func runConfig(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("erro ao carregar configuração: %w", err)
	}

	w := cmd.OutOrStdout()

	apiKeyDisplay := "(não configurada)"
	if cfg.APIKey != "" {
		apiKeyDisplay = maskAPIKey(cfg.APIKey)
	}

	apiURL := cfg.APIURL
	if apiURL == "" {
		apiURL = "(padrão)"
	}

	orgSlug := cfg.OrgSlug
	if orgSlug == "" {
		orgSlug = "(não configurado)"
	}

	defaultNS := cfg.DefaultNamespace
	if defaultNS == "" {
		defaultNS = "(não configurado)"
	}

	fmt.Fprintf(w, "API Key:           %s\n", apiKeyDisplay)
	fmt.Fprintf(w, "API URL:           %s\n", apiURL)
	fmt.Fprintf(w, "Org Slug:          %s\n", orgSlug)
	fmt.Fprintf(w, "Default Namespace: %s\n", defaultNS)
	fmt.Fprintf(w, "Config file:       %s\n", config.Path())

	return nil
}

// maskAPIKey mantém o prefixo e os últimos 4 caracteres visíveis.
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
