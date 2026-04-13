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

// extensões aceitas para upload
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
	Short: "Faz upload de arquivos para a knowledge base",
	Long: `Faz upload de um ou mais arquivos para a knowledge base.
Suporta globs e, com --recursive, diretórios inteiros.

  Extensões aceitas: .md .txt .markdown .pdf .docx .pptx .xlsx .html .htm .csv .json .xml

  Exemplos:
    mindex upload doc.md
    mindex upload docs/*.md
    mindex upload ./docs --recursive
    mindex upload a.md b.md --namespace backend`,
	Args: cobra.MinimumNArgs(1),
	RunE: runUpload,
}

func init() {
	uploadCmd.Flags().BoolVarP(&uploadRecursive, "recursive", "r", false, "Inclui subdiretórios recursivamente")
	rootCmd.AddCommand(uploadCmd)
}

func runUpload(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("erro ao carregar configuração: %w", err)
	}
	if cfg.APIKey == "" {
		return fmt.Errorf("API key não configurada. Execute 'mindex auth' primeiro.")
	}

	ns := namespace
	if ns == "" {
		ns = cfg.DefaultNamespace
	}

	client := api.New(cfg.APIURL, cfg.APIKey)
	client.OrgSlug = cfg.OrgSlug

	// resolve todos os arquivos a partir dos args (globs + diretórios)
	files, err := resolveFiles(args, uploadRecursive)
	if err != nil {
		return fmt.Errorf("erro ao resolver arquivos: %w", err)
	}

	if len(files) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "Nenhum arquivo compatível encontrado.")
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
		fmt.Fprintf(cmd.OutOrStdout(), "\n%d arquivo(s) enviado(s), %d falha(s).\n", success, failed)
	}

	if failed > 0 && success == 0 {
		return fmt.Errorf("todos os uploads falharam")
	}
	return nil
}

// resolveFiles expande globs, diretórios (se recursive) e filtra por extensão.
func resolveFiles(patterns []string, recursive bool) ([]string, error) {
	seen := map[string]bool{}
	var result []string

	for _, pattern := range patterns {
		// tenta glob primeiro
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, err
		}

		// se não match de glob, trata como path literal
		if matches == nil {
			matches = []string{pattern}
		}

		for _, match := range matches {
			info, err := os.Stat(match)
			if err != nil {
				// arquivo não existe — ignora silenciosamente
				continue
			}

			if info.IsDir() {
				if !recursive {
					fmt.Fprintf(os.Stderr, "  Ignorando diretório '%s' (use --recursive para incluir)\n", match)
					continue
				}
				// percorre o diretório recursivamente
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

// isAllowedExtension verifica se a extensão do arquivo é aceita.
func isAllowedExtension(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return allowedExtensions[ext]
}
