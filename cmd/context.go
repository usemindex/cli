package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/usemindex/cli/api"
	"github.com/usemindex/cli/config"
	"github.com/usemindex/cli/output"
)

var contextCmd = &cobra.Command{
	Use:   "context <question>",
	Short: "Recupera contexto da knowledge base via GraphRAG",
	Long: `Consulta sua knowledge base com GraphRAG e retorna contexto relevante.

  Exemplos:
    mindex context "como configurar pagamentos?"
    mindex context "onde fica o código de autenticação?" --namespace backend
    mindex context "deploy process" --json`,
	Args: cobra.MinimumNArgs(1),
	RunE: runContext,
}

func init() {
	rootCmd.AddCommand(contextCmd)
}

func runContext(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("erro ao carregar configuração: %w", err)
	}
	if cfg.APIKey == "" {
		return fmt.Errorf("API key não configurada. Execute 'mindex auth' primeiro.")
	}

	question := strings.Join(args, " ")

	ns := namespace
	if ns == "" {
		ns = cfg.DefaultNamespace
	}

	client := api.New(cfg.APIURL, cfg.APIKey)
	client.OrgSlug = cfg.OrgSlug

	result, err := client.Context(question, ns)
	if err != nil {
		return err
	}

	if jsonOutput {
		return output.JSON(cmd.OutOrStdout(), result)
	}

	// extrai resultados
	results := extractResults(result)

	if len(results) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No relevant documents found.")
		return nil
	}

	if !quiet {
		fmt.Fprintf(cmd.OutOrStdout(), "  Found %d relevant results\n\n", len(results))
	}

	// monta conteúdo markdown consolidado
	var contentParts []string
	var sources []string

	for _, r := range results {
		if content, ok := r["content"].(string); ok && content != "" {
			contentParts = append(contentParts, content)
		}
		src := buildSourceEntry(r)
		if src != "" {
			sources = append(sources, src)
		}
	}

	if len(contentParts) > 0 {
		combined := strings.Join(contentParts, "\n\n---\n\n")
		if err := output.Markdown(cmd.OutOrStdout(), combined, noColor); err != nil {
			// fallback: imprime o texto bruto
			fmt.Fprintln(cmd.OutOrStdout(), combined)
		}
	}

	if len(sources) > 0 && !quiet {
		fmt.Fprintf(cmd.OutOrStdout(), "\nSources: %s\n", strings.Join(sources, ", "))
	}

	return nil
}

// extractResults tenta encontrar a lista de resultados independente do formato da resposta.
func extractResults(result map[string]any) []map[string]any {
	keys := []string{"results", "documents", "data", "items"}
	for _, k := range keys {
		if raw, ok := result[k]; ok {
			if list, ok := raw.([]any); ok {
				return toMapSlice(list)
			}
		}
	}
	// se a resposta é um array direto no nível superior (pouco provável, mas defensivo)
	return nil
}

// toMapSlice converte []any para []map[string]any, ignorando elementos que não são mapas.
func toMapSlice(list []any) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// buildSourceEntry monta a string "arquivo.md (95%)" a partir de um resultado.
func buildSourceEntry(r map[string]any) string {
	name := ""
	for _, k := range []string{"key", "filename", "name", "id"} {
		if v, ok := r[k].(string); ok && v != "" {
			name = v
			break
		}
	}
	if name == "" {
		return ""
	}

	score := ""
	for _, k := range []string{"score", "similarity", "relevance"} {
		if v, ok := r[k].(float64); ok {
			score = fmt.Sprintf("%d%%", int(v*100))
			break
		}
	}

	if score != "" {
		return fmt.Sprintf("%s (%s)", name, score)
	}
	return name
}
