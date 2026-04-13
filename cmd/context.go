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

	// Engine /context returns: { formatted_context, sources, stats }
	formattedContext, _ := result["formatted_context"].(string)
	rawSources, _ := result["sources"].([]any)

	if formattedContext == "" && len(rawSources) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "  No relevant documents found.")
		return nil
	}

	if !quiet && len(rawSources) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "  Found %d relevant sources\n\n", len(rawSources))
	}

	// Render formatted context as markdown
	if formattedContext != "" {
		if err := output.Markdown(cmd.OutOrStdout(), formattedContext, noColor); err != nil {
			fmt.Fprintln(cmd.OutOrStdout(), formattedContext)
		}
	}

	// Show sources
	if len(rawSources) > 0 && !quiet {
		var sourceNames []string
		for _, s := range rawSources {
			src, _ := s.(map[string]any)
			name, _ := src["filename"].(string)
			relevance, _ := src["relevance"].(float64)
			if name != "" {
				if relevance > 0 {
					sourceNames = append(sourceNames, fmt.Sprintf("%s (%.0f%%)", name, relevance*100))
				} else {
					sourceNames = append(sourceNames, name)
				}
			}
		}
		if len(sourceNames) > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "\n  Sources: %s\n", strings.Join(sourceNames, ", "))
		}
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
	for _, k := range []string{"key", "file_name", "filename", "name", "id"} {
		if v, ok := r[k].(string); ok && v != "" {
			name = v
			break
		}
	}
	if name == "" {
		return ""
	}

	score := ""
	for _, k := range []string{"score", "similarity", "relevance", "distance"} {
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
