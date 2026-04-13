package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/usemindex/cli/api"
	"github.com/usemindex/cli/config"
	"github.com/usemindex/cli/output"
)

var compare bool

var contextCmd = &cobra.Command{
	Use:   "context <question>",
	Short: "Retrieve context from the knowledge base via GraphRAG",
	Long: `Queries your knowledge base with GraphRAG and returns relevant context.

  Examples:
    mindex context "how to configure payments?"
    mindex context "where is the authentication code?" --namespace backend
    mindex context "deploy process" --json
    mindex context "payment flow" --compare`,
	Args: cobra.MinimumNArgs(1),
	RunE: runContext,
}

func init() {
	contextCmd.Flags().BoolVar(&compare, "compare", false, "Compare naive RAG vs Mindex GraphRAG side by side")
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

	if compare {
		return runCompare(cmd, client, question, ns)
	}

	result, err := client.Context(question, ns)
	if err != nil {
		return err
	}

	if jsonOutput {
		return output.JSON(cmd.OutOrStdout(), result)
	}

	formattedContext, _ := result["formatted_context"].(string)
	rawSources, _ := result["sources"].([]any)

	if formattedContext == "" && len(rawSources) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "  No relevant documents found.")
		return nil
	}

	if !quiet && len(rawSources) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "  Found %d relevant sources\n\n", len(rawSources))
	}

	if formattedContext != "" {
		if err := output.Markdown(cmd.OutOrStdout(), formattedContext, noColor); err != nil {
			fmt.Fprintln(cmd.OutOrStdout(), formattedContext)
		}
	}

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

func runCompare(cmd *cobra.Command, client *api.Client, question, ns string) error {
	w := cmd.OutOrStdout()

	fmt.Fprintf(w, "  Comparing: \"%s\"\n\n", question)

	// --- RAG (simple vector search) ---
	fmt.Fprintln(w, "  ┌─────────────────────────────────────────────────┐")
	fmt.Fprintln(w, "  │  NAIVE RAG (vector similarity only)             │")
	fmt.Fprintln(w, "  └─────────────────────────────────────────────────┘")

	ragResult, ragErr := client.Search(question, ns, 3)
	if ragErr != nil {
		fmt.Fprintf(w, "  Error: %s\n", ragErr)
	} else {
		items := extractResults(ragResult)
		if items == nil {
			if arr, ok := ragResult["results"].([]any); ok {
				items = toMapSlice(arr)
			}
		}
		if len(items) == 0 {
			fmt.Fprintln(w, "  No results.")
		} else {
			for _, r := range items {
				name := ""
				for _, k := range []string{"file_name", "filename", "key", "name"} {
					if v, ok := r[k].(string); ok && v != "" {
						name = v
						break
					}
				}
				text := ""
				for _, k := range []string{"text", "content", "snippet"} {
					if v, ok := r[k].(string); ok && v != "" {
						text = v
						break
					}
				}
				score := 0.0
				for _, k := range []string{"distance", "score", "similarity"} {
					if v, ok := r[k].(float64); ok {
						score = v
						break
					}
				}
				if text != "" {
					if len(text) > 150 {
						text = text[:150] + "..."
					}
					fmt.Fprintf(w, "\n  [%s] (similarity: %.0f%%)\n", name, score*100)
					fmt.Fprintf(w, "  %s\n", text)
				}
			}
		}
	}

	fmt.Fprintln(w, "")

	// --- MINDEX (GraphRAG) ---
	fmt.Fprintln(w, "  ┌─────────────────────────────────────────────────┐")
	fmt.Fprintln(w, "  │  MINDEX GRAPHRAG (similarity + relationships)   │")
	fmt.Fprintln(w, "  └─────────────────────────────────────────────────┘")

	contextResult, contextErr := client.Context(question, ns)
	if contextErr != nil {
		fmt.Fprintf(w, "  Error: %s\n", contextErr)
	} else {
		formattedContext, _ := contextResult["formatted_context"].(string)
		rawSources, _ := contextResult["sources"].([]any)

		if formattedContext == "" {
			fmt.Fprintln(w, "  No context found.")
		} else {
			// Truncate for compare view
			lines := strings.Split(formattedContext, "\n")
			shown := 0
			for _, line := range lines {
				if shown >= 15 {
					fmt.Fprintln(w, "  ...")
					break
				}
				fmt.Fprintf(w, "  %s\n", line)
				shown++
			}
		}

		if len(rawSources) > 0 {
			var names []string
			for _, s := range rawSources {
				src, _ := s.(map[string]any)
				name, _ := src["filename"].(string)
				rel, _ := src["relationship"].(string)
				if name != "" {
					if rel != "" {
						names = append(names, fmt.Sprintf("%s (via %s)", name, rel))
					} else {
						names = append(names, name)
					}
				}
			}
			if len(names) > 0 {
				fmt.Fprintf(w, "\n  Sources: %s\n", strings.Join(names, ", "))
			}
		}
	}

	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  ─────────────────────────────────────────────────")
	fmt.Fprintln(w, "  RAG = similarity matching only")
	fmt.Fprintln(w, "  Mindex = similarity + knowledge graph + relationships")
	fmt.Fprintln(w, "")

	return nil
}

func extractResults(result map[string]any) []map[string]any {
	keys := []string{"results", "documents", "data", "items"}
	for _, k := range keys {
		if raw, ok := result[k]; ok {
			if list, ok := raw.([]any); ok {
				return toMapSlice(list)
			}
		}
	}
	return nil
}

func toMapSlice(list []any) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

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
