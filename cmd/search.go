package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/usemindex/cli/api"
	"github.com/usemindex/cli/config"
	"github.com/usemindex/cli/output"
)

var searchLimit int

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Busca documentos por query semântica",
	Long: `Realiza busca semântica nos documentos da sua knowledge base.

  Exemplos:
    mindex search "autenticação JWT"
    mindex search "deploy" --limit 5
    mindex search "stripe" --namespace payments --json`,
	Args: cobra.MinimumNArgs(1),
	RunE: runSearch,
}

func init() {
	searchCmd.Flags().IntVarP(&searchLimit, "limit", "l", 10, "Número máximo de resultados")
	rootCmd.AddCommand(searchCmd)
}

func runSearch(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("erro ao carregar configuração: %w", err)
	}
	if cfg.APIKey == "" {
		return fmt.Errorf("API key não configurada. Execute 'mindex auth' primeiro.")
	}

	query := strings.Join(args, " ")

	ns := namespace
	if ns == "" {
		ns = cfg.DefaultNamespace
	}

	client := api.New(cfg.APIURL, cfg.APIKey)
	client.OrgSlug = cfg.OrgSlug

	result, err := client.Search(query, ns, searchLimit)
	if err != nil {
		return err
	}

	if jsonOutput {
		return output.JSON(cmd.OutOrStdout(), result)
	}

	results := extractResults(result)

	if len(results) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "Nenhum resultado encontrado.")
		return nil
	}

	for i, r := range results {
		name := ""
		for _, k := range []string{"key", "filename", "name", "id"} {
			if v, ok := r[k].(string); ok && v != "" {
				name = v
				break
			}
		}

		score := ""
		for _, k := range []string{"score", "similarity", "relevance"} {
			if v, ok := r[k].(float64); ok {
				score = fmt.Sprintf("%d%%", int(v*100))
				break
			}
		}

		snippet := ""
		for _, k := range []string{"snippet", "excerpt", "content"} {
			if v, ok := r[k].(string); ok && v != "" {
				snippet = v
				break
			}
		}
		// trunca o snippet para caber na tela
		if len(snippet) > 120 {
			snippet = snippet[:120] + "..."
		}

		scoreStr := ""
		if score != "" {
			scoreStr = fmt.Sprintf(" [%s]", score)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "%d. %s%s\n", i+1, name, scoreStr)
		if snippet != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "   %s\n", snippet)
		}
		fmt.Fprintln(cmd.OutOrStdout())
	}

	return nil
}
