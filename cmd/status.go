package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/usemindex/cli/api"
	"github.com/usemindex/cli/config"
	"github.com/usemindex/cli/output"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Verifica a conectividade com a API",
	Long:  `Verifica se a API key está configurada corretamente e se a API está acessível.`,
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("erro ao carregar configuração: %w", err)
	}
	if cfg.APIKey == "" {
		return fmt.Errorf("API key não configurada. Execute 'mindex auth' primeiro.")
	}

	client := api.New(cfg.APIURL, cfg.APIKey)
	client.OrgSlug = cfg.OrgSlug

	me, err := client.GetMe()
	if err != nil {
		if jsonOutput {
			return output.JSON(cmd.OutOrStdout(), map[string]any{
				"status": "error",
				"error":  err.Error(),
			})
		}
		return fmt.Errorf("falha na conexão: %w", err)
	}

	if jsonOutput {
		return output.JSON(cmd.OutOrStdout(), map[string]any{
			"status": "ok",
			"user":   me,
		})
	}

	w := cmd.OutOrStdout()

	if !quiet {
		fmt.Fprintf(w, "Status:  ok\n")
		fmt.Fprintf(w, "API URL: %s\n", cfg.APIURL)

		if email, ok := me["email"].(string); ok {
			fmt.Fprintf(w, "User:    %s\n", email)
		}
		if cfg.OrgSlug != "" {
			fmt.Fprintf(w, "Org:     %s\n", cfg.OrgSlug)
		}
	} else {
		fmt.Fprintln(w, "ok")
	}

	return nil
}
