package cmd

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/usemindex/cli/api"
	"github.com/usemindex/cli/config"
)

// accepted file extensions for upload
var allowedExtensions = map[string]bool{
	".md":       true,
	".txt":      true,
	".markdown": true,
	".pdf":      true,
	".docx":     true,
	".pptx":     true,
	".xlsx":     true,
	".html":     true,
	".htm":      true,
	".csv":      true,
	".json":     true,
	".xml":      true,
}

var uploadRecursive bool

var uploadCmd = &cobra.Command{
	Use:   "upload <files...>",
	Short: "Upload files to the knowledge base",
	Long: `Uploads one or more files to the knowledge base.
Supports globs and, with --recursive, entire directories.

  Accepted extensions: .md .txt .markdown .pdf .docx .pptx .xlsx .html .htm .csv .json .xml

  Examples:
    mindex upload doc.md
    mindex upload docs/*.md
    mindex upload ./docs --recursive
    mindex upload a.md b.md --namespace backend`,
	Args: cobra.MinimumNArgs(1),
	RunE: runUpload,
}

func init() {
	uploadCmd.Flags().BoolVarP(&uploadRecursive, "recursive", "r", false, "Include subdirectories recursively")
	rootCmd.AddCommand(uploadCmd)
}

func runUpload(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("error loading configuration: %w", err)
	}
	if cfg.APIKey == "" {
		return fmt.Errorf("API key not configured. Run 'mindex auth' first.")
	}

	ns := namespace
	if ns == "" {
		return fmt.Errorf("namespace is required. Use -n <namespace>\n\nExample: mindex upload file.md -n docs")
	}

	client := api.New(cfg.APIURL, cfg.APIKey)
	client.OrgSlug = cfg.OrgSlug

	// resolve all files from args (globs + directories)
	files, err := resolveFiles(args, uploadRecursive)
	if err != nil {
		return fmt.Errorf("error resolving files: %w", err)
	}

	if len(files) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No compatible files found.")
		return nil
	}

	success := 0
	failed := 0

	for _, f := range files {
		_, err := client.UploadFile(f, ns)
		name := filepath.Base(f)
		if err != nil {
			if !quiet {
				fmt.Fprintf(cmd.OutOrStdout(), "  x %s — %s\n", name, err)
			}
			failed++
		} else {
			if !quiet {
				fmt.Fprintf(cmd.OutOrStdout(), "  ✓ %s\n", name)
			}
			success++
		}
	}

	if !quiet {
		fmt.Fprintf(cmd.OutOrStdout(), "\n%d file(s) uploaded, %d failure(s).\n", success, failed)
	}

	if failed > 0 && success == 0 {
		return fmt.Errorf("all uploads failed")
	}
	return nil
}

// resolveFiles expands globs, directories (if recursive) and filters by extension.
func resolveFiles(patterns []string, recursive bool) ([]string, error) {
	seen := map[string]bool{}
	var result []string

	for _, pattern := range patterns {
		// try glob first
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, err
		}

		// if no glob match, treat as a literal path
		if matches == nil {
			matches = []string{pattern}
		}

		for _, match := range matches {
			info, err := os.Stat(match)
			if err != nil {
				// file does not exist — skip silently
				continue
			}

			if info.IsDir() {
				if !recursive {
					fmt.Fprintf(os.Stderr, "  Skipping directory '%s' (use --recursive to include)\n", match)
					continue
				}
				// walk the directory recursively
				err := filepath.WalkDir(match, func(path string, d fs.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if d.IsDir() {
						return nil
					}
					if isAllowedExtension(path) && !seen[path] {
						seen[path] = true
						result = append(result, path)
					}
					return nil
				})
				if err != nil {
					return nil, err
				}
			} else {
				if isAllowedExtension(match) && !seen[match] {
					seen[match] = true
					result = append(result, match)
				}
			}
		}
	}

	return result, nil
}

// isAllowedExtension checks whether the file extension is accepted.
func isAllowedExtension(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return allowedExtensions[ext]
}
