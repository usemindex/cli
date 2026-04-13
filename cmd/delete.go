package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/usemindex/cli/api"
	"github.com/usemindex/cli/config"
)

var deleteCmd = &cobra.Command{
	Use:   "delete <doc>",
	Short: "Remove um documento da knowledge base",
	Long: `Remove um documento pelo seu ID/key.

  Exemplos:
    mindex delete meu-documento.md
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
		return fmt.Errorf("erro ao carregar configuração: %w", err)
	}
	if cfg.APIKey == "" {
		return fmt.Errorf("API key não configurada. Execute 'mindex auth' primeiro.")
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
		fmt.Fprintf(cmd.OutOrStdout(), "Documento '%s' removido com sucesso.\n", docID)
	}

	return nil
}
