package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/usemindex/cli/api"
	"github.com/usemindex/cli/config"
)

// errPollTimedOut sinaliza que o upload em lote expirou aguardando o status das tarefas.
// O chamador (Execute em root.go) deve mapear esse erro para exit code 2.
var errPollTimedOut = errors.New("poll timed out")

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

const (
	batchSize    = 50
	pollInterval = 2 * time.Second
	pollTimeout  = 5 * time.Minute
)

var uploadRecursive bool

var uploadCmd = &cobra.Command{
	Use:   "upload <files...>",
	Short: "Upload files to the knowledge base",
	Long: `Uploads files to the knowledge base in batches of up to 50 per request.
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

	files, err := resolveFiles(args, uploadRecursive)
	if err != nil {
		return fmt.Errorf("error resolving files: %w", err)
	}
	if len(files) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No compatible files found.")
		return nil
	}

	chunks := chunkFiles(files, batchSize)
	out := cmd.OutOrStdout()
	if !quiet {
		fmt.Fprintf(out, "Uploading %d files in %d batches...\n", len(files), len(chunks))
	}

	startedAt := time.Now()
	taskIDs := []string{}

	for i, chunk := range chunks {
		resp, err := client.UploadBatch(chunk, ns)
		if err != nil {
			if apiErr, ok := err.(*api.APIError); ok && apiErr.Status == 402 {
				fmt.Fprintf(out, "  ✗ Batch %d/%d: storage limit reached. Cancelling remaining batches.\n", i+1, len(chunks))
				return fmt.Errorf("storage limit reached")
			}
			fmt.Fprintf(out, "  ✗ Batch %d/%d failed: %s\n", i+1, len(chunks), err)
			return err
		}
		if !quiet {
			fmt.Fprintf(out, "  ✓ Batch %d/%d enqueued (%d files)\n", i+1, len(chunks), len(chunk))
		}
		taskIDs = append(taskIDs, resp.TaskID)
	}

	ctx, cancel := signalCtx()
	defer cancel()

	totalFiles := len(files)
	succeeded, failed, status := pollTasks(ctx, client, taskIDs, out)

	elapsed := time.Since(startedAt)
	switch status {
	case pollDone:
		fmt.Fprintf(out, "Done: %d indexed, %d failed (%.1fs)\n", succeeded, failed, elapsed.Seconds())
		if failed > 0 {
			return fmt.Errorf("%d file(s) failed", failed)
		}
	case pollTimedOut:
		stillProcessing := totalFiles - succeeded - failed
		fmt.Fprintf(out, "⚠ %d files still processing after %s — check later: mindex status <task_id>\n", stillProcessing, pollTimeout)
		fmt.Fprintf(out, "  Active task IDs: %s\n", strings.Join(taskIDs, ", "))
		return errPollTimedOut
	case pollCancelled:
		fmt.Fprintf(out, "⚠ Cancelled. Uploads continue in background.\n")
		fmt.Fprintf(out, "  Active task IDs: %s\n", strings.Join(taskIDs, ", "))
	}
	return nil
}

// chunkFiles divide um slice de arquivos em sub-slices de até `size` elementos.
func chunkFiles(files []string, size int) [][]string {
	if len(files) == 0 || size <= 0 {
		return [][]string{}
	}
	var chunks [][]string
	for i := 0; i < len(files); i += size {
		end := i + size
		if end > len(files) {
			end = len(files)
		}
		chunks = append(chunks, files[i:end])
	}
	return chunks
}

type pollResult int

const (
	pollDone      pollResult = iota
	pollTimedOut  pollResult = iota
	pollCancelled pollResult = iota
)

// pollTasks aguarda todos os taskIDs completarem, exibindo progresso ao usuário.
func pollTasks(ctx context.Context, client *api.Client, taskIDs []string, out io.Writer) (succeeded, failed int, result pollResult) {
	deadline := time.Now().Add(pollTimeout)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		s, f, total, processed, allDone := aggregateTasks(client, taskIDs)
		if !quiet {
			fmt.Fprintf(out, "\r  Processing... %d/%d", processed, total)
		}
		if allDone {
			if !quiet {
				fmt.Fprint(out, "\n")
			}
			return s, f, pollDone
		}
		if time.Now().After(deadline) {
			fmt.Fprint(out, "\n")
			return s, f, pollTimedOut
		}
		select {
		case <-ctx.Done():
			fmt.Fprint(out, "\n")
			return s, f, pollCancelled
		case <-ticker.C:
		}
	}
}

// aggregateTasks agrega o status de todos os tasks num único conjunto de contadores.
func aggregateTasks(client *api.Client, taskIDs []string) (succeeded, failed, total, processed int, allDone bool) {
	allDone = true
	for _, tid := range taskIDs {
		resp, err := client.TaskStatus(tid)
		if err != nil {
			allDone = false
			continue
		}
		succeeded += resp.Succeeded
		failed += resp.Failed
		total += resp.Total
		processed += resp.Processed
		if resp.Status == "processing" {
			allDone = false
		}
	}
	return
}

// signalCtx retorna um contexto cancelado ao receber SIGINT ou SIGTERM.
func signalCtx() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		cancel()
	}()
	return ctx, cancel
}

// resolveFiles expande globs, diretórios (se recursive) e filtra por extensão.
// Retorna os arquivos ordenados alfabeticamente para chunking determinístico.
func resolveFiles(patterns []string, recursive bool) ([]string, error) {
	seen := map[string]bool{}
	var result []string

	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, err
		}
		if matches == nil {
			matches = []string{pattern}
		}

		for _, match := range matches {
			info, err := os.Stat(match)
			if err != nil {
				continue
			}
			if info.IsDir() {
				if !recursive {
					fmt.Fprintf(os.Stderr, "  Skipping directory '%s' (use --recursive to include)\n", match)
					continue
				}
				err := filepath.WalkDir(match, func(path string, d os.DirEntry, walkErr error) error {
					if walkErr != nil {
						return walkErr
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

	sort.Strings(result)
	return result, nil
}

// isAllowedExtension verifica se a extensão do arquivo é aceita.
func isAllowedExtension(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return allowedExtensions[ext]
}
