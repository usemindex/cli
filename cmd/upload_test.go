package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/usemindex/cli/api"
)

func TestChunkFiles(t *testing.T) {
	cases := []struct {
		name     string
		input    []string
		size     int
		expected [][]int // sizes of resulting chunks
	}{
		{"exact multiple", make([]string, 100), 50, [][]int{{50}, {50}}},
		{"with remainder", make([]string, 115), 50, [][]int{{50}, {50}, {15}}},
		{"smaller than chunk", make([]string, 5), 50, [][]int{{5}}},
		{"empty", []string{}, 50, [][]int{}},
		{"size 1", []string{"a", "b", "c"}, 1, [][]int{{1}, {1}, {1}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chunks := chunkFiles(tc.input, tc.size)
			if len(chunks) != len(tc.expected) {
				t.Fatalf("número de chunks: esperado %d, obtido %d", len(tc.expected), len(chunks))
			}
			for i, c := range chunks {
				if len(c) != tc.expected[i][0] {
					t.Errorf("chunk %d: esperado tamanho %d, obtido %d", i, tc.expected[i][0], len(c))
				}
			}
		})
	}
}

func TestChunkUploadFiles(t *testing.T) {
	makeFiles := func(n int) []api.UploadFile {
		out := make([]api.UploadFile, n)
		for i := range out {
			out[i] = api.UploadFile{Path: fmt.Sprintf("f%d.md", i), UploadKey: fmt.Sprintf("f%d.md", i)}
		}
		return out
	}
	cases := []struct {
		name     string
		n        int
		size     int
		expected []int
	}{
		{"exact multiple", 100, 50, []int{50, 50}},
		{"with remainder", 115, 50, []int{50, 50, 15}},
		{"smaller than chunk", 5, 50, []int{5}},
		{"empty", 0, 50, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chunks := chunkUploadFiles(makeFiles(tc.n), tc.size)
			if len(chunks) != len(tc.expected) {
				t.Fatalf("número de chunks: esperado %d, obtido %d", len(tc.expected), len(chunks))
			}
			for i, c := range chunks {
				if len(c) != tc.expected[i] {
					t.Errorf("chunk %d: esperado %d, obtido %d", i, tc.expected[i], len(c))
				}
			}
		})
	}
}

func TestUploadKey(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}

	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{"relative simple", "payments/intro.md", "payments/intro.md"},
		{"strip dot-slash", "./payments/intro.md", "payments/intro.md"},
		{"root file", "intro.md", "intro.md"},
		{"absolute under cwd", filepath.Join(cwd, "payments/intro.md"), "payments/intro.md"},
		{"absolute outside cwd — basename fallback", "/tmp/some-other/intro.md", "intro.md"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := uploadKey(tc.input)
			if got != tc.expected {
				t.Errorf("uploadKey(%q): esperado %q, obtido %q", tc.input, tc.expected, got)
			}
		})
	}
}

func TestExtractCollidingKey(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			"mensagem padrão do engine via Rails proxy",
			`Engine retornou 409: {"detail":{"code":"DOCUMENT_EXISTS","key":"docs/payments/intro.md","message":"Document already exists"}}`,
			"docs/payments/intro.md",
		},
		{
			"chave simples",
			`{"key": "docs/intro.md"}`,
			"docs/intro.md",
		},
		{
			"sem chave",
			`{"error": "conflict"}`,
			"",
		},
		{
			"string vazia",
			"",
			"",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractCollidingKey(tc.input)
			if got != tc.expected {
				t.Errorf("extractCollidingKey: esperado %q, obtido %q", tc.expected, got)
			}
		})
	}
}

func TestUploadBatchWithSkip_Sucesso(t *testing.T) {
	// Servidor responde 202 imediatamente — nenhum skip esperado.
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]any{
			"task_id": "tid-ok", "status": "processing", "total": 2, "namespace": "docs",
		})
	}))
	defer servidor.Close()

	tmp := t.TempDir()
	a := filepath.Join(tmp, "a.md")
	b := filepath.Join(tmp, "b.md")
	os.WriteFile(a, []byte("# A"), 0644)
	os.WriteFile(b, []byte("# B"), 0644)

	client := api.New(servidor.URL, "sk-test")
	client.OrgSlug = "acme"
	client.RetryMin = 1
	client.RetryMax = 1

	files := []api.UploadFile{
		{Path: a, UploadKey: "a.md"},
		{Path: b, UploadKey: "b.md"},
	}
	resp, skipped, err := uploadBatchWithSkip(client, files, "docs", false)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(skipped) != 0 {
		t.Errorf("esperado 0 skipped, obtido %d", len(skipped))
	}
	if resp.TaskID != "tid-ok" {
		t.Errorf("task_id: esperado 'tid-ok', obtido %q", resp.TaskID)
	}
}

func TestUploadBatchWithSkip_409RemoveERetenta(t *testing.T) {
	// Primeiro request: 409 com a chave de "a.md" colidindo.
	// Segundo request (retry sem a.md): 202.
	var calls int32
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			// 409 indicando que docs/a.md já existe.
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]any{
				"error": `Engine retornou 409: {"detail":{"code":"DOCUMENT_EXISTS","key":"docs/a.md","message":"Document already exists"}}`,
			})
			return
		}
		// Segundo attempt (sem a.md) deve ter 1 arquivo apenas.
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		if len(r.MultipartForm.File["files[]"]) != 1 {
			t.Errorf("esperado 1 arquivo no retry, obtido %d", len(r.MultipartForm.File["files[]"]))
		}
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]any{
			"task_id": "tid-retry", "status": "processing", "total": 1, "namespace": "docs",
		})
	}))
	defer servidor.Close()

	tmp := t.TempDir()
	a := filepath.Join(tmp, "a.md")
	b := filepath.Join(tmp, "b.md")
	os.WriteFile(a, []byte("# A"), 0644)
	os.WriteFile(b, []byte("# B"), 0644)

	client := api.New(servidor.URL, "sk-test")
	client.OrgSlug = "acme"
	client.RetryMin = 1
	client.RetryMax = 1

	files := []api.UploadFile{
		{Path: a, UploadKey: "a.md"},
		{Path: b, UploadKey: "b.md"},
	}
	resp, skipped, err := uploadBatchWithSkip(client, files, "docs", false)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(skipped) != 1 || skipped[0] != "a.md" {
		t.Errorf("skipped: esperado ['a.md'], obtido %v", skipped)
	}
	if resp.TaskID != "tid-retry" {
		t.Errorf("task_id: esperado 'tid-retry', obtido %q", resp.TaskID)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Errorf("calls: esperado 2, obtido %d", calls)
	}
}

func TestUploadBatchWithSkip_TodosColisaoRetornaVazio(t *testing.T) {
	// Todos os arquivos colidem: deve retornar resposta sintética "skipped".
	var calls int32
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		keys := []string{"docs/a.md", "docs/b.md"}
		idx := int(n) - 1
		if idx >= len(keys) {
			idx = len(keys) - 1
		}
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]any{
			"error": fmt.Sprintf(`Engine retornou 409: {"detail":{"code":"DOCUMENT_EXISTS","key":"%s","message":"already exists"}}`, keys[idx]),
		})
	}))
	defer servidor.Close()

	tmp := t.TempDir()
	a := filepath.Join(tmp, "a.md")
	b := filepath.Join(tmp, "b.md")
	os.WriteFile(a, []byte("# A"), 0644)
	os.WriteFile(b, []byte("# B"), 0644)

	client := api.New(servidor.URL, "sk-test")
	client.OrgSlug = "acme"
	client.RetryMin = 1
	client.RetryMax = 1

	files := []api.UploadFile{
		{Path: a, UploadKey: "a.md"},
		{Path: b, UploadKey: "b.md"},
	}
	resp, skipped, err := uploadBatchWithSkip(client, files, "docs", false)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(skipped) != 2 {
		t.Errorf("esperado 2 skipped, obtido %d: %v", len(skipped), skipped)
	}
	if resp.Status != "skipped" {
		t.Errorf("status: esperado 'skipped', obtido %q", resp.Status)
	}
	if resp.TaskID != "" {
		t.Errorf("task_id deveria ser vazio, obtido %q", resp.TaskID)
	}
}

func TestUploadBatchWithSkip_409SemKeyPropagaErro(t *testing.T) {
	// 409 sem chave parseável: deve propagar o erro sem loop infinito.
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]any{"error": "conflict sem detalhe"})
	}))
	defer servidor.Close()

	tmp := t.TempDir()
	a := filepath.Join(tmp, "a.md")
	os.WriteFile(a, []byte("# A"), 0644)

	client := api.New(servidor.URL, "sk-test")
	client.OrgSlug = "acme"
	client.RetryMin = 1
	client.RetryMax = 1

	files := []api.UploadFile{{Path: a, UploadKey: "a.md"}}
	_, _, err := uploadBatchWithSkip(client, files, "docs", false)
	if err == nil {
		t.Fatal("esperado erro, obtido nil")
	}
	apiErr, ok := err.(*api.APIError)
	if !ok || apiErr.Status != 409 {
		t.Errorf("esperado APIError 409, obtido %T: %v", err, err)
	}
}

func TestRunUpload_BatchAndPoll(t *testing.T) {
	var uploadCalls int32
	var statusCalls int32

	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/acme/documents":
			n := atomic.AddInt32(&uploadCalls, 1)
			w.WriteHeader(http.StatusAccepted)
			json.NewEncoder(w).Encode(map[string]any{
				"task_id":   fmt.Sprintf("tid-%d", n),
				"status":    "processing",
				"total":     50,
				"namespace": "docs",
			})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/acme/documents/tasks/"):
			c := atomic.AddInt32(&statusCalls, 1)
			// 1ª chamada de cada task_id retorna processing; depois completed
			if c <= 3 {
				json.NewEncoder(w).Encode(map[string]any{
					"task_id":   "tid-x",
					"status":    "processing",
					"total":     50,
					"succeeded": 25,
					"failed":    0,
					"processed": 25,
					"namespace": "docs",
				})
			} else {
				json.NewEncoder(w).Encode(map[string]any{
					"task_id":   "tid-x",
					"status":    "completed",
					"total":     50,
					"succeeded": 50,
					"failed":    0,
					"processed": 50,
					"namespace": "docs",
				})
			}
		default:
			t.Fatalf("path inesperado: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer servidor.Close()

	tmp := t.TempDir()
	for i := 0; i < 115; i++ {
		p := filepath.Join(tmp, fmt.Sprintf("doc-%03d.md", i))
		os.WriteFile(p, []byte("# x"), 0644)
	}

	client := api.New(servidor.URL, "sk-test")
	client.OrgSlug = "acme"

	files, err := resolveFiles([]string{tmp}, true)
	if err != nil {
		t.Fatalf("resolveFiles: %v", err)
	}
	if len(files) != 115 {
		t.Fatalf("esperado 115 arquivos, obtido %d", len(files))
	}
	chunks := chunkUploadFiles(files, 50)
	if len(chunks) != 3 {
		t.Fatalf("esperado 3 batches, obtido %d", len(chunks))
	}

	taskIDs := []string{}
	for _, c := range chunks {
		resp, skipped, err := uploadBatchWithSkip(client, c, "docs", false)
		if err != nil {
			t.Fatalf("UploadBatch erro: %v", err)
		}
		if len(skipped) != 0 {
			t.Errorf("esperado 0 skipped, obtido %d", len(skipped))
		}
		taskIDs = append(taskIDs, resp.TaskID)
	}

	if uploadCalls != 3 {
		t.Errorf("upload calls: esperado 3, obtido %d", uploadCalls)
	}
	if len(taskIDs) != 3 {
		t.Errorf("task ids: esperado 3, obtido %d", len(taskIDs))
	}
}

func TestRunUpload_StorageLimit402(t *testing.T) {
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		json.NewEncoder(w).Encode(map[string]any{
			"error":      "Storage limit reached",
			"limit_type": "storage",
			"current":    1000,
			"max":        500,
			"plan":       "free",
		})
	}))
	defer servidor.Close()

	tmp := t.TempDir()
	a := filepath.Join(tmp, "a.md")
	os.WriteFile(a, []byte("# A"), 0644)

	client := api.New(servidor.URL, "sk-test")
	client.OrgSlug = "acme"

	files := []api.UploadFile{{Path: a, UploadKey: "a.md"}}
	_, _, err := uploadBatchWithSkip(client, files, "docs", false)
	apiErr, ok := err.(*api.APIError)
	if !ok || apiErr.Status != 402 {
		t.Fatalf("esperado APIError 402, obtido %v", err)
	}
}

func TestResolveFiles_PreservaSubpath(t *testing.T) {
	// Verifica que resolveFiles retorna UploadKeys com subpath preservado.
	tmp := t.TempDir()
	dirA := filepath.Join(tmp, "payments")
	dirB := filepath.Join(tmp, "subscriptions")
	os.MkdirAll(dirA, 0755)
	os.MkdirAll(dirB, 0755)
	os.WriteFile(filepath.Join(dirA, "introduction.md"), []byte("# p"), 0644)
	os.WriteFile(filepath.Join(dirB, "introduction.md"), []byte("# s"), 0644)
	os.WriteFile(filepath.Join(tmp, "introduction.md"), []byte("# r"), 0644)

	// Muda o cwd para o diretório temporário para simular execução local.
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(tmp)

	files, err := resolveFiles([]string{"."}, true)
	if err != nil {
		t.Fatalf("resolveFiles: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("esperado 3 arquivos, obtido %d", len(files))
	}

	keys := map[string]bool{}
	for _, f := range files {
		keys[f.UploadKey] = true
	}

	want := []string{
		"introduction.md",
		"payments/introduction.md",
		"subscriptions/introduction.md",
	}
	for _, k := range want {
		if !keys[k] {
			t.Errorf("chave esperada %q não encontrada em %v", k, keys)
		}
	}
}
