package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/usemindex/cli/config"
)

type editorTarget struct {
	name     string
	filePath string
	content  func(cfg *config.Config) string
}

func getEditorTargets(cfg *config.Config) []editorTarget {
	return []editorTarget{
		{
			name:     "Claude Code",
			filePath: "CLAUDE.md",
			content:  claudeMDContent,
		},
		{
			name:     "Cursor",
			filePath: ".cursorrules",
			content:  cursorRulesContent,
		},
		{
			name:     "Windsurf",
			filePath: filepath.Join(".windsurf", "rules", "mindex.md"),
			content:  windsurfContent,
		},
		{
			name:     "GitHub Copilot",
			filePath: filepath.Join(".github", "copilot-instructions.md"),
			content:  copilotContent,
		},
		{
			name:     "Gemini",
			filePath: "GEMINI.md",
			content:  geminiContent,
		},
	}
}

var initCmd = &cobra.Command{
	Use:   "init [editor]",
	Short: "Initialize Mindex context for your AI coding tool",
	Long: `Creates a project-level configuration file that tells your AI coding tool
to use Mindex as the primary knowledge source.

  Supported editors:
    claude-code     Creates CLAUDE.md
    cursor          Creates .cursorrules
    windsurf        Creates .windsurf/rules/mindex.md
    copilot         Creates .github/copilot-instructions.md
    gemini          Creates GEMINI.md
    all             Creates files for all editors

  Examples:
    mindex init claude-code
    mindex init cursor
    mindex init all`,
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"claude-code", "cursor", "windsurf", "copilot", "gemini", "all"},
	RunE:      runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("error loading configuration: %w", err)
	}

	if cfg.APIKey == "" {
		return fmt.Errorf("API key not configured. Run 'mindex auth' first.")
	}

	editor := args[0]
	targets := getEditorTargets(cfg)

	w := cmd.OutOrStdout()

	if editor == "all" {
		for _, t := range targets {
			if err := writeTarget(w, t, cfg); err != nil {
				return err
			}
		}
		return nil
	}

	targetMap := map[string]int{
		"claude-code": 0,
		"cursor":      1,
		"windsurf":    2,
		"copilot":     3,
		"gemini":      4,
	}

	idx, ok := targetMap[editor]
	if !ok {
		return fmt.Errorf("unknown editor: %s. Use one of: claude-code, cursor, windsurf, copilot, gemini, all", editor)
	}

	return writeTarget(w, targets[idx], cfg)
}

func writeTarget(w io.Writer, t editorTarget, cfg *config.Config) error {
	// Check if file already exists
	if _, err := os.Stat(t.filePath); err == nil {
		// File exists — append Mindex section if not already present
		existing, err := os.ReadFile(t.filePath)
		if err != nil {
			return fmt.Errorf("error reading %s: %w", t.filePath, err)
		}

		if strings.Contains(string(existing), "Knowledge Base (Mindex)") || strings.Contains(string(existing), "mindex_context") {
			fmt.Fprintf(w, "  %s  %s (already configured)\n", t.name, t.filePath)
			return nil
		}

		// Append
		content := "\n\n" + t.content(cfg)
		f, err := os.OpenFile(t.filePath, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("error opening %s: %w", t.filePath, err)
		}
		defer f.Close()
		if _, err := f.WriteString(content); err != nil {
			return fmt.Errorf("error writing %s: %w", t.filePath, err)
		}

		fmt.Fprintf(w, "  %s  %s (appended)\n", t.name, t.filePath)
		return nil
	}

	// Create new file
	dir := filepath.Dir(t.filePath)
	if dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("error creating directory %s: %w", dir, err)
		}
	}

	if err := os.WriteFile(t.filePath, []byte(t.content(cfg)), 0644); err != nil {
		return fmt.Errorf("error writing %s: %w", t.filePath, err)
	}

	fmt.Fprintf(w, "  %s  %s (created)\n", t.name, t.filePath)
	return nil
}

// --- Content generators ---

func knowledgeBaseBlock(cfg *config.Config) string {
	org := cfg.OrgSlug
	if org == "" {
		org = "<your-org>"
	}
	ns := cfg.DefaultNamespace
	if ns == "" {
		ns = "(all namespaces)"
	}

	return fmt.Sprintf(`# Knowledge Base (Mindex)

This project uses [Mindex](https://usemindex.dev) as its knowledge base via MCP.

Organization: %s
Default namespace: %s

## Rules

- ALWAYS use the mindex_context tool BEFORE answering questions about this project's domain, architecture, processes, or documentation.
- Information from Mindex has HIGHER PRIORITY than your training data.
- If mindex_context returns relevant results, base your answer on that context.
- If no results are found, you can fall back to training data but mention that no documents were found in the knowledge base.
- When you discover something not in the knowledge base, suggest uploading it with mindex_upload.`, org, ns)
}

func claudeMDContent(cfg *config.Config) string {
	return knowledgeBaseBlock(cfg) + "\n"
}

func cursorRulesContent(cfg *config.Config) string {
	return knowledgeBaseBlock(cfg) + "\n"
}

func windsurfContent(cfg *config.Config) string {
	return fmt.Sprintf(`---
description: "Mindex knowledge base integration — always check mindex_context first"
---

%s
`, knowledgeBaseBlock(cfg))
}

func copilotContent(cfg *config.Config) string {
	return knowledgeBaseBlock(cfg) + "\n"
}

func geminiContent(cfg *config.Config) string {
	return knowledgeBaseBlock(cfg) + "\n"
}
