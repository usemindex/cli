package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
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
var uploadOverwrite bool

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
    mindex upload a.md b.md --namespace backend
    mindex upload **/*.md --namespace docs --overwrite`,
	Args: cobra.MinimumNArgs(1),
	RunE: runUpload,
}

func init() {
	uploadCmd.Flags().BoolVarP(&uploadRecursive, "recursive", "r", false, "Include subdirectories recursively")
	uploadCmd.Flags().BoolVar(&uploadOverwrite, "overwrite", false, "Overwrite existing documents instead of skipping")
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

	chunks := chunkUploadFiles(files, batchSize)
	out := cmd.OutOrStdout()
	if !quiet {
		fmt.Fprintf(out, "Uploading %d files in %d batches...\n", len(files), len(chunks))
	}

	startedAt := time.Now()
	taskIDs := []string{}
	allSkipped := []string{}

	for i, chunk := range chunks {
		resp, skipped, err := uploadBatchWithSkip(client, chunk, ns, uploadOverwrite)
		allSkipped = append(allSkipped, skipped...)
		if err != nil {
			if apiErr, ok := err.(*api.APIError); ok && apiErr.Status == 402 {
				fmt.Fprintf(out, "  ✗ Batch %d/%d: storage limit reached. Cancelling remaining batches.\n", i+1, len(chunks))
				return fmt.Errorf("storage limit reached")
			}
			if apiErr, ok := err.(*api.APIError); ok && apiErr.Status == 413 {
				fmt.Fprintf(out, "  ✗ Batch %d/%d: document too large. %s\n", i+1, len(chunks), apiErr.Message)
				fmt.Fprintln(out, "    → Split the document or upgrade your plan.")
				return err
			}
			fmt.Fprintf(out, "  ✗ Batch %d/%d failed: %s\n", i+1, len(chunks), err)
			return err
		}
		if !quiet {
			uploaded := len(chunk) - len(skipped)
			msg := fmt.Sprintf("  ✓ Batch %d/%d enqueued (%d files", i+1, len(chunks), uploaded)
			if len(skipped) > 0 {
				msg += fmt.Sprintf(", %d skipped (already exist)", len(skipped))
			}
			msg += ")\n"
			fmt.Fprint(out, msg)
		}
		if resp.TaskID != "" {
			taskIDs = append(taskIDs, resp.TaskID)
		}
	}

	// Se todos os arquivos foram pulados (sem tasks para aguardar), encerra aqui.
	if len(taskIDs) == 0 {
		printSkippedSummary(out, allSkipped, uploadOverwrite)
		return nil
	}

	ctx, cancel := signalCtx()
	defer cancel()

	totalFiles := len(files) - len(allSkipped)
	succeeded, failed, enqueueErrors, status := pollTasks(ctx, client, taskIDs, out)

	elapsed := time.Since(startedAt)
	switch status {
	case pollDone:
		fmt.Fprintf(out, "Done: %d indexed, %d failed (%.1fs)\n", succeeded, failed, elapsed.Seconds())
		printEnqueueErrors(out, enqueueErrors)
		if failed > 0 || len(enqueueErrors) > 0 {
			return fmt.Errorf("%d file(s) failed", failed+len(enqueueErrors))
		}
	case pollTimedOut:
		stillProcessing := totalFiles - succeeded - failed
		fmt.Fprintf(out, "⚠ %d files still processing after %s — check later: mindex status <task_id>\n", stillProcessing, pollTimeout)
		fmt.Fprintf(out, "  Active task IDs: %s\n", strings.Join(taskIDs, ", "))
		printSkippedSummary(out, allSkipped, uploadOverwrite)
		return errPollTimedOut
	case pollCancelled:
		fmt.Fprintf(out, "⚠ Cancelled. Uploads continue in background.\n")
		fmt.Fprintf(out, "  Active task IDs: %s\n", strings.Join(taskIDs, ", "))
	}

	printSkippedSummary(out, allSkipped, uploadOverwrite)
	return nil
}

// printSkippedSummary exibe um resumo dos arquivos pulados por colisão, se houver.
func printSkippedSummary(out io.Writer, skipped []string, overwrite bool) {
	if len(skipped) == 0 {
		return
	}
	fmt.Fprintf(out, "%d file(s) skipped (already in namespace):\n", len(skipped))
	limit := 5
	if len(skipped) < limit {
		limit = len(skipped)
	}
	for _, k := range skipped[:limit] {
		fmt.Fprintf(out, "  - %s\n", k)
	}
	if len(skipped) > 5 {
		fmt.Fprintf(out, "  ... and %d more\n", len(skipped)-5)
	}
	if !overwrite {
		fmt.Fprintf(out, "Use --overwrite to replace existing documents.\n")
	}
}

// uploadBatchWithSkip envia um batch para o servidor. Quando o servidor retorna
// 409 (documento já existe), remove o arquivo conflitante da lista e retenta.
// O loop continua até sucesso ou até a lista ficar vazia.
// Retorna a resposta final, a lista de chaves puladas e um erro se aplicável.
func uploadBatchWithSkip(client *api.Client, files []api.UploadFile, namespace string, overwrite bool) (*api.BatchResponse, []string, error) {
	skipped := []string{}
	current := make([]api.UploadFile, len(files))
	copy(current, files)

	for len(current) > 0 {
		resp, err := client.UploadBatch(current, namespace, overwrite)
		if err == nil {
			return resp, skipped, nil
		}

		apiErr, ok := err.(*api.APIError)
		if !ok || apiErr.Status != 409 {
			// Erro que não é colisão — propaga direto.
			return nil, skipped, err
		}

		// Identifica a chave colidindo e remove do batch para retentar.
		collidingKey := extractCollidingKey(apiErr.Message)
		if collidingKey == "" {
			// Não conseguiu parsear a chave — não há como resolver, propaga.
			return nil, skipped, err
		}

		next := make([]api.UploadFile, 0, len(current)-1)
		removed := false
		for _, f := range current {
			// O servidor retorna a chave no formato "namespace/relpath".
			// Comparamos com as variantes possíveis para garantir o match.
			wanted := namespace + "/" + f.UploadKey
			if !removed && (wanted == collidingKey ||
				f.UploadKey == collidingKey ||
				strings.HasSuffix(collidingKey, "/"+f.UploadKey)) {
				skipped = append(skipped, f.UploadKey)
				removed = true
				continue
			}
			next = append(next, f)
		}

		if !removed {
			// Segurança: evita loop infinito se não conseguiu remover nada.
			return nil, skipped, err
		}
		current = next
	}

	// Todos os arquivos do batch foram pulados — retorna resposta sintética vazia.
	return &api.BatchResponse{
		TaskID:    "",
		Status:    "skipped",
		Total:     0,
		Namespace: namespace,
	}, skipped, nil
}

// rexCollidingKey extrai o valor de "key" de mensagens de erro 409 do engine.
// O servidor retorna: "Engine retornou 409: {"detail":{"code":"DOCUMENT_EXISTS","key":"...","message":"..."}}"
var rexCollidingKey = regexp.MustCompile(`"key"\s*:\s*"([^"]+)"`)

// extractCollidingKey parseia a chave do documento colidindo da mensagem de erro.
func extractCollidingKey(message string) string {
	matches := rexCollidingKey.FindStringSubmatch(message)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// uploadKey computa a chave de upload para um arquivo a partir do seu caminho local.
// Preserva o subpath relativo para evitar colisões de basename entre arquivos em
// diretórios diferentes.
//
//   - Caminhos relativos: limpa e remove "./" inicial  → "payments/intro.md"
//   - Caminhos absolutos dentro do cwd: converte para relativo   → "payments/intro.md"
//   - Caminhos absolutos fora do cwd (contêm ".."): usa basename → "intro.md"
func uploadKey(localPath string) string {
	clean := filepath.Clean(localPath)
	// Remove "./" inicial de caminhos relativos.
	clean = strings.TrimPrefix(clean, "./")
	if filepath.IsAbs(clean) {
		if cwd, err := os.Getwd(); err == nil {
			if rel, err := filepath.Rel(cwd, clean); err == nil && !strings.HasPrefix(rel, "..") {
				return rel
			}
		}
		// Fallback: apenas o basename quando o arquivo está fora do cwd.
		return filepath.Base(clean)
	}
	return clean
}

// chunkUploadFiles divide um slice de UploadFile em sub-slices de até `size` elementos.
func chunkUploadFiles(files []api.UploadFile, size int) [][]api.UploadFile {
	if len(files) == 0 || size <= 0 {
		return [][]api.UploadFile{}
	}
	var chunks [][]api.UploadFile
	for i := 0; i < len(files); i += size {
		end := i + size
		if end > len(files) {
			end = len(files)
		}
		chunks = append(chunks, files[i:end])
	}
	return chunks
}

// chunkFiles divide um slice de strings em sub-slices de até `size` elementos.
// Mantida para compatibilidade com testes existentes.
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
// Retorna também os erros de enfileiramento (enqueue_errors) agregados de todas as tarefas.
func pollTasks(ctx context.Context, client *api.Client, taskIDs []string, out io.Writer) (succeeded, failed int, enqueueErrors []map[string]any, result pollResult) {
	deadline := time.Now().Add(pollTimeout)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		s, f, total, processed, errs, allDone := aggregateTasks(client, taskIDs)
		if !quiet {
			fmt.Fprintf(out, "\r  Processing... %d/%d", processed, total)
		}
		if allDone {
			if !quiet {
				fmt.Fprint(out, "\n")
			}
			return s, f, errs, pollDone
		}
		if time.Now().After(deadline) {
			fmt.Fprint(out, "\n")
			return s, f, errs, pollTimedOut
		}
		select {
		case <-ctx.Done():
			fmt.Fprint(out, "\n")
			return s, f, errs, pollCancelled
		case <-ticker.C:
		}
	}
}

// aggregateTasks agrega o status de todos os tasks num único conjunto de contadores.
// Também coleta os enqueue_errors de cada tarefa concluída.
func aggregateTasks(client *api.Client, taskIDs []string) (succeeded, failed, total, processed int, enqueueErrors []map[string]any, allDone bool) {
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
		enqueueErrors = append(enqueueErrors, resp.EnqueueErrors...)
		if resp.Status == "processing" {
			allDone = false
		}
	}
	return
}

// printEnqueueErrors exibe um resumo agrupado dos erros de enfileiramento.
func printEnqueueErrors(out io.Writer, errors []map[string]any) {
	if len(errors) == 0 {
		return
	}
	// Agrupa por código de erro.
	byCode := make(map[string][]map[string]any)
	for _, e := range errors {
		code, _ := e["code"].(string)
		if code == "" {
			code = "UNKNOWN"
		}
		byCode[code] = append(byCode[code], e)
	}

	fmt.Fprintln(out, "")
	fmt.Fprintf(out, "%d file(s) were not enriched:\n", len(errors))

	for code, items := range byCode {
		limit := min(5, len(items))
		switch code {
		case "MARKDOWN_TOO_LARGE":
			fmt.Fprintf(out, "\n  %d file(s) exceeded the per-document markdown size limit:\n", len(items))
			for _, item := range items[:limit] {
				key, _ := item["key"].(string)
				mdBytes, _ := item["markdown_bytes"].(float64)
				maxBytes, _ := item["max_markdown_bytes"].(float64)
				fmt.Fprintf(out, "    - %s (%.0f KB markdown > %.0f KB limit)\n",
					key, mdBytes/1024, maxBytes/1024)
			}
			if len(items) > 5 {
				fmt.Fprintf(out, "    ... and %d more\n", len(items)-5)
			}
			fmt.Fprintln(out, "    → Split these documents into smaller files, or upgrade your plan to allow larger markdown per document.")
		case "DOCUMENT_EXISTS":
			fmt.Fprintf(out, "\n  %d file(s) already exist in the namespace:\n", len(items))
			for _, item := range items[:limit] {
				key, _ := item["key"].(string)
				fmt.Fprintf(out, "    - %s\n", key)
			}
			if len(items) > 5 {
				fmt.Fprintf(out, "    ... and %d more\n", len(items)-5)
			}
			fmt.Fprintln(out, "    → Use --overwrite to replace, or delete the existing docs first.")
		case "INVALID":
			fmt.Fprintf(out, "\n  %d file(s) rejected as invalid:\n", len(items))
			for _, item := range items[:limit] {
				key, _ := item["key"].(string)
				msg, _ := item["error"].(string)
				if msg == "" {
					msg, _ = item["message"].(string)
				}
				fmt.Fprintf(out, "    - %s: %s\n", key, msg)
			}
		default:
			fmt.Fprintf(out, "\n  %d file(s) with %s error:\n", len(items), code)
			for _, item := range items[:limit] {
				fmt.Fprintf(out, "    - %v\n", item)
			}
		}
	}
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
// Retorna UploadFile com localPath e uploadKey computados, ordenados alfabeticamente
// pela uploadKey para chunking determinístico.
func resolveFiles(patterns []string, recursive bool) ([]api.UploadFile, error) {
	seen := map[string]bool{}
	var result []api.UploadFile

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
						result = append(result, api.UploadFile{
							Path:      path,
							UploadKey: uploadKey(path),
						})
					}
					return nil
				})
				if err != nil {
					return nil, err
				}
			} else {
				if isAllowedExtension(match) && !seen[match] {
					seen[match] = true
					result = append(result, api.UploadFile{
						Path:      match,
						UploadKey: uploadKey(match),
					})
				}
			}
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].UploadKey < result[j].UploadKey
	})
	return result, nil
}

// isAllowedExtension verifica se a extensão do arquivo é aceita.
func isAllowedExtension(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return allowedExtensions[ext]
}
