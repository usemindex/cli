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
	chunks := chunkFiles(files, 50)
	if len(chunks) != 3 {
		t.Fatalf("esperado 3 batches, obtido %d", len(chunks))
	}

	taskIDs := []string{}
	for _, c := range chunks {
		resp, err := client.UploadBatch(c, "docs")
		if err != nil {
			t.Fatalf("UploadBatch erro: %v", err)
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

	_, err := client.UploadBatch([]string{a}, "docs")
	apiErr, ok := err.(*api.APIError)
	if !ok || apiErr.Status != 402 {
		t.Fatalf("esperado APIError 402, obtido %v", err)
	}
}
