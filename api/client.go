package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	maxRetries429 = 5
	maxRetries5xx = 3
)

var (
	defaultRetryMin = 1 * time.Second
	defaultRetryMax = 60 * time.Second
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
	// RetryMin e RetryMax controlam o backoff exponencial para retries.
	// Exportados para que testes possam sobrescrever com valores menores.
	RetryMin time.Duration
	RetryMax time.Duration
}

// New cria um novo Client com timeout padrão de 30 segundos.
func New(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL: baseURL,
		APIKey:  apiKey,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		RetryMin: defaultRetryMin,
		RetryMax: defaultRetryMax,
	}
}

// backoffFor retorna a duração de backoff exponencial para a tentativa N.
// Sequência: RetryMin, 2×RetryMin, 4×RetryMin, ... limitado a RetryMax.
func (c *Client) backoffFor(attempt int) time.Duration {
	d := c.RetryMin * time.Duration(1<<uint(attempt))
	if d > c.RetryMax {
		d = c.RetryMax
	}
	return d
}

// parseRetryAfter lê o valor do header Retry-After (em segundos) e retorna
// como Duration. Retorna 0 se ausente, inválido ou não-positivo.
func (c *Client) parseRetryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	secs, err := strconv.Atoi(v)
	if err != nil || secs <= 0 {
		return 0
	}
	d := time.Duration(secs) * time.Second
	if d > c.RetryMax {
		d = c.RetryMax
	}
	return d
}

// doWithRetry executa uma requisição HTTP com retry automático em 429 e 5xx.
// Em 429, respeita o header Retry-After (segundos); cai em backoff exponencial
// se ausente. Em 5xx e erros de transporte, usa backoff exponencial.
// Retorna a resposta final, o corpo em bytes ou um erro se os retries se esgotarem.
func (c *Client) doWithRetry(reqFactory func() (*http.Request, error)) (*http.Response, []byte, error) {
	var lastErr error
	for attempt := 0; attempt <= maxRetries429; attempt++ {
		req, err := reqFactory()
		if err != nil {
			return nil, nil, err
		}
		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			// erro de transporte — retry limitado a maxRetries5xx
			if attempt >= maxRetries5xx {
				return nil, nil, err
			}
			lastErr = err
			time.Sleep(c.backoffFor(attempt))
			continue
		}
		body, bodyErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if bodyErr != nil {
			return resp, nil, bodyErr
		}

		// Retry em 429 (rate limit)
		if resp.StatusCode == 429 && attempt < maxRetries429 {
			wait := c.parseRetryAfter(resp.Header.Get("Retry-After"))
			if wait == 0 {
				wait = c.backoffFor(attempt)
			}
			time.Sleep(wait)
			continue
		}

		// Retry em 5xx (erro de servidor), mas não em 4xx exceto 429
		if resp.StatusCode >= 500 && attempt < maxRetries5xx {
			time.Sleep(c.backoffFor(attempt))
			continue
		}

		return resp, body, nil
	}
	if lastErr != nil {
		return nil, nil, lastErr
	}
	return nil, nil, fmt.Errorf("retries esgotados")
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
		"question": question,
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

// GetDocument retrieves the full content of a document by key.
func (c *Client) GetDocument(key, namespace string) (map[string]any, error) {
	encoded := key
	if idx := strings.Index(key, "/"); idx >= 0 {
		// key includes namespace prefix: "ns/file.md" → encode only filename
		ns := key[:idx]
		filename := key[idx+1:]
		encoded = fmt.Sprintf("%s?namespace=%s", filename, ns)
	} else if namespace != "" {
		encoded = fmt.Sprintf("%s?namespace=%s", key, namespace)
	}
	return c.do(http.MethodGet, fmt.Sprintf("/api/v1/%s/documents/%s", c.OrgSlug, encoded), nil)
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

// BatchResponse é a resposta 202 do POST /documents (batch).
type BatchResponse struct {
	TaskID    string `json:"task_id"`
	Status    string `json:"status"`
	Total     int    `json:"total"`
	Namespace string `json:"namespace"`
}

// TaskStatusResponse é a resposta do endpoint GET /documents/tasks/{task_id}.
type TaskStatusResponse struct {
	TaskID        string           `json:"task_id"`
	Status        string           `json:"status"`
	Phase         string           `json:"phase,omitempty"`
	Total         int              `json:"total"`
	Succeeded     int              `json:"succeeded"`
	Failed        int              `json:"failed"`
	Processed     int              `json:"processed"`
	Namespace     string           `json:"namespace"`
	Results       []map[string]any `json:"results,omitempty"`
	EnqueueErrors []map[string]any `json:"enqueue_errors,omitempty"`
}

// UploadBatch faz upload de múltiplos arquivos numa única request multipart.
// O servidor (API Rails) aceita até 50 arquivos por request.
// Em caso de 429 ou 5xx, retenta automaticamente com backoff.
func (c *Client) UploadBatch(filePaths []string, namespace string) (*BatchResponse, error) {
	// reqFactory reconstrói o multipart body a cada tentativa, pois o body é
	// consumido na request anterior e não pode ser reutilizado diretamente.
	reqFactory := func() (*http.Request, error) {
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)

		for _, fp := range filePaths {
			f, err := os.Open(fp)
			if err != nil {
				return nil, fmt.Errorf("erro ao abrir %s: %w", fp, err)
			}
			// Rails convention: name "files[]" → params[:files] array
			part, err := writer.CreateFormFile("files[]", filepath.Base(fp))
			if err != nil {
				f.Close()
				return nil, fmt.Errorf("erro ao criar form file: %w", err)
			}
			if _, err := io.Copy(part, f); err != nil {
				f.Close()
				return nil, fmt.Errorf("erro ao copiar %s: %w", fp, err)
			}
			f.Close()
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
		return req, nil
	}

	resp, respBody, err := c.doWithRetry(reqFactory)
	if err != nil {
		return nil, fmt.Errorf("erro na requisição: %w", err)
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

	var result BatchResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("erro ao deserializar resposta: %w", err)
	}
	return &result, nil
}

// TaskStatus consulta o status agregado de um batch de upload.
// Em caso de 429 ou 5xx, retenta automaticamente com backoff.
func (c *Client) TaskStatus(taskID string) (*TaskStatusResponse, error) {
	reqFactory := func() (*http.Request, error) {
		path := fmt.Sprintf("/api/v1/%s/documents/tasks/%s", c.OrgSlug, url.PathEscape(taskID))
		req, err := http.NewRequest(http.MethodGet, c.BaseURL+path, nil)
		if err != nil {
			return nil, fmt.Errorf("erro ao criar requisição: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("Accept", "application/json")
		return req, nil
	}

	resp, respBody, err := c.doWithRetry(reqFactory)
	if err != nil {
		return nil, fmt.Errorf("erro na requisição: %w", err)
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

	var result TaskStatusResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("erro ao deserializar resposta: %w", err)
	}
	return &result, nil
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

// RelatedDocument representa um documento relacionado via entidades compartilhadas.
type RelatedDocument struct {
	Filename       string   `json:"filename"`
	Title          *string  `json:"title"`
	SharedEntities []string `json:"shared_entities"`
	SharedCount    int      `json:"shared_count"`
	Strength       float64  `json:"strength"`
}

// EntityInDocument representa uma entidade presente no documento, com contagem de
// quantos outros documentos também a mencionam.
type EntityInDocument struct {
	Name            string `json:"name"`
	Type            string `json:"type"`
	AlsoMentionedIn int    `json:"also_mentioned_in"`
}

// RelatedDocumentsResponse é a resposta do endpoint /documents/:key/related.
type RelatedDocumentsResponse struct {
	Document          string             `json:"document"`
	Title             *string            `json:"title"`
	Related           []RelatedDocument  `json:"related"`
	EntitiesInThisDoc []EntityInDocument `json:"entities_in_this_doc"`
}

// RelatedDocuments retorna documentos relacionados a um documento via entidades compartilhadas.
// A key pode incluir prefixo de namespace (ex: "docs/slack.md") — o namespace será extraído
// e enviado como query param, e apenas o filename é colocado no path.
func (c *Client) RelatedDocuments(key string, limit, minShared int) (*RelatedDocumentsResponse, error) {
	filename := key
	ns := ""
	if idx := strings.Index(key, "/"); idx >= 0 {
		ns = key[:idx]
		filename = key[idx+1:]
	}

	path := fmt.Sprintf("/api/v1/%s/documents/%s/related", c.OrgSlug, url.PathEscape(filename))

	params := url.Values{}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	if minShared > 0 {
		params.Set("min_shared", fmt.Sprintf("%d", minShared))
	}
	if ns != "" {
		params.Set("namespace", ns)
	}
	if encoded := params.Encode(); encoded != "" {
		path += "?" + encoded
	}

	req, err := http.NewRequest(http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("erro ao criar requisição: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
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

	var result RelatedDocumentsResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("erro ao deserializar resposta: %w", err)
	}
	return &result, nil
}
