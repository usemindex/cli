package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const skillsBaseURL = "https://raw.githubusercontent.com/usemindex/cli/main/skills"

// skillMeta matches the structure in index.json
type skillMeta struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// toolSkillInstaller knows how to write a skill file for each AI tool.
type toolSkillInstaller struct {
	name    string
	write   func(name, description, content string) (string, error)
	check   func(name string) string
}

var skillInstallers = map[string]*toolSkillInstaller{
	"claude-code": {
		name: "Claude Code",
		write: func(name, description, content string) (string, error) {
			dir := filepath.Join(".claude", "skills", "mindex-"+name)
			path := filepath.Join(dir, "SKILL.md")
			body := fmt.Sprintf("---\nname: mindex-%s\ndescription: %s\n---\n\n%s\n", name, description, content)
			return path, writeFile(path, body)
		},
		check: func(name string) string {
			return filepath.Join(".claude", "skills", "mindex-"+name, "SKILL.md")
		},
	},
	"cursor": {
		name: "Cursor",
		write: func(name, description, content string) (string, error) {
			path := filepath.Join(".cursor", "rules", "mindex-"+name+".md")
			body := fmt.Sprintf("---\ndescription: \"%s\"\nalwaysApply: false\n---\n\n%s\n", description, content)
			return path, writeFile(path, body)
		},
		check: func(name string) string {
			return filepath.Join(".cursor", "rules", "mindex-"+name+".md")
		},
	},
	"windsurf": {
		name: "Windsurf",
		write: func(name, description, content string) (string, error) {
			path := filepath.Join(".windsurf", "rules", "mindex-"+name+".md")
			body := fmt.Sprintf("---\ndescription: \"%s\"\n---\n\n%s\n", description, content)
			return path, writeFile(path, body)
		},
		check: func(name string) string {
			return filepath.Join(".windsurf", "rules", "mindex-"+name+".md")
		},
	},
}

var skillInstallerOrder = []string{"claude-code", "cursor", "windsurf"}

var skillsCmd = &cobra.Command{
	Use:   "skills",
	Short: "Manage Mindex AI skills for your coding tools",
	Long: `Installs Mindex skills (reusable AI prompts) into your coding tools.

Skills teach your AI assistant how to write better documentation
and organize files for optimal knowledge graph indexing.

  Supported tools:
    claude-code     .claude/skills/ (project-level)
    cursor          .cursor/rules/ (project-level)
    windsurf        .windsurf/rules/ (project-level)

  Examples:
    mindex skills install claude-code
    mindex skills list
    mindex skills get write-docs`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var skillsInstallCmd = &cobra.Command{
	Use:   "install <tool>",
	Short: "Install all Mindex skills into an AI tool",
	Long:  "Downloads and installs Mindex skills as local prompt files.\n\n  Supported tools: claude-code, cursor, windsurf",
	Args:  cobra.ExactArgs(1),
	RunE:  runSkillsInstall,
}

var skillsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available Mindex skills",
	RunE:  runSkillsList,
}

var skillsGetCmd = &cobra.Command{
	Use:   "get <skill>",
	Short: "Print a skill's content to stdout",
	Args:  cobra.ExactArgs(1),
	RunE:  runSkillsGet,
}

var skillsUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update all installed skills to the latest version",
	RunE:  runSkillsUpdate,
}

func init() {
	skillsCmd.AddCommand(skillsInstallCmd)
	skillsCmd.AddCommand(skillsListCmd)
	skillsCmd.AddCommand(skillsGetCmd)
	skillsCmd.AddCommand(skillsUpdateCmd)
	rootCmd.AddCommand(skillsCmd)
}

func runSkillsInstall(cmd *cobra.Command, args []string) error {
	toolKey := args[0]
	installer, ok := skillInstallers[toolKey]
	if !ok {
		return fmt.Errorf("unknown tool '%s'. Supported: %s", toolKey, strings.Join(skillInstallerOrder, ", "))
	}

	index, err := fetchSkillIndex()
	if err != nil {
		return fmt.Errorf("failed to fetch skills: %w", err)
	}

	w := cmd.OutOrStdout()
	installed := 0

	for _, skill := range index {
		content, err := fetchSkillContent(skill.Name)
		if err != nil {
			fmt.Fprintf(w, "  x %s — %s\n", skill.Name, err)
			continue
		}
		path, err := installer.write(skill.Name, skill.Description, content)
		if err != nil {
			fmt.Fprintf(w, "  x %s — %s\n", skill.Name, err)
			continue
		}
		fmt.Fprintf(w, "  ✓ %s → %s\n", skill.Name, path)
		installed++
	}

	fmt.Fprintf(w, "\n%d skill(s) installed for %s.\n", installed, installer.name)
	return nil
}

func runSkillsList(cmd *cobra.Command, args []string) error {
	index, err := fetchSkillIndex()
	if err != nil {
		return fmt.Errorf("failed to fetch skills: %w", err)
	}

	w := cmd.OutOrStdout()
	fmt.Fprintln(w, "Available skills:")
	for _, skill := range index {
		fmt.Fprintf(w, "  %-16s %s\n", skill.Name, skill.Description)
	}

	fmt.Fprintln(w, "\nInstall status:")
	for _, toolKey := range skillInstallerOrder {
		inst := skillInstallers[toolKey]
		allOk := true
		for _, skill := range index {
			if _, err := os.Stat(inst.check(skill.Name)); err != nil {
				allOk = false
				break
			}
		}
		if allOk {
			fmt.Fprintf(w, "  %-16s ✓ installed\n", inst.name+":")
		} else {
			fmt.Fprintf(w, "  %-16s ✗ not installed\n", inst.name+":")
		}
	}

	return nil
}

func runSkillsUpdate(cmd *cobra.Command, args []string) error {
	index, err := fetchSkillIndex()
	if err != nil {
		return fmt.Errorf("failed to fetch skills: %w", err)
	}

	w := cmd.OutOrStdout()
	updated := 0

	for _, toolKey := range skillInstallerOrder {
		installer := skillInstallers[toolKey]

		// Check if any skill is installed for this tool
		hasAny := false
		for _, skill := range index {
			if _, err := os.Stat(installer.check(skill.Name)); err == nil {
				hasAny = true
				break
			}
		}
		if !hasAny {
			continue
		}

		// Update installed skills
		for _, skill := range index {
			if _, err := os.Stat(installer.check(skill.Name)); err != nil {
				continue // not installed, skip
			}
			content, err := fetchSkillContent(skill.Name)
			if err != nil {
				fmt.Fprintf(w, "  x %s (%s) — %s\n", skill.Name, installer.name, err)
				continue
			}
			if _, err := installer.write(skill.Name, skill.Description, content); err != nil {
				fmt.Fprintf(w, "  x %s (%s) — %s\n", skill.Name, installer.name, err)
				continue
			}
			fmt.Fprintf(w, "  ✓ %s updated for %s\n", skill.Name, installer.name)
			updated++
		}
	}

	if updated == 0 {
		fmt.Fprintln(w, "No installed skills found. Run 'mindex skills install <tool>' first.")
	} else {
		fmt.Fprintf(w, "\n%d skill(s) updated.\n", updated)
	}
	return nil
}

func runSkillsGet(cmd *cobra.Command, args []string) error {
	name := args[0]
	content, err := fetchSkillContent(name)
	if err != nil {
		return fmt.Errorf("skill '%s' not found: %w", name, err)
	}
	fmt.Fprintln(cmd.OutOrStdout(), content)
	return nil
}

// fetchSkillIndex downloads and parses index.json from GitHub.
func fetchSkillIndex() ([]skillMeta, error) {
	body, err := httpGet(skillsBaseURL + "/index.json")
	if err != nil {
		return nil, err
	}
	var index []skillMeta
	if err := json.Unmarshal(body, &index); err != nil {
		return nil, fmt.Errorf("invalid index.json: %w", err)
	}
	return index, nil
}

// fetchSkillContent downloads a skill's markdown from GitHub.
func fetchSkillContent(name string) (string, error) {
	body, err := httpGet(fmt.Sprintf("%s/%s.md", skillsBaseURL, name))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func httpGet(url string) ([]byte, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}

func writeFile(path, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0644)
}
