package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/usemindex/cli/api"
	"github.com/usemindex/cli/config"
)

var (
	relatedLimit     int
	relatedMinShared int
)

const (
	relatedMaxLimit         = 50
	relatedMaxSharedPreview = 5
)

var relatedCmd = &cobra.Command{
	Use:   "related <key>",
	Short: "List documents related via shared entities",
	Long: `Find documents that share entities with the given document.

Uses the knowledge graph to discover related docs based on common tags,
events, concepts, and other entities extracted during enrichment.

  Examples:
    mindex related docs/slack.md
    mindex related docs/slack.md --limit 5
    mindex related docs/slack.md --min-shared 2
    mindex related docs/slack.md --json`,
	Args:          cobra.ExactArgs(1),
	RunE:          runRelated,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	relatedCmd.Flags().IntVarP(&relatedLimit, "limit", "l", 10, "Maximum number of related documents (max 50)")
	relatedCmd.Flags().IntVar(&relatedMinShared, "min-shared", 1, "Minimum shared entities threshold")
	rootCmd.AddCommand(relatedCmd)
}

func runRelated(cmd *cobra.Command, args []string) error {
	if relatedLimit < 1 || relatedLimit > relatedMaxLimit {
		// usage error: cobra convention → exit 2
		return &usageError{msg: fmt.Sprintf("--limit must be between 1 and %d", relatedMaxLimit)}
	}
	if relatedMinShared < 1 {
		return &usageError{msg: "--min-shared must be >= 1"}
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("error loading configuration: %w", err)
	}
	if cfg.APIKey == "" {
		return fmt.Errorf("API key not configured. Run 'mindex auth' first.")
	}

	key := args[0]

	client := api.New(cfg.APIURL, cfg.APIKey)
	client.OrgSlug = cfg.OrgSlug

	result, err := client.RelatedDocuments(key, relatedLimit, relatedMinShared)
	if err != nil {
		if apiErr, ok := err.(*api.APIError); ok {
			switch apiErr.Status {
			case 404:
				return fmt.Errorf("document not found: %s", key)
			case 400:
				if apiErr.Message != "" {
					return fmt.Errorf("%s", apiErr.Message)
				}
			}
		}
		return err
	}

	if jsonOutput {
		return printRelatedJSON(cmd, result)
	}

	return printRelatedHuman(cmd, result)
}

func printRelatedJSON(cmd *cobra.Command, result *api.RelatedDocumentsResponse) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

func printRelatedHuman(cmd *cobra.Command, result *api.RelatedDocumentsResponse) error {
	w := cmd.OutOrStdout()

	title := ""
	if result.Title != nil {
		title = *result.Title
	}
	header := displayTitle(title, result.Document)

	fmt.Fprintf(w, "Related documents for %s (%s)\n\n", bold(fmt.Sprintf("%q", header)), dim(result.Document))

	if len(result.Related) == 0 {
		fmt.Fprintln(w, "  No related documents yet. Upload more docs or wait for enrichment.")
	} else {
		for i, rel := range result.Related {
			relTitle := ""
			if rel.Title != nil {
				relTitle = *rel.Title
			}
			name := displayTitle(relTitle, rel.Filename)
			strengthPct := int(rel.Strength*100 + 0.5)

			fmt.Fprintf(w, "  %d. %s (%s)\n", i+1, bold(name), dim(rel.Filename))
			fmt.Fprintf(w, "     %s %s  %s %s\n",
				dim("strength:"),
				green(fmt.Sprintf("%d%%", strengthPct)),
				dim("shared:"),
				formatSharedEntities(rel.SharedEntities),
			)
		}
	}

	if len(result.EntitiesInThisDoc) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Entities in this doc:")
		for _, ent := range result.EntitiesInThisDoc {
			suffix := ""
			switch ent.AlsoMentionedIn {
			case 0:
				suffix = dim("unique to this doc")
			case 1:
				suffix = dim("also in 1 other doc")
			default:
				suffix = dim(fmt.Sprintf("also in %d other docs", ent.AlsoMentionedIn))
			}
			fmt.Fprintf(w, "  %s %s %s — %s\n",
				cyan("•"),
				white(ent.Name),
				dim(fmt.Sprintf("(%s)", ent.Type)),
				suffix,
			)
		}
	}

	return nil
}

// displayTitle returns the title if non-empty, otherwise the fallback (filename).
func displayTitle(title, fallback string) string {
	if strings.TrimSpace(title) != "" {
		return title
	}
	return fallback
}

// formatSharedEntities joins entities with commas; truncates after relatedMaxSharedPreview.
func formatSharedEntities(entities []string) string {
	if len(entities) == 0 {
		return dim("(none)")
	}
	if len(entities) <= relatedMaxSharedPreview {
		return strings.Join(entities, ", ")
	}
	visible := entities[:relatedMaxSharedPreview]
	remaining := len(entities) - relatedMaxSharedPreview
	return fmt.Sprintf("%s%s", strings.Join(visible, ", "), dim(fmt.Sprintf(" ... +%d more", remaining)))
}

// usageError is returned for invalid flag combinations and triggers exit code 2.
type usageError struct {
	msg string
}

func (e *usageError) Error() string { return e.msg }
