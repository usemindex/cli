package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version é preenchida pelo main.go com o valor injetado pelo goreleaser.
var Version = "dev"

var (
	jsonOutput bool
	namespace  string
	noColor    bool
	quiet      bool
)

var rootCmd = &cobra.Command{
	Use:     "mindex",
	Short:   "Give your AI real memory",
	Version: Version,
	Long: `Mindex CLI — Give your AI real memory from the terminal.

  Get started:
    mindex auth                          Configure your API key
    mindex context "your question"       Retrieve context from your knowledge base

  Documentation: https://github.com/usemindex/docs`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func Execute() {
	rootCmd.Version = Version
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&jsonOutput, "json", "j", false, "Output as JSON")
	rootCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "", "Target namespace")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "Disable colored output")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "Minimal output")

	rootCmd.SetVersionTemplate("mindex version {{.Version}}\n")
}
