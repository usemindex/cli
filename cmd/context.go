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

	fmt.Fprintf(w, "\n  %s \"%s\"\n", bold("Query:"), cyan(question))
	fmt.Fprintln(w, gray("  ════════════════════════════════════════════════════════"))

	// --- NAIVE RAG ---
	fmt.Fprintln(w, "")
	fmt.Fprintf(w, "  ┌─── %s ────────────────────────────────────────\n", yellow("NAIVE RAG"))
	fmt.Fprintf(w, "  │  %s\n", dim("Vector similarity only — finds text, misses context"))
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
				fmt.Fprintf(w, "  │  %s %s %s\n", yellow(bar), white(shortName), dim(fmt.Sprintf("%.0f%%", score*100)))
				if text != "" {
					if len(text) > 100 {
						text = text[:100] + "..."
					}
					fmt.Fprintf(w, "  │     %s\n", dim(text))
				}
			}
		}
	}

	fmt.Fprintln(w, "  │")
	fmt.Fprintf(w, "  └─── %s\n", dim(fmt.Sprintf("%d documents found (flat list, no relationships)", ragCount)))

	// --- MINDEX GRAPHRAG ---
	fmt.Fprintln(w, "")
	fmt.Fprintf(w, "  ┌─── %s ──────────────────────────────────\n", green("MINDEX GRAPHRAG"))
	fmt.Fprintf(w, "  │  %s\n", dim("Similarity + Knowledge Graph + Document Connections"))
	fmt.Fprintln(w, "  │")

	contextResult, contextErr := client.Context(question, ns)
	if contextErr != nil {
		fmt.Fprintf(w, "  │  Error: %s\n", contextErr)
		fmt.Fprintln(w, "  └──────────────────────────────────────────────────────")
		return nil
	}

	// v2 ContextResponse shape:
	//   sources: [{filename, score, paths: ["vector"|"bm25"|"graph"], heading, snippet}]
	//   insights: [{subject, predicate, object, confidence, source:"graph"}]
	//   path_mix: {vector, bm25, graph}
	rawSources, _ := contextResult["sources"].([]any)
	rawInsights, _ := contextResult["insights"].([]any)
	rawPathMix, _ := contextResult["path_mix"].(map[string]any)

	if len(rawSources) == 0 {
		fmt.Fprintln(w, "  │  (no context found)")
	} else {
		// Direct = docs encontrados via vector ou bm25; Graph-only = so graph path.
		var directHits []map[string]any
		var graphDiscovered []map[string]any
		for _, s := range rawSources {
			src, _ := s.(map[string]any)
			paths, _ := src["paths"].([]any)
			viaVectorOrBm25 := false
			for _, p := range paths {
				if ps, _ := p.(string); ps == "vector" || ps == "bm25" {
					viaVectorOrBm25 = true
					break
				}
			}
			if viaVectorOrBm25 {
				directHits = append(directHits, src)
			} else {
				graphDiscovered = append(graphDiscovered, src)
			}
		}

		if len(directHits) > 0 {
			fmt.Fprintf(w, "  │  %s\n", bold("◆ Direct matches (vector + BM25)"))
			for _, src := range directHits {
				name, _ := src["filename"].(string)
				score, _ := src["score"].(float64)
				bar := renderBar(score, 20)
				fmt.Fprintf(w, "  │    %s %s %s\n", green(bar), white(shortenPath(name)), dim(fmt.Sprintf("%.0f%%", score*100)))
			}
		}

		if len(graphDiscovered) > 0 {
			fmt.Fprintln(w, "  │")
			fmt.Fprintf(w, "  │  %s\n", bold("◇ Discovered via knowledge graph"))
			for _, src := range graphDiscovered {
				name, _ := src["filename"].(string)
				fmt.Fprintf(w, "  │    ╰─ %s\n", cyan(shortenPath(name)))
			}
		}
	}

	// Cross-doc insights (entity triples from graph traversal)
	graphConns := len(rawInsights)
	if graphConns > 0 {
		fmt.Fprintln(w, "  │")
		fmt.Fprintf(w, "  │  %s\n", bold("◈ Graph insights (entity triples)"))
		fmt.Fprintln(w, "  │")
		type insightKey struct{ s, p, o string }
		seen := map[insightKey]bool{}
		for _, ins := range rawInsights {
			m, _ := ins.(map[string]any)
			subj, _ := m["subject"].(string)
			pred, _ := m["predicate"].(string)
			obj, _ := m["object"].(string)
			conf, _ := m["confidence"].(float64)
			if subj == "" || pred == "" || obj == "" {
				continue
			}
			k := insightKey{subj, pred, obj}
			if seen[k] {
				continue
			}
			seen[k] = true
			arrow := formatArrow(pred)
			if conf > 0 {
				fmt.Fprintf(w, "  │    %s %s %s  %s\n", cyan(subj), magenta(arrow), cyan(obj), dim(fmt.Sprintf("%.0f%%", conf*100)))
			} else {
				fmt.Fprintf(w, "  │    %s %s %s\n", cyan(subj), magenta(arrow), cyan(obj))
			}
		}
	}

	// Stats do path_mix
	fmt.Fprintln(w, "  │")
	vectorChunks := 0
	if v, ok := rawPathMix["vector"].(float64); ok {
		vectorChunks = int(v)
	}
	docsRead := len(rawSources)
	fmt.Fprintf(w, "  └─── %s\n", dim(fmt.Sprintf("%d chunks · %d connections · %d documents read", vectorChunks, graphConns, docsRead)))

	// Summary
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, gray("  ════════════════════════════════════════════════════════"))
	if graphConns > 0 {
		fmt.Fprintf(w, "  %s found %s that naive RAG missed.\n", green("GraphRAG"), bold(fmt.Sprintf("%d document relationships", graphConns)))
		fmt.Fprintf(w, "  %s\n", dim("These connections provide context about HOW documents relate,"))
		fmt.Fprintf(w, "  %s\n", dim("not just that they contain similar words."))
	} else {
		fmt.Fprintf(w, "  %s\n", yellow("No graph connections found for this query."))
		fmt.Fprintf(w, "  %s\n", dim("Upload more documents to build richer knowledge graph connections."))
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

// --- ANSI Colors ---

func c(code, s string) string {
	if noColor {
		return s
	}
	return fmt.Sprintf("\033[%sm%s\033[0m", code, s)
}

func dim(s string) string     { return c("2", s) }
func bold(s string) string    { return c("1", s) }
func cyan(s string) string    { return c("36", s) }
func green(s string) string   { return c("32", s) }
func yellow(s string) string  { return c("33", s) }
func magenta(s string) string { return c("35", s) }
func blue(s string) string    { return c("34", s) }
func gray(s string) string    { return c("90", s) }
func white(s string) string   { return c("97", s) }

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
