package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// APIError representa um erro retornado pela API com código HTTP e mensagem.
type APIError struct {
	Status  int
	Message string
}

// Error retorna uma mensagem amigável baseada no código HTTP.
func (e *APIError) Error() string {
	switch e.Status {
	case 401:
		return "Authentication failed. Run 'mindex auth' to reconfigure."
	case 403:
		return "Access denied. Check your API key permissions."
	case 404:
		return "Resource not found."
	case 429:
		return "Rate limited. Please try again later."
	case 502, 503:
		return "Service temporarily unavailable. Try again."
	default:
		return fmt.Sprintf("API error %d: %s", e.Status, e.Message)
	}
}

// Client é o cliente HTTP para a API do Mindex.
type Client struct {
	BaseURL    string
	APIKey     string
	OrgSlug    string
	HTTPClient *http.Client
}

// New cria um novo Client com timeout padrão de 30 segundos.
func New(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL: baseURL,
		APIKey:  apiKey,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// do executa uma requisição HTTP com autenticação e deserializa a resposta JSON.
func (c *Client) do(method, path string, body any) (map[string]any, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("erro ao serializar body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, c.BaseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("erro ao criar requisição: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("erro na requisição: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler resposta: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errResp map[string]any
		json.Unmarshal(respBody, &errResp)
		msg := ""
		if m, ok := errResp["error"].(string); ok {
			msg = m
		} else if m, ok := errResp["message"].(string); ok {
			msg = m
		}
		return nil, &APIError{Status: resp.StatusCode, Message: msg}
	}

	if len(respBody) == 0 {
		return map[string]any{}, nil
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("erro ao deserializar resposta: %w", err)
	}
	return result, nil
}

// GetMe retorna as informações do usuário autenticado.
func (c *Client) GetMe() (map[string]any, error) {
	return c.do(http.MethodGet, "/auth/me", nil)
}

// Search busca documentos por query semântica.
func (c *Client) Search(query, namespace string, limit int) (map[string]any, error) {
	payload := map[string]any{
		"query":     query,
		"namespace": namespace,
		"limit":     limit,
	}
	return c.do(http.MethodPost, fmt.Sprintf("/api/v1/%s/documents/search", c.OrgSlug), payload)
}

// Context recupera contexto GraphRAG para uma pergunta.
func (c *Client) Context(question, namespace string) (map[string]any, error) {
	payload := map[string]any{
		"question":  question,
	}
	if namespace != "" {
		payload["namespace"] = namespace
	}
	return c.do(http.MethodPost, fmt.Sprintf("/api/v1/%s/context", c.OrgSlug), payload)
}

// ListDocuments lista documentos de um namespace.
func (c *Client) ListDocuments(namespace string) (map[string]any, error) {
	path := fmt.Sprintf("/api/v1/%s/documents", c.OrgSlug)
	if namespace != "" {
		path += "?namespace=" + namespace
	}
	return c.do(http.MethodGet, path, nil)
}

// DeleteDocument remove um documento pelo ID.
func (c *Client) DeleteDocument(docID, namespace string) error {
	path := fmt.Sprintf("/api/v1/%s/documents/%s", c.OrgSlug, docID)
	if namespace != "" {
		path += "?namespace=" + namespace
	}
	_, err := c.do(http.MethodDelete, path, nil)
	return err
}

// UploadFile faz upload de um arquivo markdown via multipart.
func (c *Client) UploadFile(filePath, namespace string) (map[string]any, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("erro ao abrir arquivo: %w", err)
	}
	defer f.Close()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return nil, fmt.Errorf("erro ao criar form file: %w", err)
	}
	if _, err := io.Copy(part, f); err != nil {
		return nil, fmt.Errorf("erro ao copiar arquivo: %w", err)
	}
	if namespace != "" {
		writer.WriteField("namespace", namespace)
	}
	writer.Close()

	req, err := http.NewRequest(
		http.MethodPost,
		fmt.Sprintf("%s/api/v1/%s/documents", c.BaseURL, c.OrgSlug),
		&buf,
	)
	if err != nil {
		return nil, fmt.Errorf("erro ao criar requisição: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("erro na requisição: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler resposta: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errResp map[string]any
		json.Unmarshal(respBody, &errResp)
		msg := ""
		if m, ok := errResp["error"].(string); ok {
			msg = m
		}
		return nil, &APIError{Status: resp.StatusCode, Message: msg}
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("erro ao deserializar resposta: %w", err)
	}
	return result, nil
}

// ListNamespaces lista os namespaces da organização.
func (c *Client) ListNamespaces() (map[string]any, error) {
	return c.do(http.MethodGet, fmt.Sprintf("/api/v1/%s/namespaces", c.OrgSlug), nil)
}

// CreateNamespace cria um novo namespace.
func (c *Client) CreateNamespace(name string) (map[string]any, error) {
	payload := map[string]any{"name": name}
	return c.do(http.MethodPost, fmt.Sprintf("/api/v1/%s/namespaces", c.OrgSlug), payload)
}

// Status retorna o status de saúde da API.
func (c *Client) Status() (map[string]any, error) {
	return c.do(http.MethodGet, "/admin/health", nil)
}
