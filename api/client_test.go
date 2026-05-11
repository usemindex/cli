package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	c := New("https://api.example.com", "sk-test")
	if c == nil {
		t.Fatal("New() retornou nil")
	}
	if c.BaseURL != "https://api.example.com" {
		t.Errorf("BaseURL: esperado %q, obtido %q", "https://api.example.com", c.BaseURL)
	}
	if c.APIKey != "sk-test" {
		t.Errorf("APIKey: esperado %q, obtido %q", "sk-test", c.APIKey)
	}
	if c.HTTPClient == nil {
		t.Error("HTTPClient não deveria ser nil")
	}
}

func TestGetMeSuccess(t *testing.T) {
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/me" {
			t.Errorf("caminho inesperado: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Errorf("header Authorization inesperado: %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":    "user-123",
			"email": "teste@exemplo.com",
		})
	}))
	defer servidor.Close()

	c := New(servidor.URL, "sk-test")
	result, err := c.GetMe()
	if err != nil {
		t.Fatalf("GetMe() erro inesperado: %v", err)
	}
	if result["email"] != "teste@exemplo.com" {
		t.Errorf("email: esperado %q, obtido %q", "teste@exemplo.com", result["email"])
	}
}

func TestGetMe401(t *testing.T) {
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{"error": "unauthorized"})
	}))
	defer servidor.Close()

	c := New(servidor.URL, "sk-invalida")
	_, err := c.GetMe()
	if err == nil {
		t.Fatal("GetMe() deveria retornar erro para 401")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("erro deveria ser *APIError, obtido %T", err)
	}
	if apiErr.Status != 401 {
		t.Errorf("status: esperado 401, obtido %d", apiErr.Status)
	}
	expected := "Authentication failed. Run 'mindex auth' to reconfigure."
	if apiErr.Error() != expected {
		t.Errorf("mensagem: esperado %q, obtido %q", expected, apiErr.Error())
	}
}

func TestAPIErrorMessages(t *testing.T) {
	casos := []struct {
		status   int
		esperado string
	}{
		{401, "Authentication failed. Run 'mindex auth' to reconfigure."},
		{403, "Access denied. Check your API key permissions."},
		{404, "Resource not found."},
		{429, "Rate limited. Please try again later."},
		{502, "Service temporarily unavailable. Try again."},
		{503, "Service temporarily unavailable. Try again."},
	}

	for _, tc := range casos {
		err := &APIError{Status: tc.status, Message: "detalhe"}
		if err.Error() != tc.esperado {
			t.Errorf("status %d: esperado %q, obtido %q", tc.status, tc.esperado, err.Error())
		}
	}
}

func TestAPIErrorUnknownStatus(t *testing.T) {
	err := &APIError{Status: 500, Message: "erro interno"}
	if err.Error() == "" {
		t.Error("Error() não deveria retornar string vazia para status desconhecido")
	}
}

func TestSearchSuccess(t *testing.T) {
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("método esperado POST, obtido %s", r.Method)
		}

		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["query"] != "minha busca" {
			t.Errorf("query: esperado %q, obtido %q", "minha busca", body["query"])
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"results": []any{
				map[string]any{"key": "doc1.md", "score": 0.95},
			},
		})
	}))
	defer servidor.Close()

	c := New(servidor.URL, "sk-test")
	c.OrgSlug = "minha-org"
	result, err := c.Search("minha busca", "default", 10)
	if err != nil {
		t.Fatalf("Search() erro inesperado: %v", err)
	}
	if result["results"] == nil {
		t.Error("results não deveria ser nil")
	}
}

func TestStatus(t *testing.T) {
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/admin/health" {
			t.Errorf("caminho inesperado: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	}))
	defer servidor.Close()

	c := New(servidor.URL, "sk-test")
	result, err := c.Status()
	if err != nil {
		t.Fatalf("Status() erro inesperado: %v", err)
	}
	if result["status"] != "ok" {
		t.Errorf("status: esperado %q, obtido %q", "ok", result["status"])
	}
}

func TestListNamespaces(t *testing.T) {
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("método esperado GET, obtido %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"namespaces": []any{
				map[string]any{"slug": "default"},
			},
		})
	}))
	defer servidor.Close()

	c := New(servidor.URL, "sk-test")
	c.OrgSlug = "minha-org"
	result, err := c.ListNamespaces()
	if err != nil {
		t.Fatalf("ListNamespaces() erro inesperado: %v", err)
	}
	if result["namespaces"] == nil {
		t.Error("namespaces não deveria ser nil")
	}
}

func TestRelatedDocumentsSuccess(t *testing.T) {
	title := "Slack"
	relTitle := "Discord"
	expected := RelatedDocumentsResponse{
		Document: "docs/slack.md",
		Title:    &title,
		Related: []RelatedDocument{
			{
				Filename:       "docs/discord.md",
				Title:          &relTitle,
				SharedEntities: []string{"subscription.cancelled", "webhook"},
				SharedCount:    2,
				Strength:       0.67,
			},
		},
		EntitiesInThisDoc: []EntityInDocument{
			{Name: "subscription.cancelled", Type: "event", AlsoMentionedIn: 3},
		},
	}

	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("método esperado GET, obtido %s", r.Method)
		}
		// path should be /api/v1/minha-org/documents/slack.md/related
		expectedPath := "/api/v1/minha-org/documents/slack.md/related"
		if r.URL.Path != expectedPath {
			t.Errorf("caminho: esperado %q, obtido %q", expectedPath, r.URL.Path)
		}
		// query params
		if q := r.URL.Query().Get("limit"); q != "10" {
			t.Errorf("limit: esperado '10', obtido %q", q)
		}
		if q := r.URL.Query().Get("min_shared"); q != "2" {
			t.Errorf("min_shared: esperado '2', obtido %q", q)
		}
		if q := r.URL.Query().Get("namespace"); q != "docs" {
			t.Errorf("namespace: esperado 'docs', obtido %q", q)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expected)
	}))
	defer servidor.Close()

	c := New(servidor.URL, "sk-test")
	c.OrgSlug = "minha-org"
	result, err := c.RelatedDocuments("docs/slack.md", 10, 2)
	if err != nil {
		t.Fatalf("RelatedDocuments() erro inesperado: %v", err)
	}
	if result.Document != "docs/slack.md" {
		t.Errorf("document: esperado 'docs/slack.md', obtido %q", result.Document)
	}
	if len(result.Related) != 1 {
		t.Fatalf("related: esperado 1 item, obtido %d", len(result.Related))
	}
	if result.Related[0].Filename != "docs/discord.md" {
		t.Errorf("related filename: obtido %q", result.Related[0].Filename)
	}
	if result.Related[0].SharedCount != 2 {
		t.Errorf("shared_count: esperado 2, obtido %d", result.Related[0].SharedCount)
	}
	if len(result.EntitiesInThisDoc) != 1 {
		t.Errorf("entities: esperado 1 item, obtido %d", len(result.EntitiesInThisDoc))
	}
}

func TestRelatedDocuments404(t *testing.T) {
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{"error": "document not found"})
	}))
	defer servidor.Close()

	c := New(servidor.URL, "sk-test")
	c.OrgSlug = "minha-org"
	_, err := c.RelatedDocuments("docs/missing.md", 10, 1)
	if err == nil {
		t.Fatal("RelatedDocuments() deveria retornar erro para 404")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("erro deveria ser *APIError, obtido %T", err)
	}
	if apiErr.Status != 404 {
		t.Errorf("status: esperado 404, obtido %d", apiErr.Status)
	}
}

func TestRelatedDocumentsNoNamespace(t *testing.T) {
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// path without prefix should not include namespace query param
		if q := r.URL.Query().Get("namespace"); q != "" {
			t.Errorf("namespace não deveria estar presente, obtido %q", q)
		}
		expectedPath := "/api/v1/minha-org/documents/slack.md/related"
		if r.URL.Path != expectedPath {
			t.Errorf("caminho: esperado %q, obtido %q", expectedPath, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(RelatedDocumentsResponse{
			Document: "slack.md",
			Related:  []RelatedDocument{},
		})
	}))
	defer servidor.Close()

	c := New(servidor.URL, "sk-test")
	c.OrgSlug = "minha-org"
	result, err := c.RelatedDocuments("slack.md", 10, 1)
	if err != nil {
		t.Fatalf("RelatedDocuments() erro inesperado: %v", err)
	}
	if result.Document != "slack.md" {
		t.Errorf("document: obtido %q", result.Document)
	}
}

func TestUploadBatch(t *testing.T) {
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("método esperado POST, obtido %s", r.Method)
		}
		if r.URL.Path != "/api/v1/acme/documents" {
			t.Fatalf("path inesperado: %s", r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Fatalf("Content-Type inesperado: %s", r.Header.Get("Content-Type"))
		}

		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("erro parsing multipart: %v", err)
		}
		// Rails convention: form field "files[]" → params[:files] array
		files := r.MultipartForm.File["files[]"]
		if len(files) != 2 {
			t.Fatalf("esperado 2 arquivos em files[], obtido %d", len(files))
		}
		if r.FormValue("namespace") != "docs" {
			t.Fatalf("namespace esperado 'docs', obtido %q", r.FormValue("namespace"))
		}
		// overwrite não deve estar presente quando não solicitado
		if r.FormValue("overwrite") != "" {
			t.Fatalf("overwrite não esperado, obtido %q", r.FormValue("overwrite"))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]any{
			"task_id":   "tid-abc",
			"status":    "processing",
			"total":     2,
			"namespace": "docs",
		})
	}))
	defer servidor.Close()

	tmp := t.TempDir()
	a := filepath.Join(tmp, "a.md")
	b := filepath.Join(tmp, "b.md")
	if err := os.WriteFile(a, []byte("# A"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("# B"), 0644); err != nil {
		t.Fatal(err)
	}

	c := New(servidor.URL, "sk-test")
	c.OrgSlug = "acme"
	resp, err := c.UploadBatch([]UploadFile{
		{Path: a, UploadKey: "a.md"},
		{Path: b, UploadKey: "b.md"},
	}, "docs", false)
	if err != nil {
		t.Fatalf("UploadBatch erro inesperado: %v", err)
	}
	if resp.TaskID != "tid-abc" {
		t.Errorf("task_id: esperado %q, obtido %q", "tid-abc", resp.TaskID)
	}
	if resp.Total != 2 {
		t.Errorf("total: esperado 2, obtido %d", resp.Total)
	}
}

func TestUploadBatchPreservaSubpath(t *testing.T) {
	// Verifica que o UploadKey (subpath) é enviado como filename no Content-Disposition
	// do multipart — evitando colisões entre arquivos de mesmo basename em diretórios distintos.
	//
	// Nota: o Go http.Server (net/http) segue RFC 2183 e strips o diretório do
	// FileHeader.Filename por segurança. Por isso lemos o corpo raw do request
	// para confirmar que os subpaths chegam ao servidor sem truncagem pelo cliente.
	var rawBody []byte
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		rawBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("leitura do body: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]any{
			"task_id": "tid-sub", "status": "processing", "total": 2, "namespace": "docs",
		})
	}))
	defer servidor.Close()

	tmp := t.TempDir()
	// Cria dois arquivos com mesmo basename em subdiretórios distintos.
	dirA := filepath.Join(tmp, "payments")
	dirB := filepath.Join(tmp, "subscriptions")
	os.MkdirAll(dirA, 0755)
	os.MkdirAll(dirB, 0755)
	fileA := filepath.Join(dirA, "introduction.md")
	fileB := filepath.Join(dirB, "introduction.md")
	os.WriteFile(fileA, []byte("# payments"), 0644)
	os.WriteFile(fileB, []byte("# subscriptions"), 0644)

	c := New(servidor.URL, "sk-test")
	c.OrgSlug = "acme"
	_, err := c.UploadBatch([]UploadFile{
		{Path: fileA, UploadKey: "payments/introduction.md"},
		{Path: fileB, UploadKey: "subscriptions/introduction.md"},
	}, "docs", false)
	if err != nil {
		t.Fatalf("UploadBatch erro inesperado: %v", err)
	}

	// Verifica que o body bruto contém os subpaths nos headers Content-Disposition.
	// O Go mime/multipart serializa: filename="payments/introduction.md"
	body := string(rawBody)
	for _, want := range []string{`payments/introduction.md`, `subscriptions/introduction.md`} {
		if !strings.Contains(body, want) {
			t.Errorf("body multipart não contém o subpath %q", want)
		}
	}
}

func TestUploadBatchOverwrite(t *testing.T) {
	// Verifica que o campo "overwrite=true" é enviado quando a flag está ativa.
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("erro parsing multipart: %v", err)
		}
		if got := r.FormValue("overwrite"); got != "true" {
			t.Errorf("overwrite: esperado 'true', obtido %q", got)
		}
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]any{
			"task_id": "tid-ow", "status": "processing", "total": 1, "namespace": "docs",
		})
	}))
	defer servidor.Close()

	tmp := t.TempDir()
	a := filepath.Join(tmp, "a.md")
	os.WriteFile(a, []byte("# A"), 0644)

	c := New(servidor.URL, "sk-test")
	c.OrgSlug = "acme"
	resp, err := c.UploadBatch([]UploadFile{{Path: a, UploadKey: "a.md"}}, "docs", true)
	if err != nil {
		t.Fatalf("UploadBatch erro inesperado: %v", err)
	}
	if resp.TaskID != "tid-ow" {
		t.Errorf("task_id: esperado %q, obtido %q", "tid-ow", resp.TaskID)
	}
}

func TestUploadBatchRetries429ComRetryAfter(t *testing.T) {
	var attempts int32
	// O servidor responde com Retry-After: 1 nas primeiras 2 tentativas e
	// retorna 202 na terceira. Usamos RetryMin/RetryMax pequenos para agilizar
	// — porém o Retry-After ainda vale, limitado pelo RetryMax configurado.
	retryWait := 20 * time.Millisecond
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 3 {
			// Envia Retry-After em milissegundos não é padrão; o servidor
			// real envia segundos. Aqui testamos que o header é lido e que
			// o cliente respeita a espera. Valor "0" força uso do backoff.
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]any{"error": "Rate limit"})
			return
		}
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]any{"task_id": "tid-apos-retry", "total": 1, "namespace": "docs"})
	}))
	defer servidor.Close()

	tmp := t.TempDir()
	a := filepath.Join(tmp, "a.md")
	os.WriteFile(a, []byte("# A"), 0644)

	c := New(servidor.URL, "sk-test")
	c.OrgSlug = "acme"
	c.RetryMin = retryWait
	c.RetryMax = 500 * time.Millisecond

	start := time.Now()
	resp, err := c.UploadBatch([]UploadFile{{Path: a, UploadKey: "a.md"}}, "docs", false)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("UploadBatch erro inesperado: %v", err)
	}
	if resp.TaskID != "tid-apos-retry" {
		t.Errorf("task_id: esperado %q, obtido %q", "tid-apos-retry", resp.TaskID)
	}
	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("attempts: esperado 3, obtido %d", attempts)
	}
	// 2 retries com backoff exponencial: retryWait + 2×retryWait = 3×retryWait
	minExpected := 3 * retryWait
	if elapsed < minExpected {
		t.Errorf("esperado ao menos %v de elapsed, obtido %v", minExpected, elapsed)
	}
}

func TestUploadBatchEsgota429ERetornaErro(t *testing.T) {
	var attempts int32
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		// Retry-After: 0 faz o backoffFor assumir o controle (RetryMin)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]any{"error": "Rate limit"})
	}))
	defer servidor.Close()

	tmp := t.TempDir()
	a := filepath.Join(tmp, "a.md")
	os.WriteFile(a, []byte("# A"), 0644)

	c := New(servidor.URL, "sk-test")
	c.OrgSlug = "acme"
	// Backoff mínimo para que o teste não demore (maxRetries429=5 tentativas)
	c.RetryMin = 1 * time.Millisecond
	c.RetryMax = 5 * time.Millisecond

	_, err := c.UploadBatch([]UploadFile{{Path: a, UploadKey: "a.md"}}, "docs", false)
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("esperado *APIError 429 após esgotar retries, obtido %T: %v", err, err)
	}
	if apiErr.Status != 429 {
		t.Errorf("status: esperado 429, obtido %d", apiErr.Status)
	}
	// maxRetries429=5 → 6 tentativas no total (attempt 0..5)
	if atomic.LoadInt32(&attempts) != maxRetries429+1 {
		t.Errorf("attempts: esperado %d, obtido %d", maxRetries429+1, attempts)
	}
}

func TestUploadBatchRetries5xx(t *testing.T) {
	var attempts int32
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 2 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]any{"task_id": "tid", "total": 1, "namespace": "docs"})
	}))
	defer servidor.Close()

	tmp := t.TempDir()
	a := filepath.Join(tmp, "a.md")
	os.WriteFile(a, []byte("# A"), 0644)

	c := New(servidor.URL, "sk-test")
	c.OrgSlug = "acme"
	c.RetryMin = 1 * time.Millisecond
	c.RetryMax = 5 * time.Millisecond

	resp, err := c.UploadBatch([]UploadFile{{Path: a, UploadKey: "a.md"}}, "docs", false)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if resp.TaskID != "tid" {
		t.Errorf("task_id: %q", resp.TaskID)
	}
	if atomic.LoadInt32(&attempts) != 2 {
		t.Errorf("attempts: esperado 2, obtido %d", attempts)
	}
}

func TestTaskStatusRetries429(t *testing.T) {
	var attempts int32
	retryWait := 20 * time.Millisecond
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 2 {
			// Retry-After: 0 → backoff exponencial com RetryMin configurado
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"task_id": "tid-1", "status": "completed", "total": 1,
			"succeeded": 1, "failed": 0, "processed": 1, "namespace": "docs",
		})
	}))
	defer servidor.Close()

	c := New(servidor.URL, "sk-test")
	c.OrgSlug = "acme"
	c.RetryMin = retryWait
	c.RetryMax = 500 * time.Millisecond

	start := time.Now()
	resp, err := c.TaskStatus("tid-1")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if resp.Status != "completed" {
		t.Errorf("status: %q", resp.Status)
	}
	if atomic.LoadInt32(&attempts) != 2 {
		t.Errorf("attempts: esperado 2, obtido %d", attempts)
	}
	// 1 retry × retryWait de backoff
	if elapsed < retryWait {
		t.Errorf("esperado ao menos %v de elapsed, obtido %v", retryWait, elapsed)
	}
}

// TestTaskStatusRetries429ComRetryAfterReal verifica que Retry-After em segundos
// reais é honrado quando RetryMax for maior que o valor do header.
func TestTaskStatusRetries429ComRetryAfterReal(t *testing.T) {
	if testing.Short() {
		t.Skip("pulando teste de Retry-After em modo -short")
	}
	var attempts int32
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 2 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"task_id": "tid-1", "status": "completed", "total": 1,
			"succeeded": 1, "failed": 0, "processed": 1, "namespace": "docs",
		})
	}))
	defer servidor.Close()

	c := New(servidor.URL, "sk-test")
	c.OrgSlug = "acme"
	// RetryMin pequeno, RetryMax maior que 1s para que Retry-After: 1 seja respeitado
	c.RetryMin = 10 * time.Millisecond
	c.RetryMax = 60 * time.Second

	start := time.Now()
	resp, err := c.TaskStatus("tid-1")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if resp.Status != "completed" {
		t.Errorf("status: %q", resp.Status)
	}
	// Retry-After de 1s deve ter sido aguardado
	if elapsed < 1*time.Second {
		t.Errorf("esperado ao menos 1s de elapsed (Retry-After), obtido %v", elapsed)
	}
}

func TestBackoffFor(t *testing.T) {
	c := New("http://x", "k")
	c.RetryMin = time.Second
	c.RetryMax = 60 * time.Second

	esperados := []time.Duration{1, 2, 4, 8, 16, 32, 60}
	for i, want := range esperados {
		got := c.backoffFor(i)
		if got != want*time.Second {
			t.Errorf("backoffFor(%d): esperado %v, obtido %v", i, want*time.Second, got)
		}
	}
}

func TestParseRetryAfter(t *testing.T) {
	c := New("http://x", "k")
	c.RetryMax = 60 * time.Second

	casos := []struct {
		input    string
		esperado time.Duration
	}{
		{"", 0},
		{"0", 0},
		{"-1", 0},
		{"abc", 0},
		{"5", 5 * time.Second},
		{"30", 30 * time.Second},
		{"120", 60 * time.Second}, // limitado ao RetryMax
	}
	for _, tc := range casos {
		got := c.parseRetryAfter(tc.input)
		if got != tc.esperado {
			t.Errorf("parseRetryAfter(%q): esperado %v, obtido %v", tc.input, tc.esperado, got)
		}
	}
}

func TestTaskStatus(t *testing.T) {
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/acme/documents/tasks/tid-1" {
			t.Fatalf("path inesperado: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"task_id":   "tid-1",
			"status":    "completed",
			"total":     2,
			"succeeded": 2,
			"failed":    0,
			"processed": 2,
			"namespace": "docs",
			"results":   []any{},
		})
	}))
	defer servidor.Close()

	c := New(servidor.URL, "sk-test")
	c.OrgSlug = "acme"
	resp, err := c.TaskStatus("tid-1")
	if err != nil {
		t.Fatalf("TaskStatus erro: %v", err)
	}
	if resp.Status != "completed" {
		t.Errorf("status: esperado %q, obtido %q", "completed", resp.Status)
	}
	if resp.Succeeded != 2 {
		t.Errorf("succeeded: esperado 2, obtido %d", resp.Succeeded)
	}
}

func TestDelete403(t *testing.T) {
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]any{"error": "forbidden"})
	}))
	defer servidor.Close()

	c := New(servidor.URL, "sk-test")
	c.OrgSlug = "minha-org"
	err := c.DeleteDocument("doc-123", "default")
	if err == nil {
		t.Fatal("DeleteDocument() deveria retornar erro para 403")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("erro deveria ser *APIError, obtido %T", err)
	}
	if apiErr.Status != 403 {
		t.Errorf("status: esperado 403, obtido %d", apiErr.Status)
	}
}
