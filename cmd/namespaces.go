package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/usemindex/cli/api"
	"github.com/usemindex/cli/config"
	"github.com/usemindex/cli/output"
)

var namespacesCmd = &cobra.Command{
	Use:   "namespaces",
	Short: "Manage organization namespaces",
	Long: `Lists or creates namespaces in your organization.

  Examples:
    mindex namespaces
    mindex namespaces create backend
    mindex namespaces --json`,
	RunE: runListNamespaces,
}

var namespacesCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new namespace",
	Args:  cobra.ExactArgs(1),
	RunE:  runCreateNamespace,
}

func init() {
	namespacesCmd.AddCommand(namespacesCreateCmd)
	rootCmd.AddCommand(namespacesCmd)
}

func runListNamespaces(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("error loading configuration: %w", err)
	}
	if cfg.APIKey == "" {
		return fmt.Errorf("API key not configured. Run 'mindex auth' first.")
	}

	client := api.New(cfg.APIURL, cfg.APIKey)
	client.OrgSlug = cfg.OrgSlug

	result, err := client.ListNamespaces()
	if err != nil {
		return err
	}

	if jsonOutput {
		return output.JSON(cmd.OutOrStdout(), result)
	}

	namespaces := extractResults(result)

	if len(namespaces) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No namespaces found.")
		return nil
	}

	headers := []string{"Name", "Slug", "Documents"}
	rows := make([][]string, 0, len(namespaces))

	for _, ns := range namespaces {
		name := stringField(ns, "name")
		slug := stringField(ns, "slug", "id")
		docs := ""
		if v, ok := ns["document_count"].(float64); ok {
			docs = fmt.Sprintf("%d", int(v))
		} else if v, ok := ns["documents_count"].(float64); ok {
			docs = fmt.Sprintf("%d", int(v))
		}
		rows = append(rows, []string{name, slug, docs})
	}

	output.Table(cmd.OutOrStdout(), headers, rows)
	return nil
}

func runCreateNamespace(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("error loading configuration: %w", err)
	}
	if cfg.APIKey == "" {
		return fmt.Errorf("API key not configured. Run 'mindex auth' first.")
	}

	name := args[0]

	client := api.New(cfg.APIURL, cfg.APIKey)
	client.OrgSlug = cfg.OrgSlug

	result, err := client.CreateNamespace(name)
	if err != nil {
		return err
	}

	if jsonOutput {
		return output.JSON(cmd.OutOrStdout(), result)
	}

	if !quiet {
		slug := stringField(result, "slug", "id", "name")
		if slug == "" {
			slug = name
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Namespace '%s' created successfully.\n", slug)
	}

	return nil
}
