package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

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

// updateMsg receives the update notice from the background check (if any).
var updateMsg chan string

func Execute() {
	rootCmd.Version = Version

	// Check for updates in background (non-blocking)
	updateMsg = make(chan string, 1)
	go checkForUpdateQuiet()

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Show update notice at the end (if available)
	select {
	case msg := <-updateMsg:
		if msg != "" {
			fmt.Fprintln(os.Stderr, msg)
		}
	default:
	}
}

func checkForUpdateQuiet() {
	defer func() { recover() }()

	if Version == "dev" {
		return
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("https://api.github.com/repos/usemindex/cli/releases/latest")
	if err != nil || resp.StatusCode != 200 {
		return
	}
	defer resp.Body.Close()

	var release map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return
	}

	latest, _ := release["tag_name"].(string)
	latest = strings.TrimPrefix(latest, "v")
	current := strings.TrimPrefix(Version, "v")

	if latest != "" && latest != current {
		updateMsg <- fmt.Sprintf("\n  Update available: v%s → v%s\n  Run: mindex update\n", current, latest)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&jsonOutput, "json", "j", false, "Output as JSON")
	rootCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "", "Target namespace")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "Disable colored output")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "Minimal output")

	rootCmd.SetVersionTemplate("mindex version {{.Version}}\n")
}
