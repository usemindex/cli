package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/usemindex/cli/api"
	"github.com/usemindex/cli/config"
	"github.com/usemindex/cli/output"
)

var getCmd = &cobra.Command{
	Use:   "get <document-key>",
	Short: "Read the full content of a document",
	Long: `Read the full content of a document by its key.

  Examples:
    mindex get docs/guide.md
    mindex get guide.md -n docs
    mindex get guide.md --json`,
	Args: cobra.ExactArgs(1),
	RunE: runGet,
}

func init() {
	rootCmd.AddCommand(getCmd)
}

func runGet(cmd *cobra.Command, args []string) error {
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

	ns := namespace
	if ns == "" {
		ns = cfg.DefaultNamespace
	}

	result, err := client.GetDocument(key, ns)
	if err != nil {
		return err
	}

	if jsonOutput {
		return output.JSON(os.Stdout, result)
	}

	// Try to get content from various field names
	content := ""
	for _, k := range []string{"content", "text", "body"} {
		if v, ok := result[k].(string); ok && v != "" {
			content = v
			break
		}
	}

	if content == "" {
		fmt.Fprintln(cmd.OutOrStdout(), "Document is empty or not found.")
		return nil
	}

	return output.Markdown(cmd.OutOrStdout(), content, noColor)
}
