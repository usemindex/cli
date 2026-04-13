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
	Short: "Configure sua API key",
	Long:  `Valida e salva sua API key do Mindex em ~/.mindex/config.json.`,
	RunE:  runAuth,
}

func init() {
	rootCmd.AddCommand(authCmd)
}

func runAuth(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("erro ao carregar configuração: %w", err)
	}

	fmt.Print("Cole sua API key (sk-...): ")
	reader := bufio.NewReader(os.Stdin)
	apiKey, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("erro ao ler API key: %w", err)
	}
	apiKey = strings.TrimSpace(apiKey)

	if apiKey == "" {
		return fmt.Errorf("API key não pode ser vazia")
	}

	client := api.New(cfg.APIURL, apiKey)
	me, err := client.GetMe()
	if err != nil {
		return fmt.Errorf("API key inválida: %w", err)
	}

	cfg.APIKey = apiKey

	// tenta extrair org slug do usuário autenticado
	if orgs, ok := me["organizations"].([]any); ok && len(orgs) > 0 {
		if org, ok := orgs[0].(map[string]any); ok {
			if slug, ok := org["slug"].(string); ok && cfg.OrgSlug == "" {
				cfg.OrgSlug = slug
			}
		}
	}

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("erro ao salvar configuração: %w", err)
	}

	email := ""
	if e, ok := me["email"].(string); ok {
		email = e
	}

	if !quiet {
		if email != "" {
			fmt.Printf("Autenticado como %s\n", email)
		} else {
			fmt.Println("Autenticado com sucesso!")
		}
		fmt.Println()
		fmt.Println("Próximo passo: mindex context \"sua pergunta\"")
	}

	return nil
}
