package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	resp, err := c.UploadBatch([]string{a, b}, "docs")
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

func TestUploadBatch429(t *testing.T) {
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "5")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]any{"error": "Rate limit exceeded", "retry_after": 60})
	}))
	defer servidor.Close()

	tmp := t.TempDir()
	a := filepath.Join(tmp, "a.md")
	os.WriteFile(a, []byte("# A"), 0644)

	c := New(servidor.URL, "sk-test")
	c.OrgSlug = "acme"
	_, err := c.UploadBatch([]string{a}, "docs")
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("esperado *APIError, obtido %T", err)
	}
	if apiErr.Status != 429 {
		t.Errorf("status: esperado 429, obtido %d", apiErr.Status)
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
