package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/usemindex/cli/api"
	"github.com/usemindex/cli/config"
)

var deleteCmd = &cobra.Command{
	Use:   "delete <doc>",
	Short: "Remove a document from the knowledge base",
	Long: `Removes a document by its ID/key.

  Examples:
    mindex delete my-document.md
    mindex delete doc-id-123 --namespace backend
    mindex delete doc.md --quiet`,
	Args: cobra.ExactArgs(1),
	RunE: runDelete,
}

func init() {
	rootCmd.AddCommand(deleteCmd)
}

func runDelete(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("error loading configuration: %w", err)
	}
	if cfg.APIKey == "" {
		return fmt.Errorf("API key not configured. Run 'mindex auth' first.")
	}

	docID := args[0]

	ns := namespace
	if ns == "" {
		ns = cfg.DefaultNamespace
	}

	client := api.New(cfg.APIURL, cfg.APIKey)
	client.OrgSlug = cfg.OrgSlug

	if err := client.DeleteDocument(docID, ns); err != nil {
		return err
	}

	if !quiet {
		fmt.Fprintf(cmd.OutOrStdout(), "Document '%s' deleted successfully.\n", docID)
	}

	return nil
}
