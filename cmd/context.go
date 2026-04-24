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

	// v2 retorna "context" (markdown assembled); v1 retornava "formatted_context".
	formattedContext, _ := result["context"].(string)
	if formattedContext == "" {
		formattedContext, _ = result["formatted_context"].(string)
	}
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

	// V2 ja emite "## Sources" dentro do markdown assembled; v1 nao emitia,
	// entao o CLI appendava. Pra evitar duplicacao, so appenda se o contexto
	// nao contiver uma secao Sources.
	if len(rawSources) > 0 && !quiet && !strings.Contains(formattedContext, "## Sources") {
		var sourceNames []string
		for _, s := range rawSources {
			src, _ := s.(map[string]any)
			name, _ := src["filename"].(string)
			score, _ := src["score"].(float64)
			if score == 0 {
				score, _ = src["relevance"].(float64)
			}
			if name != "" {
				if score > 0 {
					sourceNames = append(sourceNames, fmt.Sprintf("%s (%.0f%%)", name, score*100))
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
	fmt.Fprintf(w, "  │  %s\n", dim("Vector similarity only — flat list, no relationships"))
	fmt.Fprintln(w, "  │")

	// Top-5 e o default convencional pra naive RAG (1-chunk-per-doc, vector only).
	const naiveLimit = 5
	ragResult, ragErr := client.Search(question, ns, naiveLimit)
	var naiveDocs []string
	naiveSet := map[string]bool{}
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
			for _, r := range items {
				name := extractField(r, "file_name", "filename", "key", "name")
				if name == "" || naiveSet[name] {
					continue
				}
				naiveSet[name] = true
				naiveDocs = append(naiveDocs, name)
			}
			for i, name := range naiveDocs {
				fmt.Fprintf(w, "  │   %s %s %s\n", dim(fmt.Sprintf("%2d.", i+1)), white(shortenPath(name)), dim("[vector]"))
			}
		}
	}

	fmt.Fprintln(w, "  │")
	fmt.Fprintf(w, "  └─── %s\n", dim(fmt.Sprintf("%d documents found", len(naiveDocs))))

	// --- MINDEX GRAPHRAG ---
	fmt.Fprintln(w, "")
	fmt.Fprintf(w, "  ┌─── %s ──────────────────────────────────\n", green("MINDEX GRAPHRAG"))
	fmt.Fprintf(w, "  │  %s\n", dim("Vector + BM25 + Knowledge Graph traversal"))
	fmt.Fprintln(w, "  │")

	contextResult, contextErr := client.Context(question, ns)
	if contextErr != nil {
		fmt.Fprintf(w, "  │  Error: %s\n", contextErr)
		fmt.Fprintln(w, "  └──────────────────────────────────────────────────────")
		return nil
	}

	rawSources, _ := contextResult["sources"].([]any)
	rawInsights, _ := contextResult["insights"].([]any)

	type graphDoc struct {
		name    string
		paths   []string
		summary string
	}
	var graphDocs []graphDoc
	graphSet := map[string]bool{}
	for _, s := range rawSources {
		src, _ := s.(map[string]any)
		name, _ := src["filename"].(string)
		if name == "" || graphSet[name] {
			continue
		}
		graphSet[name] = true
		pathList := []string{}
		if arr, ok := src["paths"].([]any); ok {
			for _, p := range arr {
				if ps, _ := p.(string); ps != "" {
					pathList = append(pathList, ps)
				}
			}
		}
		summary, _ := src["summary"].(string)
		graphDocs = append(graphDocs, graphDoc{name: name, paths: pathList, summary: summary})
	}

	if len(graphDocs) == 0 {
		fmt.Fprintln(w, "  │  (no context found)")
	} else {
		for i, d := range graphDocs {
			pathLabel := strings.Join(d.paths, "+")
			if pathLabel == "" {
				pathLabel = "?"
			}
			badge := fmt.Sprintf("[%s]", pathLabel)
			extra := ""
			if !naiveSet[d.name] {
				extra = " " + green("← only in GraphRAG")
			}
			fmt.Fprintf(w, "  │   %s %s %s%s\n",
				dim(fmt.Sprintf("%2d.", i+1)),
				white(shortenPath(d.name)),
				dim(badge),
				extra,
			)
			// Summary truncado em 1 linha, recuado, dim
			if sum := strings.TrimSpace(d.summary); sum != "" {
				if len(sum) > 140 {
					sum = sum[:140] + "..."
				}
				fmt.Fprintf(w, "  │       %s\n", dim(sum))
			}
		}
	}

	// Cross-doc insights (entity triples from graph traversal)
	graphConns := len(rawInsights)
	if graphConns > 0 {
		fmt.Fprintln(w, "  │")
		fmt.Fprintf(w, "  │  %s\n", bold("◈ Cross-document relationships"))
		fmt.Fprintln(w, "  │")
		type insightKey struct{ s, p, o string }
		seen := map[insightKey]bool{}
		for _, ins := range rawInsights {
			m, _ := ins.(map[string]any)
			subj, _ := m["subject"].(string)
			pred, _ := m["predicate"].(string)
			obj, _ := m["object"].(string)
			if subj == "" || pred == "" || obj == "" {
				continue
			}
			k := insightKey{subj, pred, obj}
			if seen[k] {
				continue
			}
			seen[k] = true
			arrow := formatArrow(pred)
			fmt.Fprintf(w, "  │    %s %s %s\n", cyan(subj), magenta(arrow), cyan(obj))
		}
	}

	fmt.Fprintln(w, "  │")
	fmt.Fprintf(w, "  └─── %s\n", dim(fmt.Sprintf("%d documents found · %d cross-document relationships", len(graphDocs), graphConns)))

	// --- Delta summary ---
	extraDocs := 0
	multiPathDocs := 0
	for _, d := range graphDocs {
		if !naiveSet[d.name] {
			extraDocs++
		}
		if len(d.paths) > 1 {
			multiPathDocs++
		}
	}
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, gray("  ════════════════════════════════════════════════════════"))
	fmt.Fprintf(w, "  %s %s\n", green("✓"), bold("GraphRAG advantage:"))
	if extraDocs > 0 {
		fmt.Fprintf(w, "    %s %s\n", green("+"), bold(fmt.Sprintf("%d documents", extraDocs))+dim(" beyond NAIVE's top results"))
	}
	if multiPathDocs > 0 {
		fmt.Fprintf(w, "    %s %s\n", green("+"),
			bold(fmt.Sprintf("%d documents confirmed via multiple signals", multiPathDocs))+
				dim(" (vector + BM25 or graph)"))
	}
	if graphConns > 0 {
		fmt.Fprintf(w, "    %s %s\n", green("+"),
			bold(fmt.Sprintf("%d cross-document relationships", graphConns))+
				dim(" from the knowledge graph"))
	}
	if extraDocs == 0 && multiPathDocs == 0 && graphConns == 0 {
		fmt.Fprintf(w, "    %s\n", dim("No significant delta for this query — try one mentioning specific entities."))
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
