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
	Short: "Retrieve context from the knowledge base via GraphRAG",
	Long: `Queries your knowledge base with GraphRAG and returns relevant context.

  Examples:
    mindex context "how to configure payments?"
    mindex context "where is the authentication code?" --namespace backend
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
		return fmt.Errorf("error loading configuration: %w", err)
	}
	if cfg.APIKey == "" {
		return fmt.Errorf("API key not configured. Run 'mindex auth' first.")
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

	// extract results
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
			// fallback: print raw text
			fmt.Fprintln(cmd.OutOrStdout(), combined)
		}
	}

	if len(sources) > 0 && !quiet {
		fmt.Fprintf(cmd.OutOrStdout(), "\nSources: %s\n", strings.Join(sources, ", "))
	}

	return nil
}

// extractResults tries to find the results list regardless of the response format.
func extractResults(result map[string]any) []map[string]any {
	keys := []string{"results", "documents", "data", "items"}
	for _, k := range keys {
		if raw, ok := result[k]; ok {
			if list, ok := raw.([]any); ok {
				return toMapSlice(list)
			}
		}
	}
	// if the response is a direct array at the top level (unlikely, but defensive)
	return nil
}

// toMapSlice converts []any to []map[string]any, ignoring elements that are not maps.
func toMapSlice(list []any) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// buildSourceEntry builds the string "file.md (95%)" from a result.
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
