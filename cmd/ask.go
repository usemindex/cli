package cmd

import (
	"github.com/spf13/cobra"
)

var askCmd = &cobra.Command{
	Use:     "ask <question>",
	Short:   "Alias for 'context' — retrieve context from your knowledge base",
	Args:    cobra.MinimumNArgs(1),
	RunE:    runContext,
	Aliases: []string{},
}

func init() {
	askCmd.Flags().BoolVar(&compare, "compare", false, "Compare naive RAG vs Mindex GraphRAG side by side")
	rootCmd.AddCommand(askCmd)
}
