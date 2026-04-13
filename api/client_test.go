package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
