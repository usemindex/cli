package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/usemindex/cli/api"
	"github.com/usemindex/cli/config"
	"github.com/usemindex/cli/output"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List documents in the knowledge base",
	Long: `Lists all documents in the selected namespace.

  Examples:
    mindex list
    mindex list --namespace backend
    mindex list --json`,
	RunE: runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("error loading configuration: %w", err)
	}
	if cfg.APIKey == "" {
		return fmt.Errorf("API key not configured. Run 'mindex auth' first.")
	}

	ns := namespace
	if ns == "" {
		ns = cfg.DefaultNamespace
	}

	client := api.New(cfg.APIURL, cfg.APIKey)
	client.OrgSlug = cfg.OrgSlug

	result, err := client.ListDocuments(ns)
	if err != nil {
		return err
	}

	if jsonOutput {
		return output.JSON(cmd.OutOrStdout(), result)
	}

	docs := extractResults(result)

	if len(docs) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No documents found.")
		return nil
	}

	headers := []string{"Key", "Namespace", "Size"}
	rows := make([][]string, 0, len(docs))

	for _, doc := range docs {
		key := stringField(doc, "key", "id", "name")
		ns := stringField(doc, "namespace", "collection")
		size := ""
		if v, ok := doc["size"].(float64); ok {
			size = formatBytes(int64(v))
		} else if v, ok := doc["size_bytes"].(float64); ok {
			size = formatBytes(int64(v))
		}
		rows = append(rows, []string{key, ns, size})
	}

	output.Table(cmd.OutOrStdout(), headers, rows)
	return nil
}

// stringField extracts the first non-empty field from a list of candidate keys.
func stringField(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// formatBytes converts bytes to a human-readable string (KB, MB, GB).
func formatBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
