package cmd

import (
	"fmt"
	"math"
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

	fmt.Fprintf(w, "\n  Query: \"%s\"\n", question)
	fmt.Fprintln(w, "  ════════════════════════════════════════════════════════")

	// --- NAIVE RAG ---
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  ┌─── NAIVE RAG ────────────────────────────────────────")
	fmt.Fprintln(w, "  │  Vector similarity only — finds text, misses context")
	fmt.Fprintln(w, "  │")

	ragResult, ragErr := client.Search(question, ns, 5)
	ragCount := 0
	if ragErr != nil {
		fmt.Fprintf(w, "  │  Error: %s\n", ragErr)
	} else {
		items := extractResults(ragResult)
		if items == nil {
			if arr, ok := ragResult["results"].([]any); ok {
				items = toMapSlice(arr)
			}
		}
		if len(items) == 0 {
			fmt.Fprintln(w, "  │  (no results)")
		} else {
			ragCount = len(items)
			for _, r := range items {
				name := extractField(r, "file_name", "filename", "key", "name")
				text := extractField(r, "text", "content", "snippet")
				score := extractScore(r)

				if name == "" {
					continue
				}
				bar := renderBar(score, 20)
				shortName := shortenPath(name)
				fmt.Fprintf(w, "  │  %s %s %.0f%%\n", bar, shortName, score*100)
				if text != "" {
					if len(text) > 100 {
						text = text[:100] + "..."
					}
					// Indent text preview
					fmt.Fprintf(w, "  │     %s\n", dimText(text))
				}
			}
		}
	}

	fmt.Fprintln(w, "  │")
	fmt.Fprintf(w, "  └─── %d documents found (flat list, no relationships)\n", ragCount)

	// --- MINDEX GRAPHRAG ---
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  ┌─── MINDEX GRAPHRAG ──────────────────────────────────")
	fmt.Fprintln(w, "  │  Similarity + Knowledge Graph + Document Connections")
	fmt.Fprintln(w, "  │")

	contextResult, contextErr := client.Context(question, ns)
	if contextErr != nil {
		fmt.Fprintf(w, "  │  Error: %s\n", contextErr)
		fmt.Fprintln(w, "  └──────────────────────────────────────────────────────")
		return nil
	}

	rawSources, _ := contextResult["sources"].([]any)
	rawConnections, _ := contextResult["graph_connections"].([]any)
	rawStats, _ := contextResult["stats"].(map[string]any)

	// Sources with relevance and relationship type
	if len(rawSources) == 0 {
		fmt.Fprintln(w, "  │  (no context found)")
	} else {
		// Separate direct hits from graph-discovered
		var directHits []map[string]any
		var graphDiscovered []map[string]any
		for _, s := range rawSources {
			src, _ := s.(map[string]any)
			rel, _ := src["relationship"].(string)
			if rel == "" {
				directHits = append(directHits, src)
			} else {
				graphDiscovered = append(graphDiscovered, src)
			}
		}

		// Direct hits (from vector search)
		if len(directHits) > 0 {
			fmt.Fprintln(w, "  │  ◆ Direct matches (semantic search)")
			for _, src := range directHits {
				name, _ := src["filename"].(string)
				relevance, _ := src["relevance"].(float64)
				bar := renderBar(relevance, 20)
				fmt.Fprintf(w, "  │    %s %s %.0f%%\n", bar, shortenPath(name), relevance*100)
			}
		}

		// Graph-discovered documents
		if len(graphDiscovered) > 0 {
			fmt.Fprintln(w, "  │")
			fmt.Fprintln(w, "  │  ◇ Discovered via knowledge graph")
			for _, src := range graphDiscovered {
				name, _ := src["filename"].(string)
				rel, _ := src["relationship"].(string)
				fmt.Fprintf(w, "  │    ╰─ %s  ← %s\n", shortenPath(name), formatRelType(rel))
			}
		}
	}

	// Graph connections visualization
	if len(rawConnections) > 0 {
		fmt.Fprintln(w, "  │")
		fmt.Fprintln(w, "  │  ◈ Document relationships")
		fmt.Fprintln(w, "  │")

		// Deduplicate connections by source-target pair
		type connKey struct{ src, tgt string }
		seen := map[connKey]bool{}

		for _, c := range rawConnections {
			conn, _ := c.(map[string]any)
			source, _ := conn["source"].(string)
			target, _ := conn["target"].(string)
			relType, _ := conn["rel_type"].(string)
			score, _ := conn["similarity_score"].(float64)
			justification, _ := conn["justification"].(string)

			if source == "" || target == "" {
				continue
			}
			key := connKey{source, target}
			if seen[key] {
				continue
			}
			seen[key] = true

			arrow := formatArrow(relType)
			srcShort := shortenPath(source)
			tgtShort := shortenPath(target)

			if score > 0 {
				fmt.Fprintf(w, "  │    %s %s %s  (%.0f%%)\n", srcShort, arrow, tgtShort, score*100)
			} else {
				fmt.Fprintf(w, "  │    %s %s %s\n", srcShort, arrow, tgtShort)
			}
			if justification != "" {
				if len(justification) > 80 {
					justification = justification[:80] + "..."
				}
				fmt.Fprintf(w, "  │      %s\n", dimText(justification))
			}
		}
	}

	// Stats
	fmt.Fprintln(w, "  │")
	vectorChunks := 0
	graphConns := 0
	docsRead := 0
	if rawStats != nil {
		if v, ok := rawStats["vector_chunks"].(float64); ok {
			vectorChunks = int(v)
		}
		if v, ok := rawStats["graph_connections"].(float64); ok {
			graphConns = int(v)
		}
		if v, ok := rawStats["documents_read"].(float64); ok {
			docsRead = int(v)
		}
	}
	fmt.Fprintf(w, "  └─── %d chunks · %d connections · %d documents read\n", vectorChunks, graphConns, docsRead)

	// Summary
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  ════════════════════════════════════════════════════════")
	if graphConns > 0 {
		fmt.Fprintf(w, "  GraphRAG found %d document relationships that naive RAG missed.\n", graphConns)
		fmt.Fprintln(w, "  These connections provide context about HOW documents relate,")
		fmt.Fprintln(w, "  not just that they contain similar words.")
	} else {
		fmt.Fprintln(w, "  No graph connections found for this query.")
		fmt.Fprintln(w, "  Upload more documents to build richer knowledge graph connections.")
	}
	fmt.Fprintln(w, "")

	return nil
}

// --- Helpers ---

func extractField(r map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := r[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func extractScore(r map[string]any) float64 {
	for _, k := range []string{"distance", "score", "similarity", "relevance"} {
		if v, ok := r[k].(float64); ok {
			return v
		}
	}
	return 0
}

func renderBar(score float64, width int) string {
	filled := int(math.Round(score * float64(width)))
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	return "█" + strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

func shortenPath(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) <= 2 {
		return path
	}
	return parts[len(parts)-2] + "/" + parts[len(parts)-1]
}

func formatRelType(rel string) string {
	switch rel {
	case "RELATES_TO":
		return "related"
	case "LINKS_TO":
		return "linked"
	case "HAS_TAG":
		return "tagged"
	default:
		if strings.HasPrefix(rel, "2-hop:") {
			inner := strings.TrimPrefix(rel, "2-hop:")
			return "2-hop " + formatRelType(inner)
		}
		return strings.ToLower(strings.ReplaceAll(rel, "_", " "))
	}
}

func formatArrow(relType string) string {
	switch relType {
	case "RELATES_TO":
		return "──relates──→"
	case "LINKS_TO":
		return "──links────→"
	case "HAS_TAG":
		return "──tagged───→"
	default:
		label := strings.ToLower(strings.ReplaceAll(relType, "_", " "))
		return fmt.Sprintf("──%s──→", label)
	}
}

func dimText(s string) string {
	// ANSI dim
	return "\033[2m" + s + "\033[0m"
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
